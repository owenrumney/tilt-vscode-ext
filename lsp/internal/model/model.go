// Package model collects the declarations, references and resource names a
// Tiltfile defines, so the handlers can answer navigation requests.
package model

import (
	"strings"

	"go.starlark.net/syntax"
)

// Kind separates the two namespaces a Tiltfile has: Starlark identifiers, and
// Tilt resource names, which are string literals.
type Kind int

const (
	// KindSymbol is a Starlark binding: a def or an assigned variable.
	KindSymbol Kind = iota
	// KindResource is a Tilt resource name, written as a string.
	KindResource
)

// Span is a half-open range in Starlark's 1-based rune coordinates.
type Span struct {
	StartLine, StartCol int
	EndLine, EndCol     int
}

// Occurrence is one place a name is written.
type Occurrence struct {
	Name string
	Kind Kind
	Span Span
	// Decl is true where the occurrence introduces the name.
	Decl bool
}

// File is everything the navigation handlers need from one Tiltfile.
type File struct {
	// Decls maps a name to where it is declared. A name declared twice keeps
	// the first, which is what Starlark's own error message points at.
	Decls map[Kind]map[string]Occurrence
	// Uses lists every occurrence, declarations included.
	Uses []Occurrence
	// Paths are string literals that name a file, with the literal's span.
	Paths []Occurrence
	// Redeclared holds the second and later declarations of a name, which
	// Decls discards. A duplicate resource name is a Tiltfile error.
	Redeclared []Occurrence
	// Loads lists every load() module path in the file.
	Loads []Occurrence
	// NonLiteralResource is true where a resource name is not a string
	// literal, as in local_resource(name, ...) inside a helper. Any check
	// that reasons about the set of resources must stand down.
	NonLiteralResource bool
}

// LoadsExternalSymbols reports whether a load() brings in names this server
// cannot see the definitions of. An "ext://" module is fetched from outside
// the workspace, so an extension can export more than the load statement
// names.
func (f *File) LoadsExternalSymbols() bool {
	for _, l := range f.Loads {
		if strings.HasPrefix(l.Name, "ext://") {
			return true
		}
	}
	return false
}

// Closed reports whether the file's set of resource names can be trusted as
// complete. Checks that would report a missing resource require it.
//
// Any load() opens the set, not just an "ext://" one. A loaded file is
// Starlark that may call local_resource itself — a helper .star, or another
// Tiltfile pulled in for its resources — and this server does not parse it.
// Until it does, a local load is exactly as opaque as a remote one.
func (f *File) Closed() bool {
	return !f.NonLiteralResource && len(f.Loads) == 0
}

// The builtins that declare a resource, and the keyword each takes its name
// from. The first positional argument is the name when no keyword is given.
var resourceBuiltins = map[string][]string{
	"local_resource":    {"name"},
	"k8s_resource":      {"new_name", "workload"},
	"dc_resource":       {"new_name", "name"},
	"k8s_custom_deploy": {"name"},
}

// Keyword arguments whose list of strings names other resources.
var resourceRefKeywords = map[string]bool{
	"resource_deps": true,
}

// Builtins whose string arguments name a file or directory. Every string
// argument is treated as a candidate; the caller decides whether it exists.
var pathBuiltins = map[string]bool{
	"docker_build":     true,
	"helm":             true,
	"include":          true,
	"k8s_yaml":         true,
	"load":             true,
	"read_file":        true,
	"read_json":        true,
	"read_yaml":        true,
	"read_yaml_stream": true,
	"watch_file":       true,
	// os.path.* and os.getcwd-adjacent helpers.
	"abspath":  true,
	"basename": true,
	"dirname":  true,
	"exists":   true,
	"join":     true,
	"realpath": true,
}

// Keyword arguments that name a directory, on builtins that are not otherwise
// path builtins.
var pathKeywords = map[string]bool{
	"context":   true,
	"dir":       true,
	"serve_dir": true,
}

