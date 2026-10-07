package handler

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/parser"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/position"
)

// Definition answers three questions with one keystroke: where a Starlark
// binding is defined, where a resource named in resource_deps is declared, and
// which file a path literal points at.
func (h *Handler) Definition(
	_ context.Context,
	params *lsp.DefinitionParams,
) ([]lsp.Location, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := h.docs.Get(uri)
	if !ok {
		return nil, nil
	}
	line, col := h.starlarkCursor(doc, params.Position)
	file := model.Analyse(doc.File)

	// A path literal wins: the cursor is inside a string, so no identifier can
	// also claim it.
	if path, found := file.PathAt(line, col); found {
		if target, resolved := resolvePath(uri, path.Name); resolved {
			return []lsp.Location{{
				URI:   lsp.DocumentURI(target),
				Range: lsp.Range{},
			}}, nil
		}
	}
	decl, found := file.DeclAt(line, col)
	if !found {
		return nil, nil
	}
	return []lsp.Location{{
		URI:   params.TextDocument.URI,
		Range: h.spanToRange(doc, decl.Span),
	}}, nil
}

// References lists every mention of the name under the cursor, declaration
// included when the client asks for it.
func (h *Handler) References(
	_ context.Context,
	params *lsp.ReferenceParams,
) ([]lsp.Location, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := h.docs.Get(uri)
	if !ok {
		return nil, nil
	}
	line, col := h.starlarkCursor(doc, params.Position)
	file := model.Analyse(doc.File)

	occs := file.References(line, col, params.Context.IncludeDeclaration)
	locations := make([]lsp.Location, 0, len(occs))
	for _, occ := range occs {
		locations = append(locations, lsp.Location{
			URI:   params.TextDocument.URI,
			Range: h.spanToRange(doc, occ.Span),
		})
	}
	return locations, nil
}

// DocumentSymbol lists the resources first, then the Starlark bindings, so the
// outline reads as the stack the Tiltfile defines rather than its plumbing.
func (h *Handler) DocumentSymbol(
	_ context.Context,
	params *lsp.DocumentSymbolParams,
) ([]lsp.DocumentSymbol, error) {
	doc, ok := h.docs.Get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	file := model.Analyse(doc.File)

	var out []lsp.DocumentSymbol
	for _, r := range file.Resources() {
		out = append(out, h.symbol(doc, r, "resource", lsp.SymbolKindObject))
	}
	for _, s := range file.Symbols() {
		out = append(out, h.symbol(doc, s, "", lsp.SymbolKindVariable))
	}
	return out, nil
}

// DocumentLink makes every path literal that exists on disk clickable.
func (h *Handler) DocumentLink(
	_ context.Context,
	params *lsp.DocumentLinkParams,
) ([]lsp.DocumentLink, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := h.docs.Get(uri)
	if !ok {
		return nil, nil
	}
	file := model.Analyse(doc.File)

	var out []lsp.DocumentLink
	for _, p := range file.Paths {
		target, resolved := resolvePath(uri, p.Name)
		if !resolved {
			continue
		}
		targetURI := lsp.DocumentURI(target)
		out = append(out, lsp.DocumentLink{
			Range:  h.spanToRange(doc, p.Span),
			Target: &targetURI,
		})
	}
	return out, nil
}

func (h *Handler) symbol(
	doc *parser.Document,
	occ model.Occurrence,
	detail string,
	kind lsp.SymbolKind,
) lsp.DocumentSymbol {
	r := h.spanToRange(doc, occ.Span)
	return lsp.DocumentSymbol{
		Name:           occ.Name,
		Detail:         detail,
		Kind:           kind,
		Range:          r,
		SelectionRange: r,
	}
}

// starlarkCursor converts an LSP position back to Starlark's 1-based rune
// coordinates, which is what the model indexes on.
func (h *Handler) starlarkCursor(doc *parser.Document, pos lsp.Position) (int, int) {
	e := position.New(doc.Text, h.Encoding())
	byteCol := e.FromWire(pos).Character
	line := e.LineText(pos.Line)
	if byteCol > len(line) {
		byteCol = len(line)
	}
	runes := len([]rune(line[:byteCol]))
	return pos.Line + 1, runes + 1
}

func (h *Handler) spanToRange(doc *parser.Document, s model.Span) lsp.Range {
	e := position.New(doc.Text, h.Encoding())
	return lsp.Range{
		Start: e.StarlarkPos(s.StartLine, s.StartCol),
		End:   e.StarlarkPos(s.EndLine, s.EndCol),
	}
}

// resolvePath turns a Tiltfile-relative path into a file URI, and reports
// false when nothing is there. A path that does not exist is not a link.
func resolvePath(docURI, rel string) (string, bool) {
	dir := filepath.Dir(uriToPath(docURI))
	if dir == "" || dir == "." {
		return "", false
	}
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, rel)
	}
	if _, err := os.Stat(target); err != nil {
		return "", false
	}
	return pathToURI(target), true
}

func uriToPath(uri string) string {
	trimmed := strings.TrimPrefix(uri, "file://")
	if trimmed == uri {
		return uri
	}
	decoded, err := url.PathUnescape(trimmed)
	if err != nil {
		return trimmed
	}
	return decoded
}

func pathToURI(path string) string {
	return "file://" + (&url.URL{Path: path}).EscapedPath()
}
