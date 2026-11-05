package rb

import "time"

const scanSettle = 20 * time.Second

type backupProgress struct {
	prevBytes  int64
	prevFiles  int64
	lastBytes  int64
	lastFiles  int64
	lastChange time.Time
	scanDone   bool
	now        func() time.Time
}

func newBackupProgress(s Settings, jobID string) *backupProgress {
	p := &backupProgress{now: time.Now}
	for _, op := range historyRead(s, M{"kind": "backup", "job": jobID}, 20) {
		if !inList(str(op["status"]), "success", "warning") {
			continue
		}
		sm, ok := op["summary"].(M)
		if !ok {
			continue
		}
		p.prevBytes = toInt64(sm["total_bytes_processed"])
		p.prevFiles = toInt64(sm["total_files_processed"])
		break
	}
	return p
}

func (p *backupProgress) update(j M) M {
	t := p.now()
	files, total := toInt64(j["total_files"]), toInt64(j["total_bytes"])
	if p.lastChange.IsZero() || files != p.lastFiles || total != p.lastBytes {
		p.lastFiles, p.lastBytes, p.lastChange = files, total, t
	}
	done := toInt64(j["bytes_done"])
	filesDone := toInt64(j["files_done"])
	if j["seconds_remaining"] != nil || toFloat(j["percent_done"]) >= 1 {
		p.scanDone = true
	}
	counting := !p.scanDone && t.Sub(p.lastChange) < scanSettle
	out := M{
		"files_done":  filesDone,
		"total_files": files,
		"bytes_done":  done,
		"total_bytes": total,
		"errors":      toInt64(j["error_count"]),
		"at":          now(),
	}
	if !counting {
		out["percent"] = toFloat(j["percent_done"])
		if j["seconds_remaining"] != nil {
			out["eta"] = toInt64(j["seconds_remaining"])
		} else {
			out["eta"] = nil
		}
		return out
	}
	out["counting"] = true
	out["eta"] = nil
	if p.prevBytes > total && p.prevBytes > 0 {
		out["total_bytes"] = p.prevBytes
		out["estimate"] = true
		if p.prevFiles > files {
			out["total_files"] = p.prevFiles
		}
		pct := float64(done) / float64(p.prevBytes)
		if pct > 0.99 {
			pct = 0.99
		}
		out["percent"] = pct
		return out
	}
	out["percent"] = nil
	return out
}
