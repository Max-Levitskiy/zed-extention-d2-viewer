package server

import "testing"

func TestServerCapabilities(t *testing.T) {
	caps := Capabilities()
	if caps.TextDocumentSync == nil {
		t.Fatal("expected TextDocumentSync to be set")
	}
	// Full sync == 1 per LSP spec.
	if v, ok := caps.TextDocumentSync.(int); !ok || v != 1 {
		t.Errorf("TextDocumentSync = %v, want int(1) (Full)", caps.TextDocumentSync)
	}
	if !caps.PublishDiagnosticsAdvertised {
		t.Error("expected diagnostics to be advertised")
	}
}
