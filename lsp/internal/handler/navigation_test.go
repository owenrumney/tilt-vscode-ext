package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const navSrc = `public = os.path.abspath('public')

def site(name, port):
    local_resource(name, cmd='echo hi')

site('web', 3010)

local_resource('db-migrate', cmd='x')
local_resource('seed-data', cmd='y', resource_deps=['db-migrate'])
`

// opened returns a handler with navSrc loaded at the given URI.
func opened(t *testing.T, uri, src string) *Handler {
	t.Helper()
	h := initialized(t)
	require.NoError(t, h.DidOpen(context.Background(), &lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{URI: lsp.DocumentURI(uri), Version: 1, Text: src},
	}))
	return h
}

// at builds the params for a 0-based line and character.
func at(uri string, line, char int) lsp.TextDocumentPositionParams {
	return lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI(uri)},
		Position:     lsp.Position{Line: line, Character: char},
	}
}

func TestInitializeAdvertisesNavigation(t *testing.T) {
	h := New("test")

	res, err := h.Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.Capabilities.DefinitionProvider)
	assert.True(t, *res.Capabilities.DefinitionProvider)
	require.NotNil(t, res.Capabilities.ReferencesProvider)
	assert.True(t, *res.Capabilities.ReferencesProvider)
	assert.NotNil(t, res.Capabilities.DocumentSymbolProvider)
	assert.NotNil(t, res.Capabilities.DocumentLinkProvider)
}

func TestDefinitionOfAFunctionCall(t *testing.T) {
	h := opened(t, uri, navSrc)

	// Line 6 (0-based 5) is "site('web', 3010)"; the cursor is on "site".
	locs, err := h.Definition(context.Background(), &lsp.DefinitionParams{
		TextDocumentPositionParams: at(uri, 5, 1),
	})
	require.NoError(t, err)
	require.Len(t, locs, 1)

	// The def is on line 3 (0-based 2), at "site" after "def ".
	assert.Equal(t, 2, locs[0].Range.Start.Line)
	assert.Equal(t, 4, locs[0].Range.Start.Character)
}

func TestDefinitionOfAResourceDep(t *testing.T) {
	h := opened(t, uri, navSrc)

	// Line 9 (0-based 8) holds resource_deps=['db-migrate'].
	line := strings.Split(navSrc, "\n")[8]
	col := strings.Index(line, "'db-migrate'") + 2

	locs, err := h.Definition(context.Background(), &lsp.DefinitionParams{
		TextDocumentPositionParams: at(uri, 8, col),
	})
	require.NoError(t, err)
	require.Len(t, locs, 1, "the dep resolves to its declaration")

	// Declared on line 8 (0-based 7).
	assert.Equal(t, 7, locs[0].Range.Start.Line)
}

func TestDefinitionOnNothingReturnsNothing(t *testing.T) {
	h := opened(t, uri, navSrc)

	locs, err := h.Definition(context.Background(), &lsp.DefinitionParams{
		TextDocumentPositionParams: at(uri, 1, 0),
	})
	require.NoError(t, err)
	assert.Empty(t, locs)
}

func TestDefinitionOnAnUnopenedDocument(t *testing.T) {
	h := initialized(t)

	locs, err := h.Definition(context.Background(), &lsp.DefinitionParams{
		TextDocumentPositionParams: at("file:///never-opened", 0, 0),
	})
	require.NoError(t, err)
	assert.Empty(t, locs)
}

func TestReferencesOfAResource(t *testing.T) {
	h := opened(t, uri, navSrc)

	// The declaration of db-migrate, line 8 (0-based 7).
	line := strings.Split(navSrc, "\n")[7]
	col := strings.Index(line, "'db-migrate'") + 2

	without, err := h.References(context.Background(), &lsp.ReferenceParams{
		TextDocumentPositionParams: at(uri, 7, col),
		Context:                    lsp.ReferenceContext{IncludeDeclaration: false},
	})
	require.NoError(t, err)
	assert.Len(t, without, 1, "one resource_deps mention")

	with, err := h.References(context.Background(), &lsp.ReferenceParams{
		TextDocumentPositionParams: at(uri, 7, col),
		Context:                    lsp.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)
	assert.Len(t, with, 2, "the declaration as well")
}

func TestDocumentSymbolListsResourcesThenBindings(t *testing.T) {
	h := opened(t, uri, navSrc)

	syms, err := h.DocumentSymbol(context.Background(), &lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI(uri)},
	})
	require.NoError(t, err)

	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"db-migrate", "seed-data", "public", "site"}, names)
	assert.Equal(t, "resource", syms[0].Detail)
	assert.Equal(t, lsp.SymbolKindObject, syms[0].Kind)
	assert.Equal(t, lsp.SymbolKindVariable, syms[3].Kind)
}

func TestDocumentLinkOnlyLinksFilesThatExist(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "sites.yaml")
	require.NoError(t, os.WriteFile(real, []byte("kind: ConfigMap\n"), 0o600))

	tiltfile := "file://" + filepath.Join(dir, "Tiltfile")
	src := "k8s_yaml(['sites.yaml', 'missing.yaml'])\n"
	h := opened(t, tiltfile, src)

	links, err := h.DocumentLink(context.Background(), &lsp.DocumentLinkParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI(tiltfile)},
	})
	require.NoError(t, err)
	require.Len(t, links, 1, "missing.yaml is not a link")
	require.NotNil(t, links[0].Target)
	assert.Contains(t, string(*links[0].Target), "sites.yaml")
}

func TestDefinitionOpensAPathLiteral(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sites.yaml"), []byte("x\n"), 0o600))

	tiltfile := "file://" + filepath.Join(dir, "Tiltfile")
	src := "k8s_yaml('sites.yaml')\n"
	h := opened(t, tiltfile, src)

	col := strings.Index(src, "'sites.yaml'") + 2
	locs, err := h.Definition(context.Background(), &lsp.DefinitionParams{
		TextDocumentPositionParams: at(tiltfile, 0, col),
	})
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Contains(t, string(locs[0].URI), "sites.yaml")
	assert.Equal(t, 0, locs[0].Range.Start.Line, "the whole file, from the top")
}

func TestNavigationIgnoresAnUnparseableFile(t *testing.T) {
	h := opened(t, uri, "local_resource('web'\n")

	syms, err := h.DocumentSymbol(context.Background(), &lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI(uri)},
	})
	require.NoError(t, err)
	assert.Empty(t, syms, "no AST means no symbols, not a crash")
}
