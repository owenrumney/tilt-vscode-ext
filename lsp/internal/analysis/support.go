package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"go.starlark.net/syntax"
)

// The builtins that declare a resource. Kept in step with internal/model by
// hand; a new one arriving in a Tilt release is one of the things the weekly
// builtins pull request asks a reviewer to check.
var resourceBuiltins = map[string]bool{
	"local_resource":    true,
	"k8s_resource":      true,
	"dc_resource":       true,
	"k8s_custom_deploy": true,
}

// Starlark's own builtins, which Tilt's api-docs dump does not list. A corpus
// of real Tiltfiles calls `set`, so omitting these produces false reports.
var starlarkBuiltins = map[string]bool{
	"abs": true, "all": true, "any": true, "bool": true, "bytes": true,
	"chr": true, "dict": true, "dir": true, "enumerate": true, "float": true,
	"getattr": true, "hasattr": true, "hash": true, "int": true, "len": true,
	"list": true, "max": true, "min": true, "ord": true, "print": true,
	"range": true, "repr": true, "reversed": true, "set": true,
	"sorted": true, "str": true, "tuple": true, "type": true, "zip": true,
}

// boundNames returns the names a binding target introduces: a plain
// identifier, tuple unpacking, a defaulted parameter or *args and **kwargs.
func boundNames(e syntax.Expr) []string {
	switch v := e.(type) {
	case *syntax.Ident:
		return []string{v.Name}
	case *syntax.ParenExpr:
		return boundNames(v.X)
	case *syntax.UnaryExpr: // *args, **kwargs
		return boundNames(v.X)
	case *syntax.BinaryExpr: // param=default
		if v.Op == syntax.EQ {
			return boundNames(v.X)
		}
	case *syntax.TupleExpr:
		return boundNamesOf(v.List)
	case *syntax.ListExpr:
		return boundNamesOf(v.List)
	}
	return nil
}

func boundNamesOf(items []syntax.Expr) []string {
	var out []string
	for _, item := range items {
		out = append(out, boundNames(item)...)
	}
	return out
}

func rootOf(dotted string) string {
	if i := strings.Index(dotted, "."); i > 0 {
		return dotted[:i]
	}
	return dotted
}

