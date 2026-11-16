package rb

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	dockerBin = envOr("RB_DOCKER", "docker")
	virshBin  = envOr("RB_VIRSH", "virsh")
	zfsBin    = envOr("RB_ZFS", "zfs")
)

type ZfsMount struct {
	Dataset string
	Point   string
}

type ZfsBind struct {
	Dataset string
	Mount   string
	Sub     string
	Target  string
}

type ZfsLive struct {
	Path string
	Why  string
}

type ZfsPlan struct {
	Datasets []string
	Binds    []ZfsBind
	Live     []ZfsLive
}

type ZfsStuck struct {
	Name string
	Why  string
}

type ZfsHolder struct {
	PID       int
	Points    []string
	Container string
	Comm      string
	Own       bool
}

type DockerContainer struct {
	ID      string
	Name    string
	Running bool
	Mounts  []string
}

type LibvirtVM struct {
	Name    string
	State   string
	Running bool
	Disks   []string
}

type DetectHit struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Path    string `json:"path"`
}

type DetectLive struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

type DetectZfs struct {
	OK       bool         `json:"ok"`
	Error    string       `json:"error"`
	Datasets []string     `json:"datasets"`
	Live     []DetectLive `json:"live"`
}

type Detection struct {
	Docker     bool        `json:"docker"`
	Libvirt    bool        `json:"libvirt"`
	Containers []DetectHit `json:"containers"`
	Broad      []DetectHit `json:"broad"`
	VMs        []DetectHit `json:"vms"`
	Zfs        DetectZfs   `json:"zfs"`
}

type Journal struct {
	Containers []string
	VMs        []string
	Snapshots  []string
	Snap       string
	Log        string
	Ended      bool
	Done       []string
	Attempts   int
	RunnerPID  int
}

type JournalNotice struct {
	Subject     string
	Description string
}

func userPathLocation(path string) (string, string) {
	mnt := strings.TrimRight(Mnt, "/")
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(mnt) + `/(user0?)/([^/]+)(/.*)?$`)
	m := re.FindStringSubmatch(path)
	if m == nil {
		return path, ""
	}
	share, rest := m[2], m[3]
	var found []string
	mntDev, mntOK := statDev(mnt)
	tops, _ := filepath.Glob(mnt + "/*")
	for _, top := range tops {
		name := filepath.Base(top)
		if strings.HasPrefix(name, ".") || !isDir(top) {
			continue
		}
		if inList(name, "user", "user0", "disks", "remotes", "addons", "rootshare") {
			continue
		}
		if dev, ok := statDev(top); ok == mntOK && dev == mntDev {
			continue
		}
		here := top + "/" + share + rest
		if isFile(here) || (isDir(here) && dirHasEntries(here)) {
			found = append(found, top+"/"+share)
		}
	}
	if len(found) == 1 {
		return found[0] + rest, ""
	}
	if len(found) == 0 {
		return "", "the share " + share + " was not found on any pool or disk"
	}
	dirs := make([]string, len(found))
	for i, f := range found {
		dirs[i] = filepath.Dir(f)
	}
	return "", "the share " + share + " is spread over " + strings.Join(dirs, ", ") +
		" - a ZFS snapshot can only cover one of them"
}

func statDev(p string) (uint64, bool) {
	var st syscall.Stat_t
	if syscall.Stat(p, &st) != nil {
		return 0, false
	}
	return uint64(st.Dev), true
}

func zfsMounts() []ZfsMount {
	code, lines, _ := rbCmd([]string{zfsBin, "list", "-H", "-p", "-t", "filesystem", "-o", "name,mountpoint,mounted"}, 30*time.Second)
	if code != 0 {
		return nil
	}
	out := []ZfsMount{}
	for _, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) == 3 && f[2] == "yes" && f[1] != "" && f[1][0] == '/' {
			p := strings.TrimRight(f[1], "/")
			if p == "" {
				p = "/"
			}
			out = append(out, ZfsMount{f[0], p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Point) > len(out[j].Point) })
	return out
}

