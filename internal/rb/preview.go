package rb

import (
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var PreviewDir = RunDir + "/preview"

const (
	previewTreeDepth = 3
	previewTreeKeep  = 12
	previewNone      = -1
	previewMount     = -2
	sIFMT            = 0o170000
	sIFDIR           = 0o040000
	sIFREG           = 0o100000
	sIFLNK           = 0o120000
	cacheDirSig      = "Signature: 8a477f597d28d172789f06886806bc55"
)

type fstat struct {
	mode uint32
	size int64
	dev  uint64
}

func (st fstat) isDir() bool  { return st.mode&sIFMT == sIFDIR }
func (st fstat) isFile() bool { return st.mode&sIFMT == sIFREG }

func lstatRaw(p string) (fstat, bool) {
	var st syscall.Stat_t
	if syscall.Lstat(p, &st) != nil {
		return fstat{}, false
	}
	return fstat{mode: uint32(st.Mode), size: st.Size, dev: uint64(st.Dev)}, true
}

func pathClean(p string) string {
	if p == "" {
		return "."
	}
	rooted := p[0] == '/'
	out := []string{}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			if len(out) > 0 && out[len(out)-1] != ".." {
				out = out[:len(out)-1]
			} else if !rooted {
				out = append(out, "..")
			}
			continue
		}
		out = append(out, seg)
	}
	s := strings.Join(out, "/")
	if rooted {
		return "/" + s
	}
	if s == "" {
		return "."
	}
	return s
}

type filterPart struct {
	p      string
	simple bool
}

type filterPattern struct {
	neg   bool
	parts []filterPart
	lower bool
	rule  int
}

func filterSplit(p string) []string {
	parts := strings.Split(p, "/")
	if parts[0] == "" {
		parts[0] = "/"
	}
	return parts
}

func filterParse(pattern string, lower bool) filterPattern {
	neg := strings.HasPrefix(pattern, "!")
	body := pattern
	if neg {
		body = pattern[1:]
	}
	if lower {
		body = strings.ToLower(body)
	}
	parts := []filterPart{}
	for _, part := range filterSplit(pathClean(body)) {
		simple := !strings.ContainsAny(part, "\\[]*?")
		if part == "**" {
			part = ""
		}
		parts = append(parts, filterPart{p: part, simple: simple})
	}
	return filterPattern{neg: neg, parts: parts, lower: lower}
}

func filterMatch(parts []filterPart, strs []string) bool {
	for pos, part := range parts {
		if part.p != "" {
			continue
		}
		prefix := parts[:pos]
		suffix := parts[pos+1:]
		for i := 0; i <= len(strs)-len(parts)+1; i++ {
			np := make([]filterPart, 0, len(prefix)+i+len(suffix))
			np = append(np, prefix...)
			for k := 0; k < i; k++ {
				np = append(np, filterPart{p: "*"})
			}
			np = append(np, suffix...)
			if filterMatch(np, strs) {
				return true
			}
		}
		return false
	}
	if len(parts) == 0 {
		return len(strs) == 0
	}
	if len(parts) > len(strs) {
		return false
	}
	min := 0
	max := len(strs) - len(parts)
	if parts[0].p == "/" {
		max = 0
	} else if strs[0] == "/" {
		min = 1
	}
outer:
	for offset := max; offset >= min; offset-- {
		for i := len(parts) - 1; i >= 0; i-- {
			s := strs[offset+i]
			if parts[i].simple {
				if parts[i].p != s {
					continue outer
				}
			} else if ok, err := path.Match(parts[i].p, s); err != nil || !ok {
				continue outer
			}
		}
		return true
	}
	return false
}

func filterList(patterns []filterPattern, p string) int {
	strs := filterSplit(p)
	var lowerStrs []string
	by := -1
	for i, pat := range patterns {
		s := strs
		if pat.lower {
			if lowerStrs == nil {
				lowerStrs = filterSplit(strings.ToLower(p))
			}
			s = lowerStrs
		}
		if !filterMatch(pat.parts, s) {
			continue
		}
		if pat.neg {
			by = -1
		} else if by == -1 {
			by = i
		}
	}
	return by
}

var reBytes = regexp.MustCompile(`(?i)^(\d+)([bkmgt]?)$`)

func parseBytes(s string) (int64, bool) {
	m := reBytes.FindStringSubmatch(strings.Trim(s, " \t\n\r\x00\x0b"))
	if m == nil {
		return 0, false
	}
	unit := map[string]int64{"": 1, "b": 1, "k": 1024, "m": 1048576, "g": 1073741824, "t": 1099511627776}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n * unit[strings.ToLower(m[2])], true
}

