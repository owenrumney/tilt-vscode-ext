package model

import (
	"go.starlark.net/syntax"
)

// CallSite is the call enclosing a cursor, and where in it the cursor sits.
type CallSite struct {
	// Callee is the qualified function name: "local_resource", "os.path.join".
	Callee string
	// ArgIndex is the 0-based position of the argument under the cursor.
	ArgIndex int
	// Keyword is the keyword name when the cursor is in a `name=value` pair.
	Keyword string
	// InKeywordName is true while the cursor is still on the keyword itself.
	InKeywordName bool
	// Supplied lists the keyword arguments already written in this call, so
	// completion can stop offering them.
	Supplied map[string]bool
}

// CallSiteAt finds the innermost call containing a position. The innermost wins
// so that link(...) inside local_resource(links=[...]) answers for link.
func CallSiteAt(file *syntax.File, line, col int) (CallSite, bool) {
	if file == nil {
		return CallSite{}, false
	}
	var best *syntax.CallExpr
	bestSpan := Span{}
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		s := callBody(call)
		if !s.contains(line, col) {
			return true
		}
		if best == nil || narrower(s, bestSpan) {
			best, bestSpan = call, s
		}
		return true
	})
	if best == nil {
		return CallSite{}, false
	}
	return describeCall(best, line, col), true
}

// callBody spans the parentheses, so a cursor on the name itself is not
// treated as being inside the argument list.
func callBody(call *syntax.CallExpr) Span {
	return Span{
		StartLine: int(call.Lparen.Line),
		StartCol:  int(call.Lparen.Col),
		EndLine:   int(call.Rparen.Line),
		EndCol:    int(call.Rparen.Col) + 1,
	}
}

func narrower(a, b Span) bool {
	if a.StartLine != b.StartLine {
		return a.StartLine > b.StartLine
	}
	return a.StartCol > b.StartCol
}

func describeCall(call *syntax.CallExpr, line, col int) CallSite {
	site := CallSite{
		Callee:   qualifiedCallee(call.Fn),
		ArgIndex: 0,
		Supplied: map[string]bool{},
	}
	for _, arg := range call.Args {
		if kw, _, ok := keywordArg(arg); ok {
			site.Supplied[kw] = true
		}
	}
	// The argument index is how many arguments end before the cursor.
	index := 0
	for _, arg := range call.Args {
		s := span(arg)
		if s.contains(line, col) {
			if kw, _, ok := keywordArg(arg); ok {
				site.Keyword = kw
				site.InKeywordName = onKeywordName(arg, line, col)
			}
			site.ArgIndex = index
			return site
		}
		if endsBefore(s, line, col) {
			index++
		}
	}
	site.ArgIndex = index
	return site
}

func onKeywordName(arg syntax.Expr, line, col int) bool {
	bin, ok := arg.(*syntax.BinaryExpr)
	if !ok {
		return false
	}
	name := span(bin.X)
	return name.contains(line, col)
}

func endsBefore(s Span, line, col int) bool {
	if s.EndLine != line {
		return s.EndLine < line
	}
	return s.EndCol <= col
}

// qualifiedCallee renders os.path.abspath as a dotted name.
func qualifiedCallee(fn syntax.Expr) string {
	switch e := fn.(type) {
	case *syntax.Ident:
		return e.Name
	case *syntax.DotExpr:
		prefix := qualifiedCallee(e.X)
		if prefix == "" {
			return e.Name.Name
		}
		return prefix + "." + e.Name.Name
	}
	return ""
}
