package rb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func init() {
	register("run", "backup|forget|prune|check|restore <id> [manual|schedule] [request file]  run one operation", cmdRun)
}

type runCleanup struct {
	label string
	fn    func() error
}

type runner struct {
	op        M
	cfg       *Config
	settings  Settings
	repo      Repo
	job       *Job
	errors    []string
	lock      *fileLock
	logFile   string
	lastWrite time.Time
	cleanups  []runCleanup
	finished  bool
}

func newRunner(kind string, cfg *Config, repo Repo, job *Job, trigger string, lock *fileLock) *runner {
	id := newOpID()
	logDir := RunDir + "/logs"
	if arrayStarted() && prepareDataDir(cfg.Settings) == "" {
		logDir = cfg.Settings.DataDir + "/logs"
	}
	r := &runner{cfg: cfg, settings: cfg.Settings, repo: repo, job: job, lock: lock}
	r.logFile = logDir + "/" + id + ".log"
	jobID, jobName := "", ""
	if job != nil {
		jobID, jobName = job.ID, job.Name
	}
	r.op = M{
		"id":         id,
		"kind":       kind,
		"job":        jobID,
		"job_name":   jobName,
		"repo":       repo.ID,
		"repo_name":  repo.Name,
		"trigger":    trigger,
		"started":    now(),
		"ended":      int64(0),
		"status":     "running",
		"phase":      "Starting",
		"progress":   nil,
		"runner_pid": os.Getpid(),
		"pid":        0,
		"log":        r.logFile,
		"error":      "",
		"summary":    nil,
	}
	return r
}

func (r *runner) id() string {
	return str(r.op["id"])
}

func (r *runner) kind() string {
	return str(r.op["kind"])
}

func (r *runner) log(msg string) {
	logAppendMax(r.logFile, msg, 16*1048576)
}

func (r *runner) phase(name string) {
	r.op["phase"] = name
	r.op["progress"] = nil
	r.write(true)
	r.log("== " + name)
}

func (r *runner) write(force bool) {
	t := time.Now()
	if force || t.Sub(r.lastWrite) >= time.Second {
		r.lastWrite = t
		opWrite(r.op)
	}
}

func (r *runner) cancelled() bool {
	return opCancelRequested(r.id())
}

func (r *runner) onCleanup(label string, fn func() error) {
	r.cleanups = append(r.cleanups, runCleanup{label, fn})
}

func ucfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (r *runner) runCleanups() {
	for len(r.cleanups) > 0 {
		c := r.cleanups[len(r.cleanups)-1]
		r.cleanups = r.cleanups[:len(r.cleanups)-1]
		r.log(ucfirst(c.label))
		if err := safeCall(c.fn); err != nil {
			r.errors = append(r.errors, c.label+" failed: "+err.Error())
			r.log(c.label + " failed: " + err.Error())
		}
	}
}

func safeCall(fn func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%v", p)
		}
	}()
	return fn()
}

var reShellSafe = regexp.MustCompile(`^[A-Za-z0-9_/.:=@%+-]+$`)

func logQuote(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if reShellSafe.MatchString(a) {
			out[i] = a
		} else {
			out[i] = shellQuote(a)
		}
	}
	return strings.Join(out, " ")
}

func decodeJSONLine(line string) (any, bool) {
	var v any
	if json.Unmarshal([]byte(line), &v) != nil {
		return nil, false
	}
	switch v.(type) {
	case M, []any:
		return v, true
	}
	return nil, false
}

