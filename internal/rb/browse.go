package rb

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	browseRetry       = 600
	browseStartGrace  = 30
	browseOpenTimeout = 900
	browseKillAfter   = 15 * time.Second
)

func init() {
	register("browse", "<repository id>  serve a restic mount for browsing snapshots", browseServe)
	register("test-browse", "", testBrowse)
}

type browseFileSet struct {
	Mnt    string
	Status string
	Used   string
	Stop   string
	Lock   string
}

type browseState struct {
	Status  string `json:"status"`
	Pid     int    `json:"pid"`
	Restic  int    `json:"restic"`
	Started int64  `json:"started,omitempty"`
	Ready   int64  `json:"ready,omitempty"`
	Ended   int64  `json:"ended,omitempty"`
	Error   string `json:"error,omitempty"`
}

type browseEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Path  string `json:"path"`
	Size  *int64 `json:"size"`
	Mtime *int64 `json:"mtime"`
	Perm  string `json:"perm"`
}

func browseDir() string {
	return RunDir + "/browse"
}

func browseIdle() int64 {
	v := os.Getenv("RB_BROWSE_IDLE")
	if v == "" || v == "0" {
		return 600
	}
	return int64(toInt(v))
}

func browseFiles(repoID string) browseFileSet {
	base := browseDir() + "/" + repoID
	return browseFileSet{Mnt: base, Status: base + ".json", Used: base + ".used", Stop: base + ".stop", Lock: base + ".lock"}
}

func browseAvailable() bool {
	return os.Getenv("RB_BROWSE") != "off" && exists("/dev/fuse")
}

func browseRead(path string) *browseState {
	m, ok := jsonRead(path)
	if !ok {
		return nil
	}
	return &browseState{
		Status:  str(m["status"]),
		Pid:     toInt(m["pid"]),
		Restic:  toInt(m["restic"]),
		Started: toInt64(m["started"]),
		Ready:   toInt64(m["ready"]),
		Ended:   toInt64(m["ended"]),
		Error:   str(m["error"]),
	}
}

func browseStatus(repoID string) *browseState {
	s := browseRead(browseFiles(repoID).Status)
	if s == nil {
		return nil
	}
	if s.Status == "error" {
		if now()-s.Ended < browseRetry {
			return s
		}
		return nil
	}
	if s.Pid == 0 {
		if now()-s.Started < browseStartGrace {
			return s
		}
		return nil
	}
	if pidAlive(s.Pid) {
		return s
	}
	return nil
}

func browseRunning(repoID string) bool {
	s := browseStatus(repoID)
	return s != nil && s.Status != "error"
}

func browseStart(repo Repo) *browseState {
	if s := browseStatus(repo.ID); s != nil {
		return s
	}
	os.MkdirAll(browseDir(), 0o755)
	s := &browseState{Status: "starting", Started: now()}
	jsonWrite(browseFiles(repo.ID).Status, s, 0o644)
	spawn("browse", repo.ID)
	return s
}

func browseStop(repoID string, wait int) {
	if !browseRunning(repoID) {
		return
	}
	browseTouch(browseFiles(repoID).Stop)
	for i := 0; i < wait*5 && browseRunning(repoID); i++ {
		time.Sleep(200 * time.Millisecond)
	}
}