type previewRule struct {
	kind     string
	label    string
	measure  bool
	files    int
	dirs     int
	bytes    int64
	examples []string
}

func (r *previewRule) out() M {
	return M{"kind": r.kind, "label": r.label, "measure": r.measure, "files": r.files, "dirs": r.dirs,
		"bytes": r.bytes, "examples": r.examples}
}

type previewLargest struct {
	path  string
	bytes int64
}

type previewNode struct {
	name     string
	bytes    int64
	files    int
	children []*previewNode
	more     M
	leaf     bool
}

func (n *previewNode) out() M {
	m := M{"name": n.name, "bytes": n.bytes, "files": n.files}
	if n.leaf {
		return m
	}
	children := []M{}
	for _, c := range n.children {
		children = append(children, c.out())
	}
	m["children"] = children
	if n.more != nil {
		m["more"] = n.more
	} else {
		m["more"] = nil
	}
	return m
}

type previewCtx struct {
	rules      []*previewRule
	patterns   []filterPattern
	caches     int
	size       int
	otherfs    int
	maxSize    int64
	listing    bool
	list       []string
	files      int
	dirs       int
	bytes      int64
	largest    []previewLargest
	errors     []string
	seen       int
	current    string
	lastReport float64
	progress   func(M)
	cancelled  func() bool
	stop       bool
	tagged     map[string]bool
	sourceDev  uint64
	started    time.Time
}

func previewRules(job Job, repo Repo, s Settings) *previewCtx {
	ctx := &previewCtx{caches: previewNone, size: previewNone, otherfs: previewNone, tagged: map[string]bool{}}
	add := func(kind, label string, measure bool) int {
		ctx.rules = append(ctx.rules, &previewRule{kind: kind, label: label, measure: measure, examples: []string{}})
		return len(ctx.rules) - 1
	}
	pattern := func(p string, lower bool, rule int) {
		fp := filterParse(p, lower)
		fp.rule = rule
		ctx.patterns = append(ctx.patterns, fp)
	}
	for _, p := range job.Excludes {
		pattern(p, false, add("pattern", p, true))
	}
	pattern(".zfs/snapshot", false, add("zfs", ".zfs/snapshot", false))
	for _, dir := range ownExcludes(job, repo, s) {
		pattern(dir, false, add("plugin", dir, true))
	}
	for _, p := range job.IExcludes {
		pattern(p, true, add("ipattern", p, true))
	}
	if job.ExcludeCaches {
		ctx.caches = add("caches", "CACHEDIR.TAG", true)
	}
	if job.OneFileSystem {
		ctx.otherfs = add("otherfs", "other file systems", false)
	}
	if job.ExcludeLargerThan != "" {
		if max, ok := parseBytes(job.ExcludeLargerThan); ok {
			ctx.size = add("size", job.ExcludeLargerThan, true)
			ctx.maxSize = max
		}
	}
	return ctx
}

func nowFloat() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

func previewRun(job Job, repo Repo, s Settings, progress func(M), cancelled func() bool, listFiles bool) M {
	ctx := previewRules(job, repo, s)
	ctx.listing = listFiles
	ctx.list = []string{}
	ctx.largest = []previewLargest{}
	ctx.errors = []string{}
	ctx.progress = progress
	ctx.cancelled = cancelled
	ctx.started = time.Now()
	sources := []M{}
	for _, src := range job.Sources {
		src = pathClean(src)
		entry := M{"path": src, "files": 0, "dirs": 0, "bytes": int64(0), "tree": nil, "missing": false}
		st, ok := lstatRaw(src)
		if !ok {
			entry["missing"] = true
			sources = append(sources, entry)
			continue
		}
		ctx.sourceDev = st.dev
		if rule := previewByName(src, ctx); rule != previewNone {
			previewExcluded(rule, src, st, ctx)
		} else if st.isDir() {
			files, dirs, bytes := ctx.files, ctx.dirs, ctx.bytes
			name := path.Base(src)
			if name == "/" || name == "." {
				name = src
			}
			entry["tree"] = previewWalk(src, name, 0, ctx).out()
			entry["files"] = ctx.files - files
			entry["dirs"] = ctx.dirs - dirs
			entry["bytes"] = ctx.bytes - bytes
		} else {
			entry["bytes"] = previewFile(src, st, ctx)
			if st.isFile() {
				entry["files"] = 1
			}
		}
		sources = append(sources, entry)
		if ctx.stop {
			break
		}
	}
	largest := []M{}
	for _, l := range ctx.largest {
		largest = append(largest, M{"path": l.path, "bytes": l.bytes})
	}
	rules := []M{}
	for _, r := range ctx.rules {
		if r.kind == "pattern" || r.kind == "ipattern" || r.files > 0 || r.dirs > 0 {
			rules = append(rules, r.out())
		}
	}
	errs := ctx.errors
	if len(errs) > 20 {
		errs = errs[:20]
	}
	out := M{
		"cancelled":   ctx.stop,
		"seconds":     math.Round(time.Since(ctx.started).Seconds()*10) / 10,
		"total":       M{"files": ctx.files, "dirs": ctx.dirs, "bytes": ctx.bytes},
		"sources":     sources,
		"rules":       rules,
		"largest":     largest,
		"errors":      errs,
		"error_count": len(ctx.errors),
	}
	if listFiles {
		out["files"] = ctx.list
	}
	return out
}