func (r *runner) restic(args []string, onJSON func(any), wrap func([]string) []string) (int, []string) {
	errLines := []string{}
	cmd := resticCmd(r.repo, r.settings, args, true)
	if wrap != nil {
		cmd = wrap(cmd)
	}
	r.log("$ restic " + logQuote(args))
	code := procStream(cmd, resticEnv(r.repo, r.settings), func(stream, line string) {
		if stream == "out" {
			j, ok := decodeJSONLine(line)
			if ok && onJSON != nil {
				onJSON(j)
			} else if line != "" {
				r.log(line)
			}
			return
		}
		if len(errLines) < 500 {
			errLines = append(errLines, line)
		}
		j, _ := decodeJSONLine(line)
		m, _ := j.(M)
		switch {
		case m != nil && str(m["message_type"]) == "error":
			msg := line
			if e, ok := m["error"].(M); ok && e["message"] != nil {
				msg = str(e["message"])
			}
			r.log("error: " + msg)
		case m != nil && str(m["message_type"]) == "exit_error":
			msg := line
			if m["message"] != nil {
				msg = str(m["message"])
			}
			r.log("restic: " + msg)
		case line != "":
			r.log(line)
		}
	}, func() bool {
		r.write(false)
		return r.cancelled()
	}, func(pid int) {
		r.op["pid"] = pid
		r.write(true)
	}, nil)
	r.op["pid"] = 0
	r.log("exit code " + strconv.Itoa(code))
	return code, errLines
}

func (r *runner) finish(status, errMsg string, summary any) {
	if r.finished {
		return
	}
	r.runCleanups()
	if status == "success" && len(r.errors) > 0 {
		status = "warning"
	}
	if errMsg == "" && len(r.errors) > 0 {
		n := min(3, len(r.errors))
		errMsg = strings.Join(r.errors[:n], "; ")
	}
	r.finished = true
	if r.kind() != "restore" {
		browseStop(r.repo.ID, 0)
	}
	if m, ok := summary.(M); ok && m == nil {
		summary = nil
	}
	r.op["status"] = status
	r.op["error"] = errMsg
	r.op["summary"] = summary
	r.op["ended"] = now()
	r.op["phase"] = "Finished"
	r.op["progress"] = nil
	r.write(true)
	line := "Finished: " + status
	if errMsg != "" {
		line += " - " + errMsg
	}
	r.log(line)

	record := M{}
	for k, v := range r.op {
		if !inList(k, "progress", "phase", "runner_pid", "pid", "updated") {
			record[k] = v
		}
	}
	historyAppend(r.settings, record)

	op := r.op
	jobID := str(op["job"])
	kind := r.kind()
	_, summaryMap := op["summary"].(M)
	counted := kind == "backup" && inList(status, "success", "warning") && summaryMap
	var backfill []any
	if counted {
		cur := stateRead(r.settings)
		if s, _ := mapOf(mapOf(cur["jobs"])[jobID])["sizes"].([]any); len(s) == 0 {
			backfill = jobSizesFromHistory(r.settings, jobID)
		} else {
			backfill = []any{}
		}
	}
	stateUpdate(r.settings, func(state M) {
		if counted {
			js := sub(state, "jobs", jobID)
			var sizes []any
			if v, ok := js["sizes"]; ok && v != nil {
				sizes, _ = v.([]any)
			} else {
				sizes = backfill
			}
			ended := toInt64(op["ended"])
			add := len(sizes) == 0
			if !add {
				last, _ := sizes[len(sizes)-1].([]any)
				add = len(last) == 0 || toInt64(last[0]) != ended
			}
			if add {
				sizes = append(sizes, sizePoint(op))
			}
			if len(sizes) > SizesKeep {
				sizes = sizes[len(sizes)-SizesKeep:]
			}
			js["sizes"] = sizes
		}
		if jobID != "" && kind == "backup" {
			js := sub(state, "jobs", jobID)
			js["last"] = M{
				"id": op["id"], "status": op["status"], "started": op["started"],
				"ended": op["ended"], "error": op["error"], "summary": op["summary"],
			}
			if inList(status, "success", "warning") {
				js["last_success"] = op["ended"]
			}
			if sched, ok := js["sched"].(M); ok {
				sched["last_start"] = op["started"]
			}
			delete(js, "queued")
		} else if inList(kind, "prune", "check") {
			sub(state, "repos", r.repo.ID, kind)["last"] = M{
				"id": op["id"], "status": op["status"], "ended": op["ended"], "error": op["error"],
			}
		}
	})
	os.Remove(RunDir + "/cache/snapshots-" + r.repo.ID + ".json")
	r.notify()
	if r.lock != nil {
		r.lock.Unlock()
		r.lock = nil
	}
	opsPrune(100)
}

