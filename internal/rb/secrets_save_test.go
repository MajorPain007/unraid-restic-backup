package rb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsStayWhenTheSettingsCannotBeSaved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only folder")
	}
	base := useTestDirs(t)
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{{ID: "r00000001", Name: "a", Type: "local"}}, Jobs: []Job{}}
	cfg.Settings.DataDir = base + "/pool"
	saveMode(t, cfg, "plain")
	secretSet("r00000001", "password", "repo-password-123")
	before, _ := os.ReadFile(ConfigFile)
	os.Chmod(ConfigDir, 0o500)
	t.Cleanup(func() { os.Chmod(ConfigDir, 0o755) })

	if _, err := apiSecretsMode(M{"mode": "key"}); err == nil {
		t.Fatal("the mode change must report that nothing was saved")
	}
	if _, err := apiRepoDelete(M{"id": "r00000001"}); err == nil {
		t.Fatal("the delete must report that nothing was saved")
	}
	if after, _ := os.ReadFile(ConfigFile); string(after) != string(before) {
		t.Fatal("config.json changed")
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("the password is gone although the settings still name the repository")
	}
	left, _ := filepath.Glob(SecretsDir + "/*")
	for _, f := range left {
		if strings.HasSuffix(f, ".enc") || strings.HasSuffix(f, ".new") {
			t.Fatalf("a half-done switch left %s", f)
		}
	}
	if exists(keyFile(cfg.Settings)) {
		t.Fatal("the key of a switch that did not happen was left behind")
	}

	os.Chmod(ConfigDir, 0o755)
	if _, err := apiSecretsMode(M{"mode": "key"}); err != nil {
		t.Fatal(err)
	}
	os.Chmod(ConfigDir, 0o500)
	if _, err := apiSecretsMode(M{"mode": "master", "password": "correct horse battery", "password_repeat": "correct horse battery"}); err == nil {
		t.Fatal("key to master must report that nothing was saved")
	}
	if configLoad().Settings.Secrets != "key" || secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("after a failed key-to-master switch the key mode must still read the password")
	}
	if exists(masterFile()) {
		t.Fatal("a master file of a switch that did not happen was left behind")
	}
}
