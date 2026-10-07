package model

import (
	"regexp"
	"strings"
)

// keywordTail matches a "name=" that the cursor sits just after.
var keywordTail = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*$`)

// suppliedKeyword matches each "name=" already written in the call.
var suppliedKeyword = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=`)

// TextCallSite recovers the enclosing call from the raw text before the cursor.
//
// Completion runs on a document the user is halfway through typing, where
// `trigger_mode=` has no closing paren and so no AST. Scanning the text is
// less precise than the AST but it does not need the file to parse, so it is
// the fallback whenever CallSiteAt finds nothing.
func TextCallSite(before string) (CallSite, bool) {
	open := unclosedParen(before)
	if open < 0 {
		return CallSite{}, false
	}
	callee := dottedNameEndingAt(before, open)
	if callee == "" {
		return CallSite{}, false
	}
	args := before[open+1:]

	site := CallSite{Callee: callee, Supplied: map[string]bool{}}
	for _, m := range suppliedKeyword.FindAllStringSubmatch(args, -1) {
		site.Supplied[m[1]] = true
	}
	segments := splitTopLevel(args)
	site.ArgIndex = len(segments) - 1
	current := segments[len(segments)-1]

	if m := keywordTail.FindStringSubmatch(current); m != nil {
		site.Keyword = m[1]
		site.InKeywordName = false
		// The keyword being typed is not yet a supplied one.
		delete(site.Supplied, m[1])
		site.Supplied[m[1]] = true
		return site, true
	}
	if name := strings.TrimSpace(current); name != "" && !strings.ContainsAny(name, "'\"[](){}") {
		if suppliedKeyword.MatchString(current) {
			site.Keyword = lastKeyword(current)
		}
	}
	return site, true
}

// unclosedParen finds the "(" that is still open at the end of the text.
func unclosedParen(text string) int {
	depth := 0
	inStr := byte(0)
	last := -1
	stack := []int{}
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case inStr != 0:
			if c == inStr {
				inStr = 0
			}
		case c == '\'' || c == '"':
			inStr = c
		case c == '#':
			// Skip to end of line; a comment cannot open a call.
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '(':
			stack = append(stack, i)
			depth++
		case c == ')':
			if depth > 0 {
				stack = stack[:len(stack)-1]
				depth--
			}
		}
	}
	if len(stack) > 0 {
		last = stack[len(stack)-1]
	}
	return last
}

// dottedNameEndingAt reads the identifier immediately before an index.
func dottedNameEndingAt(text string, at int) string {
	end := at
	for end > 0 && (text[end-1] == ' ' || text[end-1] == '\t') {
		end--
	}
	start := end
	for start > 0 {
		c := text[start-1]
		if c == '.' || c == '_' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			start--
			continue
		}
		break
	}
	return strings.Trim(text[start:end], ".")
}

// splitTopLevel splits an argument list on commas outside brackets and strings.
// The last element is whatever the cursor is currently inside.
func splitTopLevel(args string) []string {
	var out []string
	depth := 0
	inStr := byte(0)
	cur := strings.Builder{}
	for i := 0; i < len(args); i++ {
		c := args[i]
		switch {
		case inStr != 0:
			if c == inStr {
				inStr = 0
			}
		case c == '\'' || c == '"':
			inStr = c
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	out = append(out, cur.String())
	return out
}

func lastKeyword(segment string) string {
	ms := suppliedKeyword.FindAllStringSubmatch(segment, -1)
	if len(ms) == 0 {
		return ""
	}
	return ms[len(ms)-1][1]
}
