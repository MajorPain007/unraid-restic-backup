package rb

import (
	"os"
	"strings"
	"testing"
)

func fakeRestic(t *testing.T, backup string) {
	dir := t.TempDir()
	script := "#!/bin/sh\nfor a; do case \"$a\" in\nbackup) " + backup + " ;;\nforget) echo '[]'; exit 0 ;;\nstats) echo '{\"total_size\":100,\"total_file_count\":1}'; exit 0 ;;\nesac; done\necho '[]'\n"
	os.WriteFile(dir+"/restic", []byte(script), 0o755)
	old := ResticBin
	t.Cleanup(func() { ResticBin = old })
	ResticBin = dir + "/restic"
}

func runSetup(t *testing.T) *Config {
	cfg, _, _ := tickTestSetup(t)
	cfg.Settings.Secrets, cfg.Settings.IOPriority = "plain", "normal"
	src := cfg.Jobs[0].Sources[0]
	os.MkdirAll(src, 0o755)
	os.WriteFile(src+"/a.txt", []byte("hello"), 0o644)
	os.MkdirAll(cfg.Repos[0].Local.Path, 0o755)
	os.WriteFile(cfg.Repos[0].Local.Path+"/config", []byte("x"), 0o600)
	configSave(cfg)
	secretSet("r00000001", "password", "repo-password-123")
	return configLoad()
}

func TestABackupRunEndsInTheHistory(t *testing.T) {
	cfg := runSetup(t)
	fakeRestic(t, `echo '{"message_type":"summary","snapshot_id":"abcdef1234567890","files_new":1,"total_bytes_processed":5,"data_added":5}'; exit 0`)
	cmdRun([]string{"backup", "j00000001", "manual"})
	h := historyRead(cfg.Settings, M{}, 5)
	if len(h) != 1 || str(h[0]["status"]) != "success" || str(mapOf(h[0]["summary"])["snapshot_id"]) != "abcdef1234567890" {
		t.Fatalf("the run is in the history with its snapshot: %v", h)
	}
	js := mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])
	if str(mapOf(js["last"])["status"]) != "success" || toInt64(js["last_success"]) == 0 {
		t.Fatalf("the job remembers the run: %v", js)
	}
	if left, _ := os.ReadDir(runMarkDir(cfg.Settings)); len(left) != 0 {
		t.Fatal("no run mark stays")
	}
	if b, _ := os.ReadFile(str(h[0]["log"])); !strings.Contains(string(b), "Finished: success") || !strings.Contains(string(b), "Snapshot abcdef12:") {
		t.Fatalf("the log names the snapshot and ends with the result: %s", b)
	}
	if sizes, _ := js["sizes"].([]any); len(sizes) != 1 {
		t.Fatalf("one size point per successful run: %v", js["sizes"])
	}
	stateUpdate(cfg.Settings, func(st M) {
		p := sub(st, "jobs", "j00000001")["sizes"].([]any)[0].([]any)
		p[0] = toInt64(p[0]) - 3600
	})
	cmdRun([]string{"backup", "j00000001", "manual"})
	js = mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])
	if sizes, _ := js["sizes"].([]any); len(sizes) != 2 || len(historyRead(cfg.Settings, M{}, 5)) != 2 {
		t.Fatalf("a later run adds a second point: %v", js["sizes"])
	}
}

func TestABackupWithUnreadableFilesIsAWarning(t *testing.T) {
	cfg := runSetup(t)
	fakeRestic(t, `echo '{"message_type":"error","error":{"message":"open /x: permission denied"},"during":"archival","item":"/x"}'; echo '{"message_type":"summary","snapshot_id":"abcdef1234567890","files_new":1,"data_added":5}'; exit 3`)
	cmdRun([]string{"backup", "j00000001", "manual"})
	h := historyRead(cfg.Settings, M{}, 5)
	if len(h) != 1 || str(h[0]["status"]) != "warning" || str(h[0]["error"]) == "" {
		t.Fatalf("a snapshot with unreadable files is a warning that says why: %v", h)
	}
	js := mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])
	if sizes, _ := js["sizes"].([]any); len(sizes) != 1 || toInt64(js["last_success"]) == 0 {
		t.Fatalf("a warning still counts as a backup: %v", js)
	}
}

func TestABackupWithoutItsRepositoryStopsEarly(t *testing.T) {
	cfg := runSetup(t)
	fakeRestic(t, `echo '{"message_type":"summary","snapshot_id":"abcdef1234567890"}'; exit 0`)
	os.Remove(cfg.Repos[0].Local.Path + "/config")
	cmdRun([]string{"backup", "j00000001", "manual"})
	h := historyRead(cfg.Settings, M{}, 5)
	if len(h) != 1 || str(h[0]["status"]) != "error" || !strings.Contains(str(h[0]["error"]), "is the disk mounted") {
		t.Fatalf("a missing repository is named before anything runs: %v", h)
	}
	if b, _ := os.ReadFile(str(h[0]["log"])); !strings.Contains(string(b), "Finished: error - No repository") {
		t.Fatalf("the log says why: %s", b)
	}
}

func TestAFailedBackupSaysWhy(t *testing.T) {
	cfg := runSetup(t)
	fakeRestic(t, `echo 'Fatal: unable to open repository at /x: no such file' >&2; exit 1`)
	cmdRun([]string{"backup", "j00000001", "manual"})
	h := historyRead(cfg.Settings, M{}, 5)
	if len(h) != 1 || str(h[0]["status"]) != "error" || !strings.Contains(str(h[0]["error"]), "unable to open repository") {
		t.Fatalf("the error is kept with restic's reason: %v", h)
	}
	js := mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])
	if toInt64(js["last_success"]) != 0 || js["sizes"] != nil {
		t.Fatalf("a failed run is no success and adds no size point: %v", js)
	}
}
