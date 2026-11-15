package rb

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcStreamReturnsWhenTheCommandEndsEvenIfAChildKeepsItsOutput(t *testing.T) {
	pidFile := t.TempDir() + "/child"
	start := time.Now()
	lines := []string{}
	code := procStream([]string{"/bin/bash", "-c", "sleep 300 & echo $! > " + pidFile + "; echo done"}, nil,
		func(stream, line string) { lines = append(lines, line) }, nil, nil, nil)
	took := time.Since(start)
	if b, err := os.ReadFile(pidFile); err == nil {
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	if code != 0 || took > 10*time.Second || strings.Join(lines, ",") != "done" {
		t.Fatalf("code %d after %s, lines %v", code, took, lines)
	}
}

func TestProcStreamStopKillsAProcessThatIgnoresSIGINT(t *testing.T) {
	old := procKillGrace
	procKillGrace = time.Second
	t.Cleanup(func() { procKillGrace = old })
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	start := time.Now()
	procStream([]string{"/bin/bash", "-c", "trap '' INT; sleep 300 & wait"}, nil, func(string, string) {},
		func() bool { return time.Since(start) > 300*time.Millisecond }, nil, nil)
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("Stop did not end the command and its children within the grace period: %s", took)
	}
}

func TestAHookThatRunsTooLongIsStopped(t *testing.T) {
	old := hookTimeout
	hookTimeout = 500 * time.Millisecond
	t.Cleanup(func() { hookTimeout = old })
	dir := t.TempDir()
	r := &runner{op: M{"id": "x"}, logFile: dir + "/run.log"}
	start := time.Now()
	code := r.runHook("before-backup", "sleep 300", nil)
	b, _ := os.ReadFile(r.logFile)
	if time.Since(start) > 10*time.Second || code == 0 || !strings.Contains(string(b), "was stopped after") {
		t.Fatalf("code %d after %s, log %q", code, time.Since(start), b)
	}
}

func TestProcStreamKeepsWhatAChildWritesJustAfterTheCommandEnded(t *testing.T) {
	lines := []string{}
	code := procStream([]string{"/bin/bash", "-c", "(sleep 0.5; echo 'subprocess ssh: Host key verification failed.' >&2) & exit 1"}, nil,
		func(stream, line string) { lines = append(lines, stream+":"+line) }, nil, nil, nil)
	if code != 1 || strings.Join(lines, ",") != "err:subprocess ssh: Host key verification failed." {
		t.Fatalf("code %d, lines %v", code, lines)
	}
}