func (r *runner) notify() {
	op := r.op
	var what string
	if r.kind() == "backup" {
		what = "Backup \"" + str(op["job_name"]) + "\""
	} else {
		what = ucfirst(r.kind()) + " of \"" + str(op["repo_name"]) + "\""
	}
	n := Notify{Warning: true, Error: true}
	if r.job != nil {
		n = r.job.Notify
	}
	took := humanDuration(float64(toInt64(op["ended"]) - toInt64(op["started"])))
	status := str(op["status"])
	errMsg := str(op["error"])
	switch status {
	case "success":
		detail := r.summaryText(nil)
		if n.Success {
			desc := "Finished in " + took
			if detail != "" {
				desc += ". " + detail
			}
			notify(what+" finished", desc, "normal", "")
		}
	case "warning":
		if n.Warning {
			notify(what+" finished with warnings", errMsg, "warning", "")
		}
	case "cancelled":
	default:
		if n.Error {
			notify(what+" failed", errMsg, "alert", "")
		}
	}
	if r.job != nil && n.Healthcheck != "" && r.kind() == "backup" && status != "cancelled" {
		event := "success"
		if status == "error" {
			event = "fail"
		}
		body := errMsg
		if body == "" {
			body = r.summaryText(nil)
		}
		healthcheckPing(n.Healthcheck, event, body)
	}
}

func (r *runner) summaryText(s any) string {
	if s == nil {
		s = r.op["summary"]
	}
	m, ok := s.(M)
	if !ok || m["data_added"] == nil {
		return ""
	}
	added := m["data_added_packed"]
	if added == nil {
		added = m["data_added"]
	}
	return fmt.Sprintf("%d new, %d changed files; %s added", toInt64(m["files_new"]), toInt64(m["files_changed"]),
		humanBytes(toFloat(added)))
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case bool:
		if x {
			return 1
		}
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	}
	return 0
}

func runFailEarly(msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	return 1
}

func (r *runner) preflight() string {
	if !arrayStarted() {
		return "The array is not started."
	}
	if !fileExecutable(ResticBin) {
		return "restic is not installed (" + ResticBin + "). Reinstall the plugin."
	}
	if err := prepareDataDir(r.settings); err != "" {
		return err + " - is the data folder in Settings on a disk that is mounted?"
	}
	if why := secretsReady(r.settings); why != "" {
		return why
	}
	if !secretHas(r.repo.ID, "password") {
		return "The repository has no password set."
	}
	if r.repo.Type == "local" && !isFile(r.repo.Local.Path+"/config") {
		return "No repository at " + r.repo.Local.Path + " - is the disk mounted?"
	}
	if r.repo.Type == "sftp" && !sshKeyExists(r.repo.ID) {
		return "The repository has no SSH key yet."
	}
	r.phase("Reaching the repository")
	return repoReachable(r.repo, 3, 20, r.cancelled, r.log)
}

func (r *runner) runHook(name, script string, env map[string]string) int {
	r.phase("Running the " + name + " command")
	full := map[string]string{"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME": "/root"}
	for k, v := range env {
		full[k] = v
	}
	code := procStream([]string{"/bin/bash", "-c", script}, full, func(stream, line string) {
		r.log("[" + stream + "] " + line)
	}, r.cancelled, nil, nil)
	r.log(name + " command exited with " + strconv.Itoa(code))
	return code
}

func (r *runner) hookEnv(status, snapshot string) map[string]string {
	return map[string]string{
		"RB_OPERATION": r.id(),
		"RB_JOB_ID":    r.job.ID,
		"RB_JOB_NAME":  r.job.Name,
		"RB_REPO_NAME": r.repo.Name,
		"RB_SOURCES":   strings.Join(r.job.Sources, "\n"),
		"RB_STATUS":    status,
		"RB_SNAPSHOT":  snapshot,
		"RB_LOG":       str(r.op["log"]),
	}
}