// Analyse walks a parsed Tiltfile. A nil file yields an empty result, so
// callers do not have to special-case a document that failed to parse.
func Analyse(file *syntax.File) *File {
	f := &File{
		Decls: map[Kind]map[string]Occurrence{
			KindSymbol:   {},
			KindResource: {},
		},
	}
	if file == nil {
		return f
	}
	for _, stmt := range file.Stmts {
		f.collectDecls(stmt)
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		f.collectUses(n)
		return true
	})
	return f
}

// DeclAt returns the declaration of whatever name sits at a position.
func (f *File) DeclAt(line, col int) (Occurrence, bool) {
	occ, ok := f.OccurrenceAt(line, col)
	if !ok {
		return Occurrence{}, false
	}
	decl, ok := f.Decls[occ.Kind][occ.Name]
	return decl, ok
}

// OccurrenceAt returns the name written at a position, if any.
func (f *File) OccurrenceAt(line, col int) (Occurrence, bool) {
	for _, occ := range f.Uses {
		if occ.Span.contains(line, col) {
			return occ, true
		}
	}
	return Occurrence{}, false
}

// PathAt returns the file-naming string literal at a position, if any.
func (f *File) PathAt(line, col int) (Occurrence, bool) {
	for _, p := range f.Paths {
		if p.Span.contains(line, col) {
			return p, true
		}
	}
	return Occurrence{}, false
}

// References lists every occurrence of the name at a position, including or
// excluding its declaration.
func (f *File) References(line, col int, includeDecl bool) []Occurrence {
	occ, ok := f.OccurrenceAt(line, col)
	if !ok {
		return nil
	}
	var out []Occurrence
	for _, u := range f.Uses {
		if u.Kind != occ.Kind || u.Name != occ.Name {
			continue
		}
		if u.Decl && !includeDecl {
			continue
		}
		out = append(out, u)
	}
	return out
}

// Resources lists the declared resource names in source order.
func (f *File) Resources() []Occurrence {
	return sorted(f.Decls[KindResource])
}

// Symbols lists the declared Starlark bindings in source order.
func (f *File) Symbols() []Occurrence {
	return sorted(f.Decls[KindSymbol])
}

func (f *File) collectDecls(stmt syntax.Stmt) {
	switch s := stmt.(type) {
	case *syntax.DefStmt:
		f.declare(KindSymbol, s.Name.Name, span(s.Name))
	case *syntax.AssignStmt:
		if s.Op == syntax.EQ {
			if id, ok := s.LHS.(*syntax.Ident); ok {
				f.declare(KindSymbol, id.Name, span(id))
			}
		}
	case *syntax.LoadStmt:
		f.collectLoad(s)
		for _, to := range s.To {
			f.declare(KindSymbol, to.Name, span(to))
		}
	}
}

func (f *File) collectUses(n syntax.Node) {
	switch e := n.(type) {
	case *syntax.Ident:
		f.use(Occurrence{
			Name: e.Name,
			Kind: KindSymbol,
			Span: span(e),
			Decl: f.isDeclSpan(KindSymbol, e.Name, span(e)),
		})
	case *syntax.CallExpr:
		f.collectCall(e)
	}
}

func (f *File) collectCall(call *syntax.CallExpr) {
	name, ok := calleeName(call.Fn)
	if !ok {
		return
	}
	if keywords, isResource := resourceBuiltins[name]; isResource {
		f.collectResourceName(call, keywords)
	}
	isPathBuiltin := pathBuiltins[name]
	for _, arg := range call.Args {
		kw, value, isKeyword := keywordArg(arg)
		switch {
		case isKeyword && resourceRefKeywords[kw]:
			for _, lit := range stringLiterals(value) {
				f.use(Occurrence{
					Name: literalString(lit),
					Kind: KindResource,
					Span: span(lit),
				})
			}
		case isKeyword && pathKeywords[kw]:
			f.collectPaths(value)
		case isPathBuiltin:
			if isKeyword {
				f.collectPaths(value)
			} else {
				f.collectPaths(arg)
			}
		}
	}
	// load('./ext.star', 'fn') keeps its module path outside Args.
}

