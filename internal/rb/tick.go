package rb

import (
	"fmt"
	"os"
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

func cmdTick(args []string) int {
	os.MkdirAll(RunDir, 0o755)
	touchFile(RunDir + "/scheduler.tick")
	for _, n := range journalRecover() {
		notify(n.Subject, n.Description, "warning", "")
	}
	browseReap(false)
	if !arrayStarted() {
		return 0
	}
	cfg := configLoad()
	if len(cfg.Jobs) == 0 && len(cfg.Repos) == 0 {
		return 0
	}
	if prepareDataDir(cfg.Settings) != "" {
		return 0
	}
	schedulerTick(cfg, now())
	return 0
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
		if (holdoff && !queued) || started[job.Repo] || repoBusy(job.Repo) {
			continue
		}
		trigger := "manual"
		if d.Due {
			trigger = "schedule"
		}
		tickSpawn("run", "backup", job.ID, trigger)
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
			tickSpawn("run", kind, repo.ID, "schedule")
			started[repo.ID] = true
			if recordRepos[repo.ID] == nil {
				recordRepos[repo.ID] = map[string]M{}
			}
			recordRepos[repo.ID][kind] = d.Entry
		}
	}

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
