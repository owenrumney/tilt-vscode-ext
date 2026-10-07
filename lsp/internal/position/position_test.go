package position

import (
	"testing"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/stretchr/testify/assert"
)

// "# café ☕ ok" is 14 bytes, 11 UTF-16 units, 11 runes. é starts at byte 5
// and is 2 bytes; ☕ starts at byte 8 and is 3.
const multibyte = "# café ☕ ok"

func TestByteToWire(t *testing.T) {
	tests := []struct {
		name     string
		encoding lsp.PositionEncodingKind
		byteCol  int
		want     int
	}{
		{"utf8 passes through", lsp.PositionEncodingUTF8, 11, 11},
		{"empty encoding means utf8", "", 11, 11},
		{"utf16 before any multibyte", lsp.PositionEncodingUTF16, 2, 2},
		{"utf16 after e-acute", lsp.PositionEncodingUTF16, 7, 6},
		{"utf16 after coffee", lsp.PositionEncodingUTF16, 11, 8},
		{"utf32 after coffee", lsp.PositionEncodingUTF32, 11, 8},
		{"end of line", lsp.PositionEncodingUTF16, 14, 11},
		{"clamped past the line", lsp.PositionEncodingUTF16, 999, 11},
		{"negative is zero", lsp.PositionEncodingUTF16, -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(multibyte, tt.encoding)
			got := e.ToWire(lsp.Position{Line: 0, Character: tt.byteCol})
			assert.Equal(t, tt.want, got.Character)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	for _, enc := range []lsp.PositionEncodingKind{
		lsp.PositionEncodingUTF8,
		lsp.PositionEncodingUTF16,
		lsp.PositionEncodingUTF32,
	} {
		t.Run(string(enc), func(t *testing.T) {
			e := New(multibyte, enc)
			// Only offsets on a rune boundary are expected to survive.
			for _, byteCol := range []int{0, 2, 5, 7, 8, 11, 14} {
				wire := e.ToWire(lsp.Position{Character: byteCol})
				back := e.FromWire(wire)
				assert.Equal(t, byteCol, back.Character, "byte col %d", byteCol)
			}
		})
	}
}

func TestOutOfRangeLines(t *testing.T) {
	e := New("one\ntwo", lsp.PositionEncodingUTF16)

	assert.Equal(t, 5, e.ToWire(lsp.Position{Line: 99, Character: 5}).Character)
	assert.Equal(t, 5, e.FromWire(lsp.Position{Line: 99, Character: 5}).Character)
	assert.Equal(t, "", e.LineText(99))
	assert.Equal(t, "two", e.LineText(1))
}

func TestRangeToWire(t *testing.T) {
	e := New(multibyte, lsp.PositionEncodingUTF16)
	got := e.RangeToWire(lsp.Range{
		Start: lsp.Position{Line: 0, Character: 2},
		End:   lsp.Position{Line: 0, Character: 11},
	})
	assert.Equal(t, 2, got.Start.Character)
	assert.Equal(t, 8, got.End.Character)
}
func TestStarlarkPosCountsRunes(t *testing.T) {
	e := New("local_resource('a')\n"+multibyte, lsp.PositionEncodingUTF16)

	// Starlark's 1,1 is LSP's 0,0.
	assert.Equal(t, lsp.Position{Line: 0, Character: 0}, e.StarlarkPos(1, 1))
	// Line two: Starlark rune column 8 is the space after "café", which is
	// UTF-16 unit 7 and byte 7. Rune and byte agree here only by chance.
	assert.Equal(t, lsp.Position{Line: 1, Character: 7}, e.StarlarkPos(2, 8))
	// Rune column 10 is past the coffee cup: 3 bytes wide, 1 UTF-16 unit.
	assert.Equal(t, lsp.Position{Line: 1, Character: 9}, e.StarlarkPos(2, 10))
	// A zero from Starlark means "unknown", so it is not decremented past zero.
	assert.Equal(t, lsp.Position{Line: 0, Character: 0}, e.StarlarkPos(0, 0))
	// Past the end of the line clamps rather than panicking.
	assert.Equal(t, lsp.Position{Line: 1, Character: 11}, e.StarlarkPos(2, 99))
}
