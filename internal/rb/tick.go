package rb

import (
	"crypto/hmac"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func init() {
	register("tick", "run once a minute by cron: start due jobs and maintenance, clean up after dead runners", cmdTick)
	register("stop-all", "[seconds]  stop every operation, for the array stop or uninstall", cmdStopAll)
	register("event", "array-started  react to an Unraid event", cmdEvent)
}

var tickSpawn = spawn

const (
	lockedSubject     = "Restic Backup is waiting for the master password"
	lockedDescription = "Scheduled backups wait until it is entered on the plugin page."
)

func masterLocked(s Settings) bool {
	return s.Secrets == "master" && secretsLocked()
}

func touchFile(p string) {
	t := time.Now()
	if os.Chtimes(p, t, t) != nil {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Close()
		}
	}
}

func notifyOnce(name, subject, problem string) bool {
	marker := RunDir + "/" + name + ".notified"
	if problem == "" {
		os.Remove(marker)
		return false
	}
	if !exists(marker) {
		notify(subject, problem, "alert", "")
		touchFile(marker)
	}
	return true
}

func cmdTick(args []string) int {
	os.MkdirAll(RunDir, 0o755)
	l := lockFile(RunDir+"/tick.lock", false)
	if l == nil {
		return 0
	}
	defer l.Unlock()
	touchFile(RunDir + "/scheduler.tick")
	browseReap(false)
	if notifyOnce("config_problem", "Restic Backup cannot read its settings", configProblem()) {
		return 0
	}
	if !arrayStarted() {
		return 0
	}
	cfg := configLoad()
	if len(cfg.Jobs) == 0 && len(cfg.Repos) == 0 {
		return 0
	}
	if notifyOnce("data_dir_problem", "Restic Backup cannot use its data folder", prepareDataDir(cfg.Settings)) {
		return 0
	}
	for _, n := range journalRecover(cfg.Settings) {
		notify(n.Subject, n.Description, "warning", "")
	}
	notifyOnce("secrets_half_done", "Restic Backup could not finish a password mode change", secretsRepair())
	notifyOnce("journal_untrusted", "Restic Backup ignores a journal it cannot verify", journalUntrusted(cfg.Settings))
	reapDeadRuns(cfg)
	schedulerTick(cfg, now())
	return 0
}

func reapDeadRuns(cfg *Config) {
	for _, f := range opsFiles() {
		op, ok := jsonRead(f)
		if !ok || str(op["status"]) != "running" || pidAlive(op["runner_pid"]) || pidAlive(op["pid"]) {
			continue
		}
		r := &runner{op: op, cfg: cfg, settings: cfg.Settings, logFile: str(op["log"]),
			repo: Repo{ID: str(op["repo"]), Name: str(op["repo_name"])}, job: cfg.job(str(op["job"]))}
		if repo := cfg.repo(r.repo.ID); repo != nil {
			r.repo = *repo
		}
		r.finish("error", "The run was interrupted: its process ended without finishing (killed, out of memory, or the server stopped).", nil)
	}
	marks, _ := filepath.Glob(runMarkDir(cfg.Settings) + "/*.json")
	for _, f := range marks {
		m, ok := runMarkRead(f)
		if !ok || str(m["boot"]) == bootID() {
			continue
		}
		if len(historyRead(cfg.Settings, M{"id": m["id"]}, 1)) > 0 {
			os.Remove(f)
			continue
		}
		op := M{"status": "running", "ended": int64(0)}
		for _, k := range append([]string{"id"}, runMarkFields...) {
			op[k] = m[k]
		}
		delete(op, "boot")
		r := &runner{op: op, cfg: cfg, settings: cfg.Settings, logFile: str(m["log"]),
			repo: Repo{ID: str(m["repo"]), Name: str(m["repo_name"])}, job: cfg.job(str(m["job"]))}
		if repo := cfg.repo(r.repo.ID); repo != nil {
			r.repo = *repo
		}
		r.finish("error", "The run was interrupted: the server restarted or lost power while it ran.", nil)
	}
}

var runMarkFields = []string{"kind", "job", "job_name", "repo", "repo_name", "trigger", "started", "log", "boot"}

func runMarkDir(s Settings) string {
	return s.DataDir + "/runs"
}

func runMarkFile(s Settings, id string) string {
	return runMarkDir(s) + "/" + id + ".json"
}

func (r *runner) markStart() {
	key := journalKey(true)
	if key == nil {
		return
	}
	m := M{"boot": bootID()}
	for _, k := range []string{"kind", "job", "job_name", "repo", "repo_name", "trigger", "started", "log"} {
		m[k] = r.op[k]
	}
	m["mac"] = signMAC(r.id(), m, runMarkFields, key)
	jsonWrite(runMarkFile(r.settings, r.id()), m, 0o644)
}

func runMarkRead(file string) (M, bool) {
	id := strings.TrimSuffix(filepath.Base(file), ".json")
	m, ok := jsonRead(file)
	if !ok || !reOpID.MatchString(id) {
		return nil, false
	}
	want := signMAC(id, m, runMarkFields, journalKey(false))
	if want == "" || !hmac.Equal([]byte(str(m["mac"])), []byte(want)) {
		return nil, false
	}
	m["id"] = id
	return m, true
}

