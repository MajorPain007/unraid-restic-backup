package rb

import (
	"encoding/json"
	"os"
	"strings"
)

func init() {
	registerAction("job_save", apiJobSave)
	registerAction("job_preview", apiJobPreview)
	registerAction("job_tree", apiJobTree)
	registerAction("job_preview_status", apiJobPreviewStatus)
	registerAction("job_preview_stop", apiJobPreviewStop)
	registerAction("job_delete", apiJobDelete)
	registerAction("job_run", apiJobRun)
	registerAction("job_forget", apiJobForget)
	registerAction("retention_preview", apiRetentionPreview)
}

func apiJobSave(p M) (M, error) {
	in, err := pjsonMap(p, "job")
	if err != nil {
		return nil, err
	}
	return configChange(func(cfg *Config) (M, error) {
		errs := []string{}
		if !phpTruthy(in["id"]) {
			in["id"] = newID("j")
		} else if cfg.jobIndex(str(in["id"])) < 0 {
			return nil, apiFail("That job does not exist (any more).")
		}
		job := cleanJob(in, cfg.repoIDs(), &errs)
		errs = append(errs, checkJobAgainstRepos(job, cfg)...)
		if len(errs) > 0 {
			return nil, apiFail("Please correct the marked fields.", errs...)
		}
		if i := cfg.jobIndex(job.ID); i < 0 {
			cfg.Jobs = append(cfg.Jobs, job)
		} else {
			cfg.Jobs[i] = job
		}
		return M{"job": job}, nil
	})
}

func jobRepoOrEmpty(cfg *Config, id string) Repo {
	if r := cfg.repo(id); r != nil {
		return *r
	}
	return Repo{}
}

func apiJobPreview(p M) (M, error) {
	cfg := configLoad()
	in, err := pjsonMap(p, "job")
	if err != nil {
		return nil, err
	}
	errs := []string{}
	job := cleanJob(in, cfg.repoIDs(), &errs)
	if len(job.Sources) == 0 {
		return nil, apiFail("Add a folder to back up first.")
	}
	os.MkdirAll(PreviewDir, 0o755)
	previewPrune()
	id := newOpID()
	jsonWrite(previewFilePath(id, ".request.json"), previewRequest{Job: job, Repo: jobRepoOrEmpty(cfg, job.Repo),
		Settings: cfg.Settings}, 0o644)
	spawn("preview", id)
	return M{"id": id}, nil
}

func apiJobTree(p M) (M, error) {
	cfg := configLoad()
	in, err := pjsonMap(p, "job")
	if err != nil {
		return nil, err
	}
	errs := []string{}
	job := cleanJob(in, cfg.repoIDs(), &errs)
	repo := jobRepoOrEmpty(cfg, job.Repo)
	path := pstr(p, "path", "")
	src := ""
	if path != "" {
		if path[0] == '/' {
			path = pathClean(path)
		} else {
			path = ""
		}
		if path != "" {
			src = previewSourceOf(job, path)
		}
		if src == "" || path == "" {
			return nil, apiFail("That folder is not one this job backs up.")
		}
		for _, seg := range strings.Split(path[len(src):], "/") {
			if seg == ".zfs" {
				return nil, apiFail("ZFS snapshot folders are not opened here.")
			}
		}
	}
	out, err := previewFolder(job, repo, cfg.Settings, src, path, 500)
	if err != nil {
		return nil, apiFail(err.Error())
	}
	return out, nil
}

func apiJobPreviewStatus(p M) (M, error) {
	id := pstr(p, "id", "")
	if !rePreviewID.MatchString(id) {
		return nil, apiFail("There is no such preview.")
	}
	s, ok := jsonRead(previewFilePath(id, ".json"))
	if !ok || len(s) == 0 {
		if exists(previewFilePath(id, ".request.json")) {
			return M{"status": "starting"}, nil
		}
		return nil, apiFail("The preview is gone - start it again.")
	}
	if s["status"] == "running" && !pidAlive(s["pid"]) {
		s = M{"status": "error", "error": "The preview stopped unexpectedly."}
	}
	return s, nil
}

