package rb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"syscall"
)

var rePreviewID = regexp.MustCompile(`^[0-9a-z-]{6,40}$`)

func init() {
	register("preview", "<id>  count what a job would back up (detached)", cmdPreview)
	register("test-preview-compare", "", cmdTestPreviewCompare)
}

type previewRequest struct {
	Job      Job      `json:"job"`
	Repo     Repo     `json:"repo"`
	Settings Settings `json:"settings"`
}

func cmdPreview(args []string) int {
	id := ""
	if len(args) > 0 {
		id = args[0]
	}
	var req previewRequest
	if !rePreviewID.MatchString(id) || !jsonReadInto(previewFilePath(id, ".request.json"), &req) {
		return 2
	}
	s := req.Settings
	if s.Nice > 0 {
		syscall.Setpriority(syscall.PRIO_PROCESS, 0, s.Nice)
	}
	io := map[string][]string{"idle": {"-c", "3"}, "low": {"-c", "2", "-n", "7"}}[s.IOPriority]
	if io != nil {
		exec.Command("ionice", append(io, "-p", strconv.Itoa(os.Getpid()))...).Run()
	}

	status := M{"id": id, "status": "running", "started": now(), "pid": os.Getpid(), "progress": nil}
	write := func() {
		jsonWrite(previewFilePath(id, ".json"), status, 0o644)
	}
	write()
	defer func() {
		if r := recover(); r != nil {
			if status["status"] == "running" {
				status["status"] = "error"
				status["error"] = fmt.Sprintf("The preview stopped unexpectedly: %v", r)
				write()
			}
			os.Exit(255)
		}
	}()

	result := previewRun(req.Job, req.Repo, s,
		func(p M) {
			status["progress"] = p
			write()
		},
		func() bool {
			return exists(previewFilePath(id, ".cancel"))
		}, false)
	if result["cancelled"] == true {
		status["status"] = "cancelled"
	} else {
		status["status"] = "done"
	}
	status["ended"] = now()
	status["progress"] = nil
	status["result"] = result
	write()
	os.Remove(previewFilePath(id, ".request.json"))
	os.Remove(previewFilePath(id, ".cancel"))
	return 0
}

func cmdTestPreviewCompare(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: test-preview-compare <job json> <repo path>")
		return 2
	}
	var in M
	json.Unmarshal([]byte(args[0]), &in)
	var errs []string
	job := cleanJob(mapOf(in), []string{"r00000001"}, &errs)
	repo := defaultRepo()
	repo.ID = "r00000001"
	repo.Type = "local"
	repo.Local.Path = args[1]
	settings := configLoad().Settings

	preview := previewRun(job, repo, settings, nil, nil, true)

	tmp := os.TempDir() + "/rb-compare-" + strconv.Itoa(os.Getpid())
	bargs := backupArgs(job, repo, settings, tmp)
	full := append([]string{"-r", args[1], bargs[0], "--dry-run", "-vv"}, bargs[1:]...)
	cmd := exec.Command(ResticBin, full...)
	out, _ := cmd.StdoutPipe()
	code := 0
	var lines []string
	if cmd.Start() != nil {
		code = 127
	} else {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		if err := cmd.Wait(); err != nil {
			code = -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
	}
	os.Remove(tmp + ".exclude")
	os.Remove(tmp + ".iexclude")

	files := []string{}
	var bytes, count any
	for _, line := range lines {
		var j M
		if json.Unmarshal([]byte(line), &j) != nil || j == nil {
			continue
		}
		mt := str(j["message_type"])
		item := str(j["item"])
		if mt == "verbose_status" && inList(str(j["action"]), "new", "unchanged", "modified") &&
			(item == "" || item[len(item)-1] != '/') {
			files = append(files, item)
		} else if mt == "summary" {
			bytes = j["total_bytes_processed"]
			count = j["total_files_processed"]
		}
	}
	sort.Strings(files)
	list := append([]string{}, preview["files"].([]string)...)
	sort.Strings(list)
	total := preview["total"].(M)
	os.Stdout.Write(jsonEncode(M{"preview": list, "restic": files, "preview_bytes": total["bytes"],
		"restic_bytes": bytes, "restic_count": count, "preview_count": total["files"], "restic_exit": code,
		"rules": preview["rules"]}, true))
	return 0
}
