package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func find(t *testing.T, decls []decl, qualifiedName string) decl {
	t.Helper()
	for _, d := range decls {
		if qualified(d) == qualifiedName {
			return d
		}
	}
	t.Fatalf("no declaration named %q", qualifiedName)
	return decl{}
}

func TestNamespaceOf(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"api/__init__.py", ""},
		{"api/os/__init__.py", "os"},
		{"api/os/path.py", "os.path"},
		{"api/v1alpha1/__init__.py", "v1alpha1"},
		{"api/config/__init__.py", "config"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := namespaceOf("api", filepath.FromSlash(tt.path))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseParams(t *testing.T) {
	// A real local_resource parameter list: nested generics, {} and [] defaults.
	params := parseParams(`name: str, cmd: Union[str, List[str]], resource_deps: List[str] = [], env: Dict[str, str] = {}, auto_init: bool=True`)

	require.Len(t, params, 5)
	assert.Equal(t, param{Name: "name", Type: "str"}, params[0])
	assert.Equal(t, "Union[str, List[str]]", params[1].Type, "a comma inside generics is not a separator")
	assert.Equal(t, "[]", params[2].Default)
	assert.Equal(t, "{}", params[3].Default)
	assert.Equal(t, "Dict[str, str]", params[3].Type)
	assert.Equal(t, "True", params[4].Default)
}

func TestParseParamsIgnoresStarArgs(t *testing.T) {
	params := parseParams(`a: str, *, b: str, *args, **kwargs`)

	names := []string{}
	for _, p := range params {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"a", "b"}, names)
}

func TestParseParamsHandlesCommaInAStringDefault(t *testing.T) {
	params := parseParams(`sep: str = ", ", n: int = 1`)

	require.Len(t, params, 2)
	assert.Equal(t, `", "`, params[0].Default)
	assert.Equal(t, "n", params[1].Name)
}

func TestSplitDocstringTakesProseAndArgs(t *testing.T) {
	raw := `Configures one or more commands.

  By default, Tilt performs an update on ` + "``tilt up``" + `.

  Args:
    name: will be used as the new name
    cmd: command to be executed. If a string, executed with ` + "``sh -c``" + `
      on macOS, or ` + "``cmd /S /C``" + ` on Windows.
    auto_init: whether this runs on ` + "``tilt up``" + `. Defaults to ` + "``True``" + `.
`
	prose, args := splitDocstring(raw)

	assert.Contains(t, prose, "Configures one or more commands.")
	assert.Contains(t, prose, "`tilt up`", "double backticks become single")
	assert.NotContains(t, prose, "Args:")

	assert.Equal(t, "will be used as the new name", args["name"])
	assert.Contains(t, args["cmd"], "on macOS, or `cmd /S /C` on Windows.",
		"an indented continuation line joins its entry")
	assert.Contains(t, args["auto_init"], "Defaults to `True`.")
}

func TestToMarkdownConvertsTheConstructsStubsUse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"double backtick literal", "see ``cmd``", "see `cmd`"},
		{
			"relative link becomes absolute",
			"see the `Local Resource docs <local_resource.html>`_.",
			"see the [Local Resource docs](https://api.tilt.dev/local_resource.html).",
		},
		{
			"absolute link is left alone",
			"see `dockerignore <https://docs.docker.com/x>`_",
			"see [dockerignore](https://docs.docker.com/x)",
		},
		{"class role", "the :class:`TriggerMode` value", "the `TriggerMode` value"},
		{"tilde and api prefix stripped", ":class:`~api.Link` objects", "`Link` objects"},
		{"method role", "see :meth:`k8s_resource`", "see `k8s_resource`"},
		{"func role", "see :func:`probe`", "see `probe`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toMarkdown(tt.in))
		})
	}
}

// The vendored tree is the real input, so assert against it directly.
func TestParseTreeAgainstTheVendoredStubs(t *testing.T) {
	decls, version, err := parseTree("api")
	require.NoError(t, err)
	assert.Regexp(t, `^v[0-9]+\.[0-9]+`, version)
	assert.Greater(t, len(decls), 100)

	lr := find(t, decls, "local_resource")
	assert.Equal(t, "func", lr.Kind)
	assert.Equal(t, "None", lr.Returns)
	assert.Equal(t, "name", lr.Params[0].Name)
	assert.Equal(t, "str", lr.Params[0].Type)
	assert.True(t, lr.Params[0].Doc != "", "the Args: block reached the parameter")
	cmd, ok := paramNamed(lr, "cmd")
	require.True(t, ok)
	assert.Equal(t, "Union[str, List[str]]", cmd.Type)
	tm, ok := paramNamed(lr, "trigger_mode")
	require.True(t, ok)
	assert.Equal(t, "TRIGGER_MODE_AUTO", tm.Default)

	// A namespaced function.
	abspath := find(t, decls, "os.path.abspath")
	assert.Equal(t, "os.path", abspath.Namespace)
	assert.Equal(t, "str", abspath.Returns)

	// A class, a constant and a documented variable.
	assert.Equal(t, "class", find(t, decls, "Link").Kind)
	assert.Equal(t, "const", find(t, decls, "TRIGGER_MODE_AUTO").Kind)
	argv := find(t, decls, "sys.argv")
	assert.Equal(t, "var", argv.Kind)
	assert.Equal(t, "List[str]", argv.Type)
	assert.Contains(t, argv.Doc, "command line arguments")

	// k8s_resource's workload/new_name pair is what the model keys on.
	k8s := find(t, decls, "k8s_resource")
	_, hasWorkload := paramNamed(k8s, "workload")
	_, hasNewName := paramNamed(k8s, "new_name")
	assert.True(t, hasWorkload)
	assert.True(t, hasNewName)
}

func TestParseTreeRejectsATreeWithNoVersionStamp(t *testing.T) {
	_, _, err := parseTree(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Tilt version stamp")
}

func paramNamed(d decl, name string) (param, bool) {
	for _, p := range d.Params {
		if p.Name == name {
			return p, true
		}
	}
	return param{}, false
}
