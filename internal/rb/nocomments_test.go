package rb

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoComments(t *testing.T) {
	var files []string
	for _, dir := range []string{".", "../../cmd/restic-backup"} {
		m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		files = append(files, m...)
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range parsed.Comments {
			if !strings.HasPrefix(c.Text(), "go:") {
				t.Errorf("%s has a comment: %q", f, strings.TrimSpace(c.Text()))
			}
		}
	}
}
