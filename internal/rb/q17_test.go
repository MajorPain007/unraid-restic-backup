package rb

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestASourceFolderEndingInASpaceIsKept(t *testing.T) {
	dir := t.TempDir() + "/trail "
	os.MkdirAll(dir, 0o755)
	if got := sourcePath(dir); got != dir {
		t.Fatalf("the folder with the space must be kept as it is: %q", got)
	}
	if got := sourcePath(" " + dir + "x  "); got != strings.TrimSpace(dir+"x") {
		t.Fatalf("a path that does not exist with its spaces is trimmed as before: %q", got)
	}
	if got := sourcePath(dir + "/../etc "); got != "" {
		t.Fatalf("dot-dot stays refused: %q", got)
	}
}

func TestANewPasswordIsOnlyGeneratedForANewRepository(t *testing.T) {
	base := useTestDirs(t)
	repo := defaultRepo()
	repo.ID, repo.Name, repo.Type, repo.Local.Path = "r00000001", "a", "local", "/mnt/disks/backup/restic"
	cfg := &Config{Version: 1, Settings: defaultSettings(), Repos: []Repo{repo}, Jobs: []Job{}}
	cfg.Settings.DataDir = base + "/pool"
	saveMode(t, cfg, "plain")
	secretSet("r00000001", "password", "old-password")
	var full M
	json.Unmarshal(jsonEncode(cfg.Repos[0], false), &full)
	_, err := apiRepoSave(M{"repo": full, "password_mode": "generate"})
	if err == nil || !strings.Contains(err.Error(), "new repository") {
		t.Fatalf("generate must be refused for an existing repository, got %v", err)
	}
	if secretGet("r00000001", "password") != "old-password" {
		t.Fatal("the stored password was replaced")
	}
}
