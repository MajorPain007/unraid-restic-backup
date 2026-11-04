package rb

import (
	"os"
	"strings"
	"testing"
)

func useTestDirs(t *testing.T) string {
	base := t.TempDir()
	ConfigDir = base + "/flash"
	RunDir = base + "/ram"
	ConfigFile = ConfigDir + "/config.json"
	SecretsDir = ConfigDir + "/secrets"
	SSHDir = ConfigDir + "/ssh"
	OpsDir = RunDir + "/ops"
	LockDir = RunDir + "/locks"
	os.MkdirAll(SSHDir, 0o700)
	old := Mnt
	Mnt = base
	t.Cleanup(func() { Mnt = old })
	t.Setenv("RB_ASSUME_ENCRYPTED", "1")
	return base
}

func saveMode(t *testing.T, cfg *Config, mode string) {
	cfg.Settings.Secrets = mode
	if !configSave(cfg) {
		t.Fatal("config not saved")
	}
}

func flashHolds(t *testing.T, needle string) bool {
	found := false
	for _, dir := range []string{SecretsDir, SSHDir} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			b, _ := os.ReadFile(dir + "/" + e.Name())
			if strings.Contains(string(b), needle) {
				found = true
			}
		}
	}
	return found
}

func TestSecretsSwitchModes(t *testing.T) {
	base := useTestDirs(t)
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{{ID: "r00000001", Name: "a", Type: "local"}}, Jobs: []Job{}}
	cfg.Settings.DataDir = base + "/pool"
	saveMode(t, cfg, "plain")
	secretSet("r00000001", "password", "repo-password-123")
	sshKeyStore("r00000001", []byte("PRIVATE KEY 456\n"))
	if !flashHolds(t, "repo-password-123") {
		t.Fatal("plain mode should keep the password readable")
	}

	if err := secretsSwitch(cfg, "key", ""); err != nil {
		t.Fatal(err)
	}
	saveMode(t, cfg, "key")
	if flashHolds(t, "repo-password-123") || flashHolds(t, "PRIVATE KEY 456") {
		t.Fatal("key mode left plain text on the flash drive")
	}
	if !isFile(base + "/pool/.keys/secrets.key") {
		t.Fatal("key file missing")
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("password not readable in key mode")
	}
	b, _ := os.ReadFile(secretPasswordFile("r00000001"))
	if strings.TrimSpace(string(b)) != "repo-password-123" || !strings.HasPrefix(secretPasswordFile("r00000001"), RunDir) {
		t.Fatal("restic password file must come from RAM")
	}
	k, _ := os.ReadFile(sshKeyPath("r00000001"))
	if string(k) != "PRIVATE KEY 456\n" {
		t.Fatal("ssh key not readable in key mode")
	}

	os.Rename(base+"/pool", base+"/pool-gone")
	wipeRunSecrets()
	if secretsReady(cfg.Settings) == "" {
		t.Fatal("a missing key must be reported")
	}
	os.Rename(base+"/pool-gone", base+"/pool")

	if err := secretsSwitch(cfg, "master", "short"); err == nil {
		t.Fatal("a short master password must be refused")
	}
	if err := secretsSwitch(cfg, "master", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	saveMode(t, cfg, "master")
	if isFile(base + "/pool/.keys/secrets.key") {
		t.Fatal("the old key should be gone")
	}
	if flashHolds(t, "repo-password-123") {
		t.Fatal("master mode left plain text on the flash drive")
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("unlocked right after setting the master password")
	}
	masterLock()
	if !secretsLocked() || secretGet("r00000001", "password") != "" {
		t.Fatal("locking must make the passwords unreadable")
	}
	if err := masterUnlock("wrong password!"); err == nil {
		t.Fatal("a wrong master password must fail")
	}
	if err := masterUnlock("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("unlock did not work")
	}

	if err := secretsSwitch(cfg, "plain", ""); err != nil {
		t.Fatal(err)
	}
	saveMode(t, cfg, "plain")
	if !flashHolds(t, "repo-password-123") || isFile(masterFile()) {
		t.Fatal("back to plain text: password readable, master record gone")
	}
	k, _ = os.ReadFile(sshKeyPath("r00000001"))
	if string(k) != "PRIVATE KEY 456\n" || !strings.HasPrefix(sshKeyPath("r00000001"), SSHDir) {
		t.Fatal("ssh key back in plain text on the flash drive")
	}
}
