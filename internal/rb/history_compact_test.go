package rb

import (
	"os"
	"strings"
	"testing"
)

func TestHistoryCompact(t *testing.T) {
	base := useTestDirs(t)
	s := defaultSettings()
	s.DataDir = base + "/data"
	s.HistoryDays = 30
	os.MkdirAll(s.DataDir, 0o755)
	cut := now() - 30*86400
	old := string(jsonEncode(M{"id": "old", "ended": cut - 60}, false))
	var b strings.Builder
	for b.Len() <= 4*1048576 {
		b.WriteString(old)
	}
	b.WriteString("not json\n")
	b.WriteString(string(jsonEncode(M{"id": "inside", "ended": cut + 60}, false)))
	os.WriteFile(historyFile(s), []byte(b.String()), 0o644)
	historyAppend(s, M{"id": "new", "ended": now()})
	data, _ := os.ReadFile(historyFile(s))
	var ids []string
	for _, r := range historyRead(s, M{}, 100) {
		ids = append(ids, str(r["id"]))
	}
	if len(data) > 1000 || strings.Join(ids, ",") != "new,inside" {
		t.Errorf("a history over 4 MiB keeps only entries within history_days: %d bytes, %q", len(data), ids)
	}

	b.Reset()
	for i := 0; i < 20005; i++ {
		b.WriteString(string(jsonEncode(M{"id": i, "ended": now()}, false)))
	}
	os.WriteFile(historyFile(s), []byte(b.String()), 0o644)
	historyCompact(s)
	data, _ = os.ReadFile(historyFile(s))
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 20000 || !strings.HasSuffix(lines[0], `"id":5}`) {
		t.Errorf("at most 20000 entries, the newest: %d, first %s", len(lines), lines[0])
	}

	os.WriteFile(historyFile(s), []byte(old), 0o644)
	historyCompact(s)
	if data, _ = os.ReadFile(historyFile(s)); len(data) != 0 {
		t.Errorf("nothing left means an empty file: %q", data)
	}
}
