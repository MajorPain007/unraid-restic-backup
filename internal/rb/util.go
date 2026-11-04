package rb

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

var buildName = "restic.backup"

var (
	Name       = envOr("RB_NAME", buildName)
	ConfigDir  = envOr("RB_CONFIG_DIR", "/boot/config/plugins/"+Name)
	RunDir     = envOr("RB_RUN_DIR", "/tmp/"+Name)
	PluginDir  = envOr("RB_PLUGIN_DIR", "/usr/local/emhttp/plugins/"+Name)
	ResticBin  = envOr("RB_RESTIC", "/usr/local/lib/"+Name+"/restic")
	VarIni     = envOr("RB_VAR_INI", "/var/local/emhttp/var.ini")
	SelfBin    = envOr("RB_BIN", selfPath())
	ConfigFile = ConfigDir + "/config.json"
	SecretsDir = ConfigDir + "/secrets"
	SSHDir     = ConfigDir + "/ssh"
	OpsDir     = RunDir + "/ops"
	LockDir    = RunDir + "/locks"
)

const ConfigVersion = 1

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "/usr/local/emhttp/plugins/" + Name + "/bin/restic-backup"
	}
	return p
}

type M = map[string]any

func jsonRead(path string) (M, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var m M
	if json.Unmarshal(b, &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

func jsonReadInto(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func jsonEncode(v any, pretty bool) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "    ")
	}
	if err := enc.Encode(v); err != nil {
		return []byte("null\n")
	}
	return buf.Bytes()
}

func jsonWrite(path string, v any, mode os.FileMode) bool {
	return fileWrite(path, jsonEncode(v, true), mode)
}

func fileWrite(path string, data []byte, mode os.FileMode) bool {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return true
	}
	dir := filepath.Dir(path)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if os.MkdirAll(dir, 0o755) != nil {
			return false
		}
	}
	tmp := fmt.Sprintf("%s.tmp%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		os.Remove(tmp)
		return false
	}
	os.Chmod(tmp, mode)
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false
	}
	return true
}

func newID(prefix string) string {
	b := make([]byte, 4)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func varIni() map[string]string {
	return parseIni(VarIni)
}

func parseIni(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == ';' || line[0] == '#' || line[0] == '[' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

func useLocalTimezone() {
	tz := varIni()["timeZone"]
	if tz == "" {
		if target, err := os.Readlink("/etc/localtime"); err == nil {
			if i := strings.Index(target, "zoneinfo/"); i >= 0 {
				tz = target[i+9:]
			}
		}
	}
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			time.Local = loc
			return
		}
	}
}

func arrayStarted() bool {
	ini := varIni()
	if len(ini) == 0 {
		return os.Getenv("RB_ASSUME_ARRAY") != "0"
	}
	return ini["mdState"] == "STARTED" && ini["fsState"] == "Started" && ini["startMode"] != "Maintenance"
}

func serverName() string {
	if n := varIni()["NAME"]; n != "" {
		return n
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unraid"
}

func humanBytes(n float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	for i, u := range units {
		if n < 1024 || u == "PB" {
			if i == 0 {
				return strconv.Itoa(int(n)) + " " + u
			}
			if n < 10 {
				return strconv.FormatFloat(n, 'f', 1, 64) + " " + u
			}
			return strconv.FormatFloat(math.Round(n), 'f', 0, 64) + " " + u
		}
		n /= 1024
	}
	return ""
}

func humanDuration(seconds float64) string {
	s := int(math.Round(seconds))
	if s < 60 {
		return strconv.Itoa(s) + " s"
	}
	if s < 3600 {
		return strconv.Itoa(s/60) + " min"
	}
	h, m := s/3600, (s%3600)/60
	if m > 0 {
		return fmt.Sprintf("%d h %d min", h, m)
	}
	return fmt.Sprintf("%d h", h)
}

func dateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func logAppend(path, line string) {
	logAppendMax(path, line, 1048576)
}

func logAppendMax(path, line string, max int64) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	f.WriteString(dateTime(time.Now()) + " " + strings.TrimRight(line, "\n") + "\n")
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
	if st, err := os.Stat(path); err == nil && st.Size() > max {
		logTrim(path, max/2)
	}
}

func logTrim(path string, keep int64) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX) != nil {
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	st, err := f.Stat()
	if err != nil || st.Size() <= keep {
		return
	}
	buf := make([]byte, keep)
	n, _ := f.ReadAt(buf, st.Size()-keep)
	tail := buf[:n]
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	f.Truncate(0)
	f.WriteAt(tail, 0)
}

