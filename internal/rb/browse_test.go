package rb

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

func browseGoMode(raw uint32) fs.FileMode {
	m := fs.FileMode(raw & 0o777)
	switch raw & 0o170000 {
	case 0o040000:
		m |= fs.ModeDir
	case 0o120000:
		m |= fs.ModeSymlink
	case 0o060000:
		m |= fs.ModeDevice
	case 0o020000:
		m |= fs.ModeDevice | fs.ModeCharDevice
	case 0o010000:
		m |= fs.ModeNamedPipe
	case 0o140000:
		m |= fs.ModeSocket
	}
	if raw&0o4000 != 0 {
		m |= fs.ModeSetuid
	}
	if raw&0o2000 != 0 {
		m |= fs.ModeSetgid
	}
	if raw&0o1000 != 0 {
		m |= fs.ModeSticky
	}
	return m
}

func TestBrowsePermAndType(t *testing.T) {
	cases := []struct {
		mode       uint32
		perm, kind string
	}{
		{0o100644, "-rw-r--r--", "file"},
		{0o100640, "-rw-r-----", "file"},
		{0o104755, "urwxr-xr-x", "file"},
		{0o102755, "grwxr-xr-x", "file"},
		{0o040755, "drwxr-xr-x", "dir"},
		{0o041777, "dtrwxrwxrwx", "dir"},
		{0o120777, "Lrwxrwxrwx", "symlink"},
		{0o010644, "prw-r--r--", "fifo"},
		{0o140755, "Srwxr-xr-x", "socket"},
		{0o060660, "Drw-rw----", "dev"},
		{0o020666, "Dcrw-rw-rw-", "chardev"},
		{0o000000, "----------", "irregular"},
	}
	for _, c := range cases {
		if got := browsePerm(c.mode); got != c.perm {
			t.Errorf("perm %o: got %q want %q", c.mode, got, c.perm)
		}
		if got := browseType(c.mode); got != c.kind {
			t.Errorf("type %o: got %q want %q", c.mode, got, c.kind)
		}
	}
	for _, typ := range []uint32{0, 0o040000, 0o100000, 0o120000, 0o060000, 0o020000, 0o010000, 0o140000} {
		for bits := uint32(0); bits < 0o10000; bits += 0o111 {
			raw := typ | bits
			if typ == 0 {
				raw = 0o100000 | bits
			}
			if got, want := browsePerm(raw), browseGoMode(raw).String(); got != want {
				t.Errorf("perm %o: got %q, Go writes %q", raw, got, want)
			}
		}
	}
}

func TestBrowseMountinfoEscape(t *testing.T) {
	dir := "/tmp/a b\\c\td\ne"
	if got := browseMountEscape(dir); got != `/tmp/a\040b\134c\011d\012e` {
		t.Fatalf("escape: %q", got)
	}
	info := "36 35 98:0 / / rw,noatime master:1 - ext3 /dev/root rw\n" +
		`120 36 0:55 / /tmp/a\040b\134c\011d\012e rw,nosuid,nodev - fuse restic rw` + "\n"
	if !browseMountinfoHas(info, dir) {
		t.Error("escaped mount point not found")
	}
	if browseMountinfoHas(info, "/tmp/a b") {
		t.Error("prefix matched")
	}
	if browseMountinfoHas(info, "/dev/root") || browseMountinfoHas("", "/") {
		t.Error("wrong field matched")
	}
	if !browseMountinfoHas(info, "/") {
		t.Error("root not found")
	}
}

