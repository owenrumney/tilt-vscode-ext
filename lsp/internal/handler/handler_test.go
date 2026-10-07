package handler

import (
	"context"
	"testing"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const uri = "file:///Tiltfile"

func initialized(t *testing.T, encodings ...lsp.PositionEncodingKind) *Handler {
	t.Helper()
	h := New("test")
	params := &lsp.InitializeParams{}
	if encodings != nil {
		params.Capabilities.General = &lsp.GeneralClientCapabilities{
			PositionEncodings: encodings,
		}
	}
	_, err := h.Initialize(context.Background(), params)
	require.NoError(t, err)
	return h
}

func TestInitializeAdvertisesFullSync(t *testing.T) {
	h := New("test")

	res, err := h.Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.Capabilities.TextDocumentSync)
	assert.Equal(t, lsp.SyncFull, res.Capabilities.TextDocumentSync.Change)
	require.NotNil(t, res.Capabilities.TextDocumentSync.OpenClose)
	assert.True(t, *res.Capabilities.TextDocumentSync.OpenClose)
	require.NotNil(t, res.ServerInfo)
	assert.Equal(t, "tiltfile-lsp", res.ServerInfo.Name)
}

func TestNegotiateEncoding(t *testing.T) {
	tests := []struct {
		name    string
		offered []lsp.PositionEncodingKind
		want    lsp.PositionEncodingKind
	}{
		{"nothing offered falls back to utf-16", nil, lsp.PositionEncodingUTF16},
		{"empty list falls back to utf-16", []lsp.PositionEncodingKind{}, lsp.PositionEncodingUTF16},
		{"utf-8 is preferred", []lsp.PositionEncodingKind{lsp.PositionEncodingUTF16, lsp.PositionEncodingUTF8}, lsp.PositionEncodingUTF8},
		{"utf-16 when utf-8 is absent", []lsp.PositionEncodingKind{lsp.PositionEncodingUTF32, lsp.PositionEncodingUTF16}, lsp.PositionEncodingUTF16},
		{"utf-32 only", []lsp.PositionEncodingKind{lsp.PositionEncodingUTF32}, lsp.PositionEncodingUTF32},
		{"an unknown encoding is ignored", []lsp.PositionEncodingKind{"utf-7"}, lsp.PositionEncodingUTF16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New("test")
			params := &lsp.InitializeParams{}
			if tt.offered != nil {
				params.Capabilities.General = &lsp.GeneralClientCapabilities{PositionEncodings: tt.offered}
			}
			res, err := h.Initialize(context.Background(), params)
			require.NoError(t, err)

			assert.Equal(t, tt.want, h.Encoding())
			require.NotNil(t, res.Capabilities.PositionEncoding)
			assert.Equal(t, tt.want, *res.Capabilities.PositionEncoding, "advertised encoding must match")
		})
	}
}

func TestDiagnosticsOnValidTiltfile(t *testing.T) {
	h := initialized(t)

	diags := h.Diagnostics(uri, "local_resource('web', cmd='echo hi')\n")

	assert.Empty(t, diags)
}

func TestDiagnosticsOnSyntaxError(t *testing.T) {
	h := initialized(t)

	// Line 2 is missing a closing paren.
	diags := h.Diagnostics(uri, "x = 1\nlocal_resource('web'\n")

	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "tiltfile", d.Source)
	assert.NotEmpty(t, d.Message)
	require.NotNil(t, d.Severity)
	assert.Equal(t, lsp.SeverityError, *d.Severity)
	assert.Positive(t, d.Range.End.Character, "the range must be visible, not zero width")
	assert.GreaterOrEqual(t, d.Range.End.Character, d.Range.Start.Character)
}

func TestDidCloseForgetsTheDocument(t *testing.T) {
	h := initialized(t)
	ctx := context.Background()

	require.NoError(t, h.DidOpen(ctx, &lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{URI: uri, Version: 1, Text: "x = 1\n"},
	}))
	_, ok := h.docs.Get(uri)
	require.True(t, ok)

	require.NoError(t, h.DidClose(ctx, &lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	}))
	_, ok = h.docs.Get(uri)
	assert.False(t, ok)
}

func TestDidChangeTakesTheLastFullContent(t *testing.T) {
	h := initialized(t)
	ctx := context.Background()

	require.NoError(t, h.DidChange(ctx, &lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri},
			Version:                2,
		},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{
			{Text: "first = 1\n"},
			{Text: "second = 2\n"},
		},
	}))

	doc, ok := h.docs.Get(uri)
	require.True(t, ok)
	assert.Equal(t, "second = 2\n", doc.Text)
	assert.Equal(t, 2, doc.Version)
}

func TestDidChangeWithNoChangesIsASafeNoop(t *testing.T) {
	h := initialized(t)

	err := h.DidChange(context.Background(), &lsp.DidChangeTextDocumentParams{
		ContentChanges: []lsp.TextDocumentContentChangeEvent{},
	})

	assert.NoError(t, err)
}

func TestShutdown(t *testing.T) {
	assert.NoError(t, New("test").Shutdown(context.Background()))
}
func TestDiagnosticRangeUsesTheNegotiatedEncoding(t *testing.T) {
	// Starlark reports the bad '$' at rune column 9. The coffee cup before it
	// is 3 bytes and 1 UTF-16 unit, so byte and UTF-16 columns differ.
	//   y = '☕' $
	src := "y = '☕' $\n"

	utf8 := initialized(t, lsp.PositionEncodingUTF8).Diagnostics(uri, src)
	utf16 := initialized(t, lsp.PositionEncodingUTF16).Diagnostics(uri, src)
	utf32 := initialized(t, lsp.PositionEncodingUTF32).Diagnostics(uri, src)

	require.Len(t, utf8, 1)
	require.Len(t, utf16, 1)
	require.Len(t, utf32, 1)

	assert.Equal(t, 0, utf16[0].Range.Start.Line)
	assert.Equal(t, 10, utf8[0].Range.Start.Character, "byte offset of the '$'")
	assert.Equal(t, 8, utf16[0].Range.Start.Character, "UTF-16 units before the '$'")
	assert.Equal(t, 8, utf32[0].Range.Start.Character, "runes before the '$'")
}

func TestInitializeReportsTheBuildVersion(t *testing.T) {
	h := New("v1.2.3")

	res, err := h.Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	require.NotNil(t, res.ServerInfo)
	assert.Equal(t, "v1.2.3", res.ServerInfo.Version)
}

func TestInitializeFallsBackToDevVersion(t *testing.T) {
	res, err := New("").Initialize(context.Background(), &lsp.InitializeParams{})
	require.NoError(t, err)

	assert.Equal(t, "dev", res.ServerInfo.Version)
}
