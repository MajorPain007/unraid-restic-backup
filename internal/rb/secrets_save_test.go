package rb

import (
	"encoding/json"
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

func TestARepositoryPasswordChangesOnlyWithTheSettings(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only folder")
	}
	base := useTestDirs(t)
	repo := defaultRepo()
	repo.ID, repo.Name, repo.Type, repo.Local.Path = "r00000001", "a", "local", "/mnt/disks/backup/restic"
	sftpRepo := defaultRepo()
	sftpRepo.ID, sftpRepo.Name, sftpRepo.Type = "r00000002", "s", "sftp"
	sftpRepo.SFTP.Host, sftpRepo.SFTP.User, sftpRepo.SFTP.Path = "backup.example", "u", "restic"
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{repo, sftpRepo}, Jobs: []Job{}}
	cfg.Settings.DataDir = base + "/pool"
	saveMode(t, cfg, "plain")
	secretSet("r00000001", "password", "old-password-1")
	secretSet("r00000002", "password", "old-password-2")
	privateDir(SSHDir)
	var keyless M
	json.Unmarshal(jsonEncode(cfg.Repos[1], false), &keyless)
	keyless["name"] = "renamed too"
	var existing M
	json.Unmarshal(jsonEncode(cfg.Repos[0], false), &existing)
	existing["name"] = "renamed"
	fresh := M{"name": "b", "type": "local", "local": M{"path": "/mnt/disks/other/restic"}}
	sftp := M{"name": "c", "type": "sftp", "sftp": M{"host": "backup.example", "user": "u", "path": "restic"}}
	os.Chmod(ConfigDir, 0o500)
	t.Cleanup(func() { os.Chmod(ConfigDir, 0o755) })

	if _, err := apiRepoSave(M{"repo": existing, "password_mode": "set", "password": "new-password-2"}); err == nil {
		t.Fatal("the save must report that nothing was saved")
	}
	if secretGet("r00000001", "password") != "old-password-1" {
		t.Fatal("the stored password changed although the settings were not saved")
	}
	if _, err := apiRepoSave(M{"repo": keyless, "password_mode": "keep"}); err == nil {
		t.Fatal("saving the keyless sftp repository must fail")
	}
	if sshKeyExists("r00000002") || exists(sshPlainKey("r00000002")+".pub") {
		t.Fatal("a key made for a save that did not happen is removed again")
	}
	for _, r := range []M{fresh, sftp} {
		if _, err := apiRepoSave(M{"repo": r, "password_mode": "generate"}); err == nil {
			t.Fatalf("saving %v must fail", r["name"])
		}
	}
	for _, dir := range []string{SecretsDir, SSHDir} {
		left, _ := filepath.Glob(dir + "/*")
		for _, f := range left {
			if !strings.Contains(f, "r00000001") && !strings.Contains(f, "r00000002.password") && !strings.HasPrefix(filepath.Base(f), ".") {
				t.Fatalf("a repository that was never saved left %s", f)
			}
		}
	}
}