func zfsPlan(sources []string, mounts []ZfsMount) (*ZfsPlan, error) {
	if mounts == nil {
		mounts = zfsMounts()
	}
	if len(mounts) == 0 {
		return nil, errors.New("No ZFS file systems are mounted on this server.")
	}
	plan := &ZfsPlan{Datasets: []string{}, Binds: []ZfsBind{}, Live: []ZfsLive{}}
	seen := map[string]bool{}
	addDataset := func(d string) {
		if !seen[d] {
			seen[d] = true
			plan.Datasets = append(plan.Datasets, d)
		}
	}
	for _, src := range sources {
		real, why := userPathLocation(src)
		var owner *ZfsMount
		if real != "" {
			for i := range mounts {
				if under(real, mounts[i].Point) {
					owner = &mounts[i]
					break
				}
			}
		}
		if owner == nil {
			if real != "" {
				why = zfsWhyNot(real)
			}
			plan.Live = append(plan.Live, ZfsLive{src, why})
			continue
		}
		addDataset(owner.Dataset)
		plan.Binds = append(plan.Binds, ZfsBind{owner.Dataset, owner.Point, real[len(strings.TrimRight(owner.Point, "/")):], src})
		for _, m := range mounts {
			if m.Point != real && under(m.Point, real) && m.Point != owner.Point {
				addDataset(m.Dataset)
				plan.Binds = append(plan.Binds, ZfsBind{m.Dataset, m.Point, "",
					strings.TrimRight(src, "/") + m.Point[len(strings.TrimRight(real, "/")):]})
			}
		}
	}
	if len(plan.Datasets) == 0 {
		if len(sources) == 1 {
			return nil, errors.New(plan.Live[0].Path + " is not on a ZFS dataset (" + plan.Live[0].Why +
				"). Choose another consistency setting for this job.")
		}
		return nil, errors.New("None of these folders is on a ZFS dataset. Choose another consistency setting for this job.")
	}
	sort.SliceStable(plan.Binds, func(i, j int) bool {
		a, b := plan.Binds[i].Target, plan.Binds[j].Target
		ca, cb := strings.Count(a, "/"), strings.Count(b, "/")
		if ca != cb {
			return ca < cb
		}
		return a < b
	})
	return plan, nil
}