func tail(path string, lines int, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	start := st.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, st.Size()-start)
	n, _ := f.ReadAt(buf, start)
	data := string(buf[:n])
	if start > 0 {
		if i := strings.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			data = ""
		}
	}
	all := strings.Split(strings.TrimRight(data, "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

func mkdirLikeParent(dir string) bool {
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return true
	}
	parent := filepath.Dir(dir)
	if parent == dir || !mkdirLikeParent(parent) || os.Mkdir(dir, 0o755) != nil {
		return false
	}
	if st, err := os.Stat(parent); err == nil {
		os.Chmod(dir, st.Mode().Perm()|(st.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)))
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			os.Chown(dir, int(sys.Uid), int(sys.Gid))
		}
	}
	return true
}

var octalEscape = regexp.MustCompile(`\\([0-7]{3})`)

func unescapeMount(s string) string {
	return octalEscape.ReplaceAllStringFunc(s, func(m string) string {
		v, _ := strconv.ParseInt(m[1:], 8, 32)
		return string(rune(v))
	})
}

type mountEntry struct {
	Point  string
	FSType string
	Source string
	Opts   string
}

func mountInfo(pid string) []mountEntry {
	b, err := os.ReadFile("/proc/" + pid + "/mountinfo")
	if err != nil {
		return nil
	}
	var out []mountEntry
	for _, line := range strings.Split(string(b), "\n") {
		a, rest, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		f := strings.Fields(a)
		r := strings.Fields(rest)
		if len(f) < 6 || len(r) < 2 {
			continue
		}
		e := mountEntry{Point: unescapeMount(f[4]), FSType: r[0], Source: unescapeMount(r[1])}
		if len(r) > 2 {
			e.Opts = r[2]
		}
		out = append(out, e)
	}
	return out
}

func fsType(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	best, typ := "", ""
	for _, m := range mountInfo("self") {
		mp := m.Point
		if (mp == "/" || real == mp || strings.HasPrefix(real, strings.TrimRight(mp, "/")+"/")) && len(mp) >= len(best) {
			best, typ = mp, m.FSType
		}
	}
	return typ
}

func dirHasEntries(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) > 0
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func fileMtime(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.ModTime().Unix()
}

func pidAlive(v any) bool {
	pid := toInt(v)
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func under(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimRight(dir, "/")+"/")
}

var controlChars = regexp.MustCompile(`[\x00-\x1f\x7f]`)

func hasControl(s string) bool {
	return controlChars.MatchString(s)
}

func scalarString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		if x {
			return "1", true
		}
		return "", true
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10), true
		}
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case json.Number:
		return x.String(), true
	}
	return "", false
}

func text(v any, max int) string {
	s, ok := scalarString(v)
	if !ok {
		return ""
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}

func str(v any) string {
	s, _ := scalarString(v)
	return s
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x == 1
	case int:
		return x == 1
	case string:
		return x == "1" || x == "true" || x == "yes" || x == "on"
	}
	return false
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case bool:
		if x {
			return 1
		}
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err == nil {
			return int(f)
		}
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	case json.Number:
		f, _ := x.Float64()
		return int(f)
	}
	return 0
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return int64(f)
	}
	return 0
}

func isNumeric(v any) bool {
	switch x := v.(type) {
	case float64, int, int64:
		return true
	case string:
		_, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return err == nil && strings.TrimSpace(x) != ""
	}
	return false
}

func count(v any, max int) int {
	if !isNumeric(v) {
		return 0
	}
	n := toInt(v)
	if n <= 0 {
		return 0
	}
	if n > max {
		return max
	}
	return n
}

var lineSplit = regexp.MustCompile(`\r?\n`)

func list(v any, max int) []string {
	var items []any
	switch x := v.(type) {
	case string:
		for _, s := range lineSplit.Split(x, -1) {
			items = append(items, s)
		}
	case []any:
		items = x
	case []string:
		for _, s := range x {
			items = append(items, s)
		}
	default:
		return []string{}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, it := range items {
		s := text(it, 4096)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func absPath(v any) string {
	p := ""
	if s, ok := scalarString(v); ok {
		p = strings.TrimSpace(s)
	}
	if p == "" || p[0] != '/' || hasControl(p) {
		return ""
	}
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return ""
		}
		parts = append(parts, seg)
	}
	return "/" + strings.Join(parts, "/")
}

func mapOf(v any) M {
	if m, ok := v.(M); ok {
		return m
	}
	return M{}
}

func has(m M, k string) bool {
	_, ok := m[k]
	return ok
}

func get(m M, k string, def any) any {
	if v, ok := m[k]; ok && v != nil {
		return v
	}
	return def
}

func inList(s string, l ...string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func now() int64 {
	return time.Now().Unix()
}
