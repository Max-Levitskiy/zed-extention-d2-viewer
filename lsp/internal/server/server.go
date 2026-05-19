// Package server wires the LSP handlers together. It owns the document
// state and dispatches didSave to the renderer.
package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/max-levitskiy/zed-extention-d2-viewer/lsp/internal/diag"
	"github.com/max-levitskiy/zed-extention-d2-viewer/lsp/internal/render"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspserver "github.com/tliron/glsp/server"
)

const lspName = "d2-lsp"
const lspVersion = "0.0.0"

// codeActionKindOpenD2Preview is the kind reported for our document-level
// "Open D2 Preview" code action. Custom (non-standard) source action kind.
const codeActionKindOpenD2Preview = "source.openD2Preview"

// commandOpenD2Preview is the workspace/executeCommand identifier the
// client invokes to request the server open the sibling SVG file.
const commandOpenD2Preview = "d2.openPreview"

// renderTimeout caps any single compile+render call. Exposed as a var so
// integration tests can shorten it; not a user setting.
var renderTimeout = 5 * time.Second

// ServerCaps is our framework-agnostic capabilities representation, kept
// distinct from glsp's struct layout so tests can assert against it
// directly.
type ServerCaps struct {
	TextDocumentSync             any
	PublishDiagnosticsAdvertised bool
	CodeActionProvider           any
	ExecuteCommandProvider       *protocol.ExecuteCommandOptions
}

func Capabilities() ServerCaps {
	return ServerCaps{
		TextDocumentSync:             1, // Full
		PublishDiagnosticsAdvertised: true,
		CodeActionProvider: &protocol.CodeActionOptions{
			CodeActionKinds: []protocol.CodeActionKind{codeActionKindOpenD2Preview},
		},
		ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
			Commands: []string{commandOpenD2Preview},
		},
	}
}

// State holds per-document text and per-document mutex so renders for
// the same URI are serialized.
type State struct {
	mu    sync.Mutex
	docs  map[string]string      // uri → text
	locks map[string]*sync.Mutex // uri → render lock
}

func NewState() *State {
	return &State{docs: map[string]string{}, locks: map[string]*sync.Mutex{}}
}

func (s *State) Set(uri, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = text
}

func (s *State) Get(uri string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.docs[uri]
	return t, ok
}

// Drop removes the per-URI document text and lock entry. Callers that
// already hold a lock pointer obtained from Lock(uri) keep using it
// safely until they release it; a subsequent Lock(uri) will return a
// fresh, distinct mutex.
func (s *State) Drop(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
	delete(s.locks, uri)
}

func (s *State) Lock(uri string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[uri]
	if !ok {
		m = &sync.Mutex{}
		s.locks[uri] = m
	}
	return m
}

// Run starts the stdio LSP loop. Blocks until shutdown.
func Run() error {
	state := NewState()
	handler := protocol.Handler{}

	handler.Initialize = func(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
		kind := protocol.TextDocumentSyncKindFull
		return protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				TextDocumentSync: kind,
				CodeActionProvider: &protocol.CodeActionOptions{
					CodeActionKinds: []protocol.CodeActionKind{codeActionKindOpenD2Preview},
				},
				ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
					Commands: []string{commandOpenD2Preview},
				},
			},
			ServerInfo: &protocol.InitializeResultServerInfo{
				Name:    lspName,
				Version: stringPtr(lspVersion),
			},
		}, nil
	}
	handler.Initialized = func(ctx *glsp.Context, params *protocol.InitializedParams) error { return nil }
	handler.Shutdown = func(ctx *glsp.Context) error { return nil }
	handler.SetTrace = func(ctx *glsp.Context, params *protocol.SetTraceParams) error { return nil }

	handler.TextDocumentDidOpen = func(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
		state.Set(params.TextDocument.URI, params.TextDocument.Text)
		return nil
	}
	handler.TextDocumentDidChange = func(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
		if len(params.ContentChanges) == 0 {
			return nil
		}
		last := params.ContentChanges[len(params.ContentChanges)-1]
		switch c := last.(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			state.Set(params.TextDocument.URI, c.Text)
		case protocol.TextDocumentContentChangeEvent:
			return fmt.Errorf("incremental sync not supported (server advertises Full only)")
		}
		return nil
	}
	handler.TextDocumentDidClose = func(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
		state.Drop(params.TextDocument.URI)
		return nil
	}
	handler.TextDocumentDidSave = func(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
		return handleSave(ctx, state, params)
	}

	handler.TextDocumentCodeAction = func(ctx *glsp.Context, params *protocol.CodeActionParams) (any, error) {
		return handleCodeAction(params), nil
	}
	handler.WorkspaceExecuteCommand = func(ctx *glsp.Context, params *protocol.ExecuteCommandParams) (any, error) {
		return handleExecuteCommand(ctx, params)
	}

	srv := glspserver.NewServer(&handler, lspName, false)
	return srv.RunStdio()
}

