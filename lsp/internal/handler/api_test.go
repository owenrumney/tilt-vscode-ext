package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cursorAfter returns the position just past needle in src.
func cursorAfter(t *testing.T, src, needle string) lsp.Position {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if c := strings.Index(line, needle); c >= 0 {
			return lsp.Position{Line: i, Character: c + len(needle)}
		}
	}
	t.Fatalf("needle %q not in source", needle)
	return lsp.Position{}
}

// cursorOn returns a position inside the first occurrence of needle.
func cursorOn(t *testing.T, src, needle string) lsp.Position {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if c := strings.Index(line, needle); c >= 0 {
			return lsp.Position{Line: i, Character: c + 1}
		}
	}
	t.Fatalf("needle %q not in source", needle)
	return lsp.Position{}
}

func labels(list *lsp.CompletionList) []string {
	out := make([]string, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, it.Label)
	}
	return out
}

func TestInitializeAdvertisesTheApiCapabilities(t *testing.T) {
	res, err := New("test").Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.Capabilities.HoverProvider)
	assert.True(t, *res.Capabilities.HoverProvider)
	require.NotNil(t, res.Capabilities.CompletionProvider)
	assert.Equal(t, []string{".", "="}, res.Capabilities.CompletionProvider.TriggerCharacters)
	require.NotNil(t, res.Capabilities.SignatureHelpProvider)
	assert.Contains(t, res.Capabilities.SignatureHelpProvider.TriggerCharacters, "(")
}

func TestHoverOnABuiltin(t *testing.T) {
	src := "local_resource('web', cmd='echo hi')\n"
	h := opened(t, uri, src)

	hover, err := h.Hover(context.Background(), &lsp.HoverParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "local_resource"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover)

	md := hover.Contents.Value()
	assert.Contains(t, md, "local_resource(name: str")
	assert.Contains(t, md, "host")
	assert.Contains(t, md, "https://api.tilt.dev/api.html#api.local_resource")
	assert.Contains(t, md, "function")
}

func TestHoverOnANamespacedBuiltin(t *testing.T) {
	src := "public = os.path.abspath('public')\n"
	h := opened(t, uri, src)

	hover, err := h.Hover(context.Background(), &lsp.HoverParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "abspath"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover, "the dotted name must resolve, not just the last part")
	assert.Contains(t, hover.Contents.Value(), "os.path.abspath(path: str) -> str")
}

func TestHoverOnAKeywordArgument(t *testing.T) {
	src := "local_resource('web', serve_cmd='python3 -m http.server')\n"
	h := opened(t, uri, src)

	hover, err := h.Hover(context.Background(), &lsp.HoverParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "serve_cmd"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, hover)

	md := hover.Contents.Value()
	assert.Contains(t, md, "serve_cmd")
	assert.Contains(t, md, "not exit", "the parameter's own documentation")
	assert.Contains(t, md, "Parameter of")
}

func TestHoverOnNothing(t *testing.T) {
	src := "x = 1\n"
	h := opened(t, uri, src)

	hover, err := h.Hover(context.Background(), &lsp.HoverParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: 0, Character: 0},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, hover)
}

func TestCompletionAtTopLevel(t *testing.T) {
	h := opened(t, uri, "\n")

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: 0, Character: 0},
		},
	})
	require.NoError(t, err)

	got := labels(list)
	for _, want := range []string{"local_resource", "k8s_resource", "k8s_yaml", "docker_build", "os", "config"} {
		assert.Contains(t, got, want)
	}
}

func TestCompletionAfterANamespaceDot(t *testing.T) {
	src := "x = os.path.\n"
	h := opened(t, uri, src)

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorAfter(t, src, "os.path."),
		},
	})
	require.NoError(t, err)

	got := labels(list)
	assert.Contains(t, got, "abspath")
	assert.Contains(t, got, "join")
	assert.NotContains(t, got, "local_resource", "a namespace does not offer the top level")
}

