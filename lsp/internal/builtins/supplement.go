package builtins

// Declarations missing from `tilt dump api-docs`, added back by hand.
//
// Keep this list as short as evidence allows, and record why each entry is
// here. Every one is a bet that Tilt's stub is wrong, which is a bet that can
// go stale: if a later dump declares the symbol, the generated table wins and
// the entry here should go.
var supplement = map[string]Symbol{
	// api/__init__.py declares TRIGGER_MODE_AUTO with a type() sentinel but
	// never declares TRIGGER_MODE_MANUAL, though its docstrings reference the
	// pair and demo/Tiltfile uses it. Checked against Tilt v0.37.8.
	"TRIGGER_MODE_MANUAL": {
		Name: "TRIGGER_MODE_MANUAL",
		Kind: KindConst,
		Doc:  "User manually triggers an update for dirty resources via a button in the UI. The initial build still happens automatically.",
	},
}

// The generated table is authoritative. A supplement entry only fills a hole.
func init() {
	for name, sym := range supplement {
		if _, exists := Symbols[name]; !exists {
			Symbols[name] = sym
		}
	}
}
