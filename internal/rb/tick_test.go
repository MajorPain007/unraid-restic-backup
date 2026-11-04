package rb

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func tickTestSetup(t *testing.T) (*Config, string, *[][]string) {
	base := useTestDirs(t)
	oldIni, oldNotify, oldSpawn := VarIni, notifyBin, tickSpawn
	t.Cleanup(func() { VarIni, notifyBin, tickSpawn = oldIni, oldNotify, oldSpawn })
	VarIni = base + "/var.ini"
	os.WriteFile(VarIni, []byte("mdState=\"STARTED\"\nfsState=\"Started\"\nstartMode=\"Normal\"\nmdResync=\"0\"\nmdResyncPos=\"0\"\n"), 0o644)
	notes := base + "/notifications"
	notifyBin = base + "/notify"
	os.WriteFile(notifyBin, []byte("#!/bin/sh\necho \"$*\" >> "+notes+"\n"), 0o755)
	os.MkdirAll(base+"/bin", 0o755)
	os.WriteFile(base+"/bin/setsid", []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755)
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Setenv("PATH", base+"/bin:"+os.Getenv("PATH"))
	}
	spawned := &[][]string{}
	tickSpawn = func(args ...string) { *spawned = append(*spawned, args) }

	repo := defaultRepo()
	repo.ID, repo.Name, repo.Type, repo.Local.Path = "r00000001", "Disk", "local", base+"/repo"
	repo.Maintenance.Prune.Enabled = false
	repo.Maintenance.Check.Enabled = false
	job := defaultJob()
	job.ID, job.Name, job.Repo, job.Sources = "j00000001", "Docs", repo.ID, []string{base + "/src"}
	job.Schedule = Schedule{Mode: "cron", Cron: "0 3 * * *", CatchUp: true}
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{repo}, Jobs: []Job{job}}
	cfg.Settings.DataDir = base + "/pool"
	cfg.Settings.Secrets = "master"
	if !configSave(cfg) || prepareDataDir(cfg.Settings) != "" {
		t.Fatal("setup failed")
	}
	return cfg, notes, spawned
}

func waitNotes(path string, n int) []string {
	var lines []string
	for i := 0; i < 50; i++ {
		b, _ := os.ReadFile(path)
		lines = nil
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) >= n {
			return lines
		}
		time.Sleep(50 * time.Millisecond)
	}
	return lines
}

func TestTickWaitsForMasterPassword(t *testing.T) {
	cfg, notes, spawned := tickTestSetup(t)
	slot := cronLocal(2026, 11, 3, 3, 0)
	stateUpdate(cfg.Settings, func(s M) {
		sub(s, "jobs", "j00000001")["sched"] = M{"key": "cron:0 3 * * *", "since": slot - 86400, "slot": slot - 86400}
	})
	at := slot + 3600

	if !masterLocked(cfg.Settings) {
		t.Fatal("no unlocked key: master mode must be locked")
	}
	schedulerTick(configLoad(), at)
	schedulerTick(configLoad(), at+60)
	if len(*spawned) != 0 {
		t.Fatalf("nothing may start while locked, got %v", *spawned)
	}
	sched := mapOf(mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])["sched"])
	if toInt64(sched["slot"]) != slot-86400 {
		t.Fatalf("the due slot must not be used up while locked: %v", sched)
	}
	lines := waitNotes(notes, 1)
	time.Sleep(200 * time.Millisecond)
	lines = waitNotes(notes, 1)
	if len(lines) != 1 || !strings.Contains(lines[0], lockedSubject) || !strings.Contains(lines[0], "-i warning") {
		t.Fatalf("expected one warning notification, got %q", lines)
	}
	if !has(stateRead(cfg.Settings), "locked_notified") {
		t.Fatal("the notification must be remembered in state.json")
	}

	key := make([]byte, 32)
	rand.Read(key)
	os.MkdirAll(runSecretsDir(), 0o700)
	os.WriteFile(unlockedKeyFile(), []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600)
	if masterLocked(cfg.Settings) {
		t.Fatal("the key in RAM must unlock")
	}
	schedulerTick(configLoad(), at+120)
	if len(*spawned) != 1 || strings.Join((*spawned)[0], " ") != "run backup j00000001 schedule" {
		t.Fatalf("the missed slot must run once unlocked, got %v", *spawned)
	}
	st := stateRead(cfg.Settings)
	if toInt64(mapOf(mapOf(mapOf(st["jobs"])["j00000001"])["sched"])["slot"]) != slot {
		t.Fatal("the slot must be recorded once the run started")
	}
	if has(st, "locked_notified") {
		t.Fatal("unlocked: the notification mark must be cleared")
	}
	if n := waitNotes(notes, 2); len(n) != 1 {
		t.Fatalf("no further notification expected, got %q", n)
	}
}

func TestEventArrayStartedNotifiesWhenLocked(t *testing.T) {
	cfg, notes, _ := tickTestSetup(t)
	if code := cmdEvent([]string{"array-started"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := waitNotes(notes, 1)
	if len(lines) != 1 || !strings.Contains(lines[0], lockedSubject) {
		t.Fatalf("expected the locked notification, got %q", lines)
	}
	if !has(stateRead(cfg.Settings), "locked_notified") {
		t.Fatal("the notification must be remembered")
	}
}
