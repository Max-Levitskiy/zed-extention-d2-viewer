// Package diag converts d2 render errors to LSP diagnostics.
//
// We keep the LSP types as plain structs here so this package has no
// dependency on the glsp framework — the server layer translates these
// into glsp protocol structs at the boundary.
package diag

import "github.com/max-levitskiy/zed-extention-d2-viewer/lsp/internal/render"

type Severity int

const (
	SeverityError       Severity = 1
	SeverityWarning     Severity = 2
	SeverityInformation Severity = 3
	SeverityHint        Severity = 4
)

type Position struct {
	Line, Character int // 0-based per LSP
}

type Range struct {
	Start, End Position
}

type Diagnostic struct {
	Range    Range
	Severity Severity
	Source   string
	Message  string
}

// FromCompileError turns a render.CompileError (1-based line/col from d2)
// into LSP diagnostics (0-based). Returns a single-element slice.
func FromCompileError(ce render.CompileError) []Diagnostic {
	return []Diagnostic{{
		Range: Range{
			Start: Position{Line: clamp0(ce.Line - 1), Character: clamp0(ce.Column - 1)},
			End:   Position{Line: clamp0(ce.EndLine - 1), Character: clamp0(ce.EndCol - 1)},
		},
		Severity: SeverityError,
		Source:   "d2",
		Message:  ce.Message,
	}}
}

func clamp0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