func (r *runner) resticUnlocking(args []string, onJSON func(any), wrap func([]string) []string) (int, []string) {
	code, errs := r.restic(args, onJSON, wrap)
	if code == 11 && r.repo.UnlockStale && !r.cancelled() {
		r.log("The repository is locked; removing stale locks and trying again.")
		r.restic([]string{"unlock"}, nil, nil)
		code, errs = r.restic(args, onJSON, wrap)
	}
	return code, errs
}

func (r *runner) prepareConsistency(job Job) (bool, func([]string) []string) {
	c := job.Consistency
	mode := c.Mode
	opID := r.id()
	j := Journal{Containers: []string{}, VMs: []string{}, Snapshots: []string{}, Log: str(r.op["log"])}
	save := func() {
		journalWrite(opID, j)
	}
	r.onCleanup("clearing the journal", func() error {
		if len(j.Snapshots) > 0 {
			j.Ended = true
			save()
		} else {
			journalClear(opID)
		}
		return nil
	})
	startAgain := func() error {
		if len(j.Containers) > 0 {
			for _, f := range dockerStart(j.Containers, r.log) {
				r.errors = append(r.errors, "Container "+f+" did not start again")
			}
			j.Containers = []string{}
		}
		if len(j.VMs) > 0 {
			for _, f := range vmStart(j.VMs, r.log) {
				r.errors = append(r.errors, "VM "+f+" did not start again")
			}
			j.VMs = []string{}
		}
		save()
		return nil
	}

	var plan *ZfsPlan
	if mode == "zfs" || mode == "zfs-stop" {
		var err error
		plan, err = zfsPlan(job.Sources, nil)
		if err != nil {
			r.finish("error", err.Error(), nil)
			return false, nil
		}
		if len(plan.Live) > 0 {
			parts := make([]string, len(plan.Live))
			for i, l := range plan.Live {
				parts[i] = l.Path + " (" + l.Why + ")"
			}
			r.log("Not on ZFS, read as they are: " + strings.Join(parts, ", "))
		}
		if code, _, _ := rbCmd([]string{"unshare", "--help"}, 10*time.Second); code != 0 {
			r.finish("error", "unshare is not available on this server, so the ZFS snapshot cannot be mounted for restic.", nil)
			return false, nil
		}
	}

	if mode == "stop" || mode == "zfs-stop" {
		r.phase("Stopping what uses the data")
		detected := consistencyDetect(job.Sources)
		running := map[string]bool{}
		containers, _ := dockerContainers()
		for _, ct := range containers {
			if ct.Running {
				running[ct.Name] = true
			}
		}
		runningVMs := map[string]bool{}
		machines, _ := vms()
		for _, vm := range machines {
			if vm.Running {
				runningVMs[vm.Name] = true
			}
		}
		stopC := []string{}
		for _, n := range c.Containers {
			if running[n] {
				stopC = append(stopC, n)
			}
		}
		stopV := []string{}
		for _, n := range c.VMs {
			if runningVMs[n] {
				stopV = append(stopV, n)
			}
		}
		for _, x := range append(append([]DetectHit{}, detected.Containers...), detected.VMs...) {
			if x.Running && !inList(x.Name, c.Containers...) && !inList(x.Name, c.VMs...) && !inList(x.Name, c.Skip...) {
				r.errors = append(r.errors, x.Name+" uses "+x.Path+" but is not in the job's list, so it kept running")
			}
		}
		r.onCleanup("starting containers and VMs again", startAgain)
		err := safeCall(func() error {
			if len(stopC) > 0 {
				j.Containers = stopC
				save()
				if err := dockerStop(stopC, r.log); err != nil {
					return err
				}
			}
			if len(stopV) > 0 {
				stopped := []string{}
				j.VMs = stopV
				save()
				return vmStop(stopV, c.Timeout, r.log, &stopped, r.cancelled)
			}
			return nil
		})
		if err != nil {
			status := "error"
			if r.cancelled() {
				status = "cancelled"
			}
			r.finish(status, err.Error(), nil)
			return false, nil
		}
		if len(stopC) == 0 && len(stopV) == 0 {
			r.log("Nothing to stop: none of the listed containers or VMs is running.")
		}
	}

	if plan == nil {
		return true, nil
	}
	r.phase("Taking ZFS snapshots")
	snap := "restic-backup-" + opID
	j.Snap = snap
	j.Snapshots = append([]string{}, plan.Datasets...)
	save()
	stage := RunDir + "/ns/" + opID
	r.onCleanup("removing the ZFS snapshots", func() error {
		zfsUnstage(stage)
		left := zfsDestroy(plan.Datasets, snap, r.log)
		for _, l := range left {
			r.errors = append(r.errors, "Snapshot "+l.Name+" could not be removed yet ("+l.Why+"); it is tried again every minute")
		}
		keep := []string{}
		for _, d := range plan.Datasets {
			if zfsStuckHas(left, d+"@"+snap) {
				keep = append(keep, d)
			}
		}
		j.Snapshots = keep
		save()
		return nil
	})
	if err := zfsSnapshot(plan.Datasets, snap); err != nil {
		r.finish("error", err.Error(), nil)
		return false, nil
	}
	names := make([]string, len(plan.Datasets))
	for i, d := range plan.Datasets {
		names[i] = d + "@" + snap
	}
	r.log("Snapshots taken: " + strings.Join(names, ", "))
	if mode == "zfs-stop" {
		r.phase("Starting containers and VMs again")
		startAgain()
	}
	writable := []string{r.settings.DataDir + "/cache"}
	return true, func(cmd []string) []string {
		return zfsWrap(cmd, plan, snap, stage, writable)
	}
}