func TestCompletionAfterOsDotOffersThePathModule(t *testing.T) {
	src := "x = os.\n"
	h := opened(t, uri, src)

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorAfter(t, src, "os."),
		},
	})
	require.NoError(t, err)

	got := labels(list)
	assert.Contains(t, got, "getcwd")
	assert.Contains(t, got, "path", "os.path is reachable from os.")
}

func TestCompletionOfParameterNamesInsideACall(t *testing.T) {
	src := "local_resource('web', )\n"
	h := opened(t, uri, src)

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorAfter(t, src, "'web', "),
		},
	})
	require.NoError(t, err)

	got := labels(list)
	assert.Contains(t, got, "cmd")
	assert.Contains(t, got, "serve_cmd")
	assert.Contains(t, got, "resource_deps")
	assert.NotContains(t, got, "local_resource", "inside a call, parameters not builtins")

	// A required parameter sorts ahead of an optional one.
	for _, it := range list.Items {
		if it.Label == "cmd" {
			assert.Equal(t, "0cmd", it.SortText)
			assert.Contains(t, it.Detail, "required")
			assert.Equal(t, "cmd=", it.InsertText)
		}
	}
}

func TestCompletionDropsKeywordsAlreadySupplied(t *testing.T) {
	src := "local_resource('web', cmd='x', labels=['a'], )\n"
	h := opened(t, uri, src)

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorAfter(t, src, "labels=['a'], "),
		},
	})
	require.NoError(t, err)

	got := labels(list)
	assert.NotContains(t, got, "cmd", "already written")
	assert.NotContains(t, got, "labels", "already written")
	assert.Contains(t, got, "serve_cmd")
}

func TestCompletionOffersTriggerModeValues(t *testing.T) {
	src := "local_resource('web', cmd='x', trigger_mode=)\n"
	h := opened(t, uri, src)

	list, err := h.Completion(context.Background(), &lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorAfter(t, src, "trigger_mode="),
		},
	})
	require.NoError(t, err)

	assert.ElementsMatch(t,
		[]string{"TRIGGER_MODE_AUTO", "TRIGGER_MODE_MANUAL"},
		labels(list),
	)
}

func TestSignatureHelpHighlightsThePositionalArgument(t *testing.T) {
	src := "local_resource('web', )\n"
	h := opened(t, uri, src)

	help, err := h.SignatureHelp(context.Background(), &lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "'web'"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, help)
	require.Len(t, help.Signatures, 1)

	sig := help.Signatures[0]
	assert.True(t, strings.HasPrefix(sig.Label, "local_resource(name: str"))
	assert.NotEmpty(t, sig.Parameters)
	require.NotNil(t, sig.ActiveParameter)
	assert.Equal(t, 0, *sig.ActiveParameter, "the cursor is on the name")

	// Parameter labels are offsets into the signature, so the editor can
	// highlight the exact span.
	offsets, ok := sig.Parameters[0].Label.([]int)
	require.True(t, ok, "label must be an offset pair")
	assert.Equal(t, "name: str", sig.Label[offsets[0]:offsets[1]])
}

func TestSignatureHelpFollowsTheKeyword(t *testing.T) {
	src := "local_resource('web', serve_cmd='x')\n"
	h := opened(t, uri, src)

	help, err := h.SignatureHelp(context.Background(), &lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "serve_cmd"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, help)

	sym := help.Signatures[0]
	require.NotNil(t, sym.ActiveParameter)
	// serve_cmd is the 8th parameter of local_resource, not the 2nd argument.
	assert.Equal(t, "serve_cmd", paramNameAt(t, sym, *sym.ActiveParameter))
}

func TestSignatureHelpOnTheInnermostCall(t *testing.T) {
	src := "local_resource('web', cmd='x', links=[link('http://x', 'x')])\n"
	h := opened(t, uri, src)

	help, err := h.SignatureHelp(context.Background(), &lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "'http://x'"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, help)
	assert.True(t, strings.HasPrefix(help.Signatures[0].Label, "link("),
		"the inner call wins, got %q", help.Signatures[0].Label)
}

