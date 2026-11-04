package rb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func init() {
	registerAction("repo_save", apiRepoSave)
	registerAction("repo_delete", apiRepoDelete)
	registerAction("repo_password", apiRepoPassword)
	registerAction("repo_keygen", apiRepoKeygen)
	registerAction("repo_hostkey", apiRepoHostkey)
	registerAction("repo_hostkey_trust", apiRepoHostkeyTrust)
	registerAction("repo_test", apiRepoTest)
	registerAction("repo_init", apiRepoInit)
	registerAction("repo_unlock", apiRepoUnlock)
	registerAction("repo_stats", apiRepoStats)
	registerAction("size_history", apiSizeHistory)
	registerAction("repo_keys", apiRepoKeys)
	registerAction("repo_run", apiRepoRun)
}

func apiRepoSave(p M) (M, error) {
	in, err := pjsonMap(p, "repo")
	if err != nil {
		return nil, err
	}
	passwordMode := pstr(p, "password_mode", "keep")
	password := pstr(p, "password", "")
	restPassword := pstr(p, "rest_password", "")
	restPasswordMode := pstr(p, "rest_password_mode", "keep")
	return configChange(func(cfg *Config) (M, error) {
		errs := []string{}
		isNew := !phpTruthy(in["id"])
		if isNew {
			in["id"] = newID("r")
		} else if cfg.repoIndex(str(in["id"])) < 0 {
			return nil, apiFail("That repository does not exist (any more).")
		}
		repo := cleanRepo(in, &errs)
		if repo.ID == "" {
			return nil, apiFail("Malformed repository ID")
		}
		if passwordMode == "set" && password == "" {
			errs = append(errs, "Password: enter the repository password")
		} else if passwordMode == "set" && strings.ContainsAny(password, "\r\n") {
			errs = append(errs, "Password: a single line")
		}
		if passwordMode == "keep" && !secretHas(repo.ID, "password") {
			errs = append(errs, "Password: a repository needs one")
		}
		if repo.Type == "rest" && restPasswordMode == "set" && strings.ContainsAny(restPassword, "\r\n") {
			errs = append(errs, "REST password: a single line")
		}
		for _, other := range cfg.Repos {
			if other.ID != repo.ID && repoURL(other) == repoURL(repo) {
				errs = append(errs, fmt.Sprintf("Location: \"%s\" already uses this repository", other.Name))
			}
		}
		if len(errs) > 0 {
			return nil, apiFail("Please correct the marked fields.", errs...)
		}
		generated := ""
		if passwordMode == "generate" {
			generated = generatePassword()
			password = generated
		}
		if passwordMode != "keep" && !secretSet(repo.ID, "password", password) {
			return nil, apiFail("The password could not be stored.")
		}
		if repo.Type == "rest" && restPasswordMode == "set" {
			secretSet(repo.ID, "rest", restPassword)
		}
		if i := cfg.repoIndex(repo.ID); i < 0 {
			cfg.Repos = append(cfg.Repos, repo)
		} else {
			cfg.Repos[i] = repo
		}
		if repo.Type == "sftp" && !sshKeyExists(repo.ID) {
			if _, err := sshKeygen(repo.ID); err != nil {
				return nil, err
			}
		}
		browseStop(repo.ID, 0)
		return M{"repo": repoPublic(repo), "generated_password": generated, "created": isNew}, nil
	})
}

func apiRepoDelete(p M) (M, error) {
	id := pstr(p, "id", "")
	browseStop(id, 20)
	return configChange(func(cfg *Config) (M, error) {
		repo, err := needRepo(cfg, id)
		if err != nil {
			return nil, err
		}
		users := []string{}
		for _, job := range cfg.Jobs {
			if job.Repo == id {
				users = append(users, job.Name)
			}
		}
		if len(users) > 0 {
			return nil, apiFail("Jobs still back up to this repository: " + strings.Join(users, ", ") + ". Delete them or point them elsewhere first.")
		}
		if repoBusy(id) {
			return nil, apiFail("An operation on this repository is running.")
		}
		i := cfg.repoIndex(id)
		cfg.Repos = append(cfg.Repos[:i], cfg.Repos[i+1:]...)
		secretRemove(id)
		os.Remove(cacheDir() + "/snapshots-" + id + ".json")
		return M{"deleted": repo.Name}, nil
	})
}