func (r *runner) applyRetention(job Job) (int, int, bool) {
	r.phase("Applying the retention policy")
	var groups any
	code, errs := r.resticUnlocking(forgetArgs(job, false), func(j any) {
		if m, ok := j.(M); ok && m["message_type"] != nil {
			return
		}
		groups = j
	}, nil)
	if code != 0 {
		msg := resticError(errs, code)
		if strings.Contains(strings.ToLower(strings.Join(errs, "\n")), "unable to remove") {
			msg = "The server keeps this repository append-only, so old snapshots cannot be removed from here. " +
				"Clean it up on the server, and mark the repository append-only in its settings."
		}
		r.errors = append(r.errors, "Retention: "+msg)
		return 0, 0, false
	}
	removed, kept := 0, 0
	var list []any
	switch g := groups.(type) {
	case []any:
		list = g
	case M:
		for _, v := range g {
			list = append(list, v)
		}
	}
	for _, g := range list {
		gm := mapOf(g)
		if a, ok := gm["remove"].([]any); ok {
			removed += len(a)
		}
		if a, ok := gm["keep"].([]any); ok {
			kept += len(a)
		}
	}
	r.log(fmt.Sprintf("Retention: kept %d, removed %d snapshot(s)", kept, removed))
	return removed, kept, true
}

func (r *runner) recordStats() M {
	var stats M
	code, _ := r.restic([]string{"stats", "--json", "--mode", "raw-data", "--no-lock"}, func(j any) {
		if m, ok := j.(M); ok && m["total_size"] != nil {
			stats = m
		}
	}, nil)
	if code == 0 && stats != nil {
		stats["at"] = now()
		repoID := r.repo.ID
		stateUpdate(r.settings, func(state M) {
			sub(state, "repos", repoID)["stats"] = stats
			stateAddRepoSize(state, repoID, stats)
		})
	}
	return stats
}

