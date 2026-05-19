package render

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSiblingPath(t *testing.T) {
	cases := map[string]string{
		"/abs/path/foo.d2": "/abs/path/.foo.d2.svg",
		"rel/bar.d2":       "rel/.bar.d2.svg",
		"nodir.d2":         ".nodir.d2.svg",
	}
	for in, want := range cases {
		got := SiblingPath(in)
		if got != want {
			t.Errorf("SiblingPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteSVGAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".foo.d2.svg")
	if err := WriteSVGAtomic(target, []byte("<svg/>")); err != nil {
		t.Fatalf("WriteSVGAtomic: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "<svg/>" {
		t.Errorf("file contents = %q, want %q", got, "<svg/>")
	}
}

func TestWriteSVGAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".foo.d2.svg")
	payloads := []string{
		"<svg>" + strings.Repeat("A", 4096) + "</svg>",
		"<svg>" + strings.Repeat("B", 4096) + "</svg>",
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		p := payloads[i%2]
		wg.Add(1)
		go func(payload string) {
			defer wg.Done()
			if err := WriteSVGAtomic(target, []byte(payload)); err != nil {
				t.Errorf("concurrent write: %v", err)
			}
		}(p)
	}
	wg.Wait()

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	final := string(got)
	if final != payloads[0] && final != payloads[1] {
		t.Errorf("torn write detected: final content has length %d", len(final))
	}
}
