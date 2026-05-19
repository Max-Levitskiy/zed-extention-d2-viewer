package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationRenderOnSave(t *testing.T) {
	bin := buildBinary(t)
	workdir := t.TempDir()
	src := filepath.Join(workdir, "hello.d2")
	if err := os.WriteFile(src, []byte("hello -> world"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cmd := exec.Command(bin)
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lsp: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	rd := bufio.NewReader(stdout)
	uri := pathToURI(src)

	send(t, stdin, "initialize", 1, map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      pathToURI(workdir),
		"capabilities": map[string]any{},
	})
	expectResponse(t, rd, 1)

	sendNotify(t, stdin, "initialized", map[string]any{})

	sendNotify(t, stdin, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "d2", "version": 1, "text": "hello -> world",
		},
	})
	sendNotify(t, stdin, "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"text":         "hello -> world",
	})

	waitForFile(t, render_sibling(src), 5*time.Second)
}

func TestIntegrationSyntaxErrorPublishesDiagnostics(t *testing.T) {
	bin := buildBinary(t)
	workdir := t.TempDir()
	src := filepath.Join(workdir, "broken.d2")
	bad := "this -> "
	if err := os.WriteFile(src, []byte(bad), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cmd := exec.Command(bin)
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lsp: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	rd := bufio.NewReader(stdout)
	uri := pathToURI(src)

	send(t, stdin, "initialize", 1, map[string]any{
		"processId": os.Getpid(), "rootUri": pathToURI(workdir),
		"capabilities": map[string]any{},
	})
	expectResponse(t, rd, 1)
	sendNotify(t, stdin, "initialized", map[string]any{})
	sendNotify(t, stdin, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "d2", "version": 1, "text": bad,
		},
	})
	sendNotify(t, stdin, "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"text":         bad,
	})

	diags := waitForDiagnostics(t, rd, uri, 5*time.Second)
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for malformed d2 source")
	}
	if _, err := os.Stat(render_sibling(src)); !os.IsNotExist(err) {
		t.Errorf("expected SVG to be absent for failing render, got err=%v", err)
	}
}

// --- helpers --------------------------------------------------------

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "d2-lsp")
	cmd := exec.Command("go", "build", "-o", out, "../..")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build lsp: %v", err)
	}
	return out
}

func pathToURI(p string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}

func send(t *testing.T, w io.Writer, method string, id int, params any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	w.Write(body)
}

func sendNotify(t *testing.T, w io.Writer, method string, params any) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": method, "params": params,
	})
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	w.Write(body)
}

func readMessage(rd *bufio.Reader) (map[string]any, error) {
	var contentLength int
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
		}
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(rd, body); err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func expectResponse(t *testing.T, rd *bufio.Reader, id int) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		msg, err := readMessage(rd)
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
		if got, ok := msg["id"].(float64); ok && int(got) == id {
			return msg
		}
	}
	t.Fatalf("timed out waiting for response id=%d", id)
	return nil
}

func waitForDiagnostics(t *testing.T, rd *bufio.Reader, wantURI string, timeout time.Duration) []any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg, err := readMessage(rd)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if msg["method"] == "textDocument/publishDiagnostics" {
			params, _ := msg["params"].(map[string]any)
			if params["uri"] == wantURI {
				ds, _ := params["diagnostics"].([]any)
				return ds
			}
		}
	}
	t.Fatalf("timed out waiting for diagnostics for %q", wantURI)
	return nil
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("file %q never appeared", path)
}

func render_sibling(src string) string {
	dir, base := filepath.Split(src)
	return filepath.Join(dir, "."+base+".svg")
}

// TestIntegrationCodeAction drives the full code-action flow: initialize,
// didOpen, didSave (to produce the sibling SVG), then textDocument/codeAction
// (must include "Open D2 Preview"), then workspace/executeCommand for
// d2.openPreview, and finally assert the server sends a window/showDocument
// request targeting the sibling SVG URI.
func TestIntegrationCodeAction(t *testing.T) {
	bin := buildBinary(t)
	workdir := t.TempDir()
	src := filepath.Join(workdir, "hello.d2")
	if err := os.WriteFile(src, []byte("hello -> world"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	cmd := exec.Command(bin)
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lsp: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	rd := bufio.NewReader(stdout)
	uri := pathToURI(src)
	siblingURI := pathToURI(render_sibling(src))

	send(t, stdin, "initialize", 1, map[string]any{
		"processId":    os.Getpid(),
		"rootUri":      pathToURI(workdir),
		"capabilities": map[string]any{},
	})
	initResp := expectResponse(t, rd, 1)
	result, _ := initResp["result"].(map[string]any)
	caps, _ := result["capabilities"].(map[string]any)
	if caps["codeActionProvider"] == nil {
		t.Error("initialize response missing codeActionProvider")
	}
	if caps["executeCommandProvider"] == nil {
		t.Error("initialize response missing executeCommandProvider")
	}

	sendNotify(t, stdin, "initialized", map[string]any{})
	sendNotify(t, stdin, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "d2", "version": 1, "text": "hello -> world",
		},
	})
	sendNotify(t, stdin, "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"text":         "hello -> world",
	})
	// Wait until the sibling SVG exists, so the executeCommand path takes
	// the showDocument branch (not the "save first" diagnostic branch).
	waitForFile(t, render_sibling(src), 5*time.Second)

	// Drain any in-flight notifications (publishDiagnostics) before the
	// code action exchange.
	send(t, stdin, "textDocument/codeAction", 2, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"range": map[string]any{
			"start": map[string]any{"line": 0, "character": 0},
			"end":   map[string]any{"line": 0, "character": 0},
		},
		"context": map[string]any{"diagnostics": []any{}},
	})
	caResp := expectResponse(t, rd, 2)
	caResult, _ := caResp["result"].([]any)
	if len(caResult) == 0 {
		t.Fatalf("expected at least one code action, got %v", caResp["result"])
	}
	first, _ := caResult[0].(map[string]any)
	cmdEntry, _ := first["command"].(map[string]any)
	if cmdEntry == nil || cmdEntry["command"] != "d2.openPreview" {
		t.Fatalf("expected first code action command == d2.openPreview, got %v", first)
	}

	// Trigger executeCommand. The server will issue a window/showDocument
	// request to us; we must respond with a result so the server's Call
	// completes.
	send(t, stdin, "workspace/executeCommand", 3, map[string]any{
		"command":   "d2.openPreview",
		"arguments": []any{uri},
	})

	// Loop reading messages until we see the server-initiated
	// window/showDocument request. Respond to it, then look for the
	// executeCommand response.
	deadline := time.Now().Add(5 * time.Second)
	sawShow := false
	sawExecResp := false
	for time.Now().Before(deadline) && !(sawShow && sawExecResp) {
		msg, err := readMessage(rd)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if method, ok := msg["method"].(string); ok && method == "window/showDocument" {
			sawShow = true
			params, _ := msg["params"].(map[string]any)
			if params["uri"] != siblingURI {
				t.Errorf("showDocument uri = %v, want %v", params["uri"], siblingURI)
			}
			// Respond to the request so server's Call() returns.
			if id, ok := msg["id"]; ok {
				body, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  map[string]any{"success": true},
				})
				fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n", len(body))
				stdin.Write(body)
			}
			continue
		}
		if id, ok := msg["id"].(float64); ok && int(id) == 3 {
			sawExecResp = true
		}
	}
	if !sawShow {
		t.Fatal("server never sent window/showDocument")
	}
}
