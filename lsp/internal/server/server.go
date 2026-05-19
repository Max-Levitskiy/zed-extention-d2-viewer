// Package server wires the LSP handlers together. It owns the document
// state and dispatches didSave to the renderer.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
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

// renderTimeout caps any single compile+render call. Exposed as a var so
// integration tests can shorten it; not a user setting.
var renderTimeout = 5 * time.Second

// ServerCaps is our framework-agnostic capabilities representation, kept
// distinct from glsp's struct layout so tests can assert against it
// directly.
type ServerCaps struct {
	TextDocumentSync             any
	PublishDiagnosticsAdvertised bool
}

func Capabilities() ServerCaps {
	return ServerCaps{
		TextDocumentSync:             1, // Full
		PublishDiagnosticsAdvertised: true,
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

	srv := glspserver.NewServer(&handler, lspName, false)
	return srv.RunStdio()
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
