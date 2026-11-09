package core

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// TestPinyinTableCoversNames fails when a Han character that a tool name or a
// catalog message can show is missing from the table, which happens when a
// Chinese name is added without regenerating pinyin_table.go. It checks the
// same two sources as dev/genpinyin: the catalogs and the string literals of
// the non-test Go files under internal/tools.
func TestPinyinTableCoversNames(t *testing.T) {
	have := map[rune]bool{}
	for _, e := range pinyinTable {
		have[e.r] = true
	}
	reported := map[rune]bool{}
	check := func(source, text string) {
		for _, r := range text {
			if unicode.Is(unicode.Han, r) && !have[r] && !reported[r] {
				reported[r] = true
				t.Errorf("%c (%U) in %s is not in pinyin_table.go; run: go generate ./internal/core", r, r, source)
			}
		}
	}
	for _, name := range []string{"en.json", "zh-Hans.json"} {
		for _, msg := range catalogMessages(t, filepath.Join("..", "i18n", name)) {
			check(name, msg)
		}
	}
	for _, path := range goSources(t, filepath.Join("..", "tools")) {
		for _, lit := range stringLiterals(t, path) {
			check(path, lit)
		}
	}
}

// catalogMessages returns the message values of a catalog file. Keys starting
// with "@" are metadata, not messages.
func catalogMessages(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	var msgs []string
	for key, value := range entries {
		if strings.HasPrefix(key, "@") {
			continue
		}
		var msg string
		if err := json.Unmarshal(value, &msg); err != nil {
			t.Fatalf("%s: %s: %v", path, key, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// goSources returns the non-test Go files under dir.
func goSources(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// stringLiterals returns the unquoted values of the string literals of a Go file.
func stringLiterals(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lits []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		lits = append(lits, s)
		return true
	})
	return lits
}

// TestPinyinTableSorted checks the strict order the binary search in
// hanReadings relies on.
func TestPinyinTableSorted(t *testing.T) {
	runes := make([]rune, len(pinyinTable))
	for i, e := range pinyinTable {
		runes[i] = e.r
	}
	if !slices.IsSorted(runes) {
		t.Fatal("pinyin_table.go is not sorted by rune; run: go generate ./internal/core")
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] == runes[i-1] {
			t.Fatalf("%U is in pinyin_table.go twice", runes[i])
		}
	}
}

func TestHanReadings(t *testing.T) {
	if got := hanReadings('重'); !slices.Contains(got, "zhong") || !slices.Contains(got, "chong") {
		t.Errorf("重 readings %v, want zhong and chong", got)
	}
	if got := hanReadings('a'); got != nil {
		t.Errorf("a readings %v, want none", got)
	}
}
