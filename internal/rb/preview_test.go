package rb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFilterPathClean(t *testing.T) {
	cases := map[string]string{"": ".", "/": "/", "a/b/../..": ".", "../../a": "../../a", "a/../../b": "../b",
		"/../x": "/x", "a//b/./c/": "a/b/c", "**/x/": "**/x"}
	for in, want := range cases {
		if got := pathClean(in); got != want {
			t.Errorf("pathClean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterMatchCases(t *testing.T) {
	cases := []struct {
		pattern string
		lower   bool
		path    string
		want    bool
	}{
		{"*.tmp", false, "/mnt/a/b.tmp", true},
		{"*.tmp", false, "/mnt/a/b.tmp/c", true},
		{"/mnt/a", false, "/mnt/a/b", true},
		{"/mnt/a", false, "/x/mnt/a", false},
		{"mnt/a", false, "/x/mnt/a", true},
		{"**/cache", false, "/a/b/cache", true},
		{"a/**/c", false, "/x/a/c", true},
		{"a/**/c", false, "/x/a/b/b/c", true},
		{"a/*/c", false, "/x/a/c", false},
		{"[ab]", false, "/x/b", true},
		{"[^a]", false, "/a", false},
		{"[^a]", false, "/b", true},
		{"\\*", false, "/x/*", true},
		{"\\*", false, "/x/y", false},
		{"*.jpg", true, "/x/Y.JPG", true},
		{"äpfel", true, "/x/ÄPFEL", true},
	}
	for _, c := range cases {
		p := filterParse(c.pattern, c.lower)
		got := filterList([]filterPattern{p}, c.path) == 0
		if got != c.want {
			t.Errorf("%q (lower %v) on %q: got %v", c.pattern, c.lower, c.path, got)
		}
	}
	pats := []filterPattern{filterParse("*.log", false), filterParse("!important.log", false), filterParse("x*", false)}
	if got := filterList(pats, "/a/important.log"); got != -1 {
		t.Errorf("negation: got %d", got)
	}
	if got := filterList(pats, "/a/xa.log"); got != 0 {
		t.Errorf("first match wins: got %d", got)
	}
	if got := filterList(pats, "/a/ximportant.log"); got != 0 {
		t.Errorf("negation does not match: got %d", got)
	}
}

func TestPreviewStrnatcasecmp(t *testing.T) {
	in := []string{"file10", "File2", "file1", "a", "B", "file02", " c", "x 1"}
	sort.SliceStable(in, func(a, b int) bool { return strnatcasecmp(in[a], in[b]) < 0 })
	want := []string{"a", "B", " c", "file02", "file1", "File2", "file10", "x 1"}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("got %q", in)
	}
}

func TestPreviewParseBytes(t *testing.T) {
	for in, want := range map[string]int64{"1M": 1048576, " 5k": 5120, "7": 7, "2t": 2199023255552} {
		if got, ok := parseBytes(in); !ok || got != want {
			t.Errorf("%q: %d %v", in, got, ok)
		}
	}
	if _, ok := parseBytes("1.5G"); ok {
		t.Errorf("1.5G parsed")
	}
}

func resticForTest(t *testing.T) string {
	if p := os.Getenv("RB_RESTIC"); p != "" && fileExecutable(p) {
		return p
	}
	p, err := exec.LookPath("restic")
	if err != nil {
		t.Skip("restic not installed")
	}
	return p
}

func previewTestTree(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "src")
	files := []string{"a/b/c.txt", "a/x.TXT", "b/a/c.txt", "[x]/f", "x/f", "*lit/f", "?q", "!bang/f", "Äpfel/f",
		"äpfel2/f", "dot/.hidden", "back\\slash", "$HOME/f", "deep/a/b/c/d.txt", "ab", "ba", ":a]", "a]", "cache/z",
		"keep/cache.txt", "Photos/d.JPG", "Photos/e.jpg", ".zfs/snapshot/s/f", "big.bin"}
	for _, f := range files {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		data := []byte(f)
		if f == "big.bin" {
			data = bytes.Repeat([]byte("x"), 3000)
		}
		os.WriteFile(p, data, 0o644)
	}
	os.MkdirAll(filepath.Join(root, "cached/inner"), 0o755)
	os.WriteFile(filepath.Join(root, "cached/CACHEDIR.TAG"), []byte(cacheDirSig+"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "cached/inner/i"), []byte("i"), 0o644)
	os.Symlink("a/x.TXT", filepath.Join(root, "link"))
	return root
}

func TestPreviewFilterAgainstRestic(t *testing.T) {
	restic := resticForTest(t)
	root := previewTestTree(t)
	repoDir := filepath.Join(t.TempDir(), "repo")
	env := append(os.Environ(), "RESTIC_PASSWORD=test", "RESTIC_CACHE_DIR="+t.TempDir())
	init := exec.Command(restic, "init", "-r", repoDir)
	init.Env = env
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("restic init: %v %s", err, out)
	}
	cases := []struct {
		ex, iex []string
		caches  bool
		larger  string
	}{
		{ex: []string{"**/c.txt"}},
		{ex: []string{"a"}},
		{ex: []string{root + "/a"}},
		{ex: []string{"/a"}},
		{ex: []string{"a/"}},
		{ex: []string{"*.txt", "!c.txt"}},
		{ex: []string{"!c.txt", "*.txt"}},
		{ex: []string{"[ab]"}},
		{ex: []string{"[!a]*"}},
		{ex: []string{"[^a]*"}},
		{ex: []string{"\\*lit"}},
		{ex: []string{"\\[x]"}},
		{ex: []string{"[[]x]"}},
		{ex: []string{"[[:alpha:]]"}},
		{ex: []string{"[a-b]?"}},
		{ex: []string{"a/**"}},
		{ex: []string{"**/a/**/c.txt"}},
		{ex: []string{"a/*/c.txt"}},
		{ex: []string{"deep/**/d.txt"}},
		{ex: []string{"**"}},
		{ex: []string{"?q", ".hidden", "$HOME"}},
		{ex: []string{"back\\\\slash"}},
		{ex: []string{"!**/x", "x"}},
		{ex: []string{"**/cache", "Ä*"}},
		{iex: []string{"*.txt"}},
		{iex: []string{"äpfel", "*.JPG"}},
		{iex: []string{"!*.jpg", "PHOTOS"}},
		{caches: true},
		{larger: "2k"},
		{ex: []string{"*.bin"}, caches: true, larger: "1k"},
	}
	for _, c := range cases {
		job := testJob([]string{root})
		job.Excludes = append([]string{}, c.ex...)
		job.IExcludes = append([]string{}, c.iex...)
		job.ExcludeCaches = c.caches
		job.ExcludeLargerThan = c.larger
		s := Settings{DataDir: t.TempDir(), Host: "test"}
		preview := previewRun(job, testRepo("sftp", ""), s, nil, nil, true)
		got := append([]string{}, preview["files"].([]string)...)
		sort.Strings(got)

		args := backupArgs(job, testRepo("sftp", ""), s, filepath.Join(t.TempDir(), "op"))
		full := append([]string{"-r", repoDir, args[0], "--dry-run", "-vv"}, args[1:]...)
		cmd := exec.Command(restic, full...)
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("%v %v %v: restic failed: %v %s", c.ex, c.iex, c.larger, err, stderr.String())
			continue
		}
		want := []string{}
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			var j M
			if json.Unmarshal(sc.Bytes(), &j) != nil {
				continue
			}
			item := str(j["item"])
			if str(j["message_type"]) == "verbose_status" && inList(str(j["action"]), "new", "unchanged", "modified") &&
				!strings.HasSuffix(item, "/") {
				want = append(want, item)
			}
		}
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ex %q iex %q caches %v larger %q:\npreview %q\nrestic  %q", c.ex, c.iex, c.caches, c.larger, got, want)
		}

		var tree []string
		var walk func(p string)
		walk = func(p string) {
			r, err := previewFolder(job, testRepo("sftp", ""), s, previewSourceOf(job, p), p, 500)
			if err != nil {
				t.Errorf("folder %q: %v", p, err)
				return
			}
			for _, e := range r["entries"].([]M) {
				if e["open"] == true {
					walk(e["path"].(string))
				} else if e["type"] == "file" && e["out"] == nil {
					tree = append(tree, e["path"].(string))
				}
			}
		}
		walk(root)
		sort.Strings(tree)
		if tree == nil {
			tree = []string{}
		}
		if !reflect.DeepEqual(tree, want) {
			t.Errorf("folder walk ex %q iex %q: %q vs %q", c.ex, c.iex, tree, want)
		}
	}
}