// forEachCall visits every call in the file with its dotted callee name.
func forEachCall(file *syntax.File, fn func(*syntax.CallExpr, string)) {
	syntax.Walk(file, func(n syntax.Node) bool {
		if call, ok := n.(*syntax.CallExpr); ok {
			fn(call, calleeName(call.Fn))
		}
		return true
	})
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

func keywordArg(arg syntax.Expr) (string, syntax.Expr, bool) {
	bin, ok := arg.(*syntax.BinaryExpr)
	if !ok || bin.Op != syntax.EQ {
		return "", nil, false
	}
	id, ok := bin.X.(*syntax.Ident)
	if !ok {
		return "", nil, false
	}
	return id.Name, bin.Y, true
}

func stringLiterals(e syntax.Expr) []*syntax.Literal {
	switch v := e.(type) {
	case *syntax.Literal:
		if v.Token == syntax.STRING {
			return []*syntax.Literal{v}
		}
	case *syntax.ListExpr:
		var out []*syntax.Literal
		for _, item := range v.List {
			out = append(out, stringLiterals(item)...)
		}
		return out
	case *syntax.TupleExpr:
		var out []*syntax.Literal
		for _, item := range v.List {
			out = append(out, stringLiterals(item)...)
		}
		return out
	}
	return nil
}

func spanOf(n syntax.Node) model.Span {
	start, end := n.Span()
	return model.Span{
		StartLine: int(start.Line),
		StartCol:  int(start.Col),
		EndLine:   int(end.Line),
		EndCol:    int(end.Col),
	}
}

// dependencyGraph maps each resource name to the names it declares a
// dependency on, plus the span of each resource's own name literal.
func dependencyGraph(file *syntax.File) (map[string][]string, map[string]model.Span) {
	deps := map[string][]string{}
	spans := map[string]model.Span{}

	forEachCall(file, func(call *syntax.CallExpr, callee string) {
		if !resourceBuiltins[callee] {
			return
		}
		name, nameSpan, ok := resourceNameOf(call, callee)
		if !ok {
			return
		}
		spans[name] = nameSpan
		for _, arg := range call.Args {
			kw, value, isKeyword := keywordArg(arg)
			if !isKeyword || kw != "resource_deps" {
				continue
			}
			for _, lit := range stringLiterals(value) {
				if dep, ok := lit.Value.(string); ok && dep != "" {
					deps[name] = append(deps[name], dep)
				}
			}
		}
		if _, seen := deps[name]; !seen {
			deps[name] = nil
		}
	})
	return deps, spans
}

// resourceNameOf reads a resource's name the way model does: the keyword
// first, then the first positional argument.
func resourceNameOf(call *syntax.CallExpr, callee string) (string, model.Span, bool) {
	keywords := map[string][]string{
		"local_resource":    {"name"},
		"k8s_resource":      {"new_name", "workload"},
		"dc_resource":       {"new_name", "name"},
		"k8s_custom_deploy": {"name"},
	}[callee]

	for _, want := range keywords {
		for _, arg := range call.Args {
			kw, value, ok := keywordArg(arg)
			if !ok || kw != want {
				continue
			}
			if lit, ok := value.(*syntax.Literal); ok && lit.Token == syntax.STRING {
				if s, ok := lit.Value.(string); ok && s != "" {
					return s, spanOf(lit), true
				}
			}
		}
	}
	for _, arg := range call.Args {
		if _, _, isKeyword := keywordArg(arg); isKeyword {
			continue
		}
		if lit, ok := arg.(*syntax.Literal); ok && lit.Token == syntax.STRING {
			if s, ok := lit.Value.(string); ok && s != "" {
				return s, spanOf(lit), true
			}
		}
		return "", model.Span{}, false
	}
	return "", model.Span{}, false
}

// findCycles returns one representative cycle per strongly connected group,
// each starting at its alphabetically first member so the output is stable.
func findCycles(deps map[string][]string) [][]string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	state := map[string]int{}
	var stack []string
	var cycles [][]string
	seen := map[string]bool{}

	var visit func(string)
	visit = func(node string) {
		state[node] = grey
		stack = append(stack, node)
		for _, next := range deps[node] {
			if _, known := deps[next]; !known {
				continue // a missing resource is a different check
			}
			switch state[next] {
			case white:
				visit(next)
			case grey:
				cycles = appendCycle(cycles, seen, stack, next)
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = black
	}

	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if state[name] == white {
			visit(name)
		}
	}
	return cycles
}

// appendCycle records the stack slice from the revisited node onwards, keyed
// so the same cycle is not reported from several entry points.
func appendCycle(cycles [][]string, seen map[string]bool, stack []string, at string) [][]string {
	start := -1
	for i, n := range stack {
		if n == at {
			start = i
			break
		}
	}
	if start < 0 {
		return cycles
	}
	cycle := append([]string(nil), stack[start:]...)
	cycle = rotateToSmallest(cycle)
	key := strings.Join(cycle, "\x00")
	if seen[key] {
		return cycles
	}
	seen[key] = true
	return append(cycles, cycle)
}

func rotateToSmallest(cycle []string) []string {
	at := 0
	for i, n := range cycle {
		if n < cycle[at] {
			at = i
		}
	}
	return append(append([]string(nil), cycle[at:]...), cycle[:at]...)
}

func keysOf(m map[string]model.Occurrence) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// nearest returns the closest candidate within a small edit distance, or "".
// The threshold keeps a typo fix from suggesting an unrelated name.
func nearest(name string, candidates []string) string {
	best, bestDist := "", 0
	limit := 2
	if len(name) <= 4 {
		limit = 1
	}
	for _, c := range candidates {
		d := editDistance(name, c)
		if d == 0 || d > limit {
			continue
		}
		if best == "" || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

// Builtins that create resources from a manifest, with names this server
// cannot see: the workloads in a YAML file, or the services in a compose
// file. A Tiltfile calling any of them has an open resource set, so no check
// may report a resource as missing.
//
// Measured: four of four "unknown resource" reports across a corpus of real
// Tiltfiles came from this, including a resource_deps on a docker_compose
// service.
var manifestLoaders = map[string]bool{
	"docker_compose": true,
	"helm":           true,
	// include() runs another Tiltfile, whose resources are declared outside
	// this file.
	"include":      true,
	"k8s_yaml":     true,
	"kustomize":    true,
	"load_dynamic": true,
}

func hasManifestLoader(file *syntax.File) bool {
	found := false
	forEachCall(file, func(_ *syntax.CallExpr, callee string) {
		if manifestLoaders[callee] {
			found = true
		}
	})
	return found
}

// duplicateLocalResources reports a local_resource name used twice.
//
// Restricted to local_resource on purpose. A second k8s_resource call for a
// name that already exists is how Tilt attaches objects to an existing
// resource, not a redeclaration:
//
//	k8s_resource(workload='brigade-apiserver', new_name='apiserver')
//	k8s_resource(workload='apiserver', objects=[...])
//
// Treating that as a duplicate produced fourteen false reports in one real
// Tiltfile.
func duplicateLocalResources(file *syntax.File) []Finding {
	first := map[string]model.Span{}
	conditional := conditionalCalls(file)
	var out []Finding

	forEachCall(file, func(call *syntax.CallExpr, callee string) {
		if callee != "local_resource" || conditional[call] {
			return
		}
		name, span, ok := resourceNameOf(call, callee)
		if !ok {
			return
		}
		if earlier, seen := first[name]; seen {
			out = append(out, Finding{
				Code:     "duplicate-resource",
				Severity: Error,
				Message: fmt.Sprintf(
					"local_resource %q is already declared on line %d",
					name, earlier.StartLine),
				Span: span,
			})
			return
		}
		first[name] = span
	})
	return out
}

// conditionalCalls returns the calls that sit inside an if or else body.
//
// Two branches are alternatives, not repetitions: a Tiltfile that declares
// local_resource('apply-sample') in both arms of `if ENABLE_WEBHOOKS` runs
// exactly one of them. Reporting that as a duplicate is wrong, and static
// analysis cannot tell which arm is taken.
func conditionalCalls(file *syntax.File) map[*syntax.CallExpr]bool {
	out := map[*syntax.CallExpr]bool{}
	syntax.Walk(file, func(n syntax.Node) bool {
		ifStmt, ok := n.(*syntax.IfStmt)
		if !ok {
			return true
		}
		for _, body := range [][]syntax.Stmt{ifStmt.True, ifStmt.False} {
			for _, stmt := range body {
				syntax.Walk(stmt, func(inner syntax.Node) bool {
					if call, ok := inner.(*syntax.CallExpr); ok {
						out[call] = true
					}
					return true
				})
			}
		}
		return true
	})
	return out
}

// hasConditionalResource reports whether any resource is declared inside a
// branch, which makes the file's set of resources depend on configuration
// this server cannot evaluate.
func hasConditionalResource(file *syntax.File) bool {
	conditional := conditionalCalls(file)
	found := false
	forEachCall(file, func(call *syntax.CallExpr, callee string) {
		if resourceBuiltins[callee] && conditional[call] {
			found = true
		}
	})
	return found
}
