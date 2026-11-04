package rb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

func prepareDataDir(s Settings) string {
	for _, sub := range []string{"", "/cache", "/logs"} {
		dir := s.DataDir + sub
		if !isDir(dir) && os.MkdirAll(dir, 0o755) != nil {
			return "Cannot create " + dir
		}
	}
	if !isDir(RunDir + "/tmp") {
		os.MkdirAll(RunDir+"/tmp", 0o755)
	}
	return ""
}

func newOpID() string {
	return time.Now().Format("20060102-150405") + "-" + randHex(2)
}

func opFile(id string) string {
	return OpsDir + "/" + id + ".json"
}

func opWrite(op M) bool {
	op["updated"] = now()
	return jsonWrite(opFile(str(op["id"])), op, 0o644)
}

var reOpID = regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{4}$`)

func opRead(id string) M {
	if !reOpID.MatchString(id) {
		return nil
	}
	m, _ := jsonRead(opFile(id))
	return m
}

func opsFiles() []string {
	files, _ := filepath.Glob(OpsDir + "/*.json")
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

func opsList(limit int) []M {
	out := []M{}
	files := opsFiles()
	if len(files) > limit {
		files = files[:limit]
	}
	for _, f := range files {
		op, ok := jsonRead(f)
		if !ok {
			continue
		}
		if str(op["status"]) == "running" && !pidAlive(op["runner_pid"]) {
			op["status"] = "error"
			op["error"] = "The operation ended unexpectedly."
		}
		out = append(out, op)
	}
	return out
}

func opsRunning() []M {
	out := []M{}
	for _, op := range opsList(200) {
		if str(op["status"]) == "running" {
			out = append(out, op)
		}
	}
	return out
}

func opRunningOn(repoID string, kinds ...string) M {
	for _, op := range opsRunning() {
		if str(op["repo"]) == repoID && inList(str(op["kind"]), kinds...) {
			return op
		}
	}
	return nil
}

func opsPrune(keep int) {
	files := opsFiles()
	if len(files) <= keep {
		return
	}
	for _, f := range files[keep:] {
		op, ok := jsonRead(f)
		if !ok || str(op["status"]) != "running" || !pidAlive(op["runner_pid"]) {
			os.Remove(f)
			os.Remove(strings.TrimSuffix(f, ".json") + ".cancel")
		}
	}
}

func opCancelFile(id string) string {
	return OpsDir + "/" + id + ".cancel"
}

func opCancelRequested(id string) bool {
	return exists(opCancelFile(id))
}

func opRequestCancel(id string) bool {
	op := opRead(id)
	if op == nil || str(op["status"]) != "running" {
		return false
	}
	f, err := os.Create(opCancelFile(id))
	if err == nil {
		f.Close()
	}
	return true
}

func repoLock(repoID string) *fileLock {
	return lockFile(LockDir+"/"+repoID+".lock", false)
}

func repoBusy(repoID string) bool {
	l := repoLock(repoID)
	if l == nil {
		return true
	}
	l.Unlock()
	return false
}

func historyFile(s Settings) string {
	return s.DataDir + "/history.jsonl"
}

func historyAppend(s Settings, record M) bool {
	file := historyFile(s)
	if !isDir(filepath.Dir(file)) && os.MkdirAll(filepath.Dir(file), 0o755) != nil {
		return false
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	_, err = f.Write(jsonEncode(record, false))
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
	if st, e := os.Stat(file); e == nil && st.Size() > 4*1048576 {
		historyCompact(s)
	}
	return err == nil
}

func historyCompact(s Settings) {
	file := historyFile(s)
	f, err := os.OpenFile(file, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX) != nil {
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	cut := now() - int64(s.HistoryDays)*86400
	var keep []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var r M
		if json.Unmarshal(sc.Bytes(), &r) == nil && r != nil && toInt64(r["ended"]) >= cut {
			keep = append(keep, sc.Text())
		}
	}
	if len(keep) > 20000 {
		keep = keep[len(keep)-20000:]
	}
	f.Truncate(0)
	data := ""
	if len(keep) > 0 {
		data = strings.Join(keep, "\n") + "\n"
	}
	f.WriteAt([]byte(data), 0)
}

func historyRead(s Settings, filter M, limit int) []M {
	b, err := os.ReadFile(historyFile(s))
	if err != nil {
		return []M{}
	}
	lines := strings.Split(string(b), "\n")
	out := []M{}
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		var r M
		if json.Unmarshal([]byte(lines[i]), &r) != nil || r == nil {
			continue
		}
		match := true
		for k, v := range filter {
			if v == nil || v == "" {
				continue
			}
			if fmt.Sprint(r[k]) != fmt.Sprint(v) {
				match = false
				break
			}
		}
		if match {
			out = append(out, r)
		}
	}
	return out
}

const SizesKeep = 1100

func sizePoint(op M) []any {
	s := mapOf(op["summary"])
	added := s["data_added_packed"]
	if added == nil {
		added = s["data_added"]
	}
	return []any{toInt64(op["ended"]), toInt64(s["total_bytes_processed"]), toInt64(added), toInt64(s["total_files_processed"])}
}

func jobSizesFromHistory(s Settings, jobID string) []any {
	points := []any{}
	for _, op := range historyRead(s, M{"kind": "backup", "job": jobID}, SizesKeep) {
		if inList(str(op["status"]), "success", "warning") {
			if _, ok := op["summary"].(M); ok {
				points = append(points, sizePoint(op))
			}
		}
	}
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points
}

func stateAddRepoSize(state M, repoID string, stats M) {
	repos := mapOf(state["repos"])
	state["repos"] = repos
	rs := mapOf(repos[repoID])
	repos[repoID] = rs
	sizes, _ := rs["sizes"].([]any)
	point := []any{toInt64(stats["at"]), toInt64(stats["total_size"]), toInt64(stats["snapshots_count"])}
	if len(sizes) > 0 {
		if last, ok := sizes[len(sizes)-1].([]any); ok && len(last) > 0 {
			if time.Unix(toInt64(last[0]), 0).Format("2006-01-02") == time.Unix(toInt64(point[0]), 0).Format("2006-01-02") {
				sizes = sizes[:len(sizes)-1]
			}
		}
	}
	sizes = append(sizes, point)
	if len(sizes) > SizesKeep {
		sizes = sizes[len(sizes)-SizesKeep:]
	}
	rs["sizes"] = sizes
}

func stateFile(s Settings) string {
	return s.DataDir + "/state.json"
}

func stateRead(s Settings) M {
	m, _ := jsonRead(stateFile(s))
	if m == nil {
		m = M{}
	}
	for _, k := range []string{"jobs", "repos"} {
		if _, ok := m[k].(M); !ok {
			m[k] = M{}
		}
	}
	return m
}

func stateUpdate(s Settings, fn func(state M)) bool {
	l := lockFile(LockDir+"/state.lock", true)
	defer l.Unlock()
	state := stateRead(s)
	fn(state)
	return isDir(s.DataDir) && jsonWrite(stateFile(s), state, 0o644)
}

func sub(m M, keys ...string) M {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(M)
		if !ok {
			next = M{}
			cur[k] = next
		}
		cur = next
	}
	return cur
}

func spawn(args ...string) {
	cmd := exec.Command(SelfBin, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = os.Environ()
	if cmd.Start() == nil {
		cmd.Process.Release()
	}
}