func TestBrowseList(t *testing.T) {
	old := RunDir
	RunDir = t.TempDir()
	defer func() { RunDir = old }()
	id := "repo1"
	f := browseFiles(id)
	snap := f.Mnt + "/ids/abcdef12"
	src := snap + "/src"
	for _, d := range []string{src + "/dir/sub", src + "/empty", src + "/odd name ü"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(src+"/dir/a.txt", []byte("a\n"), 0o640)
	os.Chmod(src+"/dir/a.txt", 0o640)
	mt := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(src+"/dir/a.txt", mt, mt)
	os.WriteFile(src+"/odd name ü/f [1].txt", []byte("x"), 0o644)
	os.Symlink("/etc", src+"/etclink")
	os.Symlink("dir", src+"/rellink")
	os.Symlink("dir/a.txt", src+"/filelink")
	syscall.Mkfifo(src+"/fifo", 0o644)

	if got := browseList(id, "ffffffff00", "/"); got != nil {
		t.Errorf("unknown snapshot: %v", got)
	}
	if _, err := os.Stat(f.Used); err != nil {
		t.Error("used file not touched")
	}
	root := browseList(id, "abcdef1234567890", "/")
	if len(root) != 1 || root[0].Name != "src" || root[0].Path != "/src" || root[0].Type != "dir" || root[0].Size != nil {
		t.Errorf("root: %+v", root)
	}
	top := browseList(id, "abcdef12", "/src")
	names := []string{}
	byName := map[string]browseEntry{}
	for _, e := range top {
		names = append(names, e.Name)
		byName[e.Name] = e
	}
	want := []string{"dir", "empty", "etclink", "fifo", "filelink", "odd name ü", "rellink"}
	if len(names) != len(want) {
		t.Fatalf("names: %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names: %v", names)
		}
	}
	if e := byName["etclink"]; e.Type != "symlink" || e.Perm[0] != 'L' || e.Size != nil || e.Path != "/src/etclink" {
		t.Errorf("etclink: %+v", e)
	}
	if e := byName["fifo"]; e.Type != "fifo" || e.Perm != "prw-r--r--" {
		t.Errorf("fifo: %+v", e)
	}
	files := browseList(id, "abcdef12", "/src/dir")
	if len(files) != 2 || files[0].Name != "a.txt" || files[0].Size == nil || *files[0].Size != 2 ||
		files[0].Mtime == nil || *files[0].Mtime != mt.Unix() || files[0].Perm != "-rw-r-----" || files[0].Path != "/src/dir/a.txt" {
		t.Errorf("dir: %+v", files)
	}
	if odd := browseList(id, "abcdef12", "/src/odd name ü"); len(odd) != 1 || odd[0].Path != "/src/odd name ü/f [1].txt" {
		t.Errorf("odd: %+v", odd)
	}
	for _, p := range []string{"/src/etclink", "/src/etclink/ssh", "/src/rellink", "/src/dir/../dir", "/src/../../../../etc",
		"/src/dir/a.txt", "/src/nothing", "/src/dir/", "//src", "/src/filelink"} {
		got := browseList(id, "abcdef12", p)
		if got == nil || len(got) != 0 {
			t.Errorf("%s: want empty, got %+v", p, got)
		}
	}
	if got := browseList(id, "abcdef12", "/src/empty"); got == nil || len(got) != 0 {
		t.Errorf("empty dir: %+v", got)
	}
}

func TestBrowseStatusAndReap(t *testing.T) {
	old := RunDir
	RunDir = t.TempDir()
	defer func() { RunDir = old }()
	write := func(id string, s browseState) { jsonWrite(browseFiles(id).Status, s, 0o644) }
	write("starting", browseState{Status: "starting", Started: now()})
	write("stale", browseState{Status: "starting", Started: now() - 60})
	write("live", browseState{Status: "ready", Pid: os.Getpid(), Started: now()})
	write("dead", browseState{Status: "ready", Pid: 999999, Started: now()})
	write("err", browseState{Status: "error", Ended: now()})
	write("olderr", browseState{Status: "error", Ended: now() - browseRetry})
	if !browseRunning("starting") || browseRunning("stale") || !browseRunning("live") || browseRunning("dead") {
		t.Error("running")
	}
	if s := browseStatus("err"); s == nil || browseRunning("err") || browseStatus("olderr") != nil {
		t.Error("error states")
	}
	if len(browseRepos()) != 6 {
		t.Errorf("repos: %v", browseRepos())
	}
	browseReap(false)
	left := map[string]bool{}
	for _, id := range browseRepos() {
		left[id] = true
	}
	if !left["starting"] || !left["live"] || !left["err"] || left["stale"] || left["dead"] || left["olderr"] || len(left) != 3 {
		t.Errorf("after reap: %v", left)
	}
}
