package render

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenderProducesSVG(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "hello.d2"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	svg, rerr := Render(ctx, string(src))
	if rerr != nil {
		t.Fatalf("Render returned error: %v", rerr)
	}
	if !bytes.HasPrefix(svg, []byte("<")) {
		t.Fatalf("expected SVG-like output, got %q", string(svg[:min(40, len(svg))]))
	}
	if !bytes.Contains(svg, []byte("svg")) {
		t.Fatalf("expected SVG payload to contain 'svg' tag")
	}
}

func TestRenderReportsCompileError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connection with missing destination → d2 reports a parse error
	// whose Range.Start is line 1, column 1 (0-based internally, 1-based
	// by our contract).
	_, err := Render(ctx, "a ->")
	if err == nil {
		t.Fatal("expected Render to return an error for malformed source")
	}

	var ce CompileError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *render.CompileError via errors.As, got %T (%v)", err, err)
	}
	if ce.Line != 1 || ce.Column != 1 {
		t.Errorf("expected position 1:1, got %d:%d", ce.Line, ce.Column)
	}
	if ce.Message == "" {
		t.Error("expected non-empty Message")
	}
}
