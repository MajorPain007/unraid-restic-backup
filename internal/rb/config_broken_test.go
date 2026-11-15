package rb

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBrokenConfigIsNeverReadAsEmptyOrOverwritten(t *testing.T) {
	for _, kind := range []string{"truncated", "empty"} {
		t.Run(kind, func(t *testing.T) {
			_, notes, spawned := tickTestSetup(t)
			good, _ := os.ReadFile(ConfigFile)
			broken := good[:len(good)/2]
			if kind == "empty" {
				broken = []byte{}
			}
			os.WriteFile(ConfigFile, broken, 0o644)

			if _, err := configChange(func(cfg *Config) (M, error) { return M{}, nil }); err == nil || !strings.Contains(err.Error(), "cannot be read") {
				t.Fatalf("saving over a broken config.json must fail with the reason, got %v", err)
			}
			if got, _ := os.ReadFile(ConfigFile); !bytes.Equal(got, broken) {
				t.Fatal("config.json was overwritten")
			}

			cmdTick(nil)
			cmdTick(nil)
			waitNotes(notes, 1)
			time.Sleep(200 * time.Millisecond)
			lines := waitNotes(notes, 1)
			if len(lines) != 1 || !strings.Contains(lines[0], "cannot read its settings") || !strings.Contains(lines[0], "-i alert") {
				t.Fatalf("expected one alert notification, got %q", lines)
			}
			if len(*spawned) != 0 {
				t.Fatalf("nothing may start, got %v", *spawned)
			}

			ov, err := apiOverview(M{})
			if err != nil || !strings.Contains(str(mapOf(ov["system"])["config_problem"]), "cannot be read") {
				t.Fatalf("the overview must name the problem, got %v %v", err, mapOf(ov["system"])["config_problem"])
			}

			os.WriteFile(ConfigFile, good, 0o644)
			cmdTick(nil)
			ov, _ = apiOverview(M{})
			if str(mapOf(ov["system"])["config_problem"]) != "" {
				t.Fatal("a readable config.json clears the problem")
			}
			os.WriteFile(ConfigFile, broken, 0o644)
			cmdTick(nil)
			if n := waitNotes(notes, 2); len(n) != 2 {
				t.Fatalf("breaking it again notifies again, got %q", n)
			}
		})
	}
}

func TestMissingConfigIsNoProblem(t *testing.T) {
	useTestDirs(t)
	if p := configProblem(); p != "" {
		t.Fatalf("a missing config.json is a fresh install, got %q", p)
	}
}
