package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/builtins"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/parser"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/position"
)

// Hover shows the signature, the first doc paragraph, and a link to the
// published docs. A parameter keyword hovers to its own description.
func (h *Handler) Hover(
	_ context.Context,
	params *lsp.HoverParams,
) (*lsp.Hover, error) {
	doc, ok := h.docs.Get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	line, col := h.starlarkCursor(doc, params.Position)

	if md, found := h.hoverKeyword(doc, line, col); found {
		contents := lsp.NewHoverContents(lsp.Markdown, md)
		return &lsp.Hover{Contents: contents}, nil
	}

	name, found := identifierAt(doc, line, col)
	if !found {
		return nil, nil
	}
	sym, found := builtins.Lookup(name)
	if !found {
		return nil, nil
	}
	contents := lsp.NewHoverContents(lsp.Markdown, symbolMarkdown(sym))
	return &lsp.Hover{Contents: contents}, nil
}

// Completion offers, in order of specificity: namespace members after a dot,
// parameter names inside a call, enum values for a known keyword, and the
// top-level builtins everywhere else.
func (h *Handler) Completion(
	_ context.Context,
	params *lsp.CompletionParams,
) (*lsp.CompletionList, error) {
	doc, ok := h.docs.Get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	line, col := h.starlarkCursor(doc, params.Position)
	prefixText := h.textBeforeCursor(doc, params.Position)

	if ns, found := namespacePrefix(prefixText); found {
		return itemList(namespaceItems(ns)), nil
	}
	if site, found := h.callSite(doc, line, col, params.Position); found {
		if items, handled := callItems(site, prefixText); handled {
			return itemList(items), nil
		}
	}
	return itemList(topLevelItems()), nil
}

// SignatureHelp shows the signature of the enclosing call with the current
// parameter highlighted.
func (h *Handler) SignatureHelp(
	_ context.Context,
	params *lsp.SignatureHelpParams,
) (*lsp.SignatureHelp, error) {
	doc, ok := h.docs.Get(string(params.TextDocument.URI))
	if !ok {
		return nil, nil
	}
	line, col := h.starlarkCursor(doc, params.Position)
	site, found := h.callSite(doc, line, col, params.Position)
	if !found {
		return nil, nil
	}
	sym, found := builtins.Lookup(site.Callee)
	if !found || sym.Kind != builtins.KindFunc {
		return nil, nil
	}

	label := sym.Signature()
	info := lsp.SignatureInformation{
		Label:      label,
		Parameters: parameterInfo(sym, label),
	}
	if sym.Doc != "" {
		info.Documentation = &lsp.MarkupContent{
			Kind:  lsp.Markdown,
			Value: firstParagraph(sym.Doc),
		}
	}
	active := activeParameter(sym, site)
	info.ActiveParameter = &active

	zero := 0
	return &lsp.SignatureHelp{
		Signatures:      []lsp.SignatureInformation{info},
		ActiveSignature: &zero,
		ActiveParameter: &active,
	}, nil
}

// hoverKeyword answers for the cursor sitting on a `name=` inside a call.
func (h *Handler) hoverKeyword(doc *parser.Document, line, col int) (string, bool) {
	site, found := model.CallSiteAt(doc.File, line, col)
	if !found || !site.InKeywordName || site.Keyword == "" {
		return "", false
	}
	sym, found := builtins.Lookup(site.Callee)
	if !found {
		return "", false
	}
	p, found := sym.Param(site.Keyword)
	if !found {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "```python\n%s\n```\n\n", p.String())
	if p.Doc != "" {
		b.WriteString(p.Doc + "\n\n")
	}
	fmt.Fprintf(&b, "Parameter of [`%s`](%s).", sym.Qualified(), sym.URL())
	return b.String(), true
}

func symbolMarkdown(sym builtins.Symbol) string {
	var b strings.Builder
	fmt.Fprintf(&b, "```python\n%s\n```\n\n", sym.Signature())
	if sym.Doc != "" {
		b.WriteString(sym.Doc + "\n\n")
	}
	fmt.Fprintf(&b, "Tilt %s %s — [docs](%s)",
		builtins.TiltVersion, sym.Kind, sym.URL())
	return b.String()
}

// namespacePrefix reads a trailing "os.path." from the text before the cursor.
func namespacePrefix(before string) (string, bool) {
	trimmed := strings.TrimRight(before, " \t")
	if !strings.HasSuffix(trimmed, ".") {
		return "", false
	}
	head := strings.TrimSuffix(trimmed, ".")
	// Walk back over the dotted identifier.
	start := len(head)
	for start > 0 {
		c := head[start-1]
		if c == '.' || c == '_' || (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			start--
			continue
		}
		break
	}
	candidate := head[start:]
	if candidate == "" {
		return "", false
	}
	for _, ns := range builtins.Namespaces() {
		if ns == candidate {
			return ns, true
		}
	}
	return "", false
}

