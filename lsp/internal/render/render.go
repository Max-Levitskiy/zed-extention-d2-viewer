// Package render compiles D2 source text to SVG using the d2 library.
package render

import (
	"context"
	"errors"
	"fmt"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

// CompileError carries a single d2 compile error with position info.
// Line and Column are 1-based (as d2 reports them); the diag package
// converts these to LSP's 0-based positions at the boundary.
type CompileError struct {
	Line    int
	Column  int
	EndLine int
	EndCol  int
	Message string
}

func (e CompileError) Error() string {
	return fmt.Sprintf("d2 compile error at %d:%d: %s", e.Line, e.Column, e.Message)
}

// Render compiles `text` and returns SVG bytes. On compile failure it
// returns a CompileError with position info if d2 supplied it, otherwise
// a CompileError with Line=1 Column=1 pointing at the document start.
func Render(ctx context.Context, text string) ([]byte, error) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("ruler init: %w", err)
	}
	themeID := d2themescatalog.NeutralDefault.ID
	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}
	// d2 logs debug/warn messages via lib/log; without a slog.Logger in the
	// context it spams stack traces. Inject the default logger.
	ctx = d2log.WithDefault(ctx)
	diagram, _, err := d2lib.Compile(ctx, text, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
	}, nil)
	if err != nil {
		return nil, toCompileError(err)
	}
	svg, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		ThemeID: go2.Pointer(themeID),
	})
	if err != nil {
		return nil, fmt.Errorf("svg render: %w", err)
	}
	return svg, nil
}

// toCompileError converts whatever d2lib.Compile returned into our
// CompileError. As of d2 v0.7.1, compile failures surface as
// *d2parser.ParseError, which wraps a slice of d2ast.Error values, each
// carrying a Range with 0-indexed Start/End Positions. We take the first
// error (consistent with single-diagnostic LSP delivery for now) and add
// 1 to convert to the 1-based positions documented on CompileError.
//
// Fallbacks (in order):
//  1. *d2parser.ParseError with >=1 wrapped error -> extract Range
//  2. errors.As into our own CompileError (already shaped)
//  3. Generic error -> point at document start (1:1)
func toCompileError(err error) error {
	var pe *d2parser.ParseError
	if errors.As(err, &pe) && len(pe.Errors) > 0 {
		first := pe.Errors[0]
		return CompileError{
			Line:    first.Range.Start.Line + 1,
			Column:  first.Range.Start.Column + 1,
			EndLine: first.Range.End.Line + 1,
			EndCol:  first.Range.End.Column + 1,
			Message: first.Message,
		}
	}
	var ce CompileError
	if errors.As(err, &ce) {
		return ce
	}
	return CompileError{Line: 1, Column: 1, EndLine: 1, EndCol: 1, Message: err.Error()}
}