func previewByName(item string, ctx *previewCtx) int {
	if i := filterList(ctx.patterns, item); i != -1 {
		return ctx.patterns[i].rule
	}
	if ctx.caches != previewNone && path.Base(item) != "CACHEDIR.TAG" {
		dir := path.Dir(item)
		tagged, seen := ctx.tagged[dir]
		if !seen {
			tagged = cacheDirTagged(dir + "/CACHEDIR.TAG")
			ctx.tagged[dir] = tagged
		}
		if tagged {
			return ctx.caches
		}
	}
	return previewNone
}

func cacheDirTagged(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, len(cacheDirSig))
	n, _ := io.ReadFull(f, buf)
	return string(buf[:n]) == cacheDirSig
}

func previewByStat(item string, st fstat, ctx *previewCtx) int {
	if ctx.otherfs != previewNone && st.dev != ctx.sourceDev {
		parent, ok := lstatRaw(path.Dir(item))
		if st.isDir() && ok && parent.dev == ctx.sourceDev {
			return previewMount
		}
		return ctx.otherfs
	}
	if ctx.size != previewNone && !st.isDir() && st.size > ctx.maxSize {
		return ctx.size
	}
	return previewNone
}

func joinItem(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

func readNames(dir string) ([]string, bool) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil && len(names) == 0 {
		return nil, false
	}
	return names, true
}

func previewWalk(dir, name string, depth int, ctx *previewCtx) *previewNode {
	ctx.dirs++
	ctx.current = dir
	node := &previewNode{name: name, children: []*previewNode{}}
	names, ok := readNames(dir)
	if !ok {
		ctx.errors = append(ctx.errors, dir+": cannot be read")
		return node
	}
	sort.Strings(names)
	for _, entry := range names {
		if ctx.stop {
			break
		}
		item := joinItem(dir, entry)
		st, ok := lstatRaw(item)
		if !ok {
			continue
		}
		rule := previewByName(item, ctx)
		if rule == previewNone {
			rule = previewByStat(item, st, ctx)
		}
		if rule == previewMount {
			ctx.dirs++
			continue
		}
		if rule != previewNone {
			previewExcluded(rule, item, st, ctx)
			continue
		}
		if st.isDir() {
			child := previewWalk(item, entry, depth+1, ctx)
			node.bytes += child.bytes
			node.files += child.files
			if depth < previewTreeDepth {
				node.children = append(node.children, child)
			}
		} else if st.isFile() {
			node.bytes += previewFile(item, st, ctx)
			node.files++
		}
	}
	sort.SliceStable(node.children, func(a, b int) bool {
		return node.children[a].bytes > node.children[b].bytes
	})
	if len(node.children) > previewTreeKeep {
		rest := node.children[previewTreeKeep:]
		node.children = node.children[:previewTreeKeep]
		var sum int64
		for _, c := range rest {
			sum += c.bytes
		}
		node.more = M{"count": len(rest), "bytes": sum}
	}
	if depth >= previewTreeDepth {
		node.leaf = true
	}
	return node
}

func previewFile(item string, st fstat, ctx *previewCtx) int64 {
	if !st.isFile() {
		return 0
	}
	size := st.size
	ctx.files++
	ctx.bytes += size
	if ctx.listing {
		ctx.list = append(ctx.list, item)
	}
	if size > 0 && (len(ctx.largest) < 15 || size > ctx.largest[14].bytes) {
		ctx.largest = append(ctx.largest, previewLargest{path: item, bytes: size})
		sort.SliceStable(ctx.largest, func(a, b int) bool {
			return ctx.largest[a].bytes > ctx.largest[b].bytes
		})
		if len(ctx.largest) > 15 {
			ctx.largest = ctx.largest[:15]
		}
	}
	previewTick(ctx)
	return size
}