func namespaceItems(namespace string) []lsp.CompletionItem {
	var out []lsp.CompletionItem
	for _, sym := range builtins.InNamespace(namespace) {
		out = append(out, item(sym))
	}
	// A namespace that is itself a prefix, such as os.path under os.
	for _, ns := range builtins.Namespaces() {
		if strings.HasPrefix(ns, namespace+".") &&
			!strings.Contains(strings.TrimPrefix(ns, namespace+"."), ".") {
			kind := lsp.CompletionItemKindModule
			name := strings.TrimPrefix(ns, namespace+".")
			out = append(out, lsp.CompletionItem{
				Label:  name,
				Kind:   &kind,
				Detail: fmt.Sprintf("module %s", ns),
			})
		}
	}
	return out
}

// callItems offers parameter names, or enum values once a keyword is chosen.
func callItems(site model.CallSite, before string) ([]lsp.CompletionItem, bool) {
	sym, found := builtins.Lookup(site.Callee)
	if !found || sym.Kind != builtins.KindFunc {
		return nil, false
	}
	// After "keyword=", offer the values that keyword accepts.
	if site.Keyword != "" && !site.InKeywordName {
		if values, ok := enumValues(sym, site.Keyword); ok {
			return values, true
		}
		return nil, false
	}
	// Mid-expression, a parameter name is not what the user wants.
	if endsWithOperator(before) {
		return nil, false
	}
	var out []lsp.CompletionItem
	kind := lsp.CompletionItemKindField
	for _, p := range sym.Params {
		if site.Supplied[p.Name] && p.Name != site.Keyword {
			continue
		}
		detail := p.Type
		if p.Required() {
			detail += " (required)"
		}
		it := lsp.CompletionItem{
			Label:      p.Name,
			Kind:       &kind,
			Detail:     detail,
			InsertText: p.Name + "=",
			SortText:   sortPrefix(p) + p.Name,
		}
		if p.Doc != "" {
			it.Documentation = &lsp.MarkupContent{Kind: lsp.Markdown, Value: p.Doc}
		}
		out = append(out, it)
	}
	return out, len(out) > 0
}

// enumValues offers the constants a typed parameter accepts. Only TriggerMode
// is a closed set in the Tilt API today.
func enumValues(sym builtins.Symbol, keyword string) ([]lsp.CompletionItem, bool) {
	p, found := sym.Param(keyword)
	if !found || !strings.Contains(p.Type, "TriggerMode") {
		return nil, false
	}
	kind := lsp.CompletionItemKindConstant
	var out []lsp.CompletionItem
	for _, name := range []string{"TRIGGER_MODE_AUTO", "TRIGGER_MODE_MANUAL"} {
		out = append(out, lsp.CompletionItem{Label: name, Kind: &kind})
	}
	return out, true
}

func topLevelItems() []lsp.CompletionItem {
	var out []lsp.CompletionItem
	for _, sym := range builtins.InNamespace("") {
		out = append(out, item(sym))
	}
	kind := lsp.CompletionItemKindModule
	for _, ns := range builtins.Namespaces() {
		if strings.Contains(ns, ".") {
			continue
		}
		out = append(out, lsp.CompletionItem{
			Label:  ns,
			Kind:   &kind,
			Detail: "Tilt module",
		})
	}
	return out
}

func item(sym builtins.Symbol) lsp.CompletionItem {
	kind := completionKind(sym.Kind)
	it := lsp.CompletionItem{
		Label:  sym.Name,
		Kind:   &kind,
		Detail: sym.Signature(),
	}
	if sym.Doc != "" {
		it.Documentation = &lsp.MarkupContent{
			Kind:  lsp.Markdown,
			Value: firstParagraph(sym.Doc),
		}
	}
	return it
}

func completionKind(k builtins.Kind) lsp.CompletionItemKind {
	switch k {
	case builtins.KindFunc:
		return lsp.CompletionItemKindFunction
	case builtins.KindClass:
		return lsp.CompletionItemKindClass
	case builtins.KindConst:
		return lsp.CompletionItemKindConstant
	}
	return lsp.CompletionItemKindVariable
}

