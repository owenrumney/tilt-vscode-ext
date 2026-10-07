package handler

import (
	"context"
	"encoding/json"

	"github.com/owenrumney/go-lsp/lsp"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/analysis"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/model"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/parser"
	"github.com/owenrumney/tilt-vscode-ext/lsp/internal/position"
)

// semanticFindings runs the analysis checks and converts them to diagnostics.
// A fix travels in the diagnostic's Data, so CodeAction does not have to
// reanalyse the document to answer.
func semanticFindings(doc *parser.Document, enc lsp.PositionEncodingKind) []lsp.Diagnostic {
	findings := analysis.Run(doc.File, model.Analyse(doc.File))
	if len(findings) == 0 {
		return nil
	}
	e := position.New(doc.Text, enc)
	out := make([]lsp.Diagnostic, 0, len(findings))
	for _, f := range findings {
		severity := lspSeverity(f.Severity)
		code, _ := json.Marshal(f.Code)
		d := lsp.Diagnostic{
			Range:    spanRange(e, f.Span),
			Severity: &severity,
			Source:   source,
			Message:  f.Message,
			Code:     code,
		}
		if f.Fix != nil {
			if data, err := json.Marshal(storedFix{
				Title:   f.Fix.Title,
				Range:   spanRange(e, f.Fix.Span),
				NewText: f.Fix.NewText,
			}); err == nil {
				d.Data = data
			}
		}
		out = append(out, d)
	}
	return out
}

// storedFix is the quick fix carried on a diagnostic between publishing and
// the code action request.
type storedFix struct {
	Title   string    `json:"title"`
	Range   lsp.Range `json:"range"`
	NewText string    `json:"newText"`
}

// CodeAction offers the quick fixes attached to the diagnostics the client
// sends back, so no reanalysis is needed.
func (h *Handler) CodeAction(
	_ context.Context,
	params *lsp.CodeActionParams,
) ([]lsp.CodeAction, error) {
	var out []lsp.CodeAction
	kind := lsp.CodeActionQuickFix
	preferred := true

	for _, d := range params.Context.Diagnostics {
		if d.Source != source || len(d.Data) == 0 {
			continue
		}
		var fix storedFix
		if err := json.Unmarshal(d.Data, &fix); err != nil || fix.Title == "" {
			continue
		}
		out = append(out, lsp.CodeAction{
			Title:       fix.Title,
			Kind:        &kind,
			Diagnostics: []lsp.Diagnostic{d},
			IsPreferred: &preferred,
			Edit: &lsp.WorkspaceEdit{
				Changes: map[lsp.DocumentURI][]lsp.TextEdit{
					params.TextDocument.URI: {{Range: fix.Range, NewText: fix.NewText}},
				},
			},
		})
	}
	return out, nil
}

func spanRange(e *position.Encoder, s model.Span) lsp.Range {
	return lsp.Range{
		Start: e.StarlarkPos(s.StartLine, s.StartCol),
		End:   e.StarlarkPos(s.EndLine, s.EndCol),
	}
}

func lspSeverity(s analysis.Severity) lsp.DiagnosticSeverity {
	switch s {
	case analysis.Error:
		return lsp.SeverityError
	case analysis.Warning:
		return lsp.SeverityWarning
	case analysis.Information:
		return lsp.SeverityInformation
	}
	return lsp.SeverityHint
}
