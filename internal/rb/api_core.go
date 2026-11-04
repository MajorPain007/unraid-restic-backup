package rb

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type apiError struct {
	msg    string
	errors []string
}

func (e *apiError) Error() string { return e.msg }

func apiFail(msg string, errs ...string) error {
	if errs == nil {
		errs = []string{}
	}
	return &apiError{msg: msg, errors: errs}
}

func apiFailf(format string, a ...any) error {
	return apiFail(fmt.Sprintf(format, a...))
}

type apiAction func(p M) (M, error)

var apiActions = map[string]apiAction{}

func registerAction(name string, fn apiAction) {
	apiActions[name] = fn
}

var reAction = regexp.MustCompile(`^[a-z_]{2,40}$`)

func apiDispatch(action string, p M) (out M) {
	fn, ok := apiActions[action]
	if !reAction.MatchString(action) || !ok {
		return M{"ok": false, "error": fmt.Sprintf("Unknown action \"%s\"", action)}
	}
	defer func() {
		if r := recover(); r != nil {
			out = M{"ok": false, "error": fmt.Sprint(r)}
		}
	}()
	result, err := fn(p)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			return M{"ok": false, "error": ae.msg, "errors": ae.errors}
		}
		return M{"ok": false, "error": err.Error()}
	}
	res := M{"ok": true}
	for k, v := range result {
		if k != "ok" {
			res[k] = v
		}
	}
	return res
}

func readParams(r io.Reader) M {
	b, _ := io.ReadAll(r)
	var p M
	if json.Unmarshal(b, &p) != nil || p == nil {
		p = M{}
	}
	return p
}

func init() {
	register("api", "answer one action of the web interface: api <action>, parameters as JSON on stdin", func(args []string) int {
		action := ""
		if len(args) > 0 {
			action = args[0]
		}
		p := M{}
		if len(args) < 2 {
			p = readParams(os.Stdin)
		} else {
			var q M
			if json.Unmarshal([]byte(args[1]), &q) == nil && q != nil {
				for k, v := range q {
					switch v.(type) {
					case M, []any:
						v = string(jsonEncode(v, false))
						v = strings.TrimRight(v.(string), "\n")
					}
					p[k] = v
				}
			}
		}
		out := apiDispatch(action, p)
		if len(args) > 1 {
			os.Stdout.Write(jsonEncode(out, true))
		} else {
			os.Stdout.Write(jsonEncode(out, false))
		}
		return 0
	})
	register("test-cfg", "test helper: change the configuration like the web interface", testCfg)
}

func param(p M, key string, def any) any {
	if v, ok := p[key]; ok {
		return v
	}
	return def
}

func pstr(p M, key string, def string) string {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	return str(v)
}

