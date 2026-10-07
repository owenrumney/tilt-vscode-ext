// Package builtins holds the Tiltfile API table: every builtin function,
// class, variable and constant Tilt exposes to a Tiltfile.
//
// The table in builtins_gen.go is generated from the stub tree that
// `tilt dump api-docs` writes. Run `make lsp-dump` then `make lsp-gen`.
package builtins

import (
	"sort"
	"strings"
)

// Kind separates the four shapes a stub declaration takes.
type Kind int

const (
	KindFunc Kind = iota
	KindClass
	KindVar
	KindConst
)

func (k Kind) String() string {
	switch k {
	case KindFunc:
		return "function"
	case KindClass:
		return "class"
	case KindVar:
		return "variable"
	case KindConst:
		return "constant"
	}
	return "unknown"
}

// Param is one parameter of a builtin function.
type Param struct {
	Name    string
	Type    string // the Python annotation, verbatim
	Default string // "" when the parameter is required
	Doc     string // from the stub's Args: block, as markdown
}

// Required reports whether a call must supply this parameter.
func (p Param) Required() bool {
	return p.Default == ""
}

// Symbol is one entry in the Tiltfile API.
type Symbol struct {
	Name      string // "abspath"
	Namespace string // "os.path"; "" at the top level
	Kind      Kind
	Params    []Param // KindFunc only
	Returns   string
	Doc       string // markdown
	Type      string // KindVar only: the annotation
}

// Qualified is the name as a Tiltfile writes it: "os.path.abspath".
func (s Symbol) Qualified() string {
	if s.Namespace == "" {
		return s.Name
	}
	return s.Namespace + "." + s.Name
}

// Signature renders the call signature for hover and signature help.
func (s Symbol) Signature() string {
	if s.Kind != KindFunc {
		if s.Type != "" {
			return s.Qualified() + ": " + s.Type
		}
		return s.Qualified()
	}
	parts := make([]string, 0, len(s.Params))
	for _, p := range s.Params {
		parts = append(parts, p.String())
	}
	out := s.Qualified() + "(" + strings.Join(parts, ", ") + ")"
	if s.Returns != "" {
		out += " -> " + s.Returns
	}
	return out
}

// URL is the published documentation for the symbol.
func (s Symbol) URL() string {
	if s.Namespace == "" {
		return "https://api.tilt.dev/api.html#api." + s.Name
	}
	return "https://api.tilt.dev/api.html#api." + s.Namespace + "." + s.Name
}

func (p Param) String() string {
	out := p.Name
	if p.Type != "" {
		out += ": " + p.Type
	}
	if p.Default != "" {
		out += " = " + p.Default
	}
	return out
}

// Lookup finds a symbol by qualified name.
func Lookup(qualified string) (Symbol, bool) {
	s, ok := Symbols[qualified]
	return s, ok
}

// Param finds a named parameter of a function.
func (s Symbol) Param(name string) (Param, bool) {
	for _, p := range s.Params {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}

// InNamespace lists the symbols of one namespace, sorted by name. The top
// level is the empty string.
func InNamespace(namespace string) []Symbol {
	var out []Symbol
	for _, s := range Symbols {
		if s.Namespace == namespace {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Namespaces lists every namespace prefix a Tiltfile can write, sorted.
func Namespaces() []string {
	seen := map[string]bool{}
	for _, s := range Symbols {
		if s.Namespace != "" {
			seen[s.Namespace] = true
			// "os.path" implies "os" is also a prefix worth completing.
			if i := strings.Index(s.Namespace, "."); i > 0 {
				seen[s.Namespace[:i]] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for ns := range seen {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

// FunctionNames lists every function the top level exposes, sorted. The
// TextMate grammar's builtin alternation is built from this.
func FunctionNames() []string {
	return namesOfKind("", KindFunc)
}

// ClassNames lists the top-level classes, sorted.
func ClassNames() []string {
	return namesOfKind("", KindClass)
}

func namesOfKind(namespace string, kind Kind) []string {
	var out []string
	for _, s := range Symbols {
		if s.Namespace == namespace && s.Kind == kind {
			out = append(out, s.Name)
		}
	}
	sort.Strings(out)
	return out
}

// TopLevelNamespaces lists the module prefixes a Tiltfile writes before a dot,
// such as os and config, without their nested children. The TextMate grammar's
// module alternation is built from this.
func TopLevelNamespaces() []string {
	seen := map[string]bool{}
	for _, s := range Symbols {
		if s.Namespace == "" {
			continue
		}
		root := s.Namespace
		if i := strings.Index(root, "."); i > 0 {
			root = root[:i]
		}
		seen[root] = true
	}
	out := make([]string, 0, len(seen))
	for ns := range seen {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}
