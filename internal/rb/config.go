package rb

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

type Settings struct {
	DataDir     string `json:"data_dir"`
	Host        string `json:"host"`
	Nice        int    `json:"nice"`
	IOPriority  string `json:"io_priority"`
	StartDelay  int    `json:"start_delay"`
	ParityPause bool   `json:"parity_pause"`
	HistoryDays int    `json:"history_days"`
	ShowInMenu  bool   `json:"show_in_menu"`
	Secrets     string `json:"secrets"`
}

type Schedule struct {
	Mode    string `json:"mode"`
	Cron    string `json:"cron"`
	Every   int    `json:"every"`
	Unit    string `json:"unit"`
	CatchUp bool   `json:"catch_up"`
}

type RepoSFTP struct {
	User string `json:"user"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Path string `json:"path"`
}

type RepoLocal struct {
	Path string `json:"path"`
}

type RepoRest struct {
	URL    string `json:"url"`
	User   string `json:"user"`
	CACert string `json:"cacert"`
}

type RepoOptions struct {
	Compression   string   `json:"compression"`
	LimitUpload   int      `json:"limit_upload"`
	LimitDownload int      `json:"limit_download"`
	Connections   int      `json:"connections"`
	ExtraFlags    []string `json:"extra_flags"`
}

type PruneSettings struct {
	Enabled   bool     `json:"enabled"`
	Schedule  Schedule `json:"schedule"`
	MaxUnused string   `json:"max_unused"`
}

type CheckSettings struct {
	Enabled  bool     `json:"enabled"`
	Schedule Schedule `json:"schedule"`
	ReadData string   `json:"read_data"`
	Subset   string   `json:"subset"`
}

type Maintenance struct {
	Prune PruneSettings `json:"prune"`
	Check CheckSettings `json:"check"`
}

type Repo struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	SFTP        RepoSFTP    `json:"sftp"`
	Local       RepoLocal   `json:"local"`
	Rest        RepoRest    `json:"rest"`
	Options     RepoOptions `json:"options"`
	Maintenance Maintenance `json:"maintenance"`
	UnlockStale bool        `json:"unlock_stale"`
	AppendOnly  bool        `json:"append_only"`
}

type Retention struct {
	Enabled bool   `json:"enabled"`
	Last    int    `json:"last"`
	Hourly  int    `json:"hourly"`
	Daily   int    `json:"daily"`
	Weekly  int    `json:"weekly"`
	Monthly int    `json:"monthly"`
	Yearly  int    `json:"yearly"`
	Within  string `json:"within"`
}

type Consistency struct {
	Mode       string   `json:"mode"`
	Containers []string `json:"containers"`
	VMs        []string `json:"vms"`
	Skip       []string `json:"skip"`
	Timeout    int      `json:"timeout"`
}

type Hooks struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

type Notify struct {
	Success     bool   `json:"success"`
	Warning     bool   `json:"warning"`
	Error       bool   `json:"error"`
	Healthcheck string `json:"healthcheck"`
}

type Advanced struct {
	Host            string   `json:"host"`
	ReadConcurrency int      `json:"read_concurrency"`
	SkipIfUnchanged bool     `json:"skip_if_unchanged"`
	ExtraFlags      []string `json:"extra_flags"`
}

type Job struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Enabled           bool        `json:"enabled"`
	Repo              string      `json:"repo"`
	Sources           []string    `json:"sources"`
	Excludes          []string    `json:"excludes"`
	IExcludes         []string    `json:"iexcludes"`
	ExcludeCaches     bool        `json:"exclude_caches"`
	ExcludeLargerThan string      `json:"exclude_larger_than"`
	OneFileSystem     bool        `json:"one_file_system"`
	Tags              []string    `json:"tags"`
	Schedule          Schedule    `json:"schedule"`
	Retention         Retention   `json:"retention"`
	Consistency       Consistency `json:"consistency"`
	Hooks             Hooks       `json:"hooks"`
	Notify            Notify      `json:"notify"`
	Advanced          Advanced    `json:"advanced"`
}

type Config struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
	Repos    []Repo   `json:"repos"`
	Jobs     []Job    `json:"jobs"`
}

func defaultDataDir() string {
	if isDir("/mnt/user/system") {
		return "/mnt/user/system/" + Name
	}
	return "/mnt/user/appdata/" + Name
}

func defaultSettings() Settings {
	return Settings{DataDir: defaultDataDir(), Nice: 10, IOPriority: "low", StartDelay: 5, ParityPause: true,
		HistoryDays: 90, Secrets: "plain"}
}

func defaultSchedule(cron string) Schedule {
	return Schedule{Mode: "cron", Cron: cron, Every: 24, Unit: "hours", CatchUp: true}
}

func defaultRepo() Repo {
	return Repo{
		Type:    "sftp",
		SFTP:    RepoSFTP{Port: 22},
		Options: RepoOptions{Compression: "auto", ExtraFlags: []string{}},
		Maintenance: Maintenance{
			Prune: PruneSettings{Enabled: true, Schedule: defaultSchedule("0 4 * * 0"), MaxUnused: "10%"},
			Check: CheckSettings{Enabled: true, Schedule: defaultSchedule("0 5 1 * *"), ReadData: "subset", Subset: "5%"},
		},
		UnlockStale: true,
	}
}

func defaultJob() Job {
	return Job{
		Enabled: true, Sources: []string{}, Excludes: []string{}, IExcludes: []string{}, ExcludeCaches: true,
		Tags: []string{}, Schedule: defaultSchedule("0 3 * * *"),
		Retention:   Retention{Enabled: true, Daily: 7, Weekly: 4, Monthly: 12},
		Consistency: Consistency{Mode: "none", Containers: []string{}, VMs: []string{}, Skip: []string{}, Timeout: 120},
		Notify:      Notify{Warning: true, Error: true},
		Advanced:    Advanced{ExtraFlags: []string{}},
	}
}

func configLoad() *Config {
	raw, _ := jsonRead(ConfigFile)
	if raw == nil {
		raw = M{}
	}
	var ignored []string
	cfg := &Config{Version: ConfigVersion, Settings: cleanSettings(raw["settings"], &ignored), Repos: []Repo{}, Jobs: []Job{}}
	if repos, ok := raw["repos"].([]any); ok {
		for _, r := range repos {
			var errs []string
			repo := cleanRepo(mapOf(r), &errs)
			if repo.ID != "" {
				cfg.Repos = append(cfg.Repos, repo)
			}
		}
	}
	ids := cfg.repoIDs()
	if jobs, ok := raw["jobs"].([]any); ok {
		for _, j := range jobs {
			var errs []string
			job := cleanJob(mapOf(j), ids, &errs)
			if job.ID != "" {
				cfg.Jobs = append(cfg.Jobs, job)
			}
		}
	}
	return cfg
}

func (c *Config) repoIDs() []string {
	ids := []string{}
	for _, r := range c.Repos {
		ids = append(ids, r.ID)
	}
	return ids
}

func configSave(cfg *Config) bool {
	cfg.Version = ConfigVersion
	return jsonWrite(ConfigFile, cfg, 0o644)
}

type fileLock struct{ f *os.File }

func (l *fileLock) Unlock() {
	if l != nil && l.f != nil {
		syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		l.f.Close()
		l.f = nil
	}
}

func lockFile(path string, block bool) *fileLock {
	os.MkdirAll(dirOf(path), 0o755)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil
	}
	how := syscall.LOCK_EX
	if !block {
		how |= syscall.LOCK_NB
	}
	if syscall.Flock(int(f.Fd()), how) != nil {
		f.Close()
		return nil
	}
	return &fileLock{f: f}
}

func dirOf(p string) string {
	i := strings.LastIndex(p, "/")
	if i <= 0 {
		return "/"
	}
	return p[:i]
}

func configLock() *fileLock {
	return lockFile(RunDir+"/config.lock", true)
}

func (c *Config) repoIndex(id string) int {
	for i, r := range c.Repos {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func (c *Config) jobIndex(id string) int {
	for i, j := range c.Jobs {
		if j.ID == id {
			return i
		}
	}
	return -1
}

func (c *Config) repo(id string) *Repo {
	if i := c.repoIndex(id); i >= 0 {
		r := c.Repos[i]
		return &r
	}
	return nil
}

func (c *Config) job(id string) *Job {
	if i := c.jobIndex(id); i >= 0 {
		j := c.Jobs[i]
		return &j
	}
	return nil
}

func secretGet(repoID, kind string) string {
	b, err := secretRead(repoID, kind)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\r\n")
}

func secretSet(repoID, kind, value string) bool {
	privateDir(SecretsDir)
	return secretWrite(repoID, kind, []byte(value+"\n"))
}

func secretHas(repoID, kind string) bool {
	return secretExists(repoID, kind)
}

func privateDir(dir string) {
	if !isDir(dir) {
		os.MkdirAll(dir, 0o700)
	}
	if !isFile(dir + "/.gitignore") {
		os.WriteFile(dir+"/.gitignore", []byte("*\n"), 0o600)
	}
}

func sshKeyFile(repoID string) string {
	return SSHDir + "/" + repoID
}

func knownHostsFile() string {
	return SSHDir + "/known_hosts"
}

func generatePassword() string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < 32; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String()
}

var (
	reSpaces    = regexp.MustCompile(`\s+`)
	reHostName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	reFlag      = regexp.MustCompile(`^--[a-z][a-z0-9-]*(=.*)?$`)
	reRepoID    = regexp.MustCompile(`^r[0-9a-f]{8}$`)
	reJobID     = regexp.MustCompile(`^j[0-9a-f]{8}$`)
	reSSHUser   = regexp.MustCompile(`^[A-Za-z0-9._][A-Za-z0-9._-]{0,63}$`)
	reDNSName   = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	reIPv6ish   = regexp.MustCompile(`^[0-9A-Fa-f:]{2,39}$`)
	reDiskRoot  = regexp.MustCompile(`^/mnt/[^/]+$`)
	reRestUser  = regexp.MustCompile(`^[^\s:]{1,128}$`)
	reMaxUnused = regexp.MustCompile(`(?i)^(\d{1,3}(\.\d+)?%|\d+[KMGT]?|unlimited)$`)
	reSubset    = regexp.MustCompile(`^\d{1,3}(\.\d+)?%$`)
	reSize      = regexp.MustCompile(`(?i)^\d+(\.\d+)?[KMGT]?$`)
	reTag       = regexp.MustCompile(`^[A-Za-z0-9._:@+=/-]{1,64}$`)
	reWithin    = regexp.MustCompile(`^(\d+y)?(\d+m)?(\d+d)?(\d+h)?$`)
	reContainer = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	reHTTPURL   = regexp.MustCompile(`^https?://[^\s/]+`)
)

