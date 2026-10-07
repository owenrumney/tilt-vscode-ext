package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Rules in the TextMate grammar whose alternation is generated, and the kind
// of name each lists.
var grammarRules = []struct {
	rule  string
	names func([]decl) []string
}{
	{"tilt-builtin", func(d []decl) []string { return topLevelNames(d, "func") }},
	{"tilt-type", func(d []decl) []string { return topLevelNames(d, "class") }},
	{"tilt-module", namespaceNames},
}

// alternation matches the name list inside a rule's match pattern, in either
// form: a non-capturing "(?:a|b)", or a capturing "(a|b)" where the rule needs
// group 1 for its scope. Only bare names are matched, so an adjacent construct
// such as "(?=\s*\.)" is left alone.
var alternation = regexp.MustCompile(`\((?:\?:)?[A-Za-z0-9_]+(?:\|[A-Za-z0-9_]+)*\)`)

// writeGrammar rewrites the generated alternations in a TextMate grammar,
// touching only those lines so the diff stays reviewable.
//
// The grammar is edited as text rather than decoded and re-encoded: Go's JSON
// encoder sorts object keys, which would reorder the whole file on every run.
func writeGrammar(path string, decls []decl) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	changed := 0

	for _, g := range grammarRules {
		names := g.names(decls)
		if len(names) == 0 {
			return fmt.Errorf("rule %q would get an empty alternation", g.rule)
		}
		at, err := matchLineOf(lines, g.rule)
		if err != nil {
			return err
		}
		// Preserve the existing group form, because a capturing rule reads
		// group 1 and would lose its scope if it became non-capturing.
		open := "(?:"
		if existing := alternation.FindString(lines[at]); !strings.HasPrefix(existing, "(?:") {
			open = "("
		}
		want := open + strings.Join(names, "|") + ")"
		// Literal, so a "$" in a name is not read as a capture reference.
		updated := alternation.ReplaceAllLiteralString(lines[at], want)
		if updated != lines[at] {
			lines[at] = updated
			changed++
		}
	}
	if changed == 0 {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// matchLineOf finds the "match" line belonging to a named repository rule.
func matchLineOf(lines []string, rule string) (int, error) {
	start := -1
	needle := `"` + rule + `": {`
	for i, line := range lines {
		if strings.Contains(line, needle) {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, fmt.Errorf("rule %q not found in the grammar", rule)
	}
	for i := start; i < len(lines) && i < start+12; i++ {
		if strings.Contains(lines[i], `"match":`) {
			if !alternation.MatchString(lines[i]) {
				return 0, fmt.Errorf("rule %q has no alternation to replace", rule)
			}
			return i, nil
		}
	}
	return 0, fmt.Errorf("rule %q has no match pattern", rule)
}

func topLevelNames(decls []decl, kind string) []string {
	var out []string
	for _, d := range decls {
		if d.Namespace == "" && d.Kind == kind {
			out = append(out, d.Name)
		}
	}
	return sortedUnique(out)
}

// namespaceNames lists the top-level module prefixes, such as os and config.
func namespaceNames(decls []decl) []string {
	seen := map[string]bool{}
	for _, d := range decls {
		if d.Namespace == "" {
			continue
		}
		root := d.Namespace
		if i := strings.Index(root, "."); i > 0 {
			root = root[:i]
		}
		seen[root] = true
	}
	var out []string
	for ns := range seen {
		out = append(out, ns)
	}
	return sortedUnique(out)
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	// Insertion sort keeps the dependency list empty and the slices are small.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
