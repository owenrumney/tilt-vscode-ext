// Package analysis holds the semantic checks that run on a parsed Tiltfile.
//
// Every check here is answerable from the AST. None of them evaluates the
// Tiltfile, and each one stands down rather than guess: a false "resource not
// declared" on working code costs more than a missed report.
package analysis

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/builtins"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"go.starlark.net/syntax"
)

// Severity mirrors the LSP levels without importing the protocol, so the
// checks stay testable on their own.
type Severity int

const (
	Error Severity = iota
	Warning
	Information
	Hint
)

// Finding is one reported problem.
type Finding struct {
	// Code identifies the check, for a quick fix to match on.
	Code     string
	Severity Severity
	Message  string
	Span     model.Span
	// Fix is a replacement for the exact span, when there is an obvious one.
	Fix *Fix
}

// Fix is a single-span replacement offered as a code action.
type Fix struct {
	Title   string
	Span    model.Span
	NewText string
}

// Run executes every check against a parsed file.
func Run(file *syntax.File, m *model.File) []Finding {
	if file == nil || m == nil {
		return nil
	}
	var out []Finding
	out = append(out, duplicateLocalResources(file)...)
	out = append(out, missingResourceDeps(file, m)...)
	out = append(out, dependencyCycles(file, m)...)
	out = append(out, badPortForwards(file)...)
	out = append(out, unknownCallees(file, m)...)

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Span.StartLine != out[j].Span.StartLine {
			return out[i].Span.StartLine < out[j].Span.StartLine
		}
		return out[i].Span.StartCol < out[j].Span.StartCol
	})
	return out
}

// missingResourceDeps reports a resource_deps entry naming no known resource.
//
// Suppressed unless the file's resource set is closed: a helper that builds a
// name, or an ext:// load, makes absence unprovable.
func missingResourceDeps(file *syntax.File, m *model.File) []Finding {
	if !m.Closed() || hasManifestLoader(file) || hasConditionalResource(file) {
		return nil
	}
	declared := m.Decls[model.KindResource]
	var out []Finding
	for _, use := range m.Uses {
		if use.Kind != model.KindResource || use.Decl {
			continue
		}
		if _, found := declared[use.Name]; found {
			continue
		}
		f := Finding{
			Code:     "unknown-resource-dep",
			Severity: Error,
			Message:  fmt.Sprintf("no resource named %q is declared in this Tiltfile", use.Name),
			Span:     use.Span,
		}
		if near := nearest(use.Name, keysOf(declared)); near != "" {
			f.Fix = &Fix{
				Title:   fmt.Sprintf("Change to %q", near),
				Span:    use.Span,
				NewText: quote(near),
			}
		}
		out = append(out, f)
	}
	return out
}

// dependencyCycles reports a resource_deps cycle, which deadlocks tilt up.
func dependencyCycles(file *syntax.File, m *model.File) []Finding {
	if !m.Closed() || hasManifestLoader(file) || hasConditionalResource(file) {
		return nil
	}
	deps, spans := dependencyGraph(file)
	var out []Finding
	for _, cycle := range findCycles(deps) {
		name := cycle[0]
		span, ok := spans[name]
		if !ok {
			span = m.Decls[model.KindResource][name].Span
		}
		out = append(out, Finding{
			Code:     "resource-dep-cycle",
			Severity: Error,
			Message:  "resource_deps cycle: " + strings.Join(append(cycle, cycle[0]), " -> "),
			Span:     span,
		})
	}
	return out
}

// badPortForwards reports a port_forwards string Tilt cannot parse.
//
// Only an unambiguously wrong literal is reported. A string holding "$" or
// "{" is being built by the Tiltfile and is left alone.
func badPortForwards(file *syntax.File) []Finding {
	var out []Finding
	forEachCall(file, func(call *syntax.CallExpr, callee string) {
		for _, arg := range call.Args {
			kw, value, ok := keywordArg(arg)
			if !ok || kw != "port_forwards" {
				continue
			}
			for _, lit := range stringLiterals(value) {
				text, _ := lit.Value.(string)
				if strings.ContainsAny(text, "${}") {
					continue
				}
				if validPortForward(text) {
					continue
				}
				out = append(out, Finding{
					Code:     "bad-port-forward",
					Severity: Error,
					Message: fmt.Sprintf(
						"port_forwards %q is not local_port[:container_port] or host:local_port:container_port", text),
					Span: spanOf(lit),
				})
			}
		}
	})
	return out
}

// validPortForward accepts the forms Tilt documents: "8080",
// "9000:8080" and "localhost:9000:8080".
func validPortForward(text string) bool {
	if text == "" {
		return false
	}
	parts := strings.Split(text, ":")
	if len(parts) > 3 {
		return false
	}
	// With three parts the first is a host, which Tilt does not constrain.
	if len(parts) == 3 {
		if parts[0] == "" {
			return false
		}
		parts = parts[1:]
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}

// unknownCallees reports a call to something neither the Tilt API nor the
// file declares.
//
// A Warning, never an Error, and suppressed entirely where the file loads an
// ext:// module: 13 of the 19 unknown callees measured across a corpus of
// real Tiltfiles came from extension loads whose definitions this server
// never sees. See the corpus section of the design document.
func unknownCallees(file *syntax.File, m *model.File) []Finding {
	if m.LoadsExternalSymbols() {
		return nil
	}
	local := map[string]bool{}
	for _, s := range m.Symbols() {
		local[s.Name] = true
	}
	// A nested def, a parameter, a loop variable, a comprehension variable and
	// a lambda parameter are all local bindings.
	bind := func(e syntax.Expr) {
		for _, name := range boundNames(e) {
			local[name] = true
		}
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		switch e := n.(type) {
		case *syntax.DefStmt:
			local[e.Name.Name] = true
			for _, p := range e.Params {
				bind(p)
			}
		case *syntax.LambdaExpr:
			for _, p := range e.Params {
				bind(p)
			}
		case *syntax.ForStmt:
			bind(e.Vars)
		case *syntax.Comprehension:
			for _, clause := range e.Clauses {
				if f, ok := clause.(*syntax.ForClause); ok {
					bind(f.Vars)
				}
			}
		case *syntax.AssignStmt:
			bind(e.LHS)
		}
		return true
	})

	var out []Finding
	forEachCall(file, func(call *syntax.CallExpr, callee string) {
		if callee == "" || local[callee] || local[rootOf(callee)] {
			return
		}
		if _, found := builtins.Lookup(callee); found {
			return
		}
		if starlarkBuiltins[callee] {
			return
		}
		// Any member access that the table does not know is a method on a
		// value: os.environ.get, config.main_path.replace, "a,b".split.
		// calleeName collapses a non-identifier base, so the check has to be
		// structural rather than on the dotted text.
		if _, isMember := call.Fn.(*syntax.DotExpr); isMember {
			return
		}
		f := Finding{
			Code:     "unknown-builtin",
			Severity: Warning,
			Message:  fmt.Sprintf("%s is not a Tilt builtin (Tilt %s)", callee, builtins.TiltVersion),
			Span:     spanOf(call.Fn),
		}
		if near := nearest(callee, builtins.FunctionNames()); near != "" {
			f.Fix = &Fix{
				Title:   fmt.Sprintf("Change to %s", near),
				Span:    spanOf(call.Fn),
				NewText: near,
			}
		}
		out = append(out, f)
	})
	return out
}
