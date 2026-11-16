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
	if b, _ := os.ReadFile(str(h[0]["log"])); !strings.Contains(string(b), "Finished: success") {
		t.Fatalf("the log ends with the result: %s", b)
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
	if toInt64(mapOf(mapOf(stateRead(cfg.Settings)["jobs"])["j00000001"])["last_success"]) != 0 {
		t.Fatal("a failed run is no success")
	}
}
