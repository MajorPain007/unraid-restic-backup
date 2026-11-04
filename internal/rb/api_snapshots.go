package rb

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

func init() {
	registerAction("snapshots", apiSnapshots)
	registerAction("ls", apiLs)
	registerAction("find", apiFind)
	registerAction("diff", apiDiff)
	registerAction("snapshot_forget", apiSnapshotForget)
	registerAction("restore_start", apiRestoreStart)
	register("download-info", "describe a file of a snapshot for a download, parameters as JSON on stdin", cmdDownloadInfo)
	register("download", "write a file of a snapshot (a folder as zip) to stdout, parameters as JSON on stdin", cmdDownload)
}

func snapshotsCache(repoID string) string {
	return cacheDir() + "/snapshots-" + repoID + ".json"
}

func apiSnapshots(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	cache := snapshotsCache(repo.ID)
	fresh := !toBool(param(p, "refresh", false)) && fileMtime(cache) > now()-600
	var list []any
	ok := fresh && jsonReadInto(cache, &list) && list != nil
	if !ok {
		out, err := apiRestic(cfg, repo, []string{"snapshots", "--json", "--no-lock"}, 200000)
		if err != nil {
			return nil, err
		}
		var raw []any
		json.Unmarshal([]byte(strings.Join(out, "\n")), &raw)
		snaps := []M{}
		for _, sv := range raw {
			s := mapOf(sv)
			jobID := ""
			tags := []any{}
			for _, tv := range listOf(s["tags"]) {
				t := str(tv)
				if strings.HasPrefix(t, "job:") {
					jobID = t[4:]
				} else {
					tags = append(tags, tv)
				}
			}
			id := str(s["id"])
			short := s["short_id"]
			if short == nil {
				if len(id) > 8 {
					short = id[:8]
				} else {
					short = id
				}
			}
			hostname := s["hostname"]
			if hostname == nil {
				hostname = ""
			}
			paths := s["paths"]
			if paths == nil {
				paths = []any{}
			}
			var summary any
			if sm, ok := s["summary"].(M); ok {
				added := sm["data_added_packed"]
				if added == nil {
					added = get(sm, "data_added", 0)
				}
				var duration any
				if sm["backup_start"] != nil && sm["backup_end"] != nil {
					a, b := strtotime(sm["backup_end"]), strtotime(sm["backup_start"])
					duration = toInt64(a) - toInt64(b)
				}
				summary = M{
					"added":    added,
					"size":     get(sm, "total_bytes_processed", 0),
					"files":    get(sm, "total_files_processed", 0),
					"new":      get(sm, "files_new", 0),
					"changed":  get(sm, "files_changed", 0),
					"duration": duration,
				}
			}
			snaps = append(snaps, M{
				"id":       s["id"],
				"short_id": short,
				"time":     strtotime(s["time"]),
				"hostname": hostname,
				"paths":    paths,
				"tags":     tags,
				"job":      jobID,
				"summary":  summary,
			})
		}
		sort.SliceStable(snaps, func(a, b int) bool {
			return toInt64(snaps[a]["time"]) > toInt64(snaps[b]["time"])
		})
		jsonWrite(cache, snaps, 0o644)
		list = make([]any, len(snaps))
		for i, s := range snaps {
			list[i] = s
		}
	}
	var names any = []any{}
	if len(cfg.Jobs) > 0 {
		m := M{}
		for _, j := range cfg.Jobs {
			m[j.ID] = j.Name
		}
		names = m
	}
	return M{"snapshots": list, "job_names": names, "cached_at": fileMtime(cache)}, nil
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func apiLs(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	snap, err := needSnapshotID(param(p, "snapshot", ""))
	if err != nil {
		return nil, err
	}
	path, err := needSnapshotPath(param(p, "path", "/"))
	if err != nil {
		return nil, err
	}
	dir := cacheDir() + "/ls/" + snap
	cache := dir + "/" + md5hex(path) + ".json"
	var entries []browseEntry
	if !jsonReadInto(cache, &entries) || entries == nil {
		entries = nil
		if browseAvailable() {
			s := browseStatus(repo.ID)
			if s == nil {
				if err := resticReady(cfg, repo); err != nil {
					return nil, err
				}
				if why := repoReachable(*repo, 1, 0, nil, nil); why != "" {
					return nil, apiFail(why)
				}
				s = browseStart(*repo)
			}
			if s.Status == "starting" {
				return M{"path": path, "pending": true}, nil
			}
			if s.Status == "ready" {
				entries = browseList(repo.ID, snap, path)
				if entries == nil {
					browseStop(repo.ID, 0)
				}
			}
		}
		if entries == nil {
			entries, err = lsRestic(cfg, repo, snap, path)
			if err != nil {
				return nil, err
			}
		}
		sort.SliceStable(entries, func(a, b int) bool {
			da, db := 1, 1
			if entries[a].Type == "dir" {
				da = 0
			}
			if entries[b].Type == "dir" {
				db = 0
			}
			if da != db {
				return da < db
			}
			return strnatcasecmp(entries[a].Name, entries[b].Name) < 0
		})
		os.MkdirAll(dir, 0o755)
		jsonWrite(cache, entries, 0o644)
		lsCacheTrim()
	}
	return M{"path": path, "entries": entries}, nil
}

func lsRestic(cfg *Config, repo *Repo, snap, path string) ([]browseEntry, error) {
	out, err := apiRestic(cfg, repo, []string{"ls", "--json", "--no-lock", snap, path}, 200000)
	if err != nil {
		return nil, err
	}
	entries := []browseEntry{}
	for _, n := range jsonLines(out) {
		kind := n["message_type"]
		if kind == nil {
			kind = n["struct_type"]
		}
		if str(kind) != "node" || str(n["path"]) == path {
			continue
		}
		e := browseEntry{Name: str(n["name"]), Type: str(n["type"]), Path: str(n["path"]), Perm: str(n["permissions"])}
		if n["size"] != nil {
			v := toInt64(n["size"])
			e.Size = &v
		}
		if n["mtime"] != nil {
			if t, ok := strtotime(n["mtime"]).(int64); ok {
				e.Mtime = &t
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func lsCacheTrim() {
	dirs, _ := filepath.Glob(cacheDir() + "/ls/*")
	type info struct {
		dir   string
		mtime int64
		size  int64
	}
	all := []info{}
	var total int64
	for _, d := range dirs {
		if !isDir(d) {
			continue
		}
		var s int64
		files, _ := filepath.Glob(d + "/*.json")
		for _, f := range files {
			if st, err := os.Stat(f); err == nil {
				s += st.Size()
			}
		}
		all = append(all, info{d, fileMtime(d), s})
		total += s
	}
	if total < 64*1048576 {
		return
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].mtime < all[b].mtime })
	for _, i := range all {
		if total < 48*1048576 {
			break
		}
		os.RemoveAll(i.dir)
		total -= i.size
	}
}

func apiFind(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	pattern := pstr(p, "pattern", "")
	if pattern == "" || strings.ContainsAny(pattern, "\x00\n\r") {
		return nil, apiFail("Enter a file name or a pattern such as *.jpg")
	}
	args := []string{"find", "--json", "--no-lock"}
	if pstr(p, "snapshot", "") != "" {
		snap, err := needSnapshotID(param(p, "snapshot", ""))
		if err != nil {
			return nil, err
		}
		args = append(args, "--snapshot", snap)
	}
	if toBool(param(p, "ignore_case", true)) {
		args = append(args, "--ignore-case")
	}
	args = append(args, "--", pattern)
	out, err := apiRestic(cfg, repo, args, 200000)
	if err != nil {
		return nil, err
	}
	var raw []any
	json.Unmarshal([]byte(strings.Join(out, "\n")), &raw)
	results := []M{}
	n := 0
	for _, gv := range raw {
		g := mapOf(gv)
		matches := []M{}
		for _, mv := range listOf(g["matches"]) {
			if n >= 2000 {
				n++
				break
			}
			n++
			m := mapOf(mv)
			var mtime any
			if m["mtime"] != nil {
				mtime = strtotime(m["mtime"])
			}
			matches = append(matches, M{"path": m["path"], "type": m["type"], "size": m["size"], "mtime": mtime})
		}
		hits := g["hits"]
		if hits == nil {
			hits = len(matches)
		}
		results = append(results, M{"snapshot": g["snapshot"], "hits": hits, "matches": matches})
	}
	return M{"results": results, "truncated": n > 2000}, nil
}

func apiDiff(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	a, err := needSnapshotID(param(p, "from", ""))
	if err != nil {
		return nil, err
	}
	b, err := needSnapshotID(param(p, "to", ""))
	if err != nil {
		return nil, err
	}
	out, err := apiRestic(cfg, repo, []string{"diff", "--json", "--no-lock", a, b}, 200000)
	if err != nil {
		return nil, err
	}
	changes := []M{}
	var stats any
	total := 0
	for _, j := range jsonLines(out) {
		switch str(j["message_type"]) {
		case "change":
			if total < 5000 {
				changes = append(changes, M{"path": j["path"], "modifier": j["modifier"]})
			}
			total++
		case "statistics":
			stats = j
		}
	}
	return M{"changes": changes, "total": total, "statistics": stats}, nil
}

func apiSnapshotForget(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	if repo.AppendOnly {
		return nil, apiFail("The repository is append-only; snapshots can only be removed on the server.")
	}
	in, err := pjsonList(p, "ids")
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, v := range in {
		id, err := needSnapshotID(v)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, apiFail("No snapshots chosen.")
	}
	l, err := apiLock(repo)
	if err != nil {
		return nil, err
	}
	defer l.Unlock()
	if _, err := apiRestic(cfg, repo, append([]string{"forget", "--retry-lock", "1m"}, ids...), 200000); err != nil {
		return nil, err
	}
	os.Remove(snapshotsCache(repo.ID))
	return M{"removed": len(ids)}, nil
}

var reMntRoot = regexp.MustCompile(`^/mnt(/(user|disks|remotes|[^/]+))?$`)

func apiRestoreStart(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	snap, err := needSnapshotID(param(p, "snapshot", ""))
	if err != nil {
		return nil, err
	}
	in, err := pjsonList(p, "items")
	if err != nil {
		return nil, err
	}
	items := []string{}
	for _, v := range in {
		path, err := needSnapshotPath(v)
		if err != nil {
			return nil, err
		}
		if !inList(path, items...) {
			items = append(items, path)
		}
	}
	if len(items) == 0 {
		return nil, apiFail("Choose what to restore.")
	}
	if len(items) > 1000 {
		return nil, apiFail("Choose at most 1000 items at a time.")
	}
	target := "path"
	if pstr(p, "target", "") == "original" {
		target = "original"
	}
	targetPath := ""
	if target == "path" {
		targetPath = absPath(param(p, "target_path", ""))
		if targetPath == "" || !under(targetPath, "/mnt") || reMntRoot.MatchString(targetPath) {
			return nil, apiFail("Restore into a folder on a share, a disk or a pool, such as /mnt/user/restore.")
		}
	} else {
		for _, path := range items {
			if !(under(path, "/mnt") || under(path, "/boot")) && path != "/" {
				return nil, apiFail(path + " did not come from /mnt or /boot; restore it into a folder instead.")
			}
		}
	}
	if !arrayStarted() {
		return nil, apiFail("The array is not started.")
	}
	def := "always"
	if target == "original" {
		def = "if-changed"
	}
	overwrite := pstr(p, "overwrite", def)
	if !inList(overwrite, "always", "if-changed", "if-newer", "never") {
		return nil, apiFail("Unknown overwrite setting")
	}
	if op := opRunningOn(repo.ID, "prune", "check", "forget"); op != nil {
		kind := str(op["kind"])
		if kind != "" {
			kind = strings.ToUpper(kind[:1]) + kind[1:]
		}
		return nil, apiFail(kind + " is running on this repository; restore when it has finished.")
	}
	dir := RunDir + "/requests"
	if !isDir(dir) {
		os.MkdirAll(dir, 0o700)
	}
	file := dir + "/" + randHex(8) + ".json"
	jsonWrite(file, M{"snapshot": snap, "items": items, "target": target, "target_path": targetPath,
		"overwrite": overwrite, "delete": toBool(param(p, "delete", false))}, 0o600)
	spawn("run", "restore", repo.ID, file)
	return M{"started": true}, nil
}

type downloadTarget struct {
	cfg  *Config
	repo *Repo
	snap string
	path string
	dir  bool
	size *int64
}

func downloadFind(p M) (*downloadTarget, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "repo", ""))
	if err != nil {
		return nil, err
	}
	snap, err := needSnapshotID(param(p, "snapshot", ""))
	if err != nil {
		return nil, err
	}
	path, err := needSnapshotPath(param(p, "path", ""))
	if err != nil {
		return nil, err
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	var parent M
	deadline := time.Now().Add(90 * time.Second)
	for {
		parent, err = apiLs(M{"repo": repo.ID, "snapshot": snap, "path": filepath.Dir(path)})
		if err != nil {
			return nil, err
		}
		if parent["pending"] != true || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	entries, _ := parent["entries"].([]browseEntry)
	var node *browseEntry
	for i := range entries {
		if entries[i].Path == path {
			node = &entries[i]
		}
	}
	if node == nil || !inList(node.Type, "file", "dir") {
		return nil, apiFail("That file is not in the snapshot.")
	}
	return &downloadTarget{cfg: cfg, repo: repo, snap: snap, path: path, dir: node.Type == "dir", size: node.Size}, nil
}

func cmdDownloadInfo(args []string) int {
	useLocalTimezone()
	t, err := downloadFind(readParams(os.Stdin))
	if err != nil {
		os.Stdout.Write(jsonEncode(M{"ok": false, "error": err.Error()}, false))
		return 0
	}
	name := filepath.Base(t.path)
	if t.path == "/" {
		name = ""
	}
	var size any
	if t.dir {
		name += ".zip"
	} else if t.size != nil {
		size = *t.size
	}
	os.Stdout.Write(jsonEncode(M{"ok": true, "name": name, "dir": t.dir, "size": size}, false))
	return 0
}

func cmdDownload(args []string) int {
	t, err := downloadFind(readParams(os.Stdin))
	if err != nil {
		os.Stderr.WriteString("Download failed: " + err.Error() + "\n")
		return 1
	}
	dump := []string{"dump", "--no-lock", t.snap, t.path}
	if t.dir {
		dump = []string{"dump", "--no-lock", "--archive", "zip", t.snap, t.path}
	}
	argv := resticCmd(*t.repo, t.cfg.Settings, dump, false)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = "/"
	cmd.Env = envList(resticEnv(*t.repo, t.cfg.Settings))
	cmd.Stdout = os.Stdout
	if cmd.Start() != nil {
		return 1
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGPIPE)
	go func() {
		<-sig
		cmd.Process.Kill()
	}()
	if cmd.Wait() != nil {
		return 1
	}
	return 0
}
