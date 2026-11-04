package rb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func repoURL(r Repo) string {
	switch r.Type {
	case "local":
		return r.Local.Path
	case "rest":
		return "rest:" + r.Rest.URL
	}
	host := r.SFTP.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return "sftp:" + r.SFTP.User + "@" + host + ":" + r.SFTP.Path
}

func sshArgs(r Repo) []string {
	return []string{
		"-i", sshKeyPath(r.ID),
		"-p", strconv.Itoa(r.SFTP.Port),
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + knownHostsFile(),
		"-o", "ConnectTimeout=20",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=6",
	}
}

func resticEnv(r Repo, s Settings) map[string]string {
	env := map[string]string{
		"PATH":                 "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME":                 "/root",
		"LANG":                 "C.UTF-8",
		"RESTIC_REPOSITORY":    repoURL(r),
		"RESTIC_PASSWORD_FILE": secretPasswordFile(r.ID),
		"RESTIC_CACHE_DIR":     s.DataDir + "/cache",
		"TMPDIR":               RunDir + "/tmp",
		"RESTIC_PROGRESS_FPS":  "1",
	}
	if r.Type == "rest" {
		if r.Rest.User != "" {
			env["RESTIC_REST_USERNAME"] = r.Rest.User
			env["RESTIC_REST_PASSWORD"] = secretGet(r.ID, "rest")
		}
		if r.Rest.CACert != "" {
			env["RESTIC_CACERT"] = r.Rest.CACert
		}
	}
	return env
}

func resticGlobalArgs(r Repo) []string {
	args := []string{}
	if r.Type == "sftp" {
		args = append(args, "-o", "sftp.args="+strings.Join(sshArgs(r), " "))
	}
	o := r.Options
	if o.Connections > 0 {
		args = append(args, "-o", r.Type+".connections="+strconv.Itoa(o.Connections))
	}
	if o.LimitUpload > 0 {
		args = append(args, "--limit-upload", strconv.Itoa(o.LimitUpload))
	}
	if o.LimitDownload > 0 {
		args = append(args, "--limit-download", strconv.Itoa(o.LimitDownload))
	}
	if o.Compression != "auto" {
		args = append(args, "--compression", o.Compression)
	}
	args = append(args, o.ExtraFlags...)
	return args
}

