package rb

import (
	"testing"
	"time"
)

func TestBackupProgressWhileCounting(t *testing.T) {
	clock := time.Unix(1000, 0)
	p := &backupProgress{prevBytes: 69e9, prevFiles: 569000, now: func() time.Time { return clock }}

	got := p.update(M{"percent_done": 0.99, "bytes_done": 26e9, "total_bytes": 26.2e9, "files_done": 174906, "total_files": 176810})
	if got["counting"] != true || got["estimate"] != true {
		t.Fatalf("expected counting with estimate, got %v", got)
	}
	if got["total_bytes"] != int64(69e9) || got["total_files"] != int64(569000) {
		t.Fatalf("expected totals of the last backup, got %v", got)
	}
	if pct := got["percent"].(float64); pct < 0.37 || pct > 0.38 {
		t.Fatalf("percent %v", pct)
	}
	if got["eta"] != nil {
		t.Fatalf("no eta while counting, got %v", got["eta"])
	}

	clock = clock.Add(5 * time.Second)
	got = p.update(M{"percent_done": 0.7, "bytes_done": 49e9, "total_bytes": 70e9, "files_done": 514752, "total_files": 590000})
	if got["estimate"] == true || got["percent"] != nil || got["counting"] != true {
		t.Fatalf("a total above the last backup is still counting without an estimate, got %v", got)
	}

	clock = clock.Add(2 * time.Second)
	got = p.update(M{"percent_done": 0.72, "bytes_done": 50e9, "total_bytes": 70e9, "files_done": 520000, "total_files": 590000, "seconds_remaining": 600})
	if got["counting"] != nil || got["percent"] != 0.72 || got["eta"] != int64(600) {
		t.Fatalf("restic's own figures once it gives a time left, got %v", got)
	}

	clock = clock.Add(time.Second)
	got = p.update(M{"percent_done": 0.99, "bytes_done": 69e9, "total_bytes": 70e9, "files_done": 589000, "total_files": 590000})
	if got["counting"] != nil || got["percent"] != 0.99 {
		t.Fatalf("the scan stays finished when restic leaves out a zero time left, got %v", got)
	}
}

func TestBackupProgressSettles(t *testing.T) {
	clock := time.Unix(1000, 0)
	p := &backupProgress{now: func() time.Time { return clock }}
	st := M{"percent_done": 0.5, "bytes_done": 5e9, "total_bytes": 10e9, "files_done": 10, "total_files": 20}
	if got := p.update(st); got["counting"] != true || got["percent"] != nil {
		t.Fatalf("first run without history counts, got %v", got)
	}
	clock = clock.Add(scanSettle + time.Second)
	if got := p.update(st); got["counting"] != nil || got["percent"] != 0.5 {
		t.Fatalf("totals that stopped changing are final, got %v", got)
	}
}

func TestBackupProgressUsesLastGoodRun(t *testing.T) {
	s := Settings{DataDir: t.TempDir()}
	historyAppend(s, M{"id": "a", "kind": "backup", "job": "j1", "status": "success", "summary": M{"total_bytes_processed": 100, "total_files_processed": 7}})
	historyAppend(s, M{"id": "b", "kind": "backup", "job": "j1", "status": "error"})
	p := newBackupProgress(s, "j1")
	if p.prevBytes != 100 || p.prevFiles != 7 {
		t.Fatalf("got %d %d", p.prevBytes, p.prevFiles)
	}
}