func browseRepos() []string {
	files, _ := filepath.Glob(browseDir() + "/*.json")
	ids := make([]string, 0, len(files))
	for _, f := range files {
		ids = append(ids, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	return ids
}

func browseReap(all bool) {
	for _, id := range browseRepos() {
		f := browseFiles(id)
		s := browseRead(f.Status)
		if s == nil {
			continue
		}
		alive := pidAlive(s.Pid)
		if !all && (alive || (s.Pid == 0 && now()-s.Started < browseStartGrace)) {
			continue
		}
		server := 0
		if alive {
			server = s.Pid
		}
		for _, pid := range []int{s.Restic, server} {
			if pid > 0 && pidAlive(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		browseUnmount(f.Mnt)
		if s.Status != "error" || all || now()-s.Ended >= browseRetry {
			os.Remove(f.Status)
		}
		os.Remove(f.Stop)
	}
}

func browseMountEscape(dir string) string {
	return strings.NewReplacer(`\`, `\134`, " ", `\040`, "\t", `\011`, "\n", `\012`).Replace(dir)
}

func browseMountinfoHas(mountinfo, dir string) bool {
	want := browseMountEscape(dir)
	for _, line := range strings.Split(mountinfo, "\n") {
		f := strings.Split(line, " ")
		if len(f) > 4 && f[4] == want {
			return true
		}
	}
	return false
}

func browseMounted(dir string) bool {
	b, err := os.ReadFile("/proc/self/mountinfo")
	return err == nil && browseMountinfoHas(string(b), dir)
}

func browseUnmount(dir string) {
	for i := 0; i < 3 && browseMounted(dir); i++ {
		rbCmd([]string{"umount", "-l", dir}, 15*time.Second)
	}
}

func browseTouch(path string) {
	t := time.Now()
	if os.Chtimes(path, t, t) == nil {
		return
	}
	if f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
		f.Close()
	}
}

func browseRealpath(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	return real, true
}

func browseList(repoID, snap, path string) []browseEntry {
	f := browseFiles(repoID)
	browseTouch(f.Used)
	if len(snap) > 8 {
		snap = snap[:8]
	}
	base, ok := browseRealpath(f.Mnt + "/ids/" + snap)
	if !ok || !isDir(base) {
		return nil
	}
	want := base + path
	if path == "/" {
		want = base
	}
	real, ok := browseRealpath(want)
	if !ok || real != want || !isDir(real) {
		return []browseEntry{}
	}
	d, err := os.Open(real)
	if err != nil {
		return nil
	}
	names, err := d.Readdirnames(-1)
	d.Close()
	if err != nil {
		return nil
	}
	sort.Strings(names)
	prefix := path
	if path == "/" {
		prefix = ""
	}
	entries := []browseEntry{}
	for _, name := range names {
		if name == "." || name == ".." {
			continue
		}
		var mode uint32
		var size, mtime *int64
		if st, err := os.Lstat(real + "/" + name); err == nil {
			if sys, ok := st.Sys().(*syscall.Stat_t); ok {
				mode = uint32(sys.Mode)
			}
			m := st.ModTime().Unix()
			mtime = &m
			if mode&0o170000 == 0o100000 {
				n := st.Size()
				size = &n
			}
		}
		entries = append(entries, browseEntry{
			Name:  name,
			Type:  browseType(mode),
			Path:  prefix + "/" + name,
			Size:  size,
			Mtime: mtime,
			Perm:  browsePerm(mode),
		})
	}
	return entries
}

func browseType(mode uint32) string {
	switch mode & 0o170000 {
	case 0o040000:
		return "dir"
	case 0o100000:
		return "file"
	case 0o120000:
		return "symlink"
	case 0o060000:
		return "dev"
	case 0o020000:
		return "chardev"
	case 0o010000:
		return "fifo"
	case 0o140000:
		return "socket"
	}
	return "irregular"
}

func browsePerm(mode uint32) string {
	t := mode & 0o170000
	var b strings.Builder
	flag := func(on bool, c byte) {
		if on {
			b.WriteByte(c)
		}
	}
	flag(t == 0o040000, 'd')
	flag(t == 0o120000, 'L')
	flag(t == 0o060000 || t == 0o020000, 'D')
	flag(t == 0o010000, 'p')
	flag(t == 0o140000, 'S')
	flag(mode&0o4000 != 0, 'u')
	flag(mode&0o2000 != 0, 'g')
	flag(t == 0o020000, 'c')
	flag(mode&0o1000 != 0, 't')
	if b.Len() == 0 {
		b.WriteByte('-')
	}
	for i := 0; i < 9; i++ {
		if mode&(0o400>>uint(i)) != 0 {
			b.WriteByte("rwx"[i%3])
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func browseServe(args []string) int {
	if len(args) < 1 {
		return 2
	}
	cfg := configLoad()
	repo := cfg.repo(args[0])
	if repo == nil {
		return 2
	}
	f := browseFiles(repo.ID)
	os.MkdirAll(browseDir(), 0o755)
	lock := lockFile(f.Lock, false)
	if lock == nil {
		return 0
	}
	defer lock.Unlock()
	os.Remove(f.Stop)
	browseUnmount(f.Mnt)
	os.Mkdir(f.Mnt, 0o755)

	status := browseState{Status: "starting", Pid: os.Getpid(), Started: now()}
	write := func() { jsonWrite(f.Status, status, 0o644) }
	write()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(sigs)

	restic := 0
	killRestic := func() {
		if restic > 0 && pidAlive(restic) {
			syscall.Kill(restic, syscall.SIGKILL)
		}
	}
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if status.Status == "starting" || status.Status == "ready" {
			killRestic()
			browseUnmount(f.Mnt)
			status = browseState{Status: "error", Ended: now(), Error: fmt.Sprintf("Browsing stopped unexpectedly: %v", r)}
			write()
		}
		lock.Unlock()
		os.Exit(1)
	}()

	var errLines []string
	why := ""
	var killTimer *time.Timer
	code := procStream(
		resticCmd(*repo, cfg.Settings, []string{"mount", "--no-lock", f.Mnt}, false),
		resticEnv(*repo, cfg.Settings),
		func(stream, line string) {
			if stream == "err" && len(errLines) < 200 {
				errLines = append(errLines, line)
			}
		},
		func() bool {
			if status.Status == "starting" && browseMounted(f.Mnt) {
				status.Status = "ready"
				status.Ready = now()
				write()
				browseTouch(f.Used)
			}
			select {
			case <-sigs:
				why = "stop"
			default:
			}
			switch {
			case why != "":
			case exists(f.Stop):
				why = "stop"
			case status.Status == "ready" && now()-fileMtime(f.Used) > browseIdle():
				why = "idle"
			case status.Status == "starting" && now()-status.Started > browseOpenTimeout:
				why = "Opening the repository took longer than 15 minutes."
			default:
				return false
			}
			killTimer = time.AfterFunc(browseKillAfter, killRestic)
			return true
		},
		func(pid int) {
			restic = pid
			status.Restic = pid
			write()
		},
		nil)
	if killTimer != nil {
		killTimer.Stop()
	}

	browseUnmount(f.Mnt)
	os.Remove(f.Stop)
	if why != "stop" && why != "idle" {
		message := why
		if message == "" && code != 0 {
			message = resticError(errLines, code)
		}
		if message != "" {
			status = browseState{Status: "error", Ended: now(), Error: message}
			write()
			return 1
		}
	}
	status.Status = "stopped"
	os.Remove(f.Status)
	return 0
}

func testBrowse(args []string) int {
	if len(args) == 0 {
		return 2
	}
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}
	switch args[0] {
	case "mounted":
		if browseMounted(arg) {
			return 0
		}
	case "running":
		if browseRunning(arg) {
			return 0
		}
	case "reap":
		browseReap(false)
		return 0
	default:
		return 2
	}
	return 1
}