func previewExcluded(rule int, item string, st fstat, ctx *previewCtx) {
	r := ctx.rules[rule]
	if len(r.examples) < 5 {
		r.examples = append(r.examples, item)
	}
	if st.isFile() {
		r.files++
		r.bytes += st.size
	} else if st.isDir() {
		r.dirs++
		if r.measure {
			previewMeasure(item, r, ctx)
		}
	}
	previewTick(ctx)
}

func previewMeasure(dir string, r *previewRule, ctx *previewCtx) {
	names, ok := readNames(dir)
	if !ok {
		return
	}
	for _, e := range names {
		if ctx.stop {
			break
		}
		if e == ".zfs" {
			continue
		}
		st, ok := lstatRaw(dir + "/" + e)
		if !ok {
			continue
		}
		if st.isDir() {
			r.dirs++
			previewMeasure(dir+"/"+e, r, ctx)
		} else if st.isFile() {
			r.files++
			r.bytes += st.size
			previewTick(ctx)
		}
	}
}

func previewTick(ctx *previewCtx) {
	ctx.seen++
	if ctx.seen%500 != 0 {
		return
	}
	t := nowFloat()
	if t-ctx.lastReport < 1.0 {
		return
	}
	ctx.lastReport = t
	if ctx.cancelled != nil && ctx.cancelled() {
		ctx.stop = true
	}
	if ctx.progress != nil {
		ctx.progress(M{"files": ctx.files, "dirs": ctx.dirs, "bytes": ctx.bytes, "current": ctx.current})
	}
}

func previewSourceOf(job Job, p string) string {
	found := ""
	for _, src := range job.Sources {
		src = pathClean(src)
		if under(p, src) && (found == "" || len(src) > len(found)) {
			found = src
		}
	}
	return found
}