func (r *runner) runBackup() {
	job := *r.job
	tmp := OpsDir + "/" + r.id()
	r.onCleanup("removing temporary files", func() error {
		os.Remove(tmp + ".exclude")
		os.Remove(tmp + ".iexclude")
		return nil
	})
	healthcheckPing(job.Notify.Healthcheck, "start", "")

	r.phase("Checking the sources")
	for _, src := range job.Sources {
		st, err := os.Stat(src)
		if err != nil {
			r.finish("error", "Source "+src+" does not exist.", nil)
			return
		}
		if st.IsDir() && !dirHasEntries(src) {
			r.finish("error", "Source "+src+" is empty - is the disk mounted? Nothing was backed up.", nil)
			return
		}
	}

	if job.Hooks.Before != "" {
		code := r.runHook("before-backup", job.Hooks.Before, r.hookEnv("", ""))
		if r.cancelled() {
			r.finish("cancelled", "Stopped.", nil)
			return
		}
		if code != 0 {
			r.finish("error", fmt.Sprintf("The before-backup command failed (exit code %d); nothing was backed up.", code), nil)
			return
		}
	}

	var wrap func([]string) []string
	if job.Consistency.Mode != "none" {
		var ok bool
		ok, wrap = r.prepareConsistency(job)
		if !ok {
			return
		}
		if r.cancelled() {
			r.finish("cancelled", "Stopped.", nil)
			return
		}
	}

	var summary M
	progress := newBackupProgress(r.settings, job.ID)
	r.phase("Backing up")
	code, errLines := r.resticUnlocking(backupArgs(job, r.repo, r.settings, tmp), func(v any) {
		j, ok := v.(M)
		if !ok {
			return
		}
		switch str(j["message_type"]) {
		case "status":
			current := ""
			if cf, ok := j["current_files"].([]any); ok && len(cf) > 0 && cf[0] != nil {
				current = str(cf[0])
			}
			pr := progress.update(j)
			pr["current"] = current
			r.op["progress"] = pr
		case "summary":
			delete(j, "message_type")
			summary = j
		}
	}, wrap)
	snapshot := ""
	if summary != nil {
		snapshot = str(summary["snapshot_id"])
		short := snapshot
		if len(short) > 8 {
			short = short[:8]
		}
		r.log("Snapshot " + short + ": " + r.summaryText(summary))
	}
	if job.Consistency.Mode != "none" {
		r.phase("Cleaning up")
	}
	r.runCleanups()

	if r.cancelled() || code == 130 {
		r.finish("cancelled", "Stopped - no snapshot was saved.", summary)
		return
	}
	if code != 0 && code != 3 {
		r.finish("error", resticError(errLines, code), summary)
		return
	}
	status, warning := "success", ""
	if code == 3 {
		status = "warning"
		warning = "Some files could not be read and are missing from the snapshot - see the log."
	}

	if job.Retention.Enabled && !r.repo.AppendOnly && !r.cancelled() {
		if removed, kept, ok := r.applyRetention(job); ok && summary != nil {
			summary["forget_removed"] = removed
			summary["forget_kept"] = kept
		}
	}

	state := stateRead(r.settings)
	at := toInt64(mapOf(mapOf(mapOf(state["repos"])[r.repo.ID])["stats"])["at"])
	if at < now()-6*3600 && !r.cancelled() {
		r.phase("Measuring the repository")
		r.recordStats()
	}

	if job.Hooks.After != "" {
		if code := r.runHook("after-backup", job.Hooks.After, r.hookEnv(status, snapshot)); code != 0 {
			r.errors = append(r.errors, fmt.Sprintf("The after-backup command failed (exit code %d)", code))
		}
	}
	r.finish(status, warning, summary)
}

func (r *runner) runForget() {
	if r.repo.AppendOnly {
		r.finish("error", "The repository is append-only; snapshots can only be removed on the server.", nil)
		return
	}
	removed, kept, ok := r.applyRetention(*r.job)
	if !ok {
		r.finish("error", strings.Join(r.errors, "; "), nil)
		return
	}
	r.errors = nil
	r.finish("success", "", M{"forget_removed": removed, "forget_kept": kept})
}