func schedulerTick(cfg *Config, t int64) {
	settings := cfg.Settings
	startedAt := arrayStartedAt()
	holdoff := (startedAt > 0 && t-startedAt < int64(settings.StartDelay)*60) ||
		(settings.ParityPause && parityRunning())
	locked := masterLocked(settings)
	held := false

	state := stateRead(settings)
	started := map[string]bool{}
	recordJobs := map[string]M{}
	queueJobs := map[string]bool{}
	var spawnFailed error
	recordRepos := map[string]map[string]M{}

	for _, job := range cfg.Jobs {
		if !job.Enabled || cfg.repo(job.Repo) == nil {
			continue
		}
		js := mapOf(mapOf(state["jobs"])[job.ID])
		entry := mapOf(js["sched"])
		d := schedDecide(job.Schedule, entry, t)
		queued := phpTruthy(js["queued"])
		if !d.Due && !queued {
			if d.Changed {
				recordJobs[job.ID] = d.Entry
			}
			continue
		}
		if locked {
			held = true
			continue
		}
		if holdoff && !queued {
			continue
		}
		if started[job.Repo] || repoBusy(job.Repo) || opRunningOn(job.Repo, "restore") != nil {
			if d.Due {
				recordJobs[job.ID] = d.Entry
				queueJobs[job.ID] = true
			}
			continue
		}
		trigger := "manual"
		if d.Due {
			trigger = "schedule"
		}
		if err := tickSpawn("run", "backup", job.ID, trigger); err != nil {
			spawnFailed = err
			continue
		}
		started[job.Repo] = true
		if d.Due {
			recordJobs[job.ID] = d.Entry
		}
	}

	for _, repo := range cfg.Repos {
		for _, kind := range []string{"prune", "check"} {
			var enabled bool
			var sched Schedule
			if kind == "prune" {
				enabled, sched = repo.Maintenance.Prune.Enabled && !repo.AppendOnly, repo.Maintenance.Prune.Schedule
			} else {
				enabled, sched = repo.Maintenance.Check.Enabled, repo.Maintenance.Check.Schedule
			}
			if !enabled {
				continue
			}
			entry := mapOf(mapOf(mapOf(mapOf(state["repos"])[repo.ID])[kind])["sched"])
			d := schedDecide(sched, entry, t)
			if !d.Due {
				if d.Changed {
					if recordRepos[repo.ID] == nil {
						recordRepos[repo.ID] = map[string]M{}
					}
					recordRepos[repo.ID][kind] = d.Entry
				}
				continue
			}
			if locked {
				held = true
				continue
			}
			if holdoff || started[repo.ID] || repoBusy(repo.ID) || opRunningOn(repo.ID, "restore") != nil {
				continue
			}
			if err := tickSpawn("run", kind, repo.ID, "schedule"); err != nil {
				spawnFailed = err
				continue
			}
			started[repo.ID] = true
			if recordRepos[repo.ID] == nil {
				recordRepos[repo.ID] = map[string]M{}
			}
			recordRepos[repo.ID][kind] = d.Entry
		}
	}

	why := ""
	if spawnFailed != nil {
		why = "A scheduled run could not be started: " + spawnFailed.Error() + ". It is tried again every minute; reinstalling the plugin usually helps."
	}
	notifyOnce("spawn_failed", "Restic Backup cannot start scheduled runs", why)
	notifyLocked := held && !phpTruthy(state["locked_notified"])
	forget := !locked && has(state, "locked_notified")
	if notifyLocked {
		notify(lockedSubject, lockedDescription, "warning", "")
	}
	if len(recordJobs) > 0 || len(recordRepos) > 0 || notifyLocked || forget {
		stateUpdate(settings, func(s M) {
			for id, entry := range recordJobs {
				sub(s, "jobs", id)["sched"] = entry
			}
			for id := range queueJobs {
				sub(s, "jobs", id)["queued"] = t
			}
			for id, kinds := range recordRepos {
				for kind, entry := range kinds {
					sub(s, "repos", id, kind)["sched"] = entry
				}
			}
			if notifyLocked {
				s["locked_notified"] = t
			}
			if forget {
				delete(s, "locked_notified")
			}
		})
	}
}

func cmdStopAll(args []string) int {
	wait := 60
	if len(args) > 0 {
		wait = phpInt(args[0])
	}
	wait = max(5, wait)
	running := opsRunning()
	for _, op := range running {
		opRequestCancel(str(op["id"]))
	}
	for _, id := range browseRepos() {
		browseStop(id, 0)
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for time.Now().Before(deadline) {
		left := false
		for _, op := range running {
			if pidAlive(op["runner_pid"]) {
				left = true
				break
			}
		}
		browsing := false
		for _, id := range browseRepos() {
			if browseRunning(id) {
				browsing = true
				break
			}
		}
		if !left && !browsing {
			browseReap(true)
			wipeRunSecrets()
			return 0
		}
		time.Sleep(time.Second)
	}
	browseReap(true)
	for _, op := range running {
		for _, pid := range []int{toInt(op["pid"]), toInt(op["runner_pid"])} {
			if pid > 0 && pidAlive(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}
	wipeRunSecrets()
	return 0
}

func cmdEvent(args []string) int {
	if len(args) == 0 || args[0] != "array-started" {
		fmt.Fprintln(os.Stderr, "usage: restic-backup event array-started")
		return 2
	}
	cfg := configLoad()
	if !masterLocked(cfg.Settings) {
		return 0
	}
	notify(lockedSubject, lockedDescription, "warning", "")
	t := now()
	stateUpdate(cfg.Settings, func(s M) {
		s["locked_notified"] = t
	})
	return 0
}