func apiRepoPassword(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	return M{"password": secretGet(repo.ID, "password")}, nil
}

func sshKeygen(repoID string) (string, error) {
	privateDir(SSHDir)
	dir := runSecretsDir()
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	tmp := dir + "/keygen-" + randHex(8)
	os.Remove(tmp)
	os.Remove(tmp + ".pub")
	defer func() {
		shred(tmp)
		os.Remove(tmp + ".pub")
	}()
	code := procStream([]string{"ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "restic.backup@" + serverName(), "-f", tmp},
		map[string]string{"PATH": "/usr/bin:/bin:/usr/sbin:/sbin"}, func(stream, line string) {}, nil, nil, nil)
	private, err1 := os.ReadFile(tmp)
	public, err2 := os.ReadFile(tmp + ".pub")
	if code != 0 || err1 != nil || err2 != nil || len(private) == 0 {
		return "", apiFail("ssh-keygen could not create a key.")
	}
	shred(runSecretsDir() + "/ssh-" + repoID)
	if !sshKeyStore(repoID, private) {
		return "", apiFail("The SSH key could not be stored.")
	}
	if !fileWrite(sshKeyFile(repoID)+".pub", public, 0o644) {
		return "", apiFail("The SSH key could not be stored.")
	}
	return strings.TrimSpace(string(public)), nil
}

func apiRepoKeygen(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	if repo.Type != "sftp" {
		return nil, apiFail("Only SFTP repositories use an SSH key.")
	}
	key, err := sshKeygen(repo.ID)
	if err != nil {
		return nil, err
	}
	return M{"public_key": key}, nil
}

func knownHostsName(host string, port int) string {
	if port == 22 {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

var reFingerprint = regexp.MustCompile(`^(\d+)\s+(\S+)\s+.*\((\w+)\)\s*$`)

func fingerprints(lines []string) []M {
	out := []M{}
	if len(lines) == 0 {
		return out
	}
	in := strings.Join(lines, "\n") + "\n"
	procStream([]string{"ssh-keygen", "-l", "-f", "/dev/stdin"}, map[string]string{"PATH": "/usr/bin:/bin"},
		func(stream, line string) {
			if stream == "out" {
				if m := reFingerprint.FindStringSubmatch(line); m != nil {
					bits, _ := strconv.Atoi(m[1])
					out = append(out, M{"type": m[3], "bits": bits, "fingerprint": m[2]})
				}
			}
		}, nil, nil, &in)
	return out
}

func scanHostKeys(repo *Repo) ([]string, error) {
	lines, errs := []string{}, []string{}
	procStream([]string{"ssh-keyscan", "-T", "10", "-p", strconv.Itoa(repo.SFTP.Port), repo.SFTP.Host},
		map[string]string{"PATH": "/usr/bin:/bin"},
		func(stream, line string) {
			if line == "" || line[0] == '#' {
				return
			}
			if stream == "out" {
				lines = append(lines, line)
			} else if stream == "err" {
				errs = append(errs, line)
			}
		}, nil, nil, nil)
	if len(lines) == 0 {
		tail := "."
		if len(errs) > 0 {
			tail = ": " + errs[len(errs)-1]
		}
		return nil, apiFail("The server did not answer on port " + strconv.Itoa(repo.SFTP.Port) + tail + " Check the host name and port.")
	}
	return lines, nil
}

func knownHostsLines() []string {
	b, err := os.ReadFile(knownHostsFile())
	if err != nil {
		return nil
	}
	out := []string{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimRight(l, "\r")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func knownHostsHosts(line string) []string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return nil
	}
	return strings.Split(f[0], ",")
}

func knownHostLines(repo *Repo) []string {
	name := knownHostsName(repo.SFTP.Host, repo.SFTP.Port)
	out := []string{}
	for _, line := range knownHostsLines() {
		if inList(name, knownHostsHosts(line)...) {
			out = append(out, line)
		}
	}
	return out
}

func afterFirstSpace(l string) string {
	if i := strings.Index(l, " "); i >= 0 {
		return l[i+1:]
	}
	if len(l) > 0 {
		return l[1:]
	}
	return ""
}

func apiRepoHostkey(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	if repo.Type != "sftp" {
		return nil, apiFail("Only SFTP repositories have a host key.")
	}
	lines, err := scanHostKeys(repo)
	if err != nil {
		return nil, err
	}
	scanned := fingerprints(lines)
	storedLines := []string{}
	for _, l := range knownHostLines(repo) {
		storedLines = append(storedLines, afterFirstSpace(l))
	}
	stored := fingerprints(storedLines)
	status := "unknown"
	if len(stored) > 0 {
		status = "unknown-type"
	}
	for _, k := range scanned {
		sameType := false
		match := false
		for _, s := range stored {
			if s["type"] == k["type"] {
				sameType = true
				if s["fingerprint"] == k["fingerprint"] {
					match = true
				}
			}
		}
		if !sameType {
			continue
		}
		if !match {
			status = "changed"
			break
		}
		status = "trusted"
	}
	if status == "unknown-type" {
		status = "unknown"
	}
	return M{"status": status, "scanned": scanned, "stored": stored}, nil
}

func apiRepoHostkeyTrust(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	confirmedIn, err := pjsonList(p, "fingerprints")
	if err != nil {
		return nil, err
	}
	confirmed := []string{}
	for _, c := range confirmedIn {
		if s, ok := c.(string); ok {
			confirmed = append(confirmed, s)
		}
	}
	lines, err := scanHostKeys(repo)
	if err != nil {
		return nil, err
	}
	keep := []string{}
	for _, line := range lines {
		fp := fingerprints([]string{line})
		if len(fp) > 0 && inList(str(fp[0]["fingerprint"]), confirmed...) {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return nil, apiFail("The server now shows different keys from the ones you confirmed. Check again.")
	}
	name := knownHostsName(repo.SFTP.Host, repo.SFTP.Port)
	all := []string{}
	for _, line := range knownHostsLines() {
		if !inList(name, knownHostsHosts(line)...) {
			all = append(all, line)
		}
	}
	for _, line := range keep {
		rest := ""
		if i := strings.Index(line, " "); i >= 0 {
			rest = line[i:]
		}
		all = append(all, name+rest)
	}
	privateDir(SSHDir)
	if !fileWrite(knownHostsFile(), []byte(strings.Join(all, "\n")+"\n"), 0o600) {
		return nil, apiFail("known_hosts could not be written.")
	}
	return M{"trusted": len(keep)}, nil
}

var reMntTop = regexp.MustCompile(`^/mnt/[^/]+$`)

func apiRepoTest(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	if repo.Type == "sftp" {
		if !sshKeyExists(repo.ID) {
			return nil, apiFail("The repository has no SSH key yet.")
		}
		hk, err := apiRepoHostkey(M{"id": repo.ID})
		if err != nil {
			return nil, err
		}
		if hk["status"] != "trusted" {
			return M{"status": "hostkey", "hostkey": hk}, nil
		}
	}
	if repo.Type == "local" {
		parent := filepath.Dir(repo.Local.Path)
		for parent != "/" && !isDir(parent) {
			parent = filepath.Dir(parent)
		}
		if reMntTop.MatchString(parent) && !dirHasEntries(parent) {
			return nil, apiFail(parent + " is empty - is the disk mounted?")
		}
	}
	if why := repoReachable(*repo, 1, 0, nil, nil); why != "" {
		return nil, apiFail(why)
	}
	code, out, errs := resticCapture(*repo, cfg.Settings, []string{"cat", "config", "--no-lock"}, 200000)
	if code == 10 {
		return M{"status": "empty"}, nil
	}
	if code != 0 {
		return nil, apiFail(resticError(errs, code))
	}
	var c M
	json.Unmarshal([]byte(strings.Join(out, "\n")), &c)
	return M{"status": "ok", "repository_id": str(c["id"]), "version": toInt(c["version"])}, nil
}

func apiRepoInit(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	test, err := apiRepoTest(p)
	if err != nil {
		return nil, err
	}
	if test["status"] == "ok" {
		return nil, apiFail("There already is a repository at this location.")
	}
	if test["status"] != "empty" {
		return nil, apiFail("Confirm the server's host key first.")
	}
	l, err := apiLock(repo)
	if err != nil {
		return nil, err
	}
	defer l.Unlock()
	out, err := apiRestic(cfg, repo, []string{"init", "--json"}, 200000)
	if err != nil {
		return nil, err
	}
	id := ""
	if j := jsonLines(out); len(j) > 0 {
		id = str(j[0]["id"])
	}
	return M{"repository_id": id}, nil
}

func apiRepoUnlock(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	args := []string{"unlock"}
	if toBool(param(p, "all", false)) {
		args = append(args, "--remove-all")
	}
	l, err := apiLock(repo)
	if err != nil {
		return nil, err
	}
	defer l.Unlock()
	if _, err := apiRestic(cfg, repo, args, 200000); err != nil {
		return nil, err
	}
	return M{}, nil
}

func apiRepoStats(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	out, err := apiRestic(cfg, repo, []string{"stats", "--json", "--mode", "raw-data", "--no-lock"}, 200000)
	if err != nil {
		return nil, err
	}
	var stats M
	if json.Unmarshal([]byte(strings.Join(out, "\n")), &stats) != nil || stats == nil {
		return nil, apiFail("restic stats gave no result.")
	}
	stats["at"] = now()
	stateUpdate(cfg.Settings, func(state M) {
		sub(state, "repos", repo.ID)["stats"] = stats
		stateAddRepoSize(state, repo.ID, stats)
	})
	return M{"stats": stats}, nil
}

func apiSizeHistory(p M) (M, error) {
	cfg := configLoad()
	state := stateRead(cfg.Settings)
	var repo *Repo
	jobs := []Job{}
	if pstr(p, "job", "") != "" {
		job, err := needJob(cfg, param(p, "job", ""))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
		if repo, err = needRepo(cfg, job.Repo); err != nil {
			return nil, err
		}
	} else {
		var err error
		if repo, err = needRepo(cfg, param(p, "repo", "")); err != nil {
			return nil, err
		}
		for _, j := range cfg.Jobs {
			if j.Repo == repo.ID {
				jobs = append(jobs, j)
			}
		}
	}
	points := mapOf(mapOf(state["repos"])[repo.ID])["sizes"]
	if points == nil {
		points = []any{}
	}
	jobList := []M{}
	for _, j := range jobs {
		jp := mapOf(mapOf(state["jobs"])[j.ID])["sizes"]
		if jp == nil {
			jp = jobSizesFromHistory(cfg.Settings, j.ID)
		}
		jobList = append(jobList, M{"id": j.ID, "name": j.Name, "points": jp})
	}
	return M{"repo": M{"id": repo.ID, "name": repo.Name, "points": points}, "jobs": jobList}, nil
}

func apiRepoKeys(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	out, err := apiRestic(cfg, repo, []string{"key", "list", "--json", "--no-lock"}, 200000)
	if err != nil {
		return nil, err
	}
	var keys any
	json.Unmarshal([]byte(strings.Join(out, "\n")), &keys)
	switch keys.(type) {
	case []any, M:
	default:
		keys = []any{}
	}
	return M{"keys": keys}, nil
}

func apiRepoRun(p M) (M, error) {
	cfg := configLoad()
	repo, err := needRepo(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	kind := pstr(p, "kind", "")
	if !inList(kind, "prune", "check") {
		return nil, apiFail("Unknown operation")
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	if repoBusy(repo.ID) || opRunningOn(repo.ID, "restore") != nil {
		return nil, apiFail("An operation on this repository is running. Try again when it has finished.")
	}
	spawn("run", kind, repo.ID, "manual")
	return M{"started": true}, nil
}
