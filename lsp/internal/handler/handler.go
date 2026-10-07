// Package handler implements the LSP surface of the Tiltfile language server.
package handler

import (
	"context"
	"slices"
	"sync"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/owenrumney/go-lsp/server"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/parser"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/position"
)

const (
	name = "tiltfile-lsp"
	// The source string shown beside every diagnostic in the editor.
	source = "tiltfile"
)

// Handler holds the open documents and the negotiated position encoding.
type Handler struct {
	docs    *parser.Cache
	client  *server.Client
	version string

	mu       sync.RWMutex
	encoding lsp.PositionEncodingKind
}

// New takes the build version, so Initialize reports the same string the
// binary prints for --version.
func New(version string) *Handler {
	if version == "" {
		version = "dev"
	}
	return &Handler{
		docs:     parser.NewCache(),
		version:  version,
		encoding: lsp.PositionEncodingUTF16,
	}
}

// SetClient satisfies server.ClientHandler, giving the handler a way to push
// diagnostics.
func (h *Handler) SetClient(client *server.Client) {
	h.client = client
}

// Initialize negotiates the position encoding. UTF-16 is mandatory for every
// client, so it is the fallback when the client lists nothing it prefers.
func (h *Handler) Initialize(
	_ context.Context,
	params *lsp.InitializeParams,
) (*lsp.InitializeResult, error) {
	enc := negotiateEncoding(params)
	h.mu.Lock()
	h.encoding = enc
	h.mu.Unlock()

	openClose := true
	yes := true
	return &lsp.InitializeResult{
		ServerInfo: &lsp.ServerInfo{Name: name, Version: h.version},
		Capabilities: lsp.ServerCapabilities{
			PositionEncoding: &enc,
			TextDocumentSync: &lsp.TextDocumentSyncOptions{
				OpenClose: &openClose,
				Change:    lsp.SyncFull,
			},
			DefinitionProvider:     &yes,
			ReferencesProvider:     &yes,
			DocumentSymbolProvider: &lsp.DocumentSymbolOptions{},
			DocumentLinkProvider:   &lsp.DocumentLinkOptions{},
			CodeActionProvider:     &lsp.CodeActionOptions{},
			HoverProvider:          &yes,
			CompletionProvider: &lsp.CompletionOptions{
				// A dot opens namespace members; "=" opens a keyword value.
				TriggerCharacters: []string{".", "="},
			},
			SignatureHelpProvider: &lsp.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ","},
				RetriggerCharacters: []string{","},
			},
		},
	}, nil
}

func (h *Handler) Shutdown(_ context.Context) error {
	return nil
}

func (h *Handler) DidOpen(
	ctx context.Context,
	params *lsp.DidOpenTextDocumentParams,
) error {
	doc := params.TextDocument
	return h.analyse(ctx, string(doc.URI), doc.Version, doc.Text)
}

// DidChange takes the last content change, because the server advertises full
// sync and so every change carries the whole document.
func (h *Handler) DidChange(
	ctx context.Context,
	params *lsp.DidChangeTextDocumentParams,
) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}
	last := params.ContentChanges[len(params.ContentChanges)-1]
	doc := params.TextDocument
	return h.analyse(ctx, string(doc.URI), doc.Version, last.Text)
}

// DidClose clears the diagnostics, or the editor keeps showing them for a file
// that is no longer open.
func (h *Handler) DidClose(
	ctx context.Context,
	params *lsp.DidCloseTextDocumentParams,
) error {
	uri := string(params.TextDocument.URI)
	h.docs.Delete(uri)
	return h.publish(ctx, uri, nil)
}

// Encoding is the negotiated position encoding, for tests and other handlers.
func (h *Handler) Encoding() lsp.PositionEncodingKind {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.encoding
}

// Diagnostics parses text and returns what the editor should show, without
// touching the cache. Exported for tests.
func (h *Handler) Diagnostics(uri, text string) []lsp.Diagnostic {
	return diagnose(parser.Parse(uri, 0, text), h.Encoding())
}

func (h *Handler) analyse(ctx context.Context, uri string, version int, text string) error {
	doc := h.docs.Set(uri, version, text)
	return h.publish(ctx, uri, diagnose(doc, h.Encoding()))
}

func (h *Handler) publish(ctx context.Context, uri string, diags []lsp.Diagnostic) error {
	if h.client == nil {
		return nil
	}
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	return h.client.PublishDiagnostics(ctx, &lsp.PublishDiagnosticsParams{
		URI:         lsp.DocumentURI(uri),
		Diagnostics: diags,
	})
}

// diagnose reports the syntax error, if there is one. A Tiltfile that does not
// parse has no useful semantics, so there is nothing else worth saying.
func diagnose(doc *parser.Document, enc lsp.PositionEncodingKind) []lsp.Diagnostic {
	if doc.Err == nil {
		// Semantics are only meaningful once the file parses.
		if semantic := semanticFindings(doc, enc); semantic != nil {
			return semantic
		}
		return []lsp.Diagnostic{}
	}
	e := position.New(doc.Text, enc)
	start := e.StarlarkPos(int(doc.Err.Pos.Line), int(doc.Err.Pos.Col))
	severity := lsp.SeverityError
	return []lsp.Diagnostic{{
		Range:    lsp.Range{Start: start, End: endOfToken(e, start)},
		Severity: &severity,
		Source:   source,
		Message:  doc.Err.Msg,
	}}
}

// Starlark reports a point, not a span. Underline to the end of the line so
// the squiggle is visible, rather than a zero-width marker.
func endOfToken(e *position.Encoder, start lsp.Position) lsp.Position {
	text := e.LineText(start.Line)
	end := e.ToWire(lsp.Position{Line: start.Line, Character: len(text)})
	if end.Character <= start.Character {
		return lsp.Position{Line: start.Line, Character: start.Character + 1}
	}
	return end
}

func negotiateEncoding(params *lsp.InitializeParams) lsp.PositionEncodingKind {
	if params == nil || params.Capabilities.General == nil {
		return lsp.PositionEncodingUTF16
	}
	offered := params.Capabilities.General.PositionEncodings
	// Prefer UTF-8: it needs no conversion from the parser's byte offsets.
	for _, want := range []lsp.PositionEncodingKind{
		lsp.PositionEncodingUTF8,
		lsp.PositionEncodingUTF16,
		lsp.PositionEncodingUTF32,
	} {
		if slices.Contains(offered, want) {
			return want
		}
	}
	return lsp.PositionEncodingUTF16
}