func zfsWhyNot(real string) string {
	typ := fsType(real)
	where := ""
	if under(real, "/boot") {
		where = "flash drive"
	} else if m := regexp.MustCompile(`^` + regexp.QuoteMeta(strings.TrimRight(Mnt, "/")) + `/([^/]+)/`).FindStringSubmatch(real + "/"); m != nil {
		where = m[1]
	}
	if typ == "" {
		typ = "not ZFS"
	}
	var parts []string
	for _, p := range []string{where, typ} {
		if p != "" && p != "0" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func zfsSnapshot(datasets []string, snap string) error {
	var pools []string
	byPool := map[string][]string{}
	for _, d := range datasets {
		pool, _, _ := strings.Cut(strings.TrimLeft(d, "/"), "/")
		if _, ok := byPool[pool]; !ok {
			pools = append(pools, pool)
		}
		byPool[pool] = append(byPool[pool], d+"@"+snap)
	}
	for _, pool := range pools {
		code, _, errs := rbCmd(append([]string{zfsBin, "snapshot"}, byPool[pool]...), 120*time.Second)
		if code != 0 {
			if errs == "" {
				errs = "exit code " + strconv.Itoa(code)
			}
			return errors.New("zfs snapshot failed: " + errs)
		}
	}
	return nil
}

func zfsDestroy(datasets []string, snap string, log func(string)) []ZfsStuck {
	left := []ZfsStuck{}
	for _, d := range datasets {
		name := d + "@" + snap
		var code int
		var errs string
		gone := false
		for try := 1; ; try++ {
			code, _, errs = rbCmd([]string{zfsBin, "destroy", name}, 120*time.Second)
			low := strings.ToLower(errs)
			if code == 0 || strings.Contains(low, "could not find") || strings.Contains(low, "does not exist") {
				gone = true
				break
			}
			if !strings.Contains(low, "busy") || try == 4 {
				break
			}
			if try == 2 {
				zfsRelease(name, log)
			}
			time.Sleep(time.Second)
		}
		if gone {
			continue
		}
		why := "exit code " + strconv.Itoa(code)
		if errs != "" {
			why = strings.ReplaceAll(errs, "cannot destroy snapshot "+name+": ", "")
		}
		for _, h := range zfsHolders(name) {
			if h.Container != "" {
				why += "; container " + h.Container + " has it mounted"
			} else if h.Own {
				why += "; mounted at " + strings.Join(h.Points, ", ") + " and in use"
			} else {
				why += "; mounted at " + strings.Join(h.Points, ", ") + " for " + h.Comm + " (pid " + strconv.Itoa(h.PID) + ")"
			}
		}
		left = append(left, ZfsStuck{name, why})
	}
	return left
}

func zfsStuckHas(left []ZfsStuck, name string) bool {
	for _, l := range left {
		if l.Name == name {
			return true
		}
	}
	return false
}

var mountEscaper = strings.NewReplacer(" ", `\040`, "\t", `\011`, "\n", `\012`, `\`, `\134`)

func zfsMountNeedle(name string) string {
	return " - zfs " + mountEscaper.Replace(name) + " "
}

func mountinfoPoints(info, needle string) []string {
	var points []string
	for _, line := range strings.Split(info, "\n") {
		if strings.Contains(line, needle) {
			f := strings.Split(line, " ")
			if len(f) > 4 {
				points = append(points, unescapeMount(f[4]))
			}
		}
	}
	return points
}

var dockerCgroup = regexp.MustCompile(`/docker[/-]([0-9a-f]{64})`)

func zfsHoldersIn(proc, name string) []ZfsHolder {
	needle := zfsMountNeedle(name)
	own, ownErr := os.Readlink(proc + "/self/ns/mnt")
	seen := map[string]bool{}
	found := []ZfsHolder{}
	dirs, _ := filepath.Glob(proc + "/[0-9]*")
	for _, dir := range dirs {
		if !isDir(dir) {
			continue
		}
		ns, err := os.Readlink(dir + "/ns/mnt")
		if err != nil || seen[ns] {
			continue
		}
		b, err := os.ReadFile(dir + "/mountinfo")
		if err != nil {
			continue
		}
		seen[ns] = true
		info := string(b)
		if !strings.Contains(info, needle) {
			continue
		}
		pid, _ := strconv.Atoi(filepath.Base(dir))
		h := ZfsHolder{PID: pid, Points: mountinfoPoints(info, needle), Own: ownErr == nil && ns == own}
		cg, _ := os.ReadFile(dir + "/cgroup")
		if m := dockerCgroup.FindStringSubmatch(string(cg)); m != nil {
			h.Container = m[1]
		}
		comm, _ := os.ReadFile(dir + "/comm")
		h.Comm = strings.TrimSpace(string(comm))
		found = append(found, h)
	}
	return found
}

func zfsHolders(name string) []ZfsHolder {
	found := zfsHoldersIn("/proc", name)
	if len(found) == 0 {
		return found
	}
	names := map[string]string{}
	cs, _ := dockerContainers()
	for _, c := range cs {
		names[c.ID] = c.Name
	}
	for i, h := range found {
		if h.Container != "" {
			if n, ok := names[h.Container]; ok {
				found[i].Container = n
			} else {
				found[i].Container = h.Container[:12]
			}
		}
	}
	return found
}

func zfsRelease(name string, log func(string)) {
	if !strings.Contains(name, "@restic-backup-") {
		return
	}
	for _, h := range zfsHolders(name) {
		if h.Container == "" {
			continue
		}
		pid := strconv.Itoa(h.PID)
		for _, point := range h.Points {
			code, _, errs := rbCmd([]string{"nsenter", "-t", pid, "-p", "--", "umount", "-N", "/proc/" + pid + "/ns/mnt", "-l", point}, 30*time.Second)
			if log != nil {
				if code == 0 {
					log("Unmounted the copy of " + name + " that container " + h.Container + " held")
				} else {
					log("Could not unmount the copy of " + name + " that container " + h.Container + " holds: " + errs)
				}
			}
		}
	}
}

func zfsWrapScript(plan *ZfsPlan, snap, stageDir string, writable []string) string {
	var sb strings.Builder
	sb.WriteString("set -e\n")
	var keepOrder []string
	keep := map[string]string{}
	for _, w := range writable {
		for _, b := range plan.Binds {
			if under(w, b.Target) {
				idx := len(keepOrder)
				if _, ok := keep[w]; !ok {
					keepOrder = append(keepOrder, w)
				}
				keep[w] = stageDir + "/w" + strconv.Itoa(idx)
				os.MkdirAll(w, 0o755)
				os.MkdirAll(keep[w], 0o700)
				sb.WriteString("mount --bind " + shellQuote(w) + " " + shellQuote(keep[w]) + "\n")
				break
			}
		}
	}
	dirs := map[string]string{}
	for _, b := range plan.Binds {
		if _, ok := dirs[b.Dataset]; !ok {
			dirs[b.Dataset] = stageDir + "/" + strconv.Itoa(len(dirs))
			os.MkdirAll(dirs[b.Dataset], 0o700)
			sb.WriteString("mount -t zfs -o ro " + shellQuote(b.Dataset+"@"+snap) + " " + shellQuote(dirs[b.Dataset]) + "\n")
		}
	}
	for _, b := range plan.Binds {
		from := dirs[b.Dataset] + b.Sub
		sub := b.Sub
		if sub == "" {
			sub = "/"
		}
		missing := "The snapshot of " + b.Dataset + " has no " + sub + "."
		sb.WriteString("[ -e " + shellQuote(from) + " ] || { echo " + shellQuote(missing) + " >&2; exit 1; }\n")
		sb.WriteString("mount --bind " + shellQuote(from) + " " + shellQuote(b.Target) + "\n")
	}
	for _, w := range keepOrder {
		sb.WriteString("[ -d " + shellQuote(w) + " ] || { echo " + shellQuote(w+" is not in the snapshot, so restic cannot write there.") + " >&2; exit 1; }\n")
		sb.WriteString("mount --bind " + shellQuote(keep[w]) + " " + shellQuote(w) + "\n")
	}
	sb.WriteString("exec \"$@\"\n")
	return sb.String()
}

func zfsWrap(cmd []string, plan *ZfsPlan, snap, stageDir string, writable []string) []string {
	out := []string{"unshare", "--mount", "--propagation", "private", "--", "/bin/sh", "-c", zfsWrapScript(plan, snap, stageDir, writable), "sh"}
	return append(out, cmd...)
}

func zfsUnstage(stageDir string) {
	dirs, _ := filepath.Glob(stageDir + "/*")
	for _, d := range dirs {
		if isDir(d) {
			syscall.Rmdir(d)
		}
	}
	syscall.Rmdir(stageDir)
}

func dockerContainers() ([]DockerContainer, bool) {
	code, idLines, _ := rbCmd([]string{dockerBin, "ps", "-aq", "--no-trunc"}, 30*time.Second)
	if code != 0 {
		return nil, false
	}
	var ids []string
	for _, id := range idLines {
		if id != "" && id != "0" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []DockerContainer{}, true
	}
	code, lines, _ := rbCmd(append([]string{dockerBin, "inspect"}, ids...), 60*time.Second)
	var data []M
	if json.Unmarshal([]byte(strings.Join(lines, "\n")), &data) != nil || code != 0 {
		return nil, false
	}
	out := []DockerContainer{}
	for _, c := range data {
		mounts := []string{}
		ms, _ := c["Mounts"].([]any)
		for _, x := range ms {
			m := mapOf(x)
			src := str(m["Source"])
			if str(m["Type"]) == "bind" && phpTruthy(m["RW"]) && src != "" && src != "0" {
				mounts = append(mounts, src)
			}
		}
		out = append(out, DockerContainer{
			ID:      str(c["Id"]),
			Name:    strings.TrimLeft(str(c["Name"]), "/"),
			Running: phpTruthy(mapOf(c["State"])["Running"]),
			Mounts:  mounts,
		})
	}
	return out, true
}

func phpTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != "" && x != "0"
	case []any:
		return len(x) > 0
	case M:
		return len(x) > 0
	}
	return true
}

var vmDiskLine = regexp.MustCompile(`^\s*file\s+\S+\s+\S+\s+(/.+?)\s*$`)

func vmDisks(lines []string) []string {
	disks := []string{}
	for _, line := range lines {
		if m := vmDiskLine.FindStringSubmatch(line); m != nil {
			disks = append(disks, m[1])
		}
	}
	return disks
}

func vms() ([]LibvirtVM, bool) {
	code, names, _ := rbCmd([]string{virshBin, "list", "--all", "--name"}, 30*time.Second)
	if code != 0 {
		return nil, false
	}
	out := []LibvirtVM{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || name == "0" {
			continue
		}
		_, lines, _ := rbCmd([]string{virshBin, "domblklist", name, "--details"}, 30*time.Second)
		state := vmState(name)
		out = append(out, LibvirtVM{Name: name, State: state, Running: state != "shut off", Disks: vmDisks(lines)})
	}
	return out, true
}

func vmState(name string) string {
	_, state, _ := rbCmd([]string{virshBin, "domstate", name}, 30*time.Second)
	return strings.TrimSpace(strings.Join(state, " "))
}

type orderedHits struct {
	names []string
	hits  map[string]DetectHit
}

func (o *orderedHits) set(h DetectHit) {
	if o.hits == nil {
		o.hits = map[string]DetectHit{}
	}
	if _, ok := o.hits[h.Name]; !ok {
		o.names = append(o.names, h.Name)
	}
	o.hits[h.Name] = h
}

func (o *orderedHits) has(name string) bool {
	_, ok := o.hits[name]
	return ok
}

func (o *orderedHits) unset(name string) {
	if !o.has(name) {
		return
	}
	delete(o.hits, name)
	for i, n := range o.names {
		if n == name {
			o.names = append(o.names[:i], o.names[i+1:]...)
			break
		}
	}
}

func (o *orderedHits) list() []DetectHit {
	out := []DetectHit{}
	for _, n := range o.names {
		out = append(out, o.hits[n])
	}
	return out
}

func consistencyDetect(sources []string) Detection {
	containers, dockerOK := dockerContainers()
	machines, virshOK := vms()
	var hitC, broad, hitV orderedHits
	for _, c := range containers {
	mounts:
		for _, m := range c.Mounts {
			for _, src := range sources {
				if pathWithin(m, src) {
					hitC.set(DetectHit{c.Name, c.Running, m})
					broad.unset(c.Name)
					break mounts
				}
				if pathWithin(src, m) && !broad.has(c.Name) {
					broad.set(DetectHit{c.Name, c.Running, m})
				}
			}
		}
	}
	for _, v := range machines {
	disks:
		for _, d := range v.Disks {
			for _, src := range sources {
				if pathWithin(d, src) {
					hitV.set(DetectHit{v.Name, v.Running, d})
					break disks
				}
			}
		}
	}
	z := DetectZfs{OK: true, Datasets: []string{}, Live: []DetectLive{}}
	if plan, err := zfsPlan(sources, nil); err != nil {
		z.OK = false
		z.Error = err.Error()
	} else {
		z.Datasets = plan.Datasets
		for _, l := range plan.Live {
			z.Live = append(z.Live, DetectLive{l.Path, l.Why})
		}
	}
	return Detection{
		Docker:     dockerOK,
		Libvirt:    virshOK,
		Containers: hitC.list(),
		Broad:      broad.list(),
		VMs:        hitV.list(),
		Zfs:        z,
	}
}

func dockerAutostartOrder() ([]string, map[string]int) {
	var names []string
	order := map[string]int{}
	b, err := os.ReadFile("/var/lib/docker/unraid-autostart")
	if err != nil {
		return names, order
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			f = []string{""}
		}
		if _, ok := order[f[0]]; !ok {
			names = append(names, f[0])
		}
		order[f[0]] = 0
		if len(f) > 1 {
			order[f[0]] = phpInt(f[1])
		}
	}
	return names, order
}

var leadingInt = regexp.MustCompile(`^\s*[+-]?\d+`)

func phpInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(leadingInt.FindString(s)))
	return n
}

func dockerStopTimeout() int {
	cfg := parseIni(envOr("RB_DOCKER_CFG", "/boot/config/docker.cfg"))
	t := 10
	if v, ok := cfg["DOCKER_TIMEOUT"]; ok {
		t = phpInt(v)
	}
	if t == 0 {
		t = 10
	}
	return max(1, min(600, t))
}

func dockerStop(names []string, log func(string)) error {
	if len(names) == 0 {
		return nil
	}
	timeout := dockerStopTimeout()
	log("Stopping containers: " + strings.Join(names, ", "))
	code, _, errs := rbCmd(append([]string{dockerBin, "stop", "-t", strconv.Itoa(timeout)}, names...), time.Duration(timeout+60)*time.Second)
	if code != 0 {
		if errs == "" {
			errs = "exit code " + strconv.Itoa(code)
		}
		return errors.New("docker stop failed: " + errs)
	}
	return nil
}

func dockerStart(names []string, log func(string)) []string {
	keys, order := dockerAutostartOrder()
	pos := map[string]int{}
	for i, k := range keys {
		pos[k] = i
	}
	rank := func(n string) int {
		if p, ok := pos[n]; ok {
			return p
		}
		return int(^uint(0) >> 1)
	}
	sorted := append([]string(nil), names...)
	sort.SliceStable(sorted, func(i, j int) bool { return rank(sorted[i]) < rank(sorted[j]) })
	failed := []string{}
	for i, name := range sorted {
		log("Starting container " + name)
		code, _, errs := rbCmd([]string{dockerBin, "start", name}, 120*time.Second)
		if code != 0 {
			failed = append(failed, name+" ("+errs+")")
		} else if order[name] != 0 && i < len(sorted)-1 {
			time.Sleep(time.Duration(min(120, order[name])) * time.Second)
		}
	}
	return failed
}

func vmStop(names []string, timeout int, log func(string), stopped *[]string, cancelled func() bool) error {
	for _, name := range names {
		if vmState(name) == "paused" {
			rbCmd([]string{virshBin, "resume", name}, 30*time.Second)
		}
		log("Shutting down VM " + name)
		rbCmd([]string{virshBin, "shutdown", name}, 60*time.Second)
	}
	deadline := time.Now().Unix() + int64(timeout)
	pending := append([]string(nil), names...)
	for len(pending) > 0 {
		var still []string
		for _, name := range pending {
			if vmState(name) == "shut off" {
				*stopped = append(*stopped, name)
			} else {
				still = append(still, name)
			}
		}
		pending = still
		if len(pending) == 0 {
			break
		}
		if time.Now().Unix() > deadline {
			return errors.New("VM " + strings.Join(pending, ", ") + " did not shut down within " + strconv.Itoa(timeout) + "s; nothing was backed up.")
		}
		if cancelled != nil && cancelled() {
			return errors.New("Stopped.")
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

func vmStart(names []string, log func(string)) []string {
	failed := []string{}
	for _, name := range names {
		log("Starting VM " + name)
		code, _, errs := rbCmd([]string{virshBin, "start", name}, 120*time.Second)
		if code != 0 && !strings.Contains(strings.ToLower(errs), "already active") {
			failed = append(failed, name+" ("+errs+")")
		}
	}
	return failed
}

func journalDir() string {
	return RunDir + "/journal"
}

func journalFile(opID string) string {
	return journalDir() + "/" + opID + ".json"
}

func stringList(v any) []string {
	out := []string{}
	xs, _ := v.([]any)
	for _, x := range xs {
		if s, ok := scalarString(x); ok {
			out = append(out, s)
		}
	}
	return out
}

func journalFromMap(m M) Journal {
	return Journal{
		Containers: stringList(m["containers"]),
		VMs:        stringList(m["vms"]),
		Snapshots:  stringList(m["snapshots"]),
		Snap:       str(m["snap"]),
		Log:        str(m["log"]),
		Ended:      phpTruthy(m["ended"]),
		Done:       stringList(m["done"]),
		Attempts:   toInt(m["attempts"]),
		RunnerPID:  toInt(m["runner_pid"]),
	}
}

func (j Journal) toMap(full bool) M {
	nz := func(s []string) []string {
		if s == nil {
			return []string{}
		}
		return s
	}
	m := M{
		"containers": nz(j.Containers),
		"vms":        nz(j.VMs),
		"snapshots":  nz(j.Snapshots),
		"snap":       j.Snap,
		"log":        j.Log,
		"runner_pid": j.RunnerPID,
	}
	if j.Ended {
		m["ended"] = true
	}
	if full {
		m["done"] = nz(j.Done)
		m["attempts"] = j.Attempts
	}
	return m
}

func journalWrite(opID string, j Journal) bool {
	j.RunnerPID = os.Getpid()
	return jsonWrite(journalFile(opID), j.toMap(false), 0o644)
}

func journalClear(opID string) {
	os.Remove(journalFile(opID))
}

func journalRecover() []JournalNotice {
	done := []JournalNotice{}
	files, _ := filepath.Glob(journalDir() + "/*.json")
	for _, file := range files {
		raw, ok := jsonRead(file)
		if !ok || len(raw) == 0 {
			continue
		}
		j := journalFromMap(raw)
		if pidAlive(j.RunnerPID) {
			continue
		}
		logf := func(msg string) {
			if j.Log != "" && j.Log != "0" {
				logAppendMax(j.Log, msg, 16*1048576)
			}
		}
		var failed []string
		if len(j.Snapshots) > 0 && j.Snap != "" && j.Snap != "0" {
			left := zfsDestroy(j.Snapshots, j.Snap, logf)
			if len(left) > 0 {
				for _, l := range left {
					failed = append(failed, l.Name+" ("+l.Why+")")
				}
				var keep []string
				for _, d := range j.Snapshots {
					if zfsStuckHas(left, d+"@"+j.Snap) {
						keep = append(keep, d)
					}
				}
				j.Snapshots = keep
			} else {
				j.Done = append(j.Done, "removed its ZFS snapshots")
				j.Snapshots = []string{}
			}
		}
		if len(j.Containers) > 0 {
			if bad := dockerStart(j.Containers, logf); len(bad) > 0 {
				failed = append(failed, bad...)
			} else {
				j.Done = append(j.Done, "started "+strings.Join(j.Containers, ", "))
				j.Containers = []string{}
			}
		}
		if len(j.VMs) > 0 {
			if bad := vmStart(j.VMs, logf); len(bad) > 0 {
				failed = append(failed, bad...)
			} else {
				j.Done = append(j.Done, "started VM "+strings.Join(j.VMs, ", "))
				j.VMs = []string{}
			}
		}
		j.Attempts++
		if len(failed) > 0 && j.Attempts < 30 {
			jsonWrite(file, j.toMap(true), 0o644)
			continue
		}
		if j.Ended {
			if len(failed) > 0 {
				logf("Gave up removing the ZFS snapshot after 30 minutes: " + strings.Join(failed, "; "))
				done = append(done, JournalNotice{"ZFS snapshot left behind", "A backup could not remove its ZFS snapshot, not in 30 minutes " +
					"of trying: " + strings.Join(failed, "; ") + ". Remove it with zfs destroy once nothing has it mounted."})
			} else {
				logf("Removed the ZFS snapshot that was still busy at the end.")
			}
		} else {
			what := "nothing needed putting back"
			if len(j.Done) > 0 {
				what = strings.Join(j.Done, ", ")
			}
			msg := "The runner was gone; the scheduler " + what
			if len(failed) > 0 {
				msg += ", but not: " + strings.Join(failed, "; ")
			}
			logf(msg + ".")
			desc := "A backup ended unexpectedly; " + what
			if len(failed) > 0 {
				desc += ". Could not, after 30 minutes of trying: " + strings.Join(failed, "; ")
			} else {
				desc += "."
			}
			done = append(done, JournalNotice{"Backup interrupted", desc})
		}
		os.Remove(file)
	}
	return done
}
