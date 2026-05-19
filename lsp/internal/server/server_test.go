package server

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

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
	if caps.CodeActionProvider == nil {
		t.Error("expected CodeActionProvider to be advertised")
	}
	if caps.ExecuteCommandProvider == nil {
		t.Fatal("expected ExecuteCommandProvider to be advertised")
	}
	cmds := caps.ExecuteCommandProvider.Commands
	found := false
	for _, c := range cmds {
		if c == "d2.openPreview" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ExecuteCommandProvider commands to include d2.openPreview, got %v", cmds)
	}
}

func TestCodeActionReturnsPreview(t *testing.T) {
	uri := "file:///tmp/hello.d2"
	params := &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range:        protocol.Range{},
		Context:      protocol.CodeActionContext{},
	}
	got := handleCodeAction(params)
	if len(got) != 1 {
		t.Fatalf("expected 1 code action, got %d", len(got))
	}
	a := got[0]
	if a.Title != "Open D2 Preview" {
		t.Errorf("Title = %q, want %q", a.Title, "Open D2 Preview")
	}
	if a.Kind == nil || *a.Kind != codeActionKindOpenD2Preview {
		t.Errorf("Kind = %v, want %q", a.Kind, codeActionKindOpenD2Preview)
	}
	if a.Command == nil {
		t.Fatal("expected Command to be set")
	}
	if a.Command.Command != "d2.openPreview" {
		t.Errorf("Command.Command = %q, want %q", a.Command.Command, "d2.openPreview")
	}
	if len(a.Command.Arguments) != 1 || a.Command.Arguments[0] != uri {
		t.Errorf("Command.Arguments = %v, want [%q]", a.Command.Arguments, uri)
	}
}

func TestCodeActionFiltersByOnly(t *testing.T) {
	uri := "file:///tmp/hello.d2"
	// Client requests only refactor kinds — we should return nothing.
	params := &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Context: protocol.CodeActionContext{
			Only: []protocol.CodeActionKind{protocol.CodeActionKindRefactor},
		},
	}
	got := handleCodeAction(params)
	if len(got) != 0 {
		t.Errorf("expected 0 actions when filtered to refactor, got %d", len(got))
	}

	// Client requests "source" — our action's kind is source.openD2Preview,
	// which should match the source hierarchy.
	params.Context.Only = []protocol.CodeActionKind{protocol.CodeActionKindSource}
	got = handleCodeAction(params)
	if len(got) != 1 {
		t.Errorf("expected 1 action when filtered to source, got %d", len(got))
	}
}
