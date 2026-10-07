package builtins

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableIsPopulated(t *testing.T) {
	assert.Greater(t, len(Symbols), 100)
	assert.Regexp(t, `^v[0-9]+\.[0-9]+`, TiltVersion)
}

func TestLocalResourceMatchesTheDemoTiltfile(t *testing.T) {
	sym, ok := Lookup("local_resource")
	require.True(t, ok)
	assert.Equal(t, KindFunc, sym.Kind)
	assert.Equal(t, "", sym.Namespace)

	// Every keyword demo/Tiltfile uses must be in the table, or completion
	// would not offer what the project's own demo relies on.
	for _, name := range []string{
		"name", "cmd", "serve_cmd", "links", "labels",
		"allow_parallel", "resource_deps", "trigger_mode", "auto_init", "dir",
	} {
		_, found := sym.Param(name)
		assert.True(t, found, "parameter %q", name)
	}

	cmd, _ := sym.Param("cmd")
	assert.Equal(t, "Union[str, List[str]]", cmd.Type)
	assert.True(t, cmd.Required(), "cmd has no default")
	assert.NotEmpty(t, cmd.Doc)

	auto, _ := sym.Param("auto_init")
	assert.Equal(t, "True", auto.Default)
	assert.False(t, auto.Required())
}

func TestNamespacedLookup(t *testing.T) {
	sym, ok := Lookup("os.path.abspath")
	require.True(t, ok)
	assert.Equal(t, "abspath", sym.Name)
	assert.Equal(t, "os.path", sym.Namespace)
	assert.Equal(t, "os.path.abspath", sym.Qualified())
	assert.Equal(t, "str", sym.Returns)
}

func TestSignatureRendering(t *testing.T) {
	sym, _ := Lookup("os.path.abspath")
	assert.Equal(t, "os.path.abspath(path: str) -> str", sym.Signature())

	v, ok := Lookup("sys.argv")
	require.True(t, ok)
	assert.Equal(t, KindVar, v.Kind)
	assert.Equal(t, "sys.argv: List[str]", v.Signature())
}

func TestNamespacesIncludeIntermediatePrefixes(t *testing.T) {
	got := Namespaces()

	for _, want := range []string{"config", "os", "os.path", "shlex", "sys", "v1alpha1"} {
		assert.Contains(t, got, want)
	}
}

func TestInNamespaceIsSortedAndScoped(t *testing.T) {
	got := InNamespace("os.path")

	require.NotEmpty(t, got)
	names := []string{}
	for _, s := range got {
		assert.Equal(t, "os.path", s.Namespace)
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "abspath")
	assert.IsNonDecreasing(t, names)
}

func TestURLs(t *testing.T) {
	top, _ := Lookup("local_resource")
	assert.Equal(t, "https://api.tilt.dev/api.html#api.local_resource", top.URL())

	nested, _ := Lookup("os.path.abspath")
	assert.Equal(t, "https://api.tilt.dev/api.html#api.os.path.abspath", nested.URL())
}

func TestTriggerModeConstantsExist(t *testing.T) {
	for _, name := range []string{"TRIGGER_MODE_AUTO", "TRIGGER_MODE_MANUAL"} {
		sym, ok := Lookup(name)
		require.True(t, ok, name)
		assert.Equal(t, KindConst, sym.Kind)
	}
}

func TestDocsAreMarkdownNotReST(t *testing.T) {
	for name, sym := range Symbols {
		assert.NotContains(t, sym.Doc, "``", "%s: double backticks survived", name)
		assert.NotRegexp(t, ":(class|meth|func|data):`", sym.Doc, "%s: a reST role survived", name)
		assert.NotRegexp(t, "`[^`]+<[^>]+>`_", sym.Doc, "%s: a reST link survived", name)
		for _, p := range sym.Params {
			assert.NotContains(t, p.Doc, "``", "%s.%s: double backticks survived", name, p.Name)
		}
	}
}

func TestDocLinksAreAbsolute(t *testing.T) {
	// Go regexp has no negative lookahead, so the targets are extracted and
	// checked individually.
	link := regexp.MustCompile(`\]\(([^)]*)\)`)
	check := func(what, doc string) {
		for _, m := range link.FindAllStringSubmatch(doc, -1) {
			assert.True(t,
				strings.HasPrefix(m[1], "http://") || strings.HasPrefix(m[1], "https://"),
				"%s has a relative link: %s", what, m[1])
		}
	}
	for name, sym := range Symbols {
		check(name, sym.Doc)
		for _, p := range sym.Params {
			check(name+"."+p.Name, p.Doc)
		}
	}
}

// The grammar's builtin alternation is written by hand today. This test is the
// drift guard: it fails when Tilt adds or removes a builtin.
func TestGrammarBuiltinsMatchTheTable(t *testing.T) {
	const grammarPath = "../../../resources/tiltfile.tmLanguage.json"
	raw, err := os.ReadFile(grammarPath)
	if os.IsNotExist(err) {
		t.Skip("grammar not present")
	}
	require.NoError(t, err)

	var grammar struct {
		Repository map[string]struct {
			Match string `json:"match"`
		} `json:"repository"`
	}
	require.NoError(t, json.Unmarshal(raw, &grammar))

	check := func(rule string, want []string) {
		t.Helper()
		match := grammar.Repository[rule].Match
		require.NotEmpty(t, match, "rule %q missing from the grammar", rule)
		// Either group form: non-capturing "(?:a|b)", or capturing "(a|b)"
		// where the rule needs group 1 for its scope.
		inner := regexp.MustCompile(
			`\((?:\?:)?([A-Za-z0-9_]+(?:\|[A-Za-z0-9_]+)*)\)`,
		).FindStringSubmatch(match)
		require.Len(t, inner, 2, "rule %q has no alternation", rule)
		assert.ElementsMatch(t, want, strings.Split(inner[1], "|"),
			"%s is out of step with the generated table; regenerate it", rule)
	}
	check("tilt-builtin", FunctionNames())
	check("tilt-type", ClassNames())
	check("tilt-module", TopLevelNamespaces())
}