func previewFolder(job Job, repo Repo, s Settings, src, p string, limit int) (M, error) {
	ctx := previewRules(job, repo, s)
	out := func(rule int, via any) M {
		r := ctx.rules[rule]
		return M{"kind": r.kind, "label": r.label, "via": via}
	}
	entryOf := func(item, name string, st fstat) M {
		typ := "other"
		switch st.mode & sIFMT {
		case sIFDIR:
			typ = "dir"
		case sIFREG:
			typ = "file"
		case sIFLNK:
			typ = "symlink"
		}
		var size any
		if typ == "file" {
			size = st.size
		}
		return M{"name": name, "path": item, "type": typ, "size": size, "open": typ == "dir" && name != ".zfs",
			"out": nil, "mount": false}
	}

	if p == "" {
		entries := []M{}
		for _, sp := range job.Sources {
			sp = pathClean(sp)
			st, ok := lstatRaw(sp)
			if !ok {
				entries = append(entries, M{"name": sp, "path": sp, "type": "missing", "size": nil, "open": false,
					"out": nil, "mount": false})
				continue
			}
			e := entryOf(sp, sp, st)
			if rule := previewByName(sp, ctx); rule != previewNone {
				e["out"] = out(rule, nil)
			}
			entries = append(entries, e)
		}
		return M{"path": "", "entries": entries, "more": 0, "more_out": 0}, nil
	}

	st, ok := lstatRaw(src)
	if !ok {
		return nil, fmt.Errorf("%s does not exist.", src)
	}
	ctx.sourceDev = st.dev
	rule := previewByName(src, ctx)
	via := ""
	if rule != previewNone {
		via = src
	}
	cur := src
	rest := ""
	if len(p) > len(src) {
		rest = p[len(src):]
	}
	if rest != "" {
		for _, name := range strings.Split(strings.TrimLeft(rest, "/"), "/") {
			cur = joinItem(cur, name)
			st, ok := lstatRaw(cur)
			if !ok || !st.isDir() {
				return nil, fmt.Errorf("%s is not a folder (any more).", cur)
			}
			if via != "" {
				continue
			}
			rule = previewByName(cur, ctx)
			if rule == previewNone {
				rule = previewByStat(cur, st, ctx)
			}
			if rule == previewMount {
				rule = ctx.otherfs
			}
			if rule != previewNone {
				via = cur
			}
		}
	}

	names, ok := readNames(p)
	if !ok {
		return nil, fmt.Errorf("%s cannot be read.", p)
	}
	sort.Strings(names)
	entries := []M{}
	for _, name := range names {
		item := joinItem(p, name)
		st, ok := lstatRaw(item)
		if !ok {
			continue
		}
		e := entryOf(item, name, st)
		if via != "" {
			e["out"] = out(rule, via)
		} else {
			r := previewByName(item, ctx)
			if r == previewNone {
				r = previewByStat(item, st, ctx)
			}
			if r == previewMount {
				e["mount"] = true
			} else if r != previewNone {
				e["out"] = out(r, nil)
			}
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(a, b int) bool {
		da, db := 1, 1
		if entries[a]["type"] == "dir" {
			da = 0
		}
		if entries[b]["type"] == "dir" {
			db = 0
		}
		if da != db {
			return da < db
		}
		return strnatcasecmp(entries[a]["name"].(string), entries[b]["name"].(string)) < 0
	})
	if limit < 0 {
		limit = 0
	}
	more, moreOut := 0, 0
	if len(entries) > limit {
		for _, e := range entries[limit:] {
			more++
			if e["out"] != nil {
				moreOut++
			}
		}
		entries = entries[:limit]
	}
	return M{"path": p, "entries": entries, "more": more, "more_out": moreOut}, nil
}

func previewFilePath(id, suffix string) string {
	return PreviewDir + "/" + id + suffix
}

func previewPrune() {
	files, _ := filepath.Glob(PreviewDir + "/*")
	for _, f := range files {
		if fileMtime(f) < now()-86400 {
			os.Remove(f)
		}
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isSpace(c byte) bool { return c == ' ' || (c >= '\t' && c <= '\r') }

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

func strnatcasecmp(a, b string) int {
	if len(a) == 0 || len(b) == 0 {
		switch {
		case len(a) == len(b):
			return 0
		case len(a) > len(b):
			return 1
		}
		return -1
	}
	at := func(i int) byte {
		if i < len(a) {
			return a[i]
		}
		return 0
	}
	bt := func(i int) byte {
		if i < len(b) {
			return b[i]
		}
		return 0
	}
	ap, bp := 0, 0
	leading := true
	for {
		ca, cb := at(ap), bt(bp)
		for leading && ca == '0' && ap+1 < len(a) && isDigit(a[ap+1]) {
			ap++
			ca = at(ap)
		}
		for leading && cb == '0' && bp+1 < len(b) && isDigit(b[bp+1]) {
			bp++
			cb = bt(bp)
		}
		leading = false
		for isSpace(ca) {
			ap++
			ca = at(ap)
		}
		for isSpace(cb) {
			bp++
			cb = bt(bp)
		}
		if isDigit(ca) && isDigit(cb) {
			var result int
			if ca == '0' || cb == '0' {
				result = natCompareLeft(a, &ap, b, &bp)
			} else {
				result = natCompareRight(a, &ap, b, &bp)
			}
			if result != 0 {
				return result
			} else if ap == len(a) && bp == len(b) {
				return 0
			} else if ap == len(a) {
				return -1
			} else if bp == len(b) {
				return 1
			}
			ca, cb = at(ap), bt(bp)
		}
		ca, cb = upper(ca), upper(cb)
		if ca < cb {
			return -1
		} else if ca > cb {
			return 1
		}
		ap++
		bp++
		if ap >= len(a) && bp >= len(b) {
			return 0
		} else if ap >= len(a) {
			return -1
		} else if bp >= len(b) {
			return 1
		}
	}
}

func natCompareRight(a string, ap *int, b string, bp *int) int {
	bias := 0
	for ; ; *ap, *bp = *ap+1, *bp+1 {
		ea := *ap >= len(a) || !isDigit(a[*ap])
		eb := *bp >= len(b) || !isDigit(b[*bp])
		switch {
		case ea && eb:
			return bias
		case ea:
			return -1
		case eb:
			return 1
		case a[*ap] < b[*bp]:
			if bias == 0 {
				bias = -1
			}
		case a[*ap] > b[*bp]:
			if bias == 0 {
				bias = 1
			}
		}
	}
}

func natCompareLeft(a string, ap *int, b string, bp *int) int {
	for ; ; *ap, *bp = *ap+1, *bp+1 {
		ea := *ap >= len(a) || !isDigit(a[*ap])
		eb := *bp >= len(b) || !isDigit(b[*bp])
		switch {
		case ea && eb:
			return 0
		case ea:
			return -1
		case eb:
			return 1
		case a[*ap] < b[*bp]:
			return -1
		case a[*ap] > b[*bp]:
			return 1
		}
	}
}
