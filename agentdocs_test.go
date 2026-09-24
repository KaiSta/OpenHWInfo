package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The code map under .claude/ is read by every agent working in this repository
// (Codebuff, Cline, Claude Code), so its line numbers are part of the
// repository's surface. Hand-maintaining them does not work: every edit above a
// symbol shifts it, and nothing would notice. So the volatile half of the map --
// which symbols exist and where -- is generated from the source, and this test is
// what keeps the committed copy honest. `go test ./...`, which CI already runs,
// fails when the tree and the index disagree, and `make map` rewrites both files.
//
// The judgement half of the map (which file owns a behaviour, the poll chain, the
// recipes) stays hand-written in SKILL.md and docs/testing.md, and those files
// carry no line numbers at all -- names and paths only -- so they cannot rot.
const (
	agentSkillDir  = ".claude/skills/code-map"
	agentSymbolDoc = agentSkillDir + "/docs/symbol-index.md"
	agentTestDoc   = agentSkillDir + "/docs/test-index.md"

	// agentDocsGuardFile is this test. It is left out of the indexes it generates
	// because the map describes the project's own code and tests, and a file
	// listing itself leads only to a self-referential diff.
	agentDocsGuardFile = "agentdocs_test.go"
)

// updateAgentDocs rewrites the two generated indexes instead of checking them.
var updateAgentDocs = flag.Bool("agentdocs.update", false,
	"rewrite the generated code-map indexes from the source instead of checking them")

func TestAgentDocsIndexesAreCurrent(t *testing.T) {
	generated := map[string]string{
		agentSymbolDoc: renderSymbolIndex(t),
		agentTestDoc:   renderTestIndex(t),
	}

	if *updateAgentDocs {
		paths := sortedKeys(generated)
		for _, path := range paths {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create %s: %v", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte(generated[path]), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			t.Logf("wrote %s", path)
		}
		return
	}

	for _, path := range sortedKeys(generated) {
		committed, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v (run `make map`)", path, err)
		}
		if string(committed) == generated[path] {
			continue
		}
		t.Errorf("%s is stale: the tree holds symbols it does not list, or lists\n"+
			"symbols the tree no longer holds. Regenerate it with the same command CI\n"+
			"uses and commit the result:\n\n\tmake map\n", path)
	}
}

// renderSymbolIndex lists the top-level symbols of every non-test Go file and of
// the frontend module, in source order, with the line each begins on.
func renderSymbolIndex(t *testing.T) string {
	t.Helper()

	var b strings.Builder
	b.WriteString("# Symbol index (generated)\n\n")
	b.WriteString("Generated from the source by `make map`; `go test ./...` fails when this file\n")
	b.WriteString("and the tree disagree, so an edit here is lost on the next run. Line numbers\n")
	b.WriteString("for any symbol in this project come from this file, or from a search.\n\n")
	b.WriteString("Document sections are deliberately absent -- they move with every prose edit:\n")
	b.WriteString("`grep -n '^## ' README.md CODE_REVIEW.md`. Tests are listed in `test-index.md`.\n")

	source, _ := agentIndexedGoFiles(t)
	for _, path := range source {
		writeFileSymbols(t, &b, path, "### ")
	}

	js := renderFrontendSymbols(t, "website/app.js")
	b.WriteString("\n### website/app.js\n\n" + js)

	return b.String()
}

// renderTestIndex lists each test file's test functions and its fixture helpers,
// which is what a new test copies instead of inventing a second pattern.
func renderTestIndex(t *testing.T) string {
	t.Helper()

	var b strings.Builder
	b.WriteString("# Test index (generated)\n\n")
	b.WriteString("Generated from the source by `make map`; `go test ./...` fails when this file\n")
	b.WriteString("and the tree disagree. Fixture helpers are the functions below `tests:`; the\n")
	b.WriteString("patterns behind them are described in `testing.md`.\n")

	_, tests := agentIndexedGoFiles(t)
	for _, path := range tests {
		writeFileSymbols(t, &b, path, "### ")
	}
	return b.String()
}