// handleCodeAction returns the single document-level "Open D2 Preview"
// code action. We return it unconditionally so users can trigger it from
// anywhere in the document; if the client narrowed by kind via
// params.Context.Only, respect that filter.
func handleCodeAction(params *protocol.CodeActionParams) []protocol.CodeAction {
	if len(params.Context.Only) > 0 {
		want := false
		for _, k := range params.Context.Only {
			// Allow exact match, or a hierarchical prefix match (e.g. "source").
			if k == codeActionKindOpenD2Preview || strings.HasPrefix(codeActionKindOpenD2Preview, k+".") || k == "source" && strings.HasPrefix(codeActionKindOpenD2Preview, "source.") {
				want = true
				break
			}
		}
		if !want {
			return nil
		}
	}
	kind := protocol.CodeActionKind(codeActionKindOpenD2Preview)
	return []protocol.CodeAction{{
		Title: "Open D2 Preview",
		Kind:  &kind,
		Command: &protocol.Command{
			Title:     "Open D2 Preview",
			Command:   commandOpenD2Preview,
			Arguments: []any{params.TextDocument.URI},
		},
	}}
}

// handleExecuteCommand dispatches workspace/executeCommand. For
// "d2.openPreview" it computes the sibling SVG path and asks the client
// to open it via window/showDocument.
func handleExecuteCommand(glspCtx *glsp.Context, params *protocol.ExecuteCommandParams) (any, error) {
	if params.Command != commandOpenD2Preview {
		return nil, nil
	}
	if len(params.Arguments) < 1 {
		return nil, nil
	}
	uri, ok := params.Arguments[0].(string)
	if !ok {
		return nil, nil
	}
	sourcePath, err := uriToPath(uri)
	if err != nil {
		publishGenericDiag(glspCtx, uri, err.Error())
		return nil, nil
	}
	siblingPath := render.SiblingPath(sourcePath)
	if _, statErr := os.Stat(siblingPath); statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			publishGenericDiag(glspCtx, uri, "Save the file to generate the preview")
			return nil, nil
		}
		publishGenericDiag(glspCtx, uri, "stat sibling svg: "+statErr.Error())
		return nil, nil
	}
	siblingURI := pathToFileURI(siblingPath)
	takeFocus := true
	// Issue the showDocument request asynchronously: the jsonrpc2 read
	// loop in glsp runs handlers synchronously, so calling Call here on
	// the same goroutine would deadlock waiting for the client's reply
	// (the reply can't be processed until this handler returns).
	go glspCtx.Call(protocol.ServerWindowShowDocument, protocol.ShowDocumentParams{
		URI:       siblingURI,
		TakeFocus: &takeFocus,
	}, &protocol.ShowDocumentResult{})
	return nil, nil
}

// pathToFileURI converts a filesystem path to a file:// URI suitable for
// window/showDocument.
func pathToFileURI(p string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}

func handleSave(glspCtx *glsp.Context, state *State, params *protocol.DidSaveTextDocumentParams) error {
	uri := params.TextDocument.URI

	var text string
	if params.Text != nil {
		text = *params.Text
		state.Set(uri, text)
	} else {
		t, ok := state.Get(uri)
		if !ok {
			return nil
		}
		text = t
	}

	sourcePath, err := uriToPath(uri)
	if err != nil {
		publishGenericDiag(glspCtx, uri, err.Error())
		return nil
	}

	lk := state.Lock(uri)
	lk.Lock()
	defer lk.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	defer cancel()

	svg, rerr := render.Render(ctx, text)
	if rerr != nil {
		var ce render.CompileError
		if asCE(rerr, &ce) {
			publishDiagnostics(glspCtx, uri, diag.FromCompileError(ce))
			return nil
		}
		publishGenericDiag(glspCtx, uri, rerr.Error())
		return nil
	}

	target := render.SiblingPath(sourcePath)
	if err := render.WriteSVGAtomic(target, svg); err != nil {
		publishGenericDiag(glspCtx, uri, "svg write failed: "+err.Error())
		return nil
	}
	publishDiagnostics(glspCtx, uri, nil) // clear stale errors
	return nil
}

func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse uri %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported uri scheme %q (only file:// supported)", u.Scheme)
	}
	p := u.Path
	// On Windows, file URIs look like file:///C:/foo — strip leading slash.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(p), nil
}

func stringPtr(s string) *string { return &s }

// publishDiagnostics sends a notification to the client. The empty slice
// clears any previous diagnostics for the URI.
func publishDiagnostics(glspCtx *glsp.Context, uri string, ds []diag.Diagnostic) {
	pd := protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: toProtocolDiags(ds),
	}
	glspCtx.Notify(protocol.ServerTextDocumentPublishDiagnostics, pd)
}

func publishGenericDiag(glspCtx *glsp.Context, uri string, message string) {
	publishDiagnostics(glspCtx, uri, []diag.Diagnostic{{
		Range:    diag.Range{},
		Severity: diag.SeverityError,
		Source:   "d2",
		Message:  message,
	}})
}

func toProtocolDiags(in []diag.Diagnostic) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(in))
	for _, d := range in {
		severity := protocol.DiagnosticSeverity(d.Severity)
		src := d.Source
		out = append(out, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(d.Range.Start.Line), Character: uint32(d.Range.Start.Character)},
				End:   protocol.Position{Line: uint32(d.Range.End.Line), Character: uint32(d.Range.End.Character)},
			},
			Severity: &severity,
			Source:   &src,
			Message:  d.Message,
		})
	}
	return out
}

func asCE(err error, out *render.CompileError) bool {
	return errors.As(err, out)
}