func TestSignatureHelpOutsideAnyCall(t *testing.T) {
	h := opened(t, uri, "x = 1\n")

	help, err := h.SignatureHelp(context.Background(), &lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     lsp.Position{Line: 0, Character: 4},
		},
	})
	require.NoError(t, err)
	assert.Nil(t, help)
}

func TestSignatureHelpForAUserFunctionIsSilent(t *testing.T) {
	src := "def site(name):\n    pass\n\nsite('web')\n"
	h := opened(t, uri, src)

	help, err := h.SignatureHelp(context.Background(), &lsp.SignatureHelpParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri},
			Position:     cursorOn(t, src, "'web'"),
		},
	})
	require.NoError(t, err)
	assert.Nil(t, help, "the builtin table knows nothing about a local def")
}

// paramNameAt reads back the parameter a signature highlights.
func paramNameAt(t *testing.T, sig lsp.SignatureInformation, index int) string {
	t.Helper()
	require.Less(t, index, len(sig.Parameters))
	offsets, ok := sig.Parameters[index].Label.([]int)
	require.True(t, ok)
	text := sig.Label[offsets[0]:offsets[1]]
	return strings.TrimSpace(strings.Split(text, ":")[0])
}

func TestSemanticDiagnosticsArePublished(t *testing.T) {
	h := initialized(t)

	diags := h.Diagnostics(uri, "local_resource('web', cmd='x')\nlocal_resource('web', cmd='y')\n")

	require.Len(t, diags, 1)
	assert.Equal(t, "tiltfile", diags[0].Source)
	require.NotNil(t, diags[0].Severity)
	assert.Equal(t, lsp.SeverityError, *diags[0].Severity)
	assert.Contains(t, diags[0].Message, "already declared")
	assert.Equal(t, `"duplicate-resource"`, string(diags[0].Code))
}

func TestASyntaxErrorHidesTheSemanticChecks(t *testing.T) {
	h := initialized(t)

	// The duplicate is there, but the file does not parse.
	diags := h.Diagnostics(uri, "local_resource('web', cmd='x')\nlocal_resource('web'\n")

	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "want ')'")
}

func TestCodeActionOffersTheFixFromTheDiagnostic(t *testing.T) {
	src := "local_resource('web', cmd='x')\nlocal_resource('api', cmd='y', resource_deps=['wen'])\n"
	h := opened(t, uri, src)

	diags := h.Diagnostics(uri, src)
	require.Len(t, diags, 1)
	require.NotEmpty(t, diags[0].Data, "the fix travels on the diagnostic")

	actions, err := h.CodeAction(context.Background(), &lsp.CodeActionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Context:      lsp.CodeActionContext{Diagnostics: diags},
	})
	require.NoError(t, err)
	require.Len(t, actions, 1)

	a := actions[0]
	assert.Equal(t, `Change to "web"`, a.Title)
	require.NotNil(t, a.Kind)
	assert.Equal(t, lsp.CodeActionQuickFix, *a.Kind)
	edits := a.Edit.Changes[uri]
	require.Len(t, edits, 1)
	assert.Equal(t, `'web'`, edits[0].NewText)
	assert.Equal(t, 1, edits[0].Range.Start.Line)
}

func TestCodeActionIgnoresForeignDiagnostics(t *testing.T) {
	h := initialized(t)

	actions, err := h.CodeAction(context.Background(), &lsp.CodeActionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
		Context: lsp.CodeActionContext{Diagnostics: []lsp.Diagnostic{
			{Source: "someone-else", Message: "not ours"},
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, actions)
}

func TestInitializeAdvertisesCodeActions(t *testing.T) {
	res, err := New("test").Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	assert.NotNil(t, res.Capabilities.CodeActionProvider)
}
