package rb

import (
	"os"
	"strconv"
	"strings"
)

func jobHost(job Job, s Settings) string {
	if job.Advanced.Host != "" {
		return job.Advanced.Host
	}
	if s.Host != "" {
		return s.Host
	}
	return serverName()
}

func patternFile(patterns []string) string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = strings.ReplaceAll(p, "$", "$$")
	}
	return strings.Join(out, "\n") + "\n"
}

func ownExcludes(job Job, repo Repo, s Settings) []string {
	own := []string{s.DataDir + "/cache"}
	if repo.Type == "local" {
		own = append(own, repo.Local.Path)
	}
	out := []string{}
	for _, p := range own {
		for _, src := range job.Sources {
			if as, ok := pathAs(p, src); ok && !inList(as, out...) {
				out = append(out, as)
			}
		}
	}
	return out
}

func pathAs(p, dir string) (string, bool) {
	if under(p, dir) {
		return p, true
	}
	ps, pr, okp := shareKey(p)
	_, _, okd := shareKey(dir)
	if !okp || !okd || !pathWithin(p, dir) {
		return "", false
	}
	top := strings.Split(strings.Trim(dir, "/"), "/")[1]
	out := strings.TrimRight(Mnt, "/") + "/" + top + "/" + ps
	if pr != "" {
		out += "/" + pr
	}
	return out, true
}

func backupArgs(job Job, repo Repo, s Settings, tmp string) []string {
	args := []string{"backup", "--json", "--host", jobHost(job, s), "--tag", "job:" + job.ID, "--group-by", "host,tags"}
	for _, tag := range job.Tags {
		args = append(args, "--tag", tag)
	}
	excludes := append([]string{}, job.Excludes...)
	excludes = append(excludes, ".zfs/snapshot")
	excludes = append(excludes, ownExcludes(job, repo, s)...)
	if len(excludes) > 0 {
		os.WriteFile(tmp+".exclude", []byte(patternFile(excludes)), 0o666)
		args = append(args, "--exclude-file", tmp+".exclude")
	}
	if len(job.IExcludes) > 0 {
		os.WriteFile(tmp+".iexclude", []byte(patternFile(job.IExcludes)), 0o666)
		args = append(args, "--iexclude-file", tmp+".iexclude")
	}
	if job.ExcludeCaches {
		args = append(args, "--exclude-caches")
	}
	if job.ExcludeLargerThan != "" {
		args = append(args, "--exclude-larger-than", job.ExcludeLargerThan)
	}
	if job.OneFileSystem {
		args = append(args, "--one-file-system")
	}
	if job.Advanced.ReadConcurrency > 0 {
		args = append(args, "--read-concurrency", strconv.Itoa(job.Advanced.ReadConcurrency))
	}
	if job.Advanced.SkipIfUnchanged {
		args = append(args, "--skip-if-unchanged")
	}
	args = append(args, job.Advanced.ExtraFlags...)
	args = append(args, "--retry-lock", "2m", "--")
	return append(args, job.Sources...)
}

func forgetArgs(job Job, dryRun bool) []string {
	args := []string{"forget", "--json", "--tag", "job:" + job.ID, "--group-by", ""}
	ret := job.Retention
	keep := []struct {
		k string
		v int
	}{{"last", ret.Last}, {"hourly", ret.Hourly}, {"daily", ret.Daily}, {"weekly", ret.Weekly},
		{"monthly", ret.Monthly}, {"yearly", ret.Yearly}}
	for _, kv := range keep {
		if kv.v > 0 {
			args = append(args, "--keep-"+kv.k, strconv.Itoa(kv.v))
		}
	}
	if ret.Within != "" {
		args = append(args, "--keep-within", ret.Within)
	}
	if dryRun {
		args = append(args, "--dry-run")
	} else {
		args = append(args, "--retry-lock", "2m")
	}
	return args
}