func (f *File) collectPaths(e syntax.Expr) {
	for _, lit := range stringLiterals(e) {
		name := literalString(lit)
		if name == "" {
			continue
		}
		f.Paths = append(f.Paths, Occurrence{Name: name, Span: span(lit)})
	}
}

// collectResourceName takes the name from the first matching keyword, and
// falls back to the first positional argument.
func (f *File) collectResourceName(call *syntax.CallExpr, keywords []string) {
	for _, want := range keywords {
		for _, arg := range call.Args {
			kw, value, ok := keywordArg(arg)
			if !ok || kw != want {
				continue
			}
			if lit, ok := value.(*syntax.Literal); ok && lit.Token == syntax.STRING {
				f.declareResource(lit)
				return
			}
		}
	}
	for _, arg := range call.Args {
		if _, _, isKeyword := keywordArg(arg); isKeyword {
			continue
		}
		if lit, ok := arg.(*syntax.Literal); ok && lit.Token == syntax.STRING {
			f.declareResource(lit)
		} else {
			f.NonLiteralResource = true
		}
		// Only the first positional argument can be the name.
		return
	}
}

func (f *File) declareResource(lit *syntax.Literal) {
	name := literalString(lit)
	if name == "" {
		return
	}
	f.declare(KindResource, name, span(lit))
	f.use(Occurrence{Name: name, Kind: KindResource, Span: span(lit), Decl: true})
}

func (f *File) declare(kind Kind, name string, s Span) {
	if name == "" {
		return
	}
	if _, exists := f.Decls[kind][name]; exists {
		f.Redeclared = append(f.Redeclared,
			Occurrence{Name: name, Kind: kind, Span: s, Decl: true})
		return
	}
	f.Decls[kind][name] = Occurrence{Name: name, Kind: kind, Span: s, Decl: true}
}

func (f *File) use(occ Occurrence) {
	f.Uses = append(f.Uses, occ)
}

func (f *File) isDeclSpan(kind Kind, name string, s Span) bool {
	decl, ok := f.Decls[kind][name]
	return ok && decl.Span == s
}

func (s Span) contains(line, col int) bool {
	if line != s.StartLine || line != s.EndLine {
		return line >= s.StartLine && line <= s.EndLine
	}
	return col >= s.StartCol && col <= s.EndCol
}

func span(n syntax.Node) Span {
	start, end := n.Span()
	return Span{
		StartLine: int(start.Line),
		StartCol:  int(start.Col),
		EndLine:   int(end.Line),
		EndCol:    int(end.Col),
	}
}

// calleeName handles both local_resource(...) and os.path.abspath(...).
func calleeName(fn syntax.Expr) (string, bool) {
	switch e := fn.(type) {
	case *syntax.Ident:
		return e.Name, true
	case *syntax.DotExpr:
		return e.Name.Name, true
	}
	return "", false
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

// stringLiterals flattens a value that is a string or a list of strings.
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

func literalString(lit *syntax.Literal) string {
	s, _ := lit.Value.(string)
	return s
}

func sorted(decls map[string]Occurrence) []Occurrence {
	out := make([]Occurrence, 0, len(decls))
	for _, occ := range decls {
		out = append(out, occ)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && before(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func before(a, b Occurrence) bool {
	if a.Span.StartLine != b.Span.StartLine {
		return a.Span.StartLine < b.Span.StartLine
	}
	return a.Span.StartCol < b.Span.StartCol
}

// collectLoad records the module path of a load() statement, which the parser
// keeps outside CallExpr.Args.
func (f *File) collectLoad(s *syntax.LoadStmt) {
	if s.Module != nil && s.Module.Token == syntax.STRING {
		name := literalString(s.Module)
		f.Loads = append(f.Loads, Occurrence{Name: name, Span: span(s.Module)})
		// An ext:// module is not a file on disk, so it is not a path link.
		if !strings.HasPrefix(name, "ext://") {
			f.collectPaths(s.Module)
		}
	}
}
