package render

import (
	"bytes"
	"context"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