func (r *runner) runPrune() {
	if r.repo.AppendOnly {
		r.finish("error", "The repository is append-only; prune has to run on the server.", nil)
		return
	}
	r.phase("Removing data no snapshot needs")
	code, errs := r.resticUnlocking([]string{"prune", "--max-unused", r.repo.Maintenance.Prune.MaxUnused, "--retry-lock", "5m"}, nil, nil)
	if r.cancelled() || code == 130 {
		r.finish("cancelled", "Stopped. Prune can be stopped safely; it continues next time.", nil)
		return
	}
	if code != 0 {
		r.finish("error", resticError(errs, code), nil)
		return
	}
	r.phase("Measuring the repository")
	r.finish("success", "", r.recordStats())
}

func (r *runner) runCheck() {
	c := r.repo.Maintenance.Check
	args := []string{"check", "--json", "--retry-lock", "5m"}
	switch c.ReadData {
	case "subset":
		args = append(args, "--read-data-subset", c.Subset)
	case "all":
		args = append(args, "--read-data")
	}
	switch c.ReadData {
	case "none":
		r.phase("Checking the repository structure")
	case "all":
		r.phase("Checking the repository and reading all data")
	default:
		r.phase("Checking the repository and reading " + c.Subset + " of the data")
	}
	var summary M
	code, errs := r.resticUnlocking(args, func(v any) {
		j, ok := v.(M)
		if !ok {
			return
		}
		switch str(j["message_type"]) {
		case "summary":
			delete(j, "message_type")
			summary = j
		case "status":
			if j["percent_done"] != nil {
				r.op["progress"] = M{"percent": toFloat(j["percent_done"]), "at": now()}
			}
		}
	}, nil)
	if r.cancelled() || code == 130 {
		r.finish("cancelled", "Stopped.", nil)
		return
	}
	problems := toInt64(summary["num_errors"])
	if code != 0 || problems > 0 {
		msg := resticError(errs, code)
		if problems > 0 {
			msg = fmt.Sprintf("The check found %d problem(s)", problems)
		}
		if phpTruthy(summary["suggest_repair_index"]) {
			msg += " - restic suggests \"repair index\""
		} else if phpTruthy(summary["suggest_prune"]) {
			msg += " - restic suggests running prune"
		}
		r.finish("error", msg+". See the log.", summary)
		return
	}
	r.finish("success", "", summary)
}

