package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every `def` and `class` at column zero in the stub tree must reach the
// table. This is the closure check: it catches a declaration shape the parser
// silently skips, which no spot-check would find.
func TestEveryDeclarationInTheDumpIsParsed(t *testing.T) {
	decls, _, err := parseTree("api")
	require.NoError(t, err)

	got := map[string]bool{}
	for _, d := range decls {
		got[qualified(d)] = true
	}

	defRe := regexp.MustCompile(`(?m)^def ([A-Za-z_][A-Za-z0-9_]*)\(`)
	classRe := regexp.MustCompile(`(?m)^class ([A-Za-z_][A-Za-z0-9_]*)\s*[:(]`)
	// A module-level annotated assignment, e.g. "argv: List[str] = []".
	varRe := regexp.MustCompile(`(?m)^([a-z_][A-Za-z0-9_]*)[ \t]*:[ \t]*[^=\n]+=`)
	constRe := regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)[ \t]*=`)
	// An unannotated, documented member: environ = Dict[str, str]
	bareRe := regexp.MustCompile(`(?m)^([a-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*[^=\n]+\n"""`)

	counts := map[string]int{}
	err = filepath.Walk("api", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".py" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ns, err := namespaceOf("api", path)
		require.NoError(t, err)

		for label, re := range map[string]*regexp.Regexp{
			"def": defRe, "class": classRe, "var": varRe, "const": constRe, "bare": bareRe,
		} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				name := m[1]
				// Dunders and the stub's own __name__ shims are not API.
				if strings.HasPrefix(name, "__") || name == "file__" {
					continue
				}
				counts[label]++
				q := name
				if ns != "" {
					q = ns + "." + name
				}
				assert.True(t, got[q], "%s %s (%s) is in the dump but not the table", label, q, path)
			}
		}
		return nil
	})
	require.NoError(t, err)

	t.Logf("dump declarations: %v", counts)
	total := 0
	for _, n := range counts {
		total += n
	}
	assert.Equal(t, total, len(decls),
		"the table has %d symbols for %d declarations in the dump", len(decls), total)
}

// Every function in the table must have usable parameter metadata, or hover
// and signature help would render an empty signature.
func TestEveryFunctionHasASignature(t *testing.T) {
	decls, _, err := parseTree("api")
	require.NoError(t, err)

	noParams := []string{}
	noDoc := []string{}
	for _, d := range decls {
		if d.Kind != "func" {
			continue
		}
		if len(d.Params) == 0 {
			noParams = append(noParams, qualified(d))
		}
		if strings.TrimSpace(d.Doc) == "" {
			noDoc = append(noDoc, qualified(d))
		}
	}
	// Some builtins genuinely take nothing, and five v1alpha1 helpers ship with
	// an empty prose section in the dump, so both are reported not asserted.
	t.Logf("functions with no parameters (%d): %v", len(noParams), noParams)
	t.Logf("functions with no prose (%d): %v", len(noDoc), noDoc)

	// What would actually be a silent hover: nothing to say at all.
	for _, d := range decls {
		if d.Kind != "func" || strings.TrimSpace(d.Doc) != "" {
			continue
		}
		documented := false
		for _, p := range d.Params {
			if strings.TrimSpace(p.Doc) != "" {
				documented = true
			}
		}
		assert.True(t, documented,
			"%s has neither prose nor a documented parameter", qualified(d))
	}
}

// TestEveryDocumentedMemberIsInTheTable is deliberately independent of the
// parser's own regexes. It finds every docstring in the stub tree and reads
// the declaration above it, so a declaration shape the parser does not know
// still shows up here.
//
// The first version of the closure check reused defRe/classRe/varRe and so
// only proved the parser agreed with itself. It passed while `os.environ`,
// an unannotated `environ = Dict[str, str]`, was missing from the table.
func TestEveryDocumentedMemberIsInTheTable(t *testing.T) {
	decls, _, err := parseTree("api")
	require.NoError(t, err)

	got := map[string]bool{}
	for _, d := range decls {
		got[qualified(d)] = true
	}

	// A name at column zero, however it is declared.
	declName := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*[=:(]`)
	keyword := regexp.MustCompile(`^(?:def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)

	checked := 0
	err = filepath.Walk("api", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".py" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ns, nsErr := namespaceOf("api", path)
		require.NoError(t, nsErr)
		lines := strings.Split(string(src), "\n")

		// Mark which lines sit inside a docstring, so that prose such as
		// "Examples:" is not mistaken for a declaration.
		inDoc := make([]bool, len(lines))
		open := false
		for i, line := range lines {
			quotes := strings.Count(line, `"""`)
			inDoc[i] = open
			if quotes%2 == 1 {
				open = !open
				// The opening line itself is a docstring line.
				inDoc[i] = true
			}
		}

		for i, line := range lines {
			if strings.Count(line, `"""`)%2 != 1 {
				continue
			}
			// Walk back to the nearest column-zero declaration outside any
			// docstring.
			name := ""
			for j := i - 1; j >= 0 && j > i-12; j-- {
				if inDoc[j] {
					continue
				}
				cand := lines[j]
				if m := keyword.FindStringSubmatch(cand); m != nil {
					name = m[1]
					break
				}
				if m := declName.FindStringSubmatch(cand); m != nil {
					name = m[1]
					break
				}
			}
			if name == "" || strings.HasPrefix(name, "__") || name == "file__" {
				continue
			}
			q := name
			if ns != "" {
				q = ns + "." + name
			}
			checked++
			assert.True(t, got[q], "%s is documented in %s but missing from the table", q, path)
		}
		return nil
	})
	require.NoError(t, err)
	t.Logf("documented members cross-checked: %d", checked)
}
