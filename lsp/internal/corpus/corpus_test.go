// Package corpus runs the parser and model over real Tiltfiles collected from
// public repositories. It is skipped unless TILT_CORPUS points at a directory.
//
//	TILT_CORPUS=/tmp/tiltcorpus/files go test ./internal/corpus/ -v
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/analysis"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/builtins"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/syntax"
)

func corpusDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("TILT_CORPUS")
	if dir == "" {
		t.Skip("set TILT_CORPUS to a directory of Tiltfiles")
	}
	return dir
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		// The fetch script writes a manifest beside the Tiltfiles.
		if e.IsDir() || filepath.Ext(e.Name()) == ".json" {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	require.NotEmpty(t, out)
	return out
}

// TestParseRate measures how many real Tiltfiles the parser accepts. A failure
// here means a diagnostic would be reported on valid code.
func TestParseRate(t *testing.T) {
	dir := corpusDir(t)
	paths := files(t, dir)

	var failed []string
	for _, p := range paths {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		doc := parser.Parse(p, 0, string(src))
		if doc.Err != nil {
			failed = append(failed, fmt.Sprintf("%s: %s", filepath.Base(p), doc.Err.Msg))
		}
	}
	t.Logf("parsed %d/%d", len(paths)-len(failed), len(paths))
	for _, f := range failed {
		t.Logf("  FAILED %s", f)
	}
	assert.Empty(t, failed, "the parser rejected valid Tiltfiles")
}

// TestUnknownCallees lists every function a real Tiltfile calls that is
// neither in the builtin table nor defined locally. This is the evidence an
// "unknown builtin" diagnostic needs before it can be an error.
func TestUnknownCallees(t *testing.T) {
	dir := corpusDir(t)
	paths := files(t, dir)

	unknown := map[string]int{}
	resourceCount, fileCount := 0, 0

	for _, p := range paths {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		doc := parser.Parse(p, 0, string(src))
		if doc.Err != nil {
			continue
		}
		fileCount++
		file := model.Analyse(doc.File)
		resourceCount += len(file.Resources())

		local := map[string]bool{}
		for _, s := range file.Symbols() {
			local[s.Name] = true
		}
		syntax.Walk(doc.File, func(n syntax.Node) bool {
			call, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(call.Fn)
			if name == "" || local[name] || local[rootOf(name)] {
				return true
			}
			if _, found := builtins.Lookup(name); found {
				return true
			}
			if isStarlarkBuiltin(name) || isMethodCall(call.Fn) {
				return true
			}
			unknown[name]++
			return true
		})
	}

	t.Logf("%d files parsed, %d resources found", fileCount, resourceCount)
	type pair struct {
		name  string
		count int
	}
	var sorted []pair
	for n, c := range unknown {
		sorted = append(sorted, pair{n, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].name < sorted[j].name
	})
	t.Logf("unknown callees: %d distinct", len(sorted))
	for _, p := range sorted {
		t.Logf("  %4d  %s", p.count, p.name)
	}
}

// TestResourceDetection reports how many Tiltfiles yield at least one resource
// name. A zero means the whole file names its resources dynamically.
func TestResourceDetection(t *testing.T) {
	dir := corpusDir(t)
	paths := files(t, dir)

	withNone, total := []string{}, 0
	for _, p := range paths {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		doc := parser.Parse(p, 0, string(src))
		if doc.Err != nil {
			continue
		}
		total++
		file := model.Analyse(doc.File)
		if len(file.Resources()) == 0 {
			withNone = append(withNone, filepath.Base(p))
		}
	}
	t.Logf("%d/%d files yielded at least one resource name", total-len(withNone), total)
	for _, f := range withNone {
		t.Logf("  no resources: %s", f)
	}
}

func calleeName(fn syntax.Expr) string {
	switch e := fn.(type) {
	case *syntax.Ident:
		return e.Name
	case *syntax.DotExpr:
		prefix := calleeName(e.X)
		if prefix == "" {
			return e.Name.Name
		}
		return prefix + "." + e.Name.Name
	}
	return ""
}

func rootOf(dotted string) string {
	for i := 0; i < len(dotted); i++ {
		if dotted[i] == '.' {
			return dotted[:i]
		}
	}
	return dotted
}

// isMethodCall is true for x.foo() where x is a value, not a Tilt module.
func isMethodCall(fn syntax.Expr) bool {
	dot, ok := fn.(*syntax.DotExpr)
	if !ok {
		return false
	}
	root := rootOf(calleeName(dot))
	for _, ns := range builtins.Namespaces() {
		if ns == root {
			return false
		}
	}
	return true
}

var starlarkBuiltins = map[string]bool{
	"all": true, "any": true, "bool": true, "bytes": true, "dict": true,
	"dir": true, "enumerate": true, "fail": true, "float": true,
	"getattr": true, "hasattr": true, "hash": true, "int": true, "len": true,
	"list": true, "max": true, "min": true, "print": true, "range": true,
	"repr": true, "reversed": true, "sorted": true, "str": true, "tuple": true,
	"type": true, "zip": true,
}

func isStarlarkBuiltin(name string) bool {
	return starlarkBuiltins[name]
}

// TestDiagnosticsOnRealTiltfiles counts what each check would report across
// the corpus. Every one of these files is working code in someone's project,
// so a report here is a false positive until proven otherwise. The counts are
// the evidence for each check's severity.
func TestDiagnosticsOnRealTiltfiles(t *testing.T) {
	dir := corpusDir(t)
	paths := files(t, dir)

	byCode := map[string]int{}
	examples := map[string][]string{}
	filesWithAny := 0

	for _, p := range paths {
		src, err := os.ReadFile(p)
		require.NoError(t, err)
		doc := parser.Parse(p, 0, string(src))
		if doc.Err != nil {
			continue
		}
		findings := analysis.Run(doc.File, model.Analyse(doc.File))
		if len(findings) > 0 {
			filesWithAny++
		}
		for _, f := range findings {
			byCode[f.Code]++
			if len(examples[f.Code]) < 4 {
				examples[f.Code] = append(examples[f.Code],
					fmt.Sprintf("%s:%d %s", filepath.Base(p), f.Span.StartLine, f.Message))
			}
		}
	}

	t.Logf("%d of %d files produced at least one finding", filesWithAny, len(paths))
	codes := make([]string, 0, len(byCode))
	for c := range byCode {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		t.Logf("  %-34s %d", c, byCode[c])
		for _, e := range examples[c] {
			t.Logf("      %s", e)
		}
	}
}
