package rb

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func phpEval(t *testing.T, code string, env ...string) []byte {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("php not installed")
	}
	root, _ := filepath.Abs("../..")
	lib := filepath.Join(root, "src/include/lib/consistency.php")
	if !isFile(lib) {
		return nil
	}
	cmd := exec.Command(php, "-r", "require "+phpString(lib)+"; "+code)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("php: %v: %s", err, out)
	}
	return out
}

func phpString(s string) string {
	return phpJSON(s)
}

func phpJSON(v any) string {
	b, _ := json.Marshal(v)
	return "json_decode(" + "'" + strings.ReplaceAll(strings.ReplaceAll(string(b), `\`, `\\`), "'", `\'`) + "'" + ", true)"
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestZfsMountinfoPoints(t *testing.T) {
	info := strings.Join([]string{
		"36 35 98:0 / /mnt1 rw,noatime master:1 - ext3 /dev/root rw,errors=continue",
		`412 30 0:77 / /mnt/cache/my\040app rw shared:200 - zfs cache/my\040app@restic-backup-1 ro,xattr`,
		`413 30 0:77 / /var/lib/x rw - zfs cache/my\040app@restic-backup-1 ro`,
		`414 30 0:78 / /var/lib/y rw - zfs cache/my\040app@restic-backup-10 ro`,
	}, "\n")
	needle := zfsMountNeedle("cache/my app@restic-backup-1")
	if needle != ` - zfs cache/my\040app@restic-backup-1 ` {
		t.Fatalf("needle %q", needle)
	}
	got := mountinfoPoints(info, needle)
	want := []string{"/mnt/cache/my app", "/var/lib/x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("points %q", got)
	}
	if zfsMountNeedle(`a\b	c`) != ` - zfs a\134b\011c ` {
		t.Fatal("escaping")
	}
}

func TestZfsHoldersFakeProc(t *testing.T) {
	proc := t.TempDir()
	name := "tank/data@restic-backup-x"
	line := `500 30 0:77 / /stage/0 ro - zfs tank/data@restic-backup-x ro` + "\n"
	cid := strings.Repeat("ab", 32)
	mk := func(pid, ns, info, cgroup, comm string) {
		d := filepath.Join(proc, pid)
		os.MkdirAll(filepath.Join(d, "ns"), 0o755)
		os.Symlink("mnt:["+ns+"]", filepath.Join(d, "ns", "mnt"))
		os.WriteFile(filepath.Join(d, "mountinfo"), []byte(info), 0o644)
		os.WriteFile(filepath.Join(d, "cgroup"), []byte(cgroup), 0o644)
		os.WriteFile(filepath.Join(d, "comm"), []byte(comm+"\n"), 0o644)
	}
	mk("1", "1", "1 0 0:1 / / rw - ext4 /dev/sda1 rw\n", "0::/\n", "init")
	mk("10", "2", line, "0::/system.slice/docker-"+cid+".scope\n", "nginx")
	mk("11", "2", line, "0::/\n", "sh")
	mk("20", "3", line, "0::/\n", "restic")
	os.MkdirAll(filepath.Join(proc, "self", "ns"), 0o755)
	os.Symlink("mnt:[3]", filepath.Join(proc, "self", "ns", "mnt"))
	got := zfsHoldersIn(proc, name)
	if len(got) != 2 {
		t.Fatalf("holders %+v", got)
	}
	if got[0].PID != 10 || got[0].Container != cid || got[0].Own || got[0].Comm != "nginx" || got[0].Points[0] != "/stage/0" {
		t.Fatalf("container holder %+v", got[0])
	}
	if got[1].PID != 20 || got[1].Container != "" || !got[1].Own || got[1].Comm != "restic" {
		t.Fatalf("own holder %+v", got[1])
	}
}

func TestZfsWrapScript(t *testing.T) {
	stage := t.TempDir() + "/ns/op"
	w := t.TempDir() + "/data/cache"
	plan := &ZfsPlan{Binds: []ZfsBind{
		{"cache/appdata", "/mnt/cache/appdata", "", "/mnt/user/appdata"},
		{"cache/appdata/db it's", "/mnt/cache/appdata/db it's", "", "/mnt/user/appdata/db it's"},
		{"tank/media", "/mnt/tank/media", "/photos", "/mnt/user/media/photos"},
	}}
	writable := []string{w, "/elsewhere"}
	plan.Binds = append(plan.Binds, ZfsBind{"cache/rb", "/mnt/cache/rb", "", filepath.Dir(w)})
	got := zfsWrap([]string{"restic", "backup", "/mnt/user/appdata"}, plan, "restic-backup-1", stage, writable)
	if !reflect.DeepEqual(got[:9], []string{"unshare", "--mount", "--propagation", "private", "--", "/bin/sh", "-c", got[7], "sh"}) {
		t.Fatalf("prefix %q", got[:9])
	}
	if !reflect.DeepEqual(got[9:], []string{"restic", "backup", "/mnt/user/appdata"}) {
		t.Fatalf("cmd %q", got[9:])
	}
	script := got[7]
	for _, want := range []string{
		"set -e\nmount --bind '" + w + "' '" + stage + "/w0'\n",
		"mount -t zfs -o ro 'cache/appdata/db it'\\''s@restic-backup-1' '" + stage + "/1'\n",
		"[ -e '" + stage + "/2/photos' ] || { echo 'The snapshot of tank/media has no /photos.' >&2; exit 1; }\n",
		"echo 'The snapshot of cache/appdata has no /.' >&2",
		"[ -d '" + w + "' ] || { echo '" + w + " is not in the snapshot, so restic cannot write there.' >&2; exit 1; }\nmount --bind '" + stage + "/w0' '" + w + "'\nexec \"$@\"\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
	if !isDir(stage+"/w0") || !isDir(stage+"/3") || !isDir(w) {
		t.Fatal("stage dirs not created")
	}
	var binds [][]string
	for _, b := range plan.Binds {
		binds = append(binds, []string{b.Dataset, b.Mount, b.Sub, b.Target})
	}
	php := phpEval(t, "echo json_encode(rb_zfs_wrap(array('restic','backup','/mnt/user/appdata'), array('binds' => "+phpJSON(binds)+
		"), 'restic-backup-1', "+phpString(stage)+", "+phpJSON(writable)+"));")
	var want []string
	if php != nil {
		if err := json.Unmarshal(php, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("differs from PHP:\nGo:  %q\nPHP: %q", got, want)
		}
	}
	zfsUnstage(stage)
	if exists(stage) {
		t.Fatal("unstage left the stage dir")
	}
}

func TestZfsPlanFakeList(t *testing.T) {
	dir := t.TempDir()
	old := zfsBin
	defer func() { zfsBin = old }()
	zfsBin = writeScript(t, dir, "zfs", `printf 'cache\t/mnt/cache\tyes\ncache/appdata\t/mnt/cache/appdata\tyes\ncache/appdata/plex\t/mnt/cache/appdata/plex\tyes\ncache/off\t/mnt/cache/off\tno\ncache/legacy\tlegacy\tyes\ntank\t/mnt/tank/\tyes\ntank/media/sub\t/mnt/tank/media/sub\tyes\n'`)
	mounts := zfsMounts()
	if len(mounts) != 5 || mounts[0].Dataset != "cache/appdata/plex" || mounts[4].Point != "/mnt/tank" && mounts[3].Point != "/mnt/tank" {
		t.Fatalf("mounts %+v", mounts)
	}
	sources := []string{"/mnt/cache/appdata", "/mnt/tank/media", "/var/tmp", "/mnt/cache/appdata/plex/Library"}
	plan, err := zfsPlan(sources, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Datasets, []string{"cache/appdata", "cache/appdata/plex", "tank", "tank/media/sub"}) {
		t.Fatalf("datasets %q", plan.Datasets)
	}
	if len(plan.Live) != 1 || plan.Live[0].Path != "/var/tmp" {
		t.Fatalf("live %+v", plan.Live)
	}
	if plan.Binds[1].Target != "/mnt/tank/media" || plan.Binds[1].Sub != "/media" || plan.Binds[4].Sub != "/Library" {
		t.Fatalf("binds %+v", plan.Binds)
	}
	var mm [][]string
	for _, m := range mounts {
		mm = append(mm, []string{m.Dataset, m.Point})
	}
	php := phpEval(t, "echo json_encode(rb_zfs_plan("+phpJSON(sources)+", "+phpJSON(mm)+"));")
	var want struct {
		Datasets []string   `json:"datasets"`
		Binds    [][]string `json:"binds"`
		Live     [][]string `json:"live"`
	}
	if php != nil {
		if err := json.Unmarshal(php, &want); err != nil {
			t.Fatal(err)
		}
	}
	var gb, gl [][]string
	for _, b := range plan.Binds {
		gb = append(gb, []string{b.Dataset, b.Mount, b.Sub, b.Target})
	}
	for _, l := range plan.Live {
		gl = append(gl, []string{l.Path, l.Why})
	}
	if php != nil && (!reflect.DeepEqual(plan.Datasets, want.Datasets) || !reflect.DeepEqual(gb, want.Binds) || !reflect.DeepEqual(gl, want.Live)) {
		t.Fatalf("differs from PHP:\nGo:  %q %q %q\nPHP: %+v", plan.Datasets, gb, gl, want)
	}
	if _, err := zfsPlan([]string{"/var/tmp"}, nil); err == nil || !strings.HasPrefix(err.Error(), "/var/tmp is not on a ZFS dataset (") ||
		!strings.HasSuffix(err.Error(), "). Choose another consistency setting for this job.") {
		t.Fatalf("single error %v", err)
	}
	if _, err := zfsPlan([]string{"/var/tmp", "/etc"}, nil); err == nil || err.Error() != "None of these folders is on a ZFS dataset. Choose another consistency setting for this job." {
		t.Fatalf("multi error %v", err)
	}
	zfsBin = writeScript(t, dir, "zfs2", "exit 1\n")
	if _, err := zfsPlan([]string{"/mnt/cache"}, nil); err == nil || err.Error() != "No ZFS file systems are mounted on this server." {
		t.Fatalf("no zfs error %v", err)
	}
}

func TestZfsSnapshotDestroyFake(t *testing.T) {
	dir := t.TempDir()
	logf := filepath.Join(dir, "calls")
	old := zfsBin
	defer func() { zfsBin = old }()
	zfsBin = writeScript(t, dir, "zfs", `echo "$@" >> `+logf+`
case "$1 $2" in
"destroy a/x@s") echo "cannot destroy snapshot a/x@s: dataset is busy" >&2; exit 1 ;;
"destroy b@s") echo "could not find any snapshots to destroy" >&2; exit 1 ;;
esac
exit 0
`)
	if err := zfsSnapshot([]string{"a/x", "b", "a/y"}, "s"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	left := zfsDestroy([]string{"a/x", "b", "c"}, "s", nil)
	if len(left) != 1 || left[0].Name != "a/x@s" || left[0].Why != "dataset is busy" {
		t.Fatalf("left %+v", left)
	}
	if time.Since(start) < 3*time.Second {
		t.Fatal("busy snapshot was not retried")
	}
	calls, _ := os.ReadFile(logf)
	want := "snapshot a/x@s a/y@s\nsnapshot b@s\ndestroy a/x@s\ndestroy a/x@s\ndestroy a/x@s\ndestroy a/x@s\ndestroy b@s\ndestroy c@s\n"
	if string(calls) != want {
		t.Fatalf("calls:\n%s", calls)
	}
	zfsBin = writeScript(t, dir, "zfs3", "echo 'cannot create snapshot: out of space' >&2; exit 1\n")
	if err := zfsSnapshot([]string{"a"}, "s"); err == nil || err.Error() != "zfs snapshot failed: cannot create snapshot: out of space" {
		t.Fatalf("snapshot error %v", err)
	}
}

func TestZfsNotifyArgs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs setsid")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	old := notifyBin
	defer func() { notifyBin = old }()
	notifyBin = writeScript(t, dir, "notify", `for a in "$@"; do printf '%s|' "$a"; done > `+out+".tmp; mv "+out+".tmp "+out+"\n")
	want := []string{notifyBin, "-e", "Restic Backup", "-s", "Backup failed", "-d", "it's broken", "-i", "alert"}
	if got := notifyArgs("Backup failed", "it's broken", "alert", ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("args %q", got)
	}
	if got := notifyArgs("s", "d", "normal", "m"); got[len(got)-2] != "-m" || got[len(got)-1] != "m" {
		t.Fatalf("message %q", got)
	}
	if !notify("Backup failed", "it's broken", "alert", "") {
		t.Fatal("notify returned false")
	}
	var b []byte
	for i := 0; i < 50 && len(b) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		b, _ = os.ReadFile(out)
	}
	if string(b) != strings.Join(want[1:], "|")+"|" {
		t.Fatalf("notify ran with %q", b)
	}
	notifyBin = filepath.Join(dir, "missing")
	if notify("a", "b", "normal", "") {
		t.Fatal("notify without binary")
	}
	if got := healthcheckArgs("https://hc/ping/x/", "start", "log"); !reflect.DeepEqual(got,
		[]string{"curl", "-fsS", "-m", "10", "--retry", "2", "-o", "/dev/null", "--data-binary", "@-", "https://hc/ping/x/start"}) {
		t.Fatalf("hc %q", got)
	}
	if got := healthcheckArgs("https://hc/x", "success", ""); got[len(got)-1] != "https://hc/x" || len(got) != 9 {
		t.Fatalf("hc %q", got)
	}
}

func TestZfsJournalRecover(t *testing.T) {
	dir := t.TempDir()
	oldRun, oldZfs, oldDocker, oldVirsh := RunDir, zfsBin, dockerBin, virshBin
	defer func() { RunDir, zfsBin, dockerBin, virshBin = oldRun, oldZfs, oldDocker, oldVirsh }()
	RunDir = dir
	calls := filepath.Join(dir, "calls")
	zfsBin = writeScript(t, dir, "zfs", `echo "zfs $*" >> `+calls+"\n")
	dockerBin = writeScript(t, dir, "docker", `echo "docker $*" >> `+calls+`
[ "$1" = start ] && [ "$2" = bad ] && { echo "no such container" >&2; exit 1; }
exit 0
`)
	virshBin = writeScript(t, dir, "virsh", `echo "virsh $*" >> `+calls+"\n")
	p := exec.Command("true")
	p.Run()
	dead := p.Process.Pid
	runLog := filepath.Join(dir, "run.log")
	os.MkdirAll(dir+"/journal", 0o755)
	php := `{
    "containers": ["web"],
    "vms": ["win"],
    "snapshots": ["cache/appdata"],
    "snap": "restic-backup-1",
    "log": "` + runLog + `",
    "runner_pid": ` + itoa(dead) + `
}
`
	os.WriteFile(journalFile("op1"), []byte(php), 0o644)
	os.WriteFile(journalFile("op2"), []byte(`{"containers":["bad"],"vms":[],"snapshots":[],"snap":"","log":"","runner_pid":`+itoa(dead)+`}`), 0o644)
	journalWrite("live", Journal{Containers: []string{"x"}})
	notes := journalRecover()
	if len(notes) != 1 || notes[0].Subject != "Backup interrupted" ||
		notes[0].Description != "A backup ended unexpectedly; removed its ZFS snapshots, started web, started VM win." {
		t.Fatalf("notes %+v", notes)
	}
	if exists(journalFile("op1")) || !exists(journalFile("live")) {
		t.Fatal("journal files")
	}
	j, _ := jsonRead(journalFile("op2"))
	if toInt(j["attempts"]) != 1 || len(stringList(j["containers"])) != 1 || len(stringList(j["done"])) != 0 {
		t.Fatalf("op2 journal %v", j)
	}
	lg, _ := os.ReadFile(runLog)
	if !strings.HasSuffix(string(lg), " The runner was gone; the scheduler removed its ZFS snapshots, started web, started VM win.\n") {
		t.Fatalf("log %q", lg)
	}
	c, _ := os.ReadFile(calls)
	if !strings.Contains(string(c), "zfs destroy cache/appdata@restic-backup-1\n") || !strings.Contains(string(c), "docker start web\n") ||
		!strings.Contains(string(c), "virsh start win\n") {
		t.Fatalf("calls %s", c)
	}
	j["attempts"] = 29
	jsonWrite(journalFile("op2"), j, 0o644)
	notes = journalRecover()
	if len(notes) != 1 || notes[0].Description != "A backup ended unexpectedly; nothing needed putting back. Could not, after 30 minutes of trying: bad (no such container)" {
		t.Fatalf("notes %+v", notes)
	}
	live, _ := jsonRead(journalFile("live"))
	if toInt(live["runner_pid"]) != os.Getpid() || has(live, "done") || has(live, "ended") {
		t.Fatalf("live journal %v", live)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestZfsDetectFake(t *testing.T) {
	dir := t.TempDir()
	oldZfs, oldDocker, oldVirsh := zfsBin, dockerBin, virshBin
	defer func() { zfsBin, dockerBin, virshBin = oldZfs, oldDocker, oldVirsh }()
	zfsBin = writeScript(t, dir, "zfs", `printf 'cache/appdata\t/mnt/cache/appdata\tyes\n'`)
	inspect := `[{"Id":"aaa","Name":"/plex","State":{"Running":true},"Mounts":[{"Type":"bind","RW":true,"Source":"/mnt/user/appdata/plex"}]},
{"Id":"bbb","Name":"/dockge","State":{"Running":false},"Mounts":[{"Type":"bind","RW":true,"Source":"/mnt/cache/appdata"},{"Type":"bind","RW":false,"Source":"/mnt/user/appdata/x"}]},
{"Id":"ccc","Name":"/other","State":{"Running":true},"Mounts":[{"Type":"volume","RW":true,"Source":"/var/lib/docker/volumes/v"}]}]`
	os.WriteFile(filepath.Join(dir, "inspect.json"), []byte(inspect), 0o644)
	dockerBin = writeScript(t, dir, "docker", `case "$1" in ps) printf 'aaa\nbbb\nccc\n' ;; inspect) cat `+dir+"/inspect.json ;; esac\n")
	virshBin = writeScript(t, dir, "virsh", `case "$1" in
list) printf 'win\n\n' ;;
domblklist) printf ' Type   Device   Target   Source\n------\n file   disk     hdc      /mnt/user/domains/win/vdisk 1.img\n file   cdrom    hda      -\n' ;;
domstate) printf 'shut off\n\n' ;;
esac
`)
	d := consistencyDetect([]string{"/mnt/user/appdata/plex", "/mnt/cache/appdata/db"})
	if v := consistencyDetect([]string{"/mnt/user/domains"}).VMs; len(v) != 1 || v[0].Path != "/mnt/user/domains/win/vdisk 1.img" || v[0].Running {
		t.Fatalf("vms %+v", v)
	}
	b := string(jsonEncode(d, false))
	want := `{"docker":true,"libvirt":true,"containers":[{"name":"plex","running":true,"path":"/mnt/user/appdata/plex"}],"broad":[{"name":"dockge","running":false,"path":"/mnt/cache/appdata"}],"vms":[],"zfs":{"ok":true,"error":"","datasets":["cache/appdata"],"live":[{"path":"/mnt/user/appdata/plex","why":"the share appdata was not found on any pool or disk"}]}}` + "\n"
	if b != want {
		t.Fatalf("detect\n%s", b)
	}
	php := phpEval(t, "echo json_encode(rb_consistency_detect(array('/mnt/user/appdata/plex', '/mnt/cache/appdata/db')), JSON_UNESCAPED_SLASHES).\"\\n\";",
		"RB_ZFS="+zfsBin, "RB_DOCKER="+dockerBin, "RB_VIRSH="+virshBin)
	if php != nil && string(php) != b {
		t.Fatalf("differs from PHP:\nGo:  %sPHP: %s", b, php)
	}
}

func TestZfsRealPool(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	if _, err := exec.LookPath("zpool"); err != nil {
		t.Skip("no zpool")
	}
	oldMnt, oldRun := Mnt, RunDir
	defer func() { Mnt, RunDir = oldMnt, oldRun }()
	base, _ := os.MkdirTemp("/var/tmp", "rbgotest")
	defer os.RemoveAll(base)
	Mnt = base + "/mnt"
	RunDir = base + "/run"
	img := base + "/pool.img"
	f, _ := os.Create(img)
	f.Truncate(256 << 20)
	f.Close()
	pool := "rbgotest"
	if out, err := exec.Command("zpool", "create", "-m", Mnt+"/"+pool, pool, img).CombinedOutput(); err != nil {
		t.Fatalf("zpool create: %v %s", err, out)
	}
	defer exec.Command("zpool", "destroy", "-f", pool).Run()
	exec.Command("zfs", "create", pool+"/appdata").Run()
	exec.Command("zfs", "create", pool+"/appdata/db").Run()
	os.WriteFile(Mnt+"/"+pool+"/appdata/a.txt", []byte("before"), 0o644)
	os.WriteFile(Mnt+"/"+pool+"/appdata/db/b.txt", []byte("before"), 0o644)
	os.MkdirAll(Mnt+"/"+pool+"/appdata/cache", 0o755)
	src := Mnt + "/user/appdata"
	os.MkdirAll(src, 0o755)
	if real, why := userPathLocation(src); real != Mnt+"/"+pool+"/appdata" {
		t.Fatalf("location %q %q", real, why)
	}
	plan, err := zfsPlan([]string{src}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Datasets, []string{pool + "/appdata", pool + "/appdata/db"}) {
		t.Fatalf("datasets %q", plan.Datasets)
	}
	snap := "restic-backup-gotest"
	if err := zfsSnapshot(plan.Datasets, snap); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(Mnt+"/"+pool+"/appdata/a.txt", []byte("after"), 0o644)
	os.WriteFile(Mnt+"/"+pool+"/appdata/db/b.txt", []byte("after"), 0o644)
	stage := RunDir + "/ns/op"
	cache := Mnt + "/user/appdata/cache"
	cmd := zfsWrap([]string{"sh", "-c", `cat "$1/a.txt" "$1/db/b.txt"; echo x > "$2/w"`, "sh", src, cache}, plan, snap, stage, []string{cache})
	out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil || string(out) != "beforebefore" {
		t.Fatalf("wrapped: %v %q", err, out)
	}
	if b, _ := os.ReadFile(cache + "/w"); string(b) != "x\n" {
		t.Fatalf("writable folder not written: %q", b)
	}
	if h := zfsHolders(pool + "/appdata@" + snap); len(h) != 0 {
		t.Fatalf("holders after the namespace ended %+v", h)
	}
	zfsUnstage(stage)
	if exists(stage) {
		t.Fatal("stage left")
	}
	exec.Command("zfs", "hold", "keep", pool+"/appdata/db@"+snap).Run()
	left := zfsDestroy(plan.Datasets, snap, nil)
	if len(left) != 1 || left[0].Name != pool+"/appdata/db@"+snap || !strings.Contains(left[0].Why, "busy") && !strings.Contains(left[0].Why, "hold") {
		t.Fatalf("left %+v", left)
	}
	exec.Command("zfs", "release", "keep", pool+"/appdata/db@"+snap).Run()
	if left := zfsDestroy(plan.Datasets, snap, nil); len(left) != 0 {
		t.Fatalf("left %+v", left)
	}
}
