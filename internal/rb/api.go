package rb

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func init() {
	registerAction("overview", apiOverview)
	registerAction("ops", apiOps)
	registerAction("history", apiHistory)
	registerAction("op_log", apiOpLog)
	registerAction("op_stop", apiOpStop)
	registerAction("settings_save", apiSettingsSave)
	registerAction("schedule_preview", apiSchedulePreview)
	registerAction("consistency_detect", apiConsistencyDetect)
	registerAction("mkdir", apiMkdir)
	registerAction("browse", apiBrowse)
}

func cacheDir() string {
	dir := RunDir + "/cache"
	if !isDir(dir) {
		os.MkdirAll(dir, 0o755)
	}
	return dir
}

func strtotime(v any) any {
	s, ok := v.(string)
	if !ok {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return t.Unix()
}

func nextRun(s Schedule, entry any, t int64) any {
	if n, ok := schedNext(s, mapOf(entry), t); ok {
		return n
	}
	return nil
}

func timezoneName() string {
	if n := time.Local.String(); n != "" && n != "Local" {
		return n
	}
	return "UTC"
}

func resticVersion() string {
	cache := RunDir + "/restic.version"
	if b, err := os.ReadFile(cache); err == nil && fileMtime(cache) >= fileMtime(ResticBin) {
		return strings.TrimSpace(string(b))
	}
	if !fileExecutable(ResticBin) {
		return ""
	}
	_, out, _ := rbCmd([]string{ResticBin, "version", "--json"}, 30*time.Second)
	var j M
	json.Unmarshal([]byte(strings.Join(out, "")), &j)
	v := str(j["version"])
	if v != "" {
		os.MkdirAll(RunDir, 0o755)
		os.WriteFile(cache, []byte(v), 0o644)
	}
	return v
}

func apiOverview(p M) (M, error) {
	cfg := configLoad()
	s := cfg.Settings
	t := now()
	state := stateRead(s)
	running := opsRunning()
	byRepo := map[string]any{}
	for _, op := range running {
		byRepo[str(op["repo"])] = op
	}
	jobs := []M{}
	for _, job := range cfg.Jobs {
		js := mapOf(mapOf(state["jobs"])[job.ID])
		var op any
		for _, r := range running {
			if str(r["job"]) == job.ID {
				op = r
			}
		}
		var next any
		if job.Enabled {
			next = nextRun(job.Schedule, js["sched"], t)
		}
		recent := []M{}
		for _, h := range historyRead(s, M{"job": job.ID, "kind": "backup"}, 30) {
			recent = append(recent, M{"id": h["id"], "status": h["status"], "started": h["started"], "ended": h["ended"],
				"added": mapOf(h["summary"])["data_added_packed"]})
		}
		jobs = append(jobs, M{
			"id":           job.ID,
			"schedule":     schedDescribe(job.Schedule),
			"next":         next,
			"last":         js["last"],
			"last_success": js["last_success"],
			"queued":       phpTruthy(js["queued"]),
			"running":      op,
			"recent":       recent,
		})
	}
	repos := []M{}
	for _, r := range cfg.Repos {
		rs := mapOf(mapOf(state["repos"])[r.ID])
		prune, check := mapOf(rs["prune"]), mapOf(rs["check"])
		var pruneNext, checkNext any
		if r.Maintenance.Prune.Enabled && !r.AppendOnly {
			pruneNext = nextRun(r.Maintenance.Prune.Schedule, prune["sched"], t)
		}
		if r.Maintenance.Check.Enabled {
			checkNext = nextRun(r.Maintenance.Check.Schedule, check["sched"], t)
		}
		repos = append(repos, M{
			"id":    r.ID,
			"url":   repoURL(r),
			"stats": rs["stats"],
			"prune": M{"last": prune["last"], "next": pruneNext,
				"schedule": schedDescribe(r.Maintenance.Prune.Schedule)},
			"check": M{"last": check["last"], "next": checkNext,
				"schedule": schedDescribe(r.Maintenance.Check.Schedule)},
			"running": byRepo[r.ID],
		})
	}
	installed := int64(0)
	if st, err := os.Lstat("/var/log/plugins/" + Name + ".plg"); err == nil {
		installed = st.ModTime().Unix()
	}
	return M{
		"now":     t,
		"config":  configPublic(cfg),
		"jobs":    jobs,
		"repos":   repos,
		"running": running,
		"recent":  historyRead(s, M{}, 15),
		"system": M{
			"array_started":  arrayStarted(),
			"restic":         resticVersion(),
			"server":         serverName(),
			"data_dir_ok":    isDir(s.DataDir),
			"timezone":       timezoneName(),
			"scheduler_tick": fileMtime(RunDir + "/scheduler.tick"),
			"installed":      installed,
			"secrets": M{
				"mode":    s.Secrets,
				"locked":  s.Secrets == "master" && secretsLocked(),
				"problem": secretsReady(s),
			},
		},
	}, nil
}

func apiOps(p M) (M, error) {
	return M{"now": now(), "running": opsRunning()}, nil
}

func apiHistory(p M) (M, error) {
	cfg := configLoad()
	filter := M{}
	for _, k := range []string{"job", "repo", "kind", "status"} {
		if v := pstr(p, k, ""); v != "" {
			filter[k] = v
		}
	}
	limit := clampInt(toInt(param(p, "limit", 200)), 1, 1000)
	return M{"history": historyRead(cfg.Settings, filter, limit)}, nil
}

func apiOpLog(p M) (M, error) {
	id := pstr(p, "id", "")
	op := opRead(id)
	if op == nil {
		cfg := configLoad()
		for _, h := range historyRead(cfg.Settings, M{}, 5000) {
			if str(h["id"]) == id {
				op = h
				break
			}
		}
	}
	if op == nil || !phpTruthy(op["log"]) {
		return nil, apiFail("No log for this operation.")
	}
	file := str(op["log"])
	if !strings.Contains(file, "/logs/") || !strings.HasSuffix(file, ".log") {
		return nil, apiFail("No log for this operation.")
	}
	var size int64
	if st, err := os.Stat(file); err == nil {
		size = st.Size()
	}
	offset := int64(toInt(param(p, "offset", -1)))
	if offset < 0 || offset > size {
		offset = size - 262144
		if offset < 0 {
			offset = 0
		}
	}
	text := ""
	if f, err := os.Open(file); err == nil {
		f.Seek(offset, io.SeekStart)
		b, _ := io.ReadAll(io.LimitReader(f, 1048576))
		f.Close()
		text = string(b)
	}
	status := op["status"]
	if status == nil {
		status = ""
	}
	return M{"text": text, "offset": offset + int64(len(text)), "size": size, "status": status, "op": op}, nil
}

func apiOpStop(p M) (M, error) {
	if !opRequestCancel(pstr(p, "id", "")) {
		return nil, apiFail("That operation is not running.")
	}
	return M{}, nil
}

func apiSettingsSave(p M) (M, error) {
	in, err := pjson(p, "settings")
	if err != nil {
		return nil, err
	}
	return configChange(func(cfg *Config) (M, error) {
		errs := []string{}
		settings := cleanSettings(in, &errs)
		if len(errs) > 0 {
			return nil, apiFail("Please correct the marked fields.", errs...)
		}
		settings.Secrets = cfg.Settings.Secrets
		cfg.Settings = settings
		return M{"settings": settings}, nil
	})
}

func apiSchedulePreview(p M) (M, error) {
	in, err := pjson(p, "schedule")
	if err != nil {
		return nil, err
	}
	errs := []string{}
	s := cleanSchedule(in, "Schedule", &errs)
	if len(errs) > 0 {
		return M{"valid": false, "error": strings.TrimPrefix(errs[0], "Schedule: "), "next": []int64{}}, nil
	}
	next := []int64{}
	t := now()
	if s.Mode == "cron" {
		if c, err := cronParse(s.Cron); err == nil {
			next = c.upcoming(t, 5)
		}
	} else if s.Mode == "interval" {
		step := schedInterval(s)
		for i := int64(1); i <= 5; i++ {
			next = append(next, t+i*step)
		}
	}
	return M{"valid": true, "description": schedDescribe(s), "next": next}, nil
}

func apiConsistencyDetect(p M) (M, error) {
	in, err := pjsonList(p, "sources")
	if err != nil {
		return nil, err
	}
	sources := []string{}
	for _, src := range in {
		path := absPath(src)
		if path != "" && (under(path, "/mnt") || under(path, "/boot")) {
			sources = append(sources, path)
		}
	}
	if len(sources) == 0 {
		return M{"docker": true, "libvirt": true, "containers": []any{}, "vms": []any{},
			"zfs": M{"ok": false, "error": "Add folders to back up first.", "datasets": []any{}}}, nil
	}
	return toMap(consistencyDetect(sources)), nil
}

var reBadName = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func apiMkdir(p M) (M, error) {
	parent := absPath(param(p, "path", ""))
	name := strings.Trim(pstr(p, "name", ""), " \t\n\r\x00\x0b")
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || len(name) > 255 || reBadName.MatchString(name) {
		return nil, apiFail("A folder name cannot contain a slash, and cannot be . or ..")
	}
	if !under(parent, "/mnt") || parent == "/mnt" || !isDir(parent) {
		return nil, apiFail("New folders can be made in a share, pool or disk under /mnt.")
	}
	if inList(fsType(parent), "", "rootfs", "tmpfs", "ramfs", "devtmpfs") {
		return nil, apiFail(parent + " is in memory, not on a disk: a folder made there would be gone after a reboot.")
	}
	path := strings.TrimRight(parent, "/") + "/" + name
	if exists(path) {
		return nil, apiFail(path + " exists already.")
	}
	if !mkdirLikeParent(path) {
		return nil, apiFail("Could not make " + path + ".")
	}
	return M{"path": path}, nil
}

type browseItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

func apiBrowse(p M) (M, error) {
	path := absPath(param(p, "path", "/mnt"))
	root := M{"path": "/", "entries": []browseItem{
		{Name: "mnt", Path: "/mnt", Dir: true},
		{Name: "boot", Path: "/boot", Dir: true},
	}}
	if path == "" || path == "/" {
		return root, nil
	}
	if !(under(path, "/mnt") || under(path, "/boot")) {
		return nil, apiFail("Only folders under /mnt and /boot can be chosen.")
	}
	for !isDir(path) && path != "/mnt" && path != "/boot" {
		path = filepath.Dir(path)
	}
	if !isDir(path) {
		return root, nil
	}
	files := toBool(param(p, "files", false))
	f, err := os.Open(path)
	if err != nil {
		return nil, apiFail("Cannot open " + path)
	}
	names, _ := f.Readdirnames(-1)
	f.Close()
	entries := []browseItem{}
	for _, e := range names {
		if len(entries) >= 5000 {
			break
		}
		full := path + "/" + e
		dir := isDir(full)
		if dir || files {
			entries = append(entries, browseItem{Name: e, Path: full, Dir: dir})
		}
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].Dir != entries[b].Dir {
			return entries[a].Dir
		}
		return strnatcasecmp(entries[a].Name, entries[b].Name) < 0
	})
	return M{"path": path, "entries": entries}, nil
}
