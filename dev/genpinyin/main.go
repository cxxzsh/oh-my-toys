// Command genpinyin writes internal/core/pinyin_table.go, the tone-less
// pinyin readings of every Han character the app can show in a tool name or
// quick action title. The command palette finds tools by pinyin, so the
// readings are needed, but the dictionary behind them is about 3 MB of heap
// built at start, and linking it into the app only for that is wasteful.
// This is the only importer of go-pinyin.
//
// The Han characters come from the catalogs and from the string literals of
// the non-test Go files under internal/tools, where some tools name
// themselves in Go rather than in the catalogs.
//
//	go run ./dev/genpinyin            (or: go generate ./internal/core)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

const (
	catalogDir = "internal/i18n"
	toolsDir   = "internal/tools"
	outFile    = "internal/core/pinyin_table.go"
)

// catalogs are the message files whose Han characters the table covers.
var catalogs = []string{"en.json", "zh-Hans.json"}

func main() {
	root, err := repoRoot()
	if err != nil {
		log.Fatal(err)
	}
	runes, err := hanRunes(root)
	if err != nil {
		log.Fatal(err)
	}
	src, err := render(runes)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, outFile), src, 0o644); err != nil { //nolint:gosec // G306: a generated source file is meant to be read by the toolchain and by people
		log.Fatal(err)
	}
}

// repoRoot walks up from the working directory to the one holding go.mod, so
// the command works from the repository root and from a package directory
// (go generate).
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// hanRunes returns the distinct Han characters of the catalog messages and of
// the tool string literals, in rune order.
func hanRunes(root string) ([]rune, error) {
	seen := map[rune]bool{}
	for _, name := range catalogs {
		msgs, err := messages(filepath.Join(root, catalogDir, name))
		if err != nil {
			return nil, err
		}
		for _, msg := range msgs {
			addHan(seen, msg)
		}
	}
	if err := addToolNames(seen, filepath.Join(root, toolsDir)); err != nil {
		return nil, err
	}
	runes := slices.Collect(maps.Keys(seen))
	slices.Sort(runes)
	return runes, nil
}

// messages returns the message values of a catalog. Keys starting with "@"
// are metadata (the locale, descriptions), not messages.
func messages(path string) ([]string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is one of the repository's own catalogs, built by this generator
	if err != nil {
		return nil, err
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var msgs []string
	for key, value := range entries {
		if strings.HasPrefix(key, "@") {
			continue
		}
		var msg string
		if err := json.Unmarshal(value, &msg); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", path, key, err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// addToolNames adds the Han characters of the string literals of every
// non-test Go file under dir.
func addToolNames(seen map[rune]bool, dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isSource(d.Name()) {
			return nil
		}
		lits, err := literals(path)
		if err != nil {
			return err
		}
		for _, lit := range lits {
			addHan(seen, lit)
		}
		return nil
	})
}

func isSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

// literals returns the unquoted values of the string literals of a Go file.
func literals(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var lits []string
	var bad error
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			bad = fmt.Errorf("%s: %w", fset.Position(lit.Pos()), err)
			return false
		}
		lits = append(lits, s)
		return true
	})
	return lits, bad
}

func addHan(seen map[rune]bool, text string) {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			seen[r] = true
		}
	}
}

// readings is what the app used to compute at start: every reading the
// character can take, tone-less, with repeats removed.
func readings(r rune) []string {
	args := pinyin.NewArgs()
	args.Heteronym = true // 重 is zhong and chong: "plcmm" finds 批量重命名
	return slices.Compact(pinyin.SinglePinyin(r, args))
}

// render writes the Go source of the table, gofmt-formatted, so that running
// the command again gives byte-identical output.
func render(runes []rune) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by dev/genpinyin; DO NOT EDIT.\n\n")
	b.WriteString("package core\n\n")
	b.WriteString("// hanEntry is a Han character and its tone-less pinyin readings.\n")
	b.WriteString("type hanEntry struct {\n\tr        rune\n\treadings []string\n}\n\n")
	b.WriteString("// pinyinTable lists every Han character of the catalogs and tools in rune order.\n")
	b.WriteString("var pinyinTable = []hanEntry{\n")
	for _, r := range runes {
		fmt.Fprintf(&b, "\t{0x%04X, %s}, // %c\n", r, literal(readings(r)), r)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// literal is the Go expression for a list of readings.
func literal(rs []string) string {
	if len(rs) == 0 {
		return "nil"
	}
	quoted := make([]string, len(rs))
	for i, s := range rs {
		quoted[i] = strconv.Quote(s)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}