func cleanSchedule(in any, label string, errs *[]string) Schedule {
	s := defaultSchedule("0 3 * * *")
	m, ok := in.(M)
	if !ok {
		return s
	}
	mode := text(get(m, "mode", "cron"), 255)
	if inList(mode, "off", "cron", "interval") {
		s.Mode = mode
	} else {
		s.Mode = "cron"
	}
	s.Cron = reSpaces.ReplaceAllString(text(get(m, "cron", s.Cron), 120), " ")
	s.Every = count(get(m, "every", s.Every), 100000)
	if s.Every < 1 {
		s.Every = 1
	}
	unit := text(get(m, "unit", "hours"), 255)
	if inList(unit, "minutes", "hours", "days") {
		s.Unit = unit
	} else {
		s.Unit = "hours"
	}
	if has(m, "catch_up") {
		s.CatchUp = toBool(m["catch_up"])
	} else {
		s.CatchUp = true
	}
	if s.Mode == "cron" {
		if p := cronProblem(s.Cron); p != "" {
			*errs = append(*errs, label+": "+p)
		}
	} else if s.Mode == "interval" && s.Unit == "minutes" && s.Every < 5 {
		*errs = append(*errs, label+": run at most every 5 minutes")
	}
	return s
}

func cleanSettings(in any, errs *[]string) Settings {
	d := defaultSettings()
	m, ok := in.(M)
	if !ok {
		return d
	}
	dir := absPath(get(m, "data_dir", d.DataDir))
	if dir == "" || !under(dir, Mnt) || inList(dir, Mnt, Mnt+"/user", Mnt+"/disks") {
		*errs = append(*errs, "Data folder: choose a folder on a disk or pool, such as /mnt/user/system/"+Name)
		dir = d.DataDir
	}
	d.DataDir = dir
	host := text(get(m, "host", ""), 255)
	if host != "" && !reHostName.MatchString(host) {
		*errs = append(*errs, "Host name: letters, digits, dots, dashes and underscores only")
		host = ""
	}
	d.Host = host
	d.Nice = count(get(m, "nice", d.Nice), 19)
	io := text(get(m, "io_priority", "low"), 255)
	if inList(io, "low", "idle", "normal") {
		d.IOPriority = io
	} else {
		d.IOPriority = "low"
	}
	d.StartDelay = count(get(m, "start_delay", d.StartDelay), 100000)
	if d.StartDelay > 120 {
		d.StartDelay = 120
	}
	if has(m, "parity_pause") {
		d.ParityPause = toBool(m["parity_pause"])
	}
	hd := count(get(m, "history_days", 90), 100000)
	if hd == 0 {
		hd = 90
	}
	d.HistoryDays = clampInt(hd, 7, 3650)
	d.ShowInMenu = toBool(get(m, "show_in_menu", false))
	sec := text(get(m, "secrets", "plain"), 20)
	if inList(sec, "plain", "key", "master") {
		d.Secrets = sec
	}
	return d
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func cleanFlags(in any, label string, errs *[]string) []string {
	out := []string{}
	for _, flag := range list(in, 50) {
		if !reFlag.MatchString(flag) {
			*errs = append(*errs, fmt.Sprintf("%s: \"%s\" is not a long option like --name or --name=value", label, flag))
			continue
		}
		out = append(out, flag)
	}
	return out
}

func cleanRepo(in M, errs *[]string) Repo {
	r := defaultRepo()
	if id := str(in["id"]); reRepoID.MatchString(id) {
		r.ID = id
	}
	r.Name = text(get(in, "name", ""), 60)
	if r.Name == "" {
		*errs = append(*errs, "Name: give the repository a name")
	}
	typ := text(get(in, "type", "sftp"), 255)
	if inList(typ, "sftp", "local", "rest") {
		r.Type = typ
	}
	sftp := mapOf(in["sftp"])
	r.SFTP = RepoSFTP{
		User: text(get(sftp, "user", ""), 64),
		Host: text(get(sftp, "host", ""), 253),
		Port: toInt(get(sftp, "port", 22)),
		Path: text(get(sftp, "path", ""), 1024),
	}
	r.Local = RepoLocal{Path: absPath(get(mapOf(in["local"]), "path", ""))}
	rest := mapOf(in["rest"])
	r.Rest = RepoRest{
		URL:    text(get(rest, "url", ""), 1024),
		User:   text(get(rest, "user", ""), 128),
		CACert: absPath(get(rest, "cacert", "")),
	}
	switch r.Type {
	case "sftp":
		s := r.SFTP
		if !reSSHUser.MatchString(s.User) {
			*errs = append(*errs, "SSH user: letters, digits, dots, dashes and underscores")
		}
		if !reDNSName.MatchString(s.Host) && !reIPv6ish.MatchString(s.Host) {
			*errs = append(*errs, "SSH host: a host name or an IP address")
		}
		if s.Port < 1 || s.Port > 65535 {
			*errs = append(*errs, "SSH port: 1-65535")
			r.SFTP.Port = 22
		}
		if s.Path == "" || hasControl(s.Path) {
			*errs = append(*errs, "Path on the server: where the repository lives, e.g. backups/tower")
		}
	case "local":
		p := r.Local.Path
		if p == "" || !under(p, "/mnt") || inList(p, "/mnt", "/mnt/user", "/mnt/disks", "/mnt/remotes") {
			*errs = append(*errs, "Folder: a folder on a disk, pool or unassigned device, such as /mnt/disks/backup/restic")
		} else if reDiskRoot.MatchString(p) {
			*errs = append(*errs, "Folder: use a folder inside "+p+", not the disk itself")
		}
	default:
		u, err := url.Parse(r.Rest.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			*errs = append(*errs, "Server URL: http:// or https:// followed by the server, e.g. https://backup.lan:8000/tower")
		} else if u.User != nil {
			*errs = append(*errs, "Server URL: put the user name and password in their own fields, not in the URL")
		}
		if r.Rest.User != "" && !reRestUser.MatchString(r.Rest.User) {
			*errs = append(*errs, "REST user: no spaces or colons")
		}
	}
	o := mapOf(in["options"])
	comp := text(get(o, "compression", "auto"), 255)
	if !inList(comp, "auto", "off", "fastest", "better", "max") {
		comp = "auto"
	}
	r.Options = RepoOptions{
		Compression:   comp,
		LimitUpload:   count(get(o, "limit_upload", 0), 10000000),
		LimitDownload: count(get(o, "limit_download", 0), 10000000),
		Connections:   count(get(o, "connections", 0), 64),
		ExtraFlags:    cleanFlags(get(o, "extra_flags", []any{}), "Extra options", errs),
	}
	mt := mapOf(in["maintenance"])
	p := mapOf(mt["prune"])
	c := mapOf(mt["check"])
	defs := defaultRepo().Maintenance
	maxUnused := text(get(p, "max_unused", "10%"), 20)
	if !reMaxUnused.MatchString(maxUnused) {
		*errs = append(*errs, `Prune: "unused space" takes a percentage like 10%, a size like 5G, or unlimited`)
		maxUnused = "10%"
	}
	readData := text(get(c, "read_data", "subset"), 255)
	subset := text(get(c, "subset", "5%"), 20)
	sv, _ := strconv.ParseFloat(strings.TrimSuffix(subset, "%"), 64)
	if !reSubset.MatchString(subset) || sv <= 0 || sv > 100 {
		*errs = append(*errs, "Check: the share of data to read is a percentage between 0 and 100")
		subset = "5%"
	}
	if !inList(readData, "none", "subset", "all") {
		readData = "subset"
	}
	var pruneSched, checkSched any = defs.Prune.Schedule, defs.Check.Schedule
	if v, ok := p["schedule"]; ok && v != nil {
		pruneSched = v
	}
	if v, ok := c["schedule"]; ok && v != nil {
		checkSched = v
	}
	r.Maintenance = Maintenance{
		Prune: PruneSettings{
			Enabled:   !has(p, "enabled") || toBool(p["enabled"]),
			Schedule:  cleanScheduleAny(pruneSched, "Prune schedule", errs),
			MaxUnused: maxUnused,
		},
		Check: CheckSettings{
			Enabled:  !has(c, "enabled") || toBool(c["enabled"]),
			Schedule: cleanScheduleAny(checkSched, "Check schedule", errs),
			ReadData: readData,
			Subset:   subset,
		},
	}
	r.UnlockStale = !has(in, "unlock_stale") || toBool(in["unlock_stale"])
	r.AppendOnly = toBool(get(in, "append_only", false))
	return r
}

func cleanScheduleAny(v any, label string, errs *[]string) Schedule {
	if s, ok := v.(Schedule); ok {
		return cleanSchedule(scheduleMap(s), label, errs)
	}
	return cleanSchedule(v, label, errs)
}

func scheduleMap(s Schedule) M {
	return M{"mode": s.Mode, "cron": s.Cron, "every": float64(s.Every), "unit": s.Unit, "catch_up": s.CatchUp}
}

func cleanJob(in M, repoIDs []string, errs *[]string) Job {
	j := defaultJob()
	if id := str(in["id"]); reJobID.MatchString(id) {
		j.ID = id
	}
	j.Name = text(get(in, "name", ""), 60)
	if j.Name == "" {
		*errs = append(*errs, "Name: give the job a name")
	}
	j.Enabled = !has(in, "enabled") || toBool(in["enabled"])
	j.Repo = text(get(in, "repo", ""), 20)
	if !inList(j.Repo, repoIDs...) {
		*errs = append(*errs, "Repository: choose where the backup goes")
	}
	for _, src := range list(get(in, "sources", []any{}), 100) {
		p := absPath(src)
		if p == "" || !(under(p, "/mnt") || under(p, "/boot")) || p == "/mnt" {
			*errs = append(*errs, fmt.Sprintf("Sources: \"%s\" - choose folders under /mnt or /boot", src))
			continue
		}
		if !inList(p, j.Sources...) {
			j.Sources = append(j.Sources, p)
		}
	}
	if len(j.Sources) == 0 {
		*errs = append(*errs, "Sources: add at least one folder to back up")
	}
	j.Excludes = list(get(in, "excludes", []any{}), 1000)
	j.IExcludes = list(get(in, "iexcludes", []any{}), 1000)
	j.ExcludeCaches = !has(in, "exclude_caches") || toBool(in["exclude_caches"])
	elt := text(get(in, "exclude_larger_than", ""), 20)
	if elt != "" && !reSize.MatchString(elt) {
		*errs = append(*errs, "Skip files larger than: a size like 500M or 4G")
		elt = ""
	}
	j.ExcludeLargerThan = elt
	j.OneFileSystem = toBool(get(in, "one_file_system", false))
	for _, tag := range list(get(in, "tags", []any{}), 20) {
		if !reTag.MatchString(tag) {
			*errs = append(*errs, fmt.Sprintf("Tags: \"%s\" - letters, digits and . _ : @ + = / - only", tag))
		} else if strings.HasPrefix(tag, "job:") {
			*errs = append(*errs, fmt.Sprintf("Tags: \"%s\" - tags starting with job: are the plugin's own", tag))
		} else {
			j.Tags = append(j.Tags, tag)
		}
	}
	j.Schedule = cleanSchedule(in["schedule"], "Schedule", errs)

	ret := mapOf(in["retention"])
	j.Retention.Enabled = !has(ret, "enabled") || toBool(ret["enabled"])
	j.Retention.Last = count(get(ret, "last", j.Retention.Last), 100000)
	j.Retention.Hourly = count(get(ret, "hourly", j.Retention.Hourly), 100000)
	j.Retention.Daily = count(get(ret, "daily", j.Retention.Daily), 100000)
	j.Retention.Weekly = count(get(ret, "weekly", j.Retention.Weekly), 100000)
	j.Retention.Monthly = count(get(ret, "monthly", j.Retention.Monthly), 100000)
	j.Retention.Yearly = count(get(ret, "yearly", j.Retention.Yearly), 100000)
	within := strings.ToLower(text(get(ret, "within", ""), 20))
	if within != "" && !reWithin.MatchString(within) {
		*errs = append(*errs, "Keep everything from the last: a duration like 14d, 6m or 1y2m")
		within = ""
	}
	j.Retention.Within = within
	r := j.Retention
	if r.Enabled && within == "" && r.Last+r.Hourly+r.Daily+r.Weekly+r.Monthly+r.Yearly == 0 {
		*errs = append(*errs, "Retention: keep at least one snapshot, or switch clean-up off")
	}

	cons := mapOf(in["consistency"])
	mode := text(get(cons, "mode", "none"), 255)
	if inList(mode, "none", "zfs", "zfs-stop", "stop") {
		j.Consistency.Mode = mode
	}
	for _, name := range list(get(cons, "containers", []any{}), 200) {
		if reContainer.MatchString(name) {
			j.Consistency.Containers = append(j.Consistency.Containers, name)
		} else {
			*errs = append(*errs, fmt.Sprintf("Containers: \"%s\" is not a container name", name))
		}
	}
	for _, name := range list(get(cons, "vms", []any{}), 100) {
		if !hasControl(name) {
			j.Consistency.VMs = append(j.Consistency.VMs, name)
		}
	}
	for _, name := range list(get(cons, "skip", []any{}), 300) {
		if !hasControl(name) {
			j.Consistency.Skip = append(j.Consistency.Skip, name)
		}
	}
	to := count(get(cons, "timeout", 120), 100000)
	if to == 0 {
		to = 120
	}
	j.Consistency.Timeout = clampInt(to, 10, 3600)

	hooks := mapOf(in["hooks"])
	for _, k := range []string{"before", "after"} {
		script := ""
		if s, ok := hooks[k].(string); ok {
			script = strings.ReplaceAll(s, "\r\n", "\n")
		}
		if len(script) > 65536 {
			script = script[:65536]
		} else {
			script = strings.TrimRight(script, " \t\n\r\x00\x0B")
		}
		if k == "before" {
			j.Hooks.Before = script
		} else {
			j.Hooks.After = script
		}
	}

	n := mapOf(in["notify"])
	j.Notify = Notify{
		Success:     toBool(get(n, "success", false)),
		Warning:     !has(n, "warning") || toBool(n["warning"]),
		Error:       !has(n, "error") || toBool(n["error"]),
		Healthcheck: text(get(n, "healthcheck", ""), 512),
	}
	if j.Notify.Healthcheck != "" && !reHTTPURL.MatchString(j.Notify.Healthcheck) {
		*errs = append(*errs, "Healthcheck URL: an http:// or https:// address")
		j.Notify.Healthcheck = ""
	}

	a := mapOf(in["advanced"])
	host := text(get(a, "host", ""), 255)
	if host != "" && !reHostName.MatchString(host) {
		*errs = append(*errs, "Host name: letters, digits, dots, dashes and underscores only")
		host = ""
	}
	j.Advanced = Advanced{
		Host:            host,
		ReadConcurrency: count(get(a, "read_concurrency", 0), 64),
		SkipIfUnchanged: toBool(get(a, "skip_if_unchanged", false)),
		ExtraFlags:      cleanFlags(get(a, "extra_flags", []any{}), "Extra options", errs),
	}
	return j
}

func checkJobAgainstRepos(job Job, cfg *Config) []string {
	errs := []string{}
	repo := cfg.repo(job.Repo)
	if repo != nil && repo.Type == "local" && repo.Local.Path != "" {
		for _, src := range job.Sources {
			if under(repo.Local.Path, src) {
				errs = append(errs, fmt.Sprintf("Sources: %s contains the repository itself (%s) - exclude it or keep the repository elsewhere",
					src, repo.Local.Path))
			}
		}
	}
	return errs
}

type RepoPublic struct {
	Repo
	HasPassword     bool   `json:"has_password"`
	HasRestPassword bool   `json:"has_rest_password"`
	SSHPublicKey    string `json:"ssh_public_key"`
}

func repoPublic(r Repo) RepoPublic {
	pub, _ := os.ReadFile(sshKeyFile(r.ID) + ".pub")
	return RepoPublic{Repo: r, HasPassword: secretHas(r.ID, "password"), HasRestPassword: secretHas(r.ID, "rest"),
		SSHPublicKey: string(pub)}
}

func configPublic(cfg *Config) M {
	repos := []RepoPublic{}
	for _, r := range cfg.Repos {
		repos = append(repos, repoPublic(r))
	}
	return M{"version": cfg.Version, "settings": cfg.Settings, "repos": repos, "jobs": cfg.Jobs}
}

func toMap(v any) M {
	var m M
	jsonUnmarshal(jsonEncode(v, false), &m)
	return m
}