func apiJobPreviewStop(p M) (M, error) {
	id := pstr(p, "id", "")
	if rePreviewID.MatchString(id) && exists(previewFilePath(id, ".json")) {
		if f, err := os.Create(previewFilePath(id, ".cancel")); err == nil {
			f.Close()
		}
	}
	return M{}, nil
}

func apiJobDelete(p M) (M, error) {
	id := pstr(p, "id", "")
	var settings Settings
	result, err := configChange(func(cfg *Config) (M, error) {
		job, err := needJob(cfg, id)
		if err != nil {
			return nil, err
		}
		i := cfg.jobIndex(id)
		cfg.Jobs = append(cfg.Jobs[:i], cfg.Jobs[i+1:]...)
		settings = cfg.Settings
		return M{"deleted": job.Name}, nil
	})
	if err != nil {
		return nil, err
	}
	stateUpdate(settings, func(state M) {
		delete(mapOf(state["jobs"]), id)
	})
	return result, nil
}

func apiJobRun(p M) (M, error) {
	cfg := configLoad()
	job, err := needJob(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	repo, err := needRepo(cfg, job.Repo)
	if err != nil {
		return nil, err
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	if !arrayStarted() {
		return nil, apiFail("The array is not started.")
	}
	for _, op := range opsRunning() {
		if str(op["job"]) == job.ID {
			return nil, apiFail("This job is running already.")
		}
	}
	if repoBusy(repo.ID) {
		stateUpdate(cfg.Settings, func(state M) {
			sub(state, "jobs", job.ID)["queued"] = now()
		})
		return M{"queued": true}, nil
	}
	spawn("run", "backup", job.ID, "manual")
	return M{"started": true}, nil
}

func apiJobForget(p M) (M, error) {
	cfg := configLoad()
	job, err := needJob(cfg, param(p, "id", ""))
	if err != nil {
		return nil, err
	}
	repo, err := needRepo(cfg, job.Repo)
	if err != nil {
		return nil, err
	}
	if err := resticReady(cfg, repo); err != nil {
		return nil, err
	}
	if repoBusy(repo.ID) {
		return nil, apiFail("An operation on this repository is running. Try again when it has finished.")
	}
	spawn("run", "forget", job.ID, "manual")
	return M{"started": true}, nil
}

func apiRetentionPreview(p M) (M, error) {
	cfg := configLoad()
	in, err := pjsonMap(p, "job")
	if err != nil {
		return nil, err
	}
	job, err := needJob(cfg, get(in, "id", ""))
	if err != nil {
		return nil, err
	}
	merged := toMap(*job)
	merged["retention"] = get(in, "retention", []any{})
	errs := []string{}
	preview := cleanJob(merged, cfg.repoIDs(), &errs)
	repo, err := needRepo(cfg, job.Repo)
	if err != nil {
		return nil, err
	}
	args := append(forgetArgs(preview, true), "--no-lock")
	out, err := apiRestic(cfg, repo, args, 200000)
	if err != nil {
		return nil, err
	}
	var groups []any
	json.Unmarshal([]byte(strings.Join(out, "\n")), &groups)
	keep, remove := []M{}, []M{}
	for _, gv := range groups {
		g := mapOf(gv)
		reasons := map[string]any{}
		for _, rv := range listOf(g["reasons"]) {
			r := mapOf(rv)
			m := r["matches"]
			if m == nil {
				m = []any{}
			}
			reasons[str(mapOf(r["snapshot"])["id"])] = m
		}
		for _, sv := range listOf(g["keep"]) {
			s := mapOf(sv)
			why, ok := reasons[str(s["id"])]
			if !ok {
				why = []any{}
			}
			keep = append(keep, M{"id": s["id"], "time": s["time"], "reasons": why})
		}
		for _, sv := range listOf(g["remove"]) {
			s := mapOf(sv)
			remove = append(remove, M{"id": s["id"], "time": s["time"]})
		}
	}
	return M{"keep": keep, "remove": remove}, nil
}

func listOf(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}