// parameterInfo labels each parameter by its offset in the signature, so the
// editor highlights the right span even where a name repeats.
func parameterInfo(sym builtins.Symbol, label string) []lsp.ParameterInformation {
	out := make([]lsp.ParameterInformation, 0, len(sym.Params))
	cursor := 0
	for _, p := range sym.Params {
		text := p.String()
		at := strings.Index(label[cursor:], text)
		if at < 0 {
			out = append(out, lsp.ParameterInformation{Label: text})
			continue
		}
		start := cursor + at
		cursor = start + len(text)
		info := lsp.ParameterInformation{Label: []int{start, cursor}}
		if p.Doc != "" {
			info.Documentation = &lsp.MarkupContent{Kind: lsp.Markdown, Value: p.Doc}
		}
		out = append(out, info)
	}
	return out
}

// activeParameter prefers the keyword under the cursor over the positional
// index, because a keyword argument can appear in any order.
func activeParameter(sym builtins.Symbol, site model.CallSite) int {
	if site.Keyword != "" {
		for i, p := range sym.Params {
			if p.Name == site.Keyword {
				return i
			}
		}
	}
	if site.ArgIndex >= len(sym.Params) && len(sym.Params) > 0 {
		return len(sym.Params) - 1
	}
	return site.ArgIndex
}

func itemList(items []lsp.CompletionItem) *lsp.CompletionList {
	if items == nil {
		items = []lsp.CompletionItem{}
	}
	return &lsp.CompletionList{IsIncomplete: false, Items: items}
}

// Required parameters sort first, so the list opens on what must be supplied.
func sortPrefix(p builtins.Param) string {
	if p.Required() {
		return "0"
	}
	return "1"
}

func endsWithOperator(before string) bool {
	trimmed := strings.TrimRight(before, " \t")
	if trimmed == "" {
		return false
	}
	switch trimmed[len(trimmed)-1] {
	case '=', '+', '-', '*', '/', '%', '(', '[', ',':
		return trimmed[len(trimmed)-1] == '='
	}
	return false
}

func firstParagraph(doc string) string {
	if i := strings.Index(doc, "\n\n"); i > 0 {
		return doc[:i]
	}
	return doc
}

// textBeforeCursor is the current line up to the cursor, which is what tells
// completion whether it follows a dot or an "=".
func (h *Handler) textBeforeCursor(doc *parser.Document, pos lsp.Position) string {
	e := position.New(doc.Text, h.Encoding())
	line := e.LineText(pos.Line)
	byteCol := e.FromWire(pos).Character
	if byteCol > len(line) {
		byteCol = len(line)
	}
	if byteCol < 0 {
		byteCol = 0
	}
	return line[:byteCol]
}

// identifierAt reads the dotted identifier under a Starlark rune position, so
// that hovering "abspath" in os.path.abspath finds the qualified symbol.
func identifierAt(doc *parser.Document, line, col int) (string, bool) {
	runes := []rune(lineAt(doc.Text, line))
	idx := col - 1
	if idx < 0 || idx > len(runes) {
		return "", false
	}
	if idx == len(runes) || !isWord(runes[idx]) {
		if idx == 0 || !isWord(runes[idx-1]) {
			return "", false
		}
		idx--
	}
	start, end := idx, idx
	for start > 0 && (isWord(runes[start-1]) || runes[start-1] == '.') {
		start--
	}
	for end+1 < len(runes) && isWord(runes[end+1]) {
		end++
	}
	return strings.Trim(string(runes[start:end+1]), "."), true
}

func lineAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return lines[line-1]
}

func isWord(r rune) bool {
	return r == '_' || (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// callSite prefers the AST, which knows argument spans exactly, and falls back
// to scanning the text when the document does not parse. A user mid-keystroke
// usually has a document that does not parse.
func (h *Handler) callSite(
	doc *parser.Document,
	line, col int,
	pos lsp.Position,
) (model.CallSite, bool) {
	if site, found := model.CallSiteAt(doc.File, line, col); found {
		return site, true
	}
	return model.TextCallSite(h.textToCursor(doc, pos))
}

// textToCursor is the whole document up to the cursor, because a call can span
// many lines.
func (h *Handler) textToCursor(doc *parser.Document, pos lsp.Position) string {
	e := position.New(doc.Text, h.Encoding())
	lines := strings.Split(doc.Text, "\n")
	if pos.Line < 0 {
		return ""
	}
	if pos.Line >= len(lines) {
		return doc.Text
	}
	byteCol := e.FromWire(pos).Character
	if byteCol > len(lines[pos.Line]) {
		byteCol = len(lines[pos.Line])
	}
	if byteCol < 0 {
		byteCol = 0
	}
	head := strings.Join(lines[:pos.Line], "\n")
	if pos.Line > 0 {
		head += "\n"
	}
	return head + lines[pos.Line][:byteCol]
}
