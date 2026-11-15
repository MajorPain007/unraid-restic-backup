package rb

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func symlinkVictim(t *testing.T) (Settings, string) {
	base := useTestDirs(t)
	s := defaultSettings()
	s.DataDir = base + "/pool/data"
	os.MkdirAll(s.DataDir, 0o755)
	victim := base + "/victim.txt"
	os.WriteFile(victim, []byte("untouched\n"), 0o644)
	return s, victim
}

func untouched(t *testing.T, victim, what string) {
	t.Helper()
	if b, _ := os.ReadFile(victim); string(b) != "untouched\n" {
		t.Fatalf("%s wrote through a symlink into %s: %q", what, victim, b)
	}
}

func TestHistoryIsNeverAppendedThroughASymlink(t *testing.T) {
	s, victim := symlinkVictim(t)
	os.Symlink(victim, historyFile(s))
	historyAppend(s, M{"id": "x", "ended": now()})
	untouched(t, victim, "historyAppend")
}

func TestStateIsNeverWrittenThroughASymlink(t *testing.T) {
	s, victim := symlinkVictim(t)
	os.Symlink(victim, s.DataDir+"/state.json")
	stateUpdate(s, func(st M) { st["x"] = 1 })
	untouched(t, victim, "stateUpdate")
}

func TestATempFileNameCannotBePlantedAhead(t *testing.T) {
	s, victim := symlinkVictim(t)
	os.Symlink(victim, fmt.Sprintf("%s/state.json.tmp%d", s.DataDir, os.Getpid()))
	jsonWrite(s.DataDir+"/state.json", M{"x": 1}, 0o644)
	untouched(t, victim, "jsonWrite")
}

func TestRunLogIsNeverAppendedThroughASymlink(t *testing.T) {
	s, victim := symlinkVictim(t)
	os.MkdirAll(s.DataDir+"/logs", 0o755)
	os.Symlink(victim, s.DataDir+"/logs/run.log")
	logAppendMax(s.DataDir+"/logs/run.log", "line", 16*1048576)
	untouched(t, victim, "logAppendMax")
}

func TestDataFolderWithSymlinkedSubfoldersIsRefused(t *testing.T) {
	for _, sub := range []string{"logs", "cache", "journal"} {
		t.Run(sub, func(t *testing.T) {
			s, _ := symlinkVictim(t)
			elsewhere := t.TempDir()
			os.Symlink(elsewhere, s.DataDir+"/"+sub)
			if p := prepareDataDir(s); !strings.Contains(p, "symbolic link") {
				t.Fatalf("a symlinked %s/ must be refused, got %q", sub, p)
			}
		})
	}
}

func TestHistoryCompactAndLogTrimNeverFollowASymlink(t *testing.T) {
	s, victim := symlinkVictim(t)
	os.Symlink(victim, historyFile(s))
	historyCompact(s)
	untouched(t, victim, "historyCompact")
	os.Symlink(victim, s.DataDir+"/trim.log")
	logTrim(s.DataDir+"/trim.log", 1)
	untouched(t, victim, "logTrim")
}

func TestTickTellsOnceThatTheDataFolderIsRefused(t *testing.T) {
	cfg, notes, spawned := tickTestSetup(t)
	os.RemoveAll(cfg.Settings.DataDir + "/logs")
	os.Symlink(t.TempDir(), cfg.Settings.DataDir+"/logs")
	cmdTick(nil)
	cmdTick(nil)
	waitNotes(notes, 1)
	if n := waitNotes(notes, 2); len(n) != 1 || !strings.Contains(n[0], "symbolic link") {
		t.Fatalf("one notification naming the link, got %q", n)
	}
	if len(*spawned) != 0 {
		t.Fatalf("nothing may start, got %v", *spawned)
	}
}
