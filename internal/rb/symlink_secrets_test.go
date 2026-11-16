package rb

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestShredNeverWritesThroughALink(t *testing.T) {
	base := useTestDirs(t)
	victim := base + "/victim"
	os.WriteFile(victim, []byte("keep"), 0o644)
	os.Symlink(victim, base+"/link")
	shred(base + "/link")
	if b, _ := os.ReadFile(victim); string(b) != "keep" {
		t.Fatalf("the link's target was overwritten: %q", b)
	}
	if _, err := os.Lstat(base + "/link"); err == nil {
		t.Fatal("the link itself is removed")
	}
	syscall.Mkfifo(base+"/fifo", 0o600)
	done := make(chan struct{})
	go func() { shred(base + "/fifo"); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a pipe in place of a key must not hang the shredder")
	}
	os.WriteFile(base+"/real", []byte("secret"), 0o600)
	shred(base + "/real")
	if exists(base + "/real") {
		t.Fatal("a real file is removed")
	}
}

func TestAKeysFolderThatIsALinkIsRefused(t *testing.T) {
	base := useTestDirs(t)
	t.Setenv("RB_ASSUME_ENCRYPTED", "1")
	outside := base + "/outside"
	os.MkdirAll(outside, 0o755)
	cfg := &Config{Version: 1, Settings: defaultSettings()}
	cfg.Settings.DataDir = base + "/data"
	os.MkdirAll(cfg.Settings.DataDir, 0o755)
	os.Symlink(outside, cfg.Settings.DataDir+"/.keys")
	if why := prepareDataDir(cfg.Settings); !strings.Contains(why, ".keys is a symbolic link") {
		t.Fatalf("the data folder is refused, got %q", why)
	}
	if _, err := secretsSwitch(cfg, "key", ""); err == nil {
		t.Fatal("switching to key mode must fail")
	}
	if exists(outside+"/secrets.key") || exists(outside+"/secrets.key.new") {
		t.Fatal("no key may land behind the link")
	}
}

func TestPrivateDirDoesNotWriteThroughALink(t *testing.T) {
	base := useTestDirs(t)
	dir := base + "/private"
	os.MkdirAll(dir, 0o700)
	os.Symlink(base+"/elsewhere", dir+"/.gitignore")
	privateDir(dir)
	if exists(base + "/elsewhere") {
		t.Fatal(".gitignore was written through a link")
	}
	privateDir(base + "/fresh")
	if b, _ := os.ReadFile(base + "/fresh/.gitignore"); string(b) != "*\n" {
		t.Fatalf("a new private folder gets its .gitignore, got %q", b)
	}
}
