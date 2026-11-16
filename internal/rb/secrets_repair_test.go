package rb

import (
	"os"
	"strings"
	"testing"
)

func halfDoneSwitch(t *testing.T) (*Config, func(bool)) {
	base := useTestDirs(t)
	t.Setenv("RB_ASSUME_ENCRYPTED", "1")
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{{ID: "r00000001", Name: "a", Type: "local"}}, Jobs: []Job{}}
	cfg.Settings.DataDir = base + "/pool"
	saveMode(t, cfg, "plain")
	secretSet("r00000001", "password", "repo-password-123")
	privateDir(SSHDir)
	sshKeyStore("r00000001", []byte("PRIVATE KEY\n"))
	then, err := secretsSwitch(cfg, "key", "")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, then
}

func TestASwitchCutOffAfterSavingIsFinished(t *testing.T) {
	cfg, _ := halfDoneSwitch(t)
	cfg.Settings.Secrets = "key"
	if !configSave(cfg) {
		t.Fatal("save")
	}
	secretsRepair()
	if len(secretsHalfDone()) != 0 {
		t.Fatalf("nothing half done stays: %v", secretsHalfDone())
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("the password reads in the new mode")
	}
	if exists(plainSecretFile("r00000001", "password")) || exists(sshPlainKey("r00000001")) {
		t.Fatal("the plain copies are gone once the change is finished")
	}
}

func TestASwitchCutOffBeforeSavingIsUndone(t *testing.T) {
	_, _ = halfDoneSwitch(t)
	secretsRepair()
	if len(secretsHalfDone()) != 0 {
		t.Fatalf("the staged files are removed: %v", secretsHalfDone())
	}
	if secretGet("r00000001", "password") != "repo-password-123" || !exists(sshPlainKey("r00000001")) {
		t.Fatal("the old mode keeps working")
	}
	if exists(encSecretFile("r00000001", "password")) {
		t.Fatal("no encrypted copy appears")
	}
}

func TestAFailedRenameKeepsTheOldFiles(t *testing.T) {
	cfg, then := halfDoneSwitch(t)
	cfg.Settings.Secrets = "key"
	configSave(cfg)
	blocker := encSecretFile("r00000001", "password")
	os.MkdirAll(blocker+"/x", 0o700)
	then(true)
	if !exists(plainSecretFile("r00000001", "password")) {
		t.Fatal("a rename that failed must not cost the old password")
	}
}

func TestRemovingARepositoryRemovesHalfDoneFiles(t *testing.T) {
	_, _ = halfDoneSwitch(t)
	secretRemove("r00000001")
	for _, f := range secretsHalfDone() {
		if strings.Contains(f, "r00000001") {
			t.Fatalf("left: %s", f)
		}
	}
}

func keyToMaster(t *testing.T) (*Config, func(bool)) {
	cfg, then := halfDoneSwitch(t)
	cfg.Settings.Secrets = "key"
	configSave(cfg)
	then(true)
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("setup: key mode reads")
	}
	then, err := secretsSwitch(cfg, "master", "a-long-master-password")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, then
}

func TestKeyToMasterCutOffBeforeSavingKeepsKeyMode(t *testing.T) {
	_, _ = keyToMaster(t)
	if msg := secretsRepair(); msg != "" {
		t.Fatal(msg)
	}
	if len(secretsHalfDone()) != 0 || exists(switchFile()) || exists(masterFile()) {
		t.Fatalf("the staged master change is dropped: %v", secretsHalfDone())
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("key mode keeps reading its passwords")
	}
}

func TestKeyToMasterCutOffAfterSavingIsFinished(t *testing.T) {
	cfg, _ := keyToMaster(t)
	cfg.Settings.Secrets = "master"
	configSave(cfg)
	if msg := secretsRepair(); msg != "" {
		t.Fatal(msg)
	}
	if exists(keyFile(cfg.Settings)) || exists(switchFile()) {
		t.Fatal("the old key and the marker are gone")
	}
	if err := masterUnlock("a-long-master-password"); err != nil {
		t.Fatal(err)
	}
	if secretGet("r00000001", "password") != "repo-password-123" {
		t.Fatal("master mode reads the passwords")
	}
}

func TestAnIncompleteStagingIsDropped(t *testing.T) {
	cfg, _ := halfDoneSwitch(t)
	os.Remove(switchFile())
	cfg.Settings.Secrets = "key"
	configSave(cfg)
	secretsRepair()
	if len(secretsHalfDone()) != 0 || !exists(plainSecretFile("r00000001", "password")) {
		t.Fatal("without the marker the staged files are dropped and the old ones stay")
	}
}

func TestARepairThatCannotRenameIsReported(t *testing.T) {
	cfg, _ := halfDoneSwitch(t)
	cfg.Settings.Secrets = "key"
	configSave(cfg)
	os.MkdirAll(encSecretFile("r00000001", "password")+"/x", 0o700)
	if msg := secretsRepair(); !strings.Contains(msg, "could not be finished") {
		t.Fatalf("a stuck change is reported, got %q", msg)
	}
	if !exists(plainSecretFile("r00000001", "password")) || !exists(switchFile()) {
		t.Fatal("nothing old is removed while the change is stuck")
	}
}