var globEscaper = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`)

func globEscape(name string) string {
	return globEscaper.Replace(name)
}

func phpDirname(p string) string {
	if p != "/" {
		p = strings.TrimRight(p, "/")
	}
	return path.Dir(p)
}

func phpBasename(p string) string {
	return path.Base(strings.TrimRight(p, "/"))
}

func (r *runner) runRestore(req M) {
	snap := str(req["snapshot"])
	items := stringList(req["items"])
	target := str(req["target"])
	overwrite := str(req["overwrite"])
	sum := M{"files_restored": int64(0), "files_skipped": int64(0), "files_deleted": int64(0),
		"bytes_restored": int64(0), "bytes_skipped": int64(0), "items": len(items)}
	for i, p := range items {
		if r.cancelled() {
			r.finish("cancelled", "Stopped. What was restored until then stays.", sum)
			return
		}
		whole := p == "/"
		parent := "/"
		if !whole {
			parent = phpDirname(p)
		}
		dest := str(req["target_path"])
		if target == "original" {
			dest = parent
		}
		if !mkdirLikeParent(dest) {
			r.finish("error", "Cannot create "+dest, sum)
			return
		}
		src := snap + ":" + parent
		if whole {
			src = snap
		}
		args := []string{"restore", "--json", src, "--target", dest, "--overwrite", overwrite, "--retry-lock", "10m"}
		if !whole {
			args = append(args, "--include", "/"+globEscape(phpBasename(p)))
		}
		if phpTruthy(req["delete"]) {
			args = append(args, "--delete")
		}
		if len(items) > 1 {
			r.phase(fmt.Sprintf("Restoring %d of %d: %s", i+1, len(items), p))
		} else {
			r.phase("Restoring " + p)
		}
		var result M
		code, errs := r.restic(args, func(v any) {
			j, ok := v.(M)
			if !ok {
				return
			}
			switch str(j["message_type"]) {
			case "status":
				r.op["progress"] = M{
					"percent":     toFloat(j["percent_done"]),
					"files_done":  toInt64(j["files_restored"]) + toInt64(j["files_skipped"]),
					"total_files": toInt64(j["total_files"]),
					"bytes_done":  toInt64(j["bytes_restored"]) + toInt64(j["bytes_skipped"]),
					"total_bytes": toInt64(j["total_bytes"]),
					"at":          now(),
				}
			case "summary":
				result = j
			}
		}, nil)
		if r.cancelled() || code == 130 {
			r.finish("cancelled", "Stopped. What was restored until then stays.", sum)
			return
		}
		if code != 0 {
			r.finish("error", resticError(errs, code), sum)
			return
		}
		for _, k := range []string{"files_restored", "files_skipped", "files_deleted", "bytes_restored", "bytes_skipped"} {
			sum[k] = toInt64(sum[k]) + toInt64(result[k])
		}
	}
	if target == "original" {
		sum["target"] = "original location"
	} else {
		sum["target"] = str(req["target_path"])
	}
	r.finish("success", "", sum)
}

func (r *runner) watchSignals() func() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				if f, err := os.Create(opCancelFile(r.id())); err == nil {
					f.Close()
				}
				r.log("Stop requested by a signal")
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

func (r *runner) execute(kind string, request M) {
	defer func() {
		if p := recover(); p != nil {
			r.finish("error", fmt.Sprintf("The operation ended unexpectedly: %v", p), nil)
		}
		if !r.finished {
			r.finish("error", "The operation ended unexpectedly.", nil)
		}
	}()
	switch kind {
	case "backup":
		r.runBackup()
	case "forget":
		r.runForget()
	case "prune":
		r.runPrune()
	case "check":
		r.runCheck()
	case "restore":
		r.runRestore(request)
	default:
		r.finish("error", kind+" is not implemented yet", nil)
	}
}

var errNoRequest = errors.New("No restore request")

func readRestoreRequest(file string) (M, error) {
	if !strings.HasPrefix(file, RunDir+"/requests/") || !isFile(file) {
		return nil, errNoRequest
	}
	req, _ := jsonRead(file)
	os.Remove(file)
	if req == nil || !phpTruthy(req["items"]) {
		return nil, errors.New("Malformed restore request")
	}
	return req, nil
}

func cmdRun(args []string) int {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	kind, target := arg(0), arg(1)
	trigger := "manual"
	if inList(arg(2), "manual", "schedule") {
		trigger = arg(2)
	}
	cfg := configLoad()
	var job *Job
	var request M
	if kind == "restore" {
		req, err := readRestoreRequest(arg(2))
		if err != nil {
			return runFailEarly(err.Error())
		}
		request = req
		trigger = "manual"
	}
	var repo *Repo
	switch kind {
	case "backup", "forget":
		job = cfg.job(target)
		if job == nil {
			return runFailEarly("No job " + target)
		}
		repo = cfg.repo(job.Repo)
	case "prune", "check", "restore":
		repo = cfg.repo(target)
	default:
		return runFailEarly("Usage: restic-backup run backup|forget|prune|check|restore <id> [manual|schedule]")
	}
	if repo == nil {
		return runFailEarly("No repository for " + target)
	}

	var lock *fileLock
	if kind != "restore" {
		lock = repoLock(repo.ID)
		if lock == nil {
			if job != nil && kind == "backup" {
				jobID := job.ID
				stateUpdate(cfg.Settings, func(state M) {
					sub(state, "jobs", jobID)["queued"] = now()
				})
			}
			return 75
		}
	}
	r := newRunner(kind, cfg, *repo, job, trigger, lock)
	stop := r.watchSignals()
	defer stop()
	r.write(true)
	if kind != "restore" {
		browseStop(repo.ID, 0)
	}
	if problem := r.preflight(); problem != "" {
		r.finish("error", problem, nil)
		return 1
	}
	r.execute(kind, request)
	if str(r.op["status"]) == "error" {
		return 1
	}
	return 0
}
