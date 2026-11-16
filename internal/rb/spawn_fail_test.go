package rb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTheStartButtonsSayWhenNothingStarted(t *testing.T) {
	cfg, _, _ := tickTestSetup(t)
	cfg.Settings.Secrets = "plain"
	configSave(cfg)
	secretSet("r00000001", "password", "repo-password-123")
	oldSelf, oldRestic := SelfBin, ResticBin
	t.Cleanup(func() { SelfBin, ResticBin = oldSelf, oldRestic })
	SelfBin, ResticBin = t.TempDir()+"/missing", "/bin/sh"
	var job M
	json.Unmarshal(jsonEncode(cfg.Jobs[0], false), &job)
	job["sources"] = []string{"/mnt/user/docs"}

	for name, call := range map[string]func() (M, error){
		"backup":  func() (M, error) { return apiJobRun(M{"id": "j00000001"}) },
		"forget":  func() (M, error) { return apiJobForget(M{"id": "j00000001"}) },
		"check":   func() (M, error) { return apiRepoRun(M{"id": "r00000001", "kind": "check"}) },
		"preview": func() (M, error) { return apiJobPreview(M{"job": job}) },
	} {
		if r, err := call(); err == nil || !strings.Contains(err.Error(), "could not be started") {
			t.Errorf("%s: a start that failed must say so, got %v %v", name, r, err)
		}
	}
	if s := browseStart(cfg.Repos[0]); s.Status != "error" || !strings.Contains(s.Error, "could not be started") {
		t.Fatalf("the browser reports it, got %+v", s)
	}
}