func resticCmd(r Repo, s Settings, args []string, lowPriority bool) []string {
	cmd := []string{}
	if lowPriority {
		if s.Nice > 0 {
			cmd = append(cmd, "nice", "-n", strconv.Itoa(s.Nice))
		}
		switch s.IOPriority {
		case "idle":
			cmd = append(cmd, "ionice", "-c", "3")
		case "low":
			cmd = append(cmd, "ionice", "-c", "2", "-n", "7")
		}
	}
	cmd = append(cmd, ResticBin)
	cmd = append(cmd, resticGlobalArgs(r)...)
	return append(cmd, args...)
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func procStream(cmdline []string, env map[string]string, onLine func(stream, line string), tick func() bool,
	onStart func(pid int), stdin *string) int {
	if len(cmdline) == 0 {
		return -1
	}
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Dir = "/"
	if env != nil {
		cmd.Env = envList(env)
	}
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		return -1
	}
	if err := cmd.Start(); err != nil {
		return -1
	}
	if onStart != nil {
		onStart(cmd.Process.Pid)
	}
	type lineMsg struct{ stream, line string }
	lines := make(chan lineMsg, 256)
	var wg sync.WaitGroup
	read := func(r io.Reader, name string) {
		defer wg.Done()
		br := bufio.NewReaderSize(r, 65536)
		for {
			s, err := br.ReadString('\n')
			if s != "" {
				lines <- lineMsg{name, strings.TrimRight(strings.TrimSuffix(s, "\n"), "\r")}
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go read(stdout, "out")
	go read(stderr, "err")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	interrupted := false
	for {
		select {
		case m := <-lines:
			onLine(m.stream, m.line)
		case <-ticker.C:
			if tick != nil && !interrupted && tick() {
				interrupted = true
				cmd.Process.Signal(syscall.SIGINT)
			}
		case <-done:
		drain:
			for {
				select {
				case m := <-lines:
					onLine(m.stream, m.line)
				default:
					break drain
				}
			}
			err := cmd.Wait()
			if err == nil {
				return 0
			}
			if ee, ok := err.(*exec.ExitError); ok {
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					return 128 + int(ws.Signal())
				}
				return ee.ExitCode()
			}
			return -1
		}
	}
}

func resticCapture(r Repo, s Settings, args []string, limitLines int) (int, []string, []string) {
	out, errs := []string{}, []string{}
	code := procStream(resticCmd(r, s, args, false), resticEnv(r, s), func(stream, line string) {
		if stream == "out" {
			if len(out) < limitLines {
				out = append(out, line)
			}
		} else if len(errs) < 200 {
			errs = append(errs, line)
		}
	}, nil, nil, nil)
	return code, out, errs
}

var reLetters = regexp.MustCompile(`[A-Za-z]{3}`)

func resticError(errLines []string, code int) string {
	var plain []string
	fatal := ""
	for _, line := range errLines {
		var j M
		if json.Unmarshal([]byte(line), &j) == nil && j != nil {
			switch str(j["message_type"]) {
			case "exit_error":
				fatal = str(j["message"])
				continue
			case "error":
				plain = append(plain, str(mapOf(j["error"])["message"]))
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			plain = append(plain, strings.TrimSpace(line))
		}
	}
	var ssh []string
	for _, line := range plain {
		if strings.HasPrefix(line, "subprocess ssh:") {
			ssh = append(ssh, strings.TrimSpace(line[15:]))
		}
	}
	for _, line := range ssh {
		if e := explainSSH(line); !strings.HasPrefix(e, "SSH: ") {
			return e
		}
	}
	for _, line := range ssh {
		if reLetters.MatchString(line) {
			return explainSSH(line)
		}
	}
	if fatal != "" {
		return explainRestic(fatal, code)
	}
	if len(plain) > 0 {
		return plain[len(plain)-1]
	}
	if code == -1 {
		return "restic could not be started"
	}
	return fmt.Sprintf("restic exited with code %d", code)
}

func explainRestic(msg string, code int) string {
	first := strings.ReplaceAll(msg, "Fatal: ", "")
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	switch code {
	case 10:
		return "There is no repository at this location. Initialize it first, or check the path."
	case 11:
		return "The repository is locked by another operation. " + first
	case 12:
		return "Wrong password for this repository."
	case 130:
		return "Stopped."
	}
	return first
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func explainSSH(msg string) string {
	switch {
	case containsFold(msg, "IDENTIFICATION HAS CHANGED"):
		return "The server's host key has changed since it was confirmed. That happens when the server is reinstalled - or when someone is in between. Check, then confirm the new key with \"Test connection\"."
	case containsFold(msg, "Host key verification failed"):
		return "The server's host key is not confirmed yet, or it has changed. Use \"Test connection\" to check and confirm it."
	case containsFold(msg, "Permission denied"):
		return "The server refused the key (" + msg + "). Add the public key shown in the repository settings to the server."
	case containsFold(msg, "Connection refused"), containsFold(msg, "timed out"), containsFold(msg, "No route to host"),
		containsFold(msg, "Could not resolve"):
		return "Cannot reach the server: " + msg
	}
	return "SSH: " + msg
}

func repoEndpoint(r Repo) (string, int, bool) {
	switch r.Type {
	case "sftp":
		return r.SFTP.Host, r.SFTP.Port, true
	case "rest":
		u, err := url.Parse(r.Rest.URL)
		if err != nil {
			return "", 0, true
		}
		port := 80
		if u.Scheme == "https" {
			port = 443
		}
		if p, err := strconv.Atoi(u.Port()); err == nil {
			port = p
		}
		return u.Hostname(), port, true
	}
	return "", 0, false
}

func repoReachable(r Repo, attempts, wait int, cancelled func() bool, log func(string)) string {
	host, port, ok := repoEndpoint(r)
	if !ok {
		return ""
	}
	h := strings.Trim(host, "[]")
	why := ""
	for i := 1; i <= attempts; i++ {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(h, strconv.Itoa(port)), 10*time.Second)
		if err == nil {
			conn.Close()
			return ""
		}
		why = fmt.Sprintf("Cannot reach %s on port %d: %s", host, port, dialReason(err))
		if log != nil {
			msg := fmt.Sprintf("Attempt %d of %d: %s", i, attempts, why)
			if i < attempts {
				msg += fmt.Sprintf(" - trying again in %ds", wait)
			}
			log(msg)
		}
		for s := 0; i < attempts && s < wait; s++ {
			if cancelled != nil && cancelled() {
				return why
			}
			time.Sleep(time.Second)
		}
	}
	return why + "."
}

func dialReason(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "Connection refused"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "timeout"):
		return "Connection timed out"
	case strings.Contains(msg, "no such host"):
		return "Name or service not known"
	case strings.Contains(msg, "no route to host"):
		return "No route to host"
	case strings.Contains(msg, "network is unreachable"):
		return "Network is unreachable"
	}
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		return msg[i+2:]
	}
	return msg
}

func jsonLines(lines []string) []M {
	out := []M{}
	for _, l := range lines {
		var j M
		if json.Unmarshal([]byte(l), &j) == nil && j != nil {
			out = append(out, j)
		}
	}
	return out
}

func rbCmd(cmdline []string, timeout time.Duration) (int, []string, string) {
	if len(cmdline) == 0 {
		return -1, nil, "no command"
	}
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "LANG=C"}
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return -1, nil, "cannot run " + cmdline[0]
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	code := 0
	select {
	case err := <-done:
		if err != nil {
			code = -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
	case <-time.After(timeout):
		cmd.Process.Kill()
		<-done
		code = -2
		errb.WriteString(fmt.Sprintf("\ntimed out after %ds", int(timeout.Seconds())))
	}
	o := strings.TrimRight(out.String(), "\n")
	var lines []string
	if o != "" {
		lines = strings.Split(o, "\n")
	}
	return code, lines, strings.TrimSpace(errb.String())
}

func fileExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}