func writeFileSymbols(t *testing.T, b *strings.Builder, path, prefix string) {
	t.Helper()

	symbols, err := readFileSymbols(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	fmt.Fprintf(b, "\n%s%s — %d lines\n", prefix, path, symbols.lines)
	for _, group := range symbols.groups {
		fmt.Fprintf(b, "- %s: %s\n", group.label, describeSymbols(group.symbols))
	}
}

func describeSymbols(symbols []namedSymbol) string {
	parts := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		parts = append(parts, fmt.Sprintf("%s %d", symbol.name, symbol.line))
	}
	return strings.Join(parts, ", ")
}

type namedSymbol struct {
	name string
	line int
}

type symbolGroup struct {
	label   string
	symbols []namedSymbol
}

type fileSymbols struct {
	lines  int
	groups []symbolGroup
}

// readFileSymbols parses one Go file and reports its top-level functions, types,
// constants and variables in source order. Test files report their test functions
// and helpers instead, since those are the symbols a reader looks for there.
func readFileSymbols(path string) (fileSymbols, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSymbols{}, err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return fileSymbols{}, err
	}

	isTest := strings.HasSuffix(path, "_test.go")
	var funcs, tests, helpers, types, consts, vars []namedSymbol

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			symbol := namedSymbol{name: funcName(d), line: fset.Position(d.Pos()).Line}
			switch {
			case !isTest:
				funcs = append(funcs, symbol)
			case isTestFunc(symbol.name):
				tests = append(tests, symbol)
			default:
				helpers = append(helpers, symbol)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					types = append(types, namedSymbol{s.Name.Name, fset.Position(s.Pos()).Line})
				case *ast.ValueSpec:
					// A var or const block declared for its doc comment rather than
					// for names has nothing to list.
					line := fset.Position(s.Pos()).Line
					target := &vars
					if d.Tok == token.CONST {
						target = &consts
					}
					for _, name := range s.Names {
						if name.Name == "_" {
							continue
						}
						*target = append(*target, namedSymbol{name.Name, line})
					}
				}
			}
		}
	}

	symbols := fileSymbols{lines: lineCount(string(data))}
	for _, group := range []symbolGroup{
		{"funcs", funcs}, {"types", types}, {"consts", consts}, {"vars", vars},
		{"tests", tests}, {"helpers", helpers},
	} {
		if len(group.symbols) > 0 {
			symbols.groups = append(symbols.groups, group)
		}
	}
	return symbols, nil
}

// funcName renders a method as receiver.method, so the index reads like the call
// sites rather than losing which type owns the method.
func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	recv := d.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

func isTestFunc(name string) bool {
	for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

var frontendDecl = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][\w$]*)`)

// renderFrontendSymbols indexes the frontend module, which has no build step and
// therefore no symbol table of its own.
func renderFrontendSymbols(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var symbols []namedSymbol
	for i, line := range strings.Split(string(data), "\n") {
		match := frontendDecl.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		symbols = append(symbols, namedSymbol{match[1], i + 1})
	}
	return "- declarations: " + describeSymbols(symbols) + "\n"
}

// agentIndexedGoFiles returns every Go file the indexes cover: the tree's own
// source and tests, ignoring hidden directories, build output and vendored code.
func agentIndexedGoFiles(t *testing.T) (source, tests []string) {
	t.Helper()

	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || name == "dist" || name == "build" || name == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == agentDocsGuardFile {
			return nil
		}
		if strings.HasSuffix(rel, "_test.go") {
			tests = append(tests, rel)
		} else {
			source = append(source, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	sort.Strings(source)
	sort.Strings(tests)
	return source, tests
}

// lineCount matches `wc -l`, so the index agrees with what a reader sees there.
func lineCount(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
