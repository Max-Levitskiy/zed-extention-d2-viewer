package diag

import (
	"testing"

	"github.com/max-levitskiy/zed-extention-d2-viewer/lsp/internal/render"
)

func TestFromCompileErrorSingle(t *testing.T) {
	ce := render.CompileError{Line: 3, Column: 5, EndLine: 3, EndCol: 9, Message: "unexpected token"}
	diags := FromCompileError(ce)
	if len(diags) != 1 {
		t.Fatalf("len = %d, want 1", len(diags))
	}
	d := diags[0]
	if d.Range.Start.Line != 2 { // LSP uses 0-based
		t.Errorf("Start.Line = %d, want 2", d.Range.Start.Line)
	}
	if d.Range.Start.Character != 4 {
		t.Errorf("Start.Character = %d, want 4", d.Range.Start.Character)
	}
	if d.Range.End.Line != 2 {
		t.Errorf("End.Line = %d, want 2", d.Range.End.Line)
	}
	if d.Range.End.Character != 8 {
		t.Errorf("End.Character = %d, want 8", d.Range.End.Character)
	}
	if d.Severity != SeverityError {
		t.Errorf("Severity = %v, want SeverityError", d.Severity)
	}
	if d.Source != "d2" {
		t.Errorf("Source = %q, want %q", d.Source, "d2")
	}
	if d.Message != "unexpected token" {
		t.Errorf("Message = %q", d.Message)
	}
}