func pjson(p M, key string) (any, error) {
	v, ok := p[key]
	if !ok || v == nil {
		return nil, apiFailf("Missing or malformed \"%s\"", key)
	}
	switch x := v.(type) {
	case M, []any:
		return x, nil
	case string:
		var d any
		if json.Unmarshal([]byte(x), &d) == nil {
			switch d.(type) {
			case M, []any:
				return d, nil
			}
		}
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}

func pjsonMap(p M, key string) (M, error) {
	v, err := pjson(p, key)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(M); ok {
		return m, nil
	}
	if l, ok := v.([]any); ok && len(l) == 0 {
		return M{}, nil
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}

func pjsonList(p M, key string) ([]any, error) {
	v, err := pjson(p, key)
	if err != nil {
		return nil, err
	}
	if l, ok := v.([]any); ok {
		return l, nil
	}
	if m, ok := v.(M); ok {
		out := []any{}
		for _, x := range m {
			out = append(out, x)
		}
		return out, nil
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}

func needRepo(cfg *Config, id any) (*Repo, error) {
	r := cfg.repo(str(id))
	if r == nil {
		return nil, apiFail("That repository does not exist (any more).")
	}
	return r, nil
}

func needJob(cfg *Config, id any) (*Job, error) {
	j := cfg.job(str(id))
	if j == nil {
		return nil, apiFail("That job does not exist (any more).")
	}
	return j, nil
}

var reSnapID = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

func needSnapshotID(v any) (string, error) {
	id := str(v)
	if !reSnapID.MatchString(id) {
		return "", apiFail("Not a snapshot ID")
	}
	return id, nil
}

func needSnapshotPath(v any) (string, error) {
	path := str(v)
	if path == "" || path[0] != '/' || strings.ContainsAny(path, "\x00\n\r") {
		return "", apiFail("Not a path inside the snapshot")
	}
	if path == "/" {
		return "/", nil
	}
	return strings.TrimRight(path, "/"), nil
}

func resticReady(cfg *Config, r *Repo) error {
	if !fileExecutable(ResticBin) {
		return apiFail("restic is not installed (" + ResticBin + "). Reinstall the plugin.")
	}
	if why := secretsReady(cfg.Settings); why != "" {
		return apiFail(why)
	}
	if !secretHas(r.ID, "password") {
		return apiFail("The repository has no password set.")
	}
	if prepareDataDir(cfg.Settings) != "" {
		return apiFail("The data folder " + cfg.Settings.DataDir + " cannot be created. Is the array started?")
	}
	return nil
}

func apiRestic(cfg *Config, r *Repo, args []string, limitLines int) ([]string, error) {
	if err := resticReady(cfg, r); err != nil {
		return nil, err
	}
	if why := repoReachable(*r, 1, 0, nil, nil); why != "" {
		return nil, apiFail(why)
	}
	code, out, errs := resticCapture(*r, cfg.Settings, args, limitLines)
	if code != 0 {
		return nil, apiFail(resticError(errs, code))
	}
	return out, nil
}

func configChange(fn func(cfg *Config) (M, error)) (M, error) {
	l := configLock()
	defer l.Unlock()
	cfg := configLoad()
	result, err := fn(cfg)
	if err != nil {
		return nil, err
	}
	if !configSave(cfg) {
		return nil, apiFail("The settings could not be saved to the flash drive.")
	}
	return result, nil
}

func testCfg(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "unknown command")
		return 2
	}
	cfg := configLoad()
	var errs []string
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	decode := func(s string) M {
		var m M
		json.Unmarshal([]byte(s), &m)
		if m == nil {
			m = M{}
		}
		return m
	}
	switch args[0] {
	case "settings":
		cur := cfg.Settings.Secrets
		cfg.Settings = cleanSettings(decode(arg(1)), &errs)
		if !has(decode(arg(1)), "secrets") {
			cfg.Settings.Secrets = cur
		}
	case "repo":
		r := cleanRepo(decode(arg(1)), &errs)
		if i := cfg.repoIndex(r.ID); i >= 0 {
			cfg.Repos[i] = r
		} else {
			cfg.Repos = append(cfg.Repos, r)
		}
	case "job":
		j := cleanJob(decode(arg(1)), cfg.repoIDs(), &errs)
		errs = append(errs, checkJobAgainstRepos(j, cfg)...)
		if i := cfg.jobIndex(j.ID); i >= 0 {
			cfg.Jobs[i] = j
		} else {
			cfg.Jobs = append(cfg.Jobs, j)
		}
	case "secret":
		secretSet(arg(1), arg(2), arg(3))
		return 0
	case "get":
		os.Stdout.Write(jsonEncode(cfg, true))
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown command")
		return 2
	}
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(errs, "\n"))
		return 1
	}
	configSave(cfg)
	return 0
}

func apiLock(r *Repo) (*fileLock, error) {
	l := repoLock(r.ID)
	if l == nil {
		return nil, apiFail("An operation on this repository is running. Try again when it has finished.")
	}
	browseStop(r.ID, 0)
	return l, nil
}
