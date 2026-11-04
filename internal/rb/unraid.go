package rb

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var notifyBin = envOr("RB_NOTIFY", "/usr/local/emhttp/webGui/scripts/notify")

func notifyArgs(subject, description, level, message string) []string {
	cmd := []string{notifyBin, "-e", "Restic Backup", "-s", subject, "-d", description, "-i", level}
	if message != "" {
		cmd = append(cmd, "-m", message)
	}
	return cmd
}

func notify(subject, description, level, message string) bool {
	if !fileExecutable(notifyBin) {
		return false
	}
	if level == "" {
		level = "normal"
	}
	quoted := make([]string, 0, 16)
	for _, a := range notifyArgs(subject, description, level, message) {
		quoted = append(quoted, shellQuote(a))
	}
	cmd := exec.Command("/bin/sh", "-c", "nohup setsid "+strings.Join(quoted, " ")+" >/dev/null 2>&1 &")
	cmd.Dir = "/"
	cmd.Run()
	return true
}

func healthcheckArgs(url, event, body string) []string {
	target := strings.TrimRight(url, "/")
	switch event {
	case "start":
		target += "/start"
	case "fail":
		target += "/fail"
	}
	cmd := []string{"curl", "-fsS", "-m", "10", "--retry", "2", "-o", "/dev/null"}
	if body != "" {
		cmd = append(cmd, "--data-binary", "@-")
	}
	return append(cmd, target)
}

func healthcheckPing(url, event, body string) {
	if url == "" {
		return
	}
	args := healthcheckArgs(url, event, body)
	if len(body) > 10000 {
		body = body[len(body)-10000:]
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(body)
	if cmd.Start() != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

func parityRunning() bool {
	ini := varIni()
	return phpInt(ini["mdResyncPos"]) > 0 && phpInt(ini["mdResync"]) > 0
}

func arrayStartedAt() int64 {
	b, err := os.ReadFile(RunDir + "/array_started")
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(leadingInt.FindString(string(b))), 10, 64)
	return n
}