func TestPreviewRunShape(t *testing.T) {
	root := previewTestTree(t)
	job := testJob([]string{root, root + "/missing"})
	job.Excludes = []string{"*.txt", "nothing"}
	job.ExcludeCaches = true
	r := previewRun(job, testRepo("sftp", ""), Settings{DataDir: "/nonexistent"}, nil, nil, false)
	if _, ok := r["files"]; ok {
		t.Errorf("file list without asking")
	}
	src := r["sources"].([]M)
	if len(src) != 2 || src[1]["missing"] != true || src[0]["tree"].(M)["name"] != "src" {
		t.Errorf("sources: %v", src)
	}
	labels := []string{}
	for _, rule := range r["rules"].([]M) {
		labels = append(labels, rule["label"].(string))
	}
	eqList(t, []string{"*.txt", "nothing", ".zfs/snapshot", "CACHEDIR.TAG"}, labels)
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"children":[`) {
		t.Errorf("no children: %s", b)
	}
	f, err := previewFolder(job, testRepo("sftp", ""), Settings{}, "", "", 500)
	if err != nil || len(f["entries"].([]M)) != 2 || f["entries"].([]M)[1]["type"] != "missing" {
		t.Errorf("top level: %v %v", f, err)
	}
	f, err = previewFolder(job, testRepo("sftp", ""), Settings{}, root, root+"/cached/inner", 500)
	if err != nil || f["entries"].([]M)[0]["out"].(M)["via"] != root+"/cached/inner" {
		t.Errorf("via: %v %v", f, err)
	}
	_, err = previewFolder(job, testRepo("sftp", ""), Settings{}, root, root+"/nope", 500)
	if err == nil || err.Error() != root+"/nope is not a folder (any more)." {
		t.Errorf("err: %v", err)
	}
	f, _ = previewFolder(job, testRepo("sftp", ""), Settings{}, root, root, 3)
	if f["more"] == 0 || len(f["entries"].([]M)) != 3 {
		t.Errorf("limit: %v", f)
	}
}
