package rb

import (
	"os"
	"testing"
	"time"
)

func TestRunLogsGoWithTheHistory(t *testing.T) {
	base := useTestDirs(t)
	s := defaultSettings()
	s.DataDir = base + "/pool/data"
	s.HistoryDays = 30
	os.MkdirAll(s.DataDir+"/logs", 0o755)
	os.MkdirAll(OpsDir, 0o755)
	log := func(id string, age time.Duration) string {
		p := s.DataDir + "/logs/" + id + ".log"
		os.WriteFile(p, []byte("x\n"), 0o644)
		at := time.Now().Add(-age)
		os.Chtimes(p, at, at)
		return p
	}
	recent := log("20261115-010000-aaaa", time.Hour)
	old := log("20261001-010000-bbbb", 40*24*time.Hour)
	orphan := log("20261114-010000-cccc", 2*time.Hour)
	running := log("20261115-020000-dddd", 2*time.Hour)
	finishing := log("20261115-030000-eeee", time.Second)
	opWrite(M{"id": "20261115-030000-eeee", "status": "success", "runner_pid": os.Getpid()})
	historyAppend(s, M{"id": "20261115-010000-aaaa", "ended": now()})
	historyAppend(s, M{"id": "20261001-010000-bbbb", "ended": now() - 40*86400})
	opWrite(M{"id": "20261115-020000-dddd", "status": "running", "runner_pid": os.Getpid()})
	logsPrune(s)
	for p, want := range map[string]bool{recent: true, old: false, orphan: false, running: true, finishing: true} {
		if exists(p) != want {
			t.Errorf("%s: kept=%v, want %v", p, exists(p), want)
		}
	}
}

func TestRunLogsStayWhenTheHistoryCannotBeRead(t *testing.T) {
	base := useTestDirs(t)
	s := defaultSettings()
	s.DataDir = base + "/pool/data"
	os.MkdirAll(s.DataDir+"/logs", 0o755)
	p := s.DataDir + "/logs/20261114-010000-cccc.log"
	os.WriteFile(p, []byte("x\n"), 0o644)
	at := time.Now().Add(-2 * time.Hour)
	os.Chtimes(p, at, at)
	os.MkdirAll(historyFile(s), 0o755)
	logsPrune(s)
	if !exists(p) {
		t.Fatal("an unreadable history must not count as empty")
	}
}
