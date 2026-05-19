# Zed D2 Viewer Extension — Design

Date: 2026-05-19
Status: Pending user review

## Goal

Provide a Zed extension that lets users view rendered D2 diagrams while editing `.d2` source files, in the closest UX possible to a "markdown preview"-style workflow. The Zed extension API does not expose a webview/preview panel, so the rendered surface is an SVG file Zed renders natively in a split pane and auto-reloads on disk change.

## Non-goals

- A custom webview/HTML preview panel inside Zed (API does not support this)
- Live render-while-typing (defer; MVP renders on save)
- Configuration UI for d2 themes, layout engines, sketch mode (defer; d2 defaults for MVP)
- Diagram editing UX beyond the source `.d2` file
- Cross-editor support (VSCode, Neovim) — out of scope, though the Go LSP could be reused later

## User experience

1. User installs the extension from Zed's extension registry (or as a dev extension during development).
2. User opens `foo.d2`. Zed activates the extension, downloads the matching `d2-lsp` binary if absent, spawns it.
3. User edits and saves `foo.d2`. The LSP renders the diagram to `.foo.d2.svg` sibling file. Parse errors appear as squiggles in the source pane.
4. User opens `.foo.d2.svg` once via Cmd-P and drags it to a split pane. Zed renders SVG natively. Every subsequent save updates the SVG on disk; Zed auto-reloads the split pane.

Documented in README: the one-time manual step of opening the SVG sibling and placing it in a split pane (no extension API to do this automatically).

## Architecture

Two units, one repository.

### Unit 1: Zed extension (Rust → WebAssembly)

Responsibilities:
- Register the `d2` language with Zed (file suffix, comments, brackets, tree-sitter grammar reference).
- Manage the `d2-lsp` binary lifecycle: detect cached binary for the user's OS+arch under Zed's extension data dir; if absent, download the matching release artifact from this repository's GitHub Releases; return a `Command` pointing Zed at the binary for stdio LSP.
- No render logic, no general file IO.

Crate type: `cdylib`, depends on `zed_extension_api`. Implements `zed::Extension`, registered via `zed::register_extension!`.

### Unit 2: `d2-lsp` (Go)

Responsibilities:
- Speak LSP over stdio.
- Maintain an in-memory copy of the latest text per open document.
- On `textDocument/didSave`: compile the document with `oss.terrastruct.com/d2/d2lib`, render with `d2svg`, atomically write a sibling SVG next to the source named `.<source-filename>.svg` (e.g. `foo.d2` → `.foo.d2.svg`, keeping the original extension as part of the dot-prefixed name).
- On compile errors: publish LSP diagnostics with line/column/severity. Keep the existing on-disk SVG (last-good) so the preview pane does not flicker or blank.
- Embeds the d2 library directly — no separate `d2` CLI dependency.

LSP framework: `tliron/glsp` or `go.lsp.dev/protocol` (final choice during implementation; both viable).

## Repository layout

```
zed-extention-d2-viewer/
├── extension.toml                # Zed manifest
├── Cargo.toml                    # WASM extension crate
├── src/lib.rs                    # Extension trait impl, LSP download
├── languages/d2/
│   ├── config.toml               # language config: name, ext .d2, comments, brackets
│   └── highlights.scm            # tree-sitter highlight queries
├── grammars/d2.toml              # tree-sitter-d2 git+rev pin
├── lsp/
│   ├── go.mod                    # imports oss.terrastruct.com/d2
│   ├── main.go                   # stdio LSP loop
│   └── internal/
│       ├── render/render.go      # compile + render + atomic write
│       └── diag/diag.go          # d2 error → LSP Diagnostic mapping
├── examples/basic.d2             # smoke-test fixture
├── LICENSE                       # Apache-2.0 (required by Zed)
└── README.md
```

## Components

### A. Zed extension (`src/lib.rs`)

Implements `zed::Extension`. Key method:

- `language_server_command(language_server_id, worktree) -> Result<Command>`
  - Resolves `(GOOS, GOARCH)` from `zed::current_platform()`.
  - Computes expected binary filename (e.g. `d2-lsp-darwin-arm64`).
  - If cached under `${ExtensionDir}/bin/<version>/`, returns it.
  - Otherwise downloads `https://github.com/<owner>/<repo>/releases/download/<version>/d2-lsp-<os>-<arch>(.exe)` via the `zed_extension_api` download helper, makes executable, caches, returns Command.
  - Errors bubble up as Zed extension activation errors visible in `zed.log`.

### B. Tree-sitter + highlights

- `grammars/d2.toml`: pins a tree-sitter-d2 grammar (best-maintained option to be validated during implementation; pleshevskiy/tree-sitter-d2 is a current candidate). Pinned by commit SHA.
- `languages/d2/config.toml`: name `D2`, `path_suffixes = ["d2"]`, `line_comments = ["# "]`, brackets, autoclose pairs.
- `languages/d2/highlights.scm`: tree-sitter highlight queries mapping nodes to standard Zed/Helix captures (`@keyword`, `@string`, `@variable`, `@punctuation.delimiter`, etc.).

### C. Go LSP (`lsp/`)

`main.go`: stdio JSON-RPC server loop. Capabilities advertised in `initialize`:

- `textDocumentSync = Full`
- `publishDiagnostics = true`
- All other capabilities absent (no completion, hover, etc., in MVP).

Document state: `map[DocumentURI]string` of latest text, protected by a `sync.RWMutex`.

Handlers:
- `initialize` / `initialized` / `shutdown` / `exit`: standard.
- `textDocument/didOpen`: store text; no render.
- `textDocument/didChange`: update text; no render.
- `textDocument/didSave`: render (see below).
- `textDocument/didClose`: drop state.

`internal/render/render.go`:
- One render goroutine per document, serialized via a per-URI mutex map.
- `Compile(ctx, text, opts)` → diagram or compile error.
- `d2svg.Render(diagram, renderOpts)` → SVG bytes.
- Write to `<dir>/.<basename>.svg.tmp.<pid>.<nanos>` then `os.Rename` to `<dir>/.<basename>.svg` (POSIX atomic).
- 5-second context timeout; cancel render if exceeded; emit a diagnostic explaining timeout.

`internal/diag/diag.go`:
- Convert d2 compile errors (which carry line/column ranges) into `protocol.Diagnostic` with `Severity = Error`, `Source = "d2"`.
- On render success, publish empty diagnostics for that URI to clear stale errors.

## Data flow

### First open
```
User opens foo.d2
  → Zed matches path_suffixes=["d2"] → activates extension
  → Extension resolves OS+arch, downloads d2-lsp if uncached, returns Command
  → Zed spawns d2-lsp over stdio
  → initialize handshake; capabilities exchanged
  → didOpen sent; LSP caches text; no render
```

### Edit and save
```
User edits (didChange events → LSP updates text; no render)
User saves
  → didSave (text included)
  → LSP per-URI mutex acquired
  → d2lib.Compile(ctx, text)
      Success → d2svg.Render → atomic rename to .foo.d2.svg
                publishDiagnostics(uri, []) clears stale errors
      Failure → publishDiagnostics(uri, [mapped errors])
                Existing .foo.d2.svg untouched (last-good preserved)
```

### View preview
```
User opens .foo.d2.svg once via Cmd-P
User drags to split pane
  → Zed renders SVG natively
  → Subsequent saves rewrite .foo.d2.svg atomically
  → Zed detects external file change → reloads split pane
```

## Error handling

| Failure | Source | Behavior |
|---|---|---|
| Tree-sitter parse error | Zed editor | Degraded highlighting; no extension action |
| D2 compile error (syntax) | LSP render | `publishDiagnostics` with line/col; keep last-good SVG |
| D2 compile error (semantic) | LSP render | Same as syntax |
| LSP binary missing/corrupt | Extension activation | Re-download attempt; on failure log to `zed.log` and surface activation error; tree-sitter highlighting still works |
| GitHub download fails | Extension activation | Surface activation error in `zed.log`; expose failure-mode fallback Zed setting `lsp.d2.binary_path` so the user can point at a manually-installed binary. This setting exists only as an escape hatch — it is not the primary configuration path |
| OS/arch not in release matrix | Extension activation | Clear error message listing supported platforms; same `lsp.d2.binary_path` fallback applies |
| SVG write fails (perm/disk) | LSP render | `window/showMessage` warning + diagnostic on doc with OS error text |
| Render timeout (5s) | LSP render | Cancel via context; diagnostic "Render timed out at 5s" |
| didSave on read-only file | LSP render | Skip render; no error (Zed normally won't issue didSave) |

Release matrix: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`. Built via GitHub Actions on git tag, attached to GitHub release.

## Testing

### Unit tests (Go)
- `internal/render`: golden tests — fixture `.d2` files compared against pinned SVG bytes (with d2 randomization seeded for determinism).
- `internal/diag`: error mapper — crafted d2 compile errors → asserted LSP Diagnostic fields.
- Atomic write: concurrent renders to the same URI assert no torn writes.

### Integration tests (Go)
- Spawn `d2-lsp` as a subprocess, drive via LSP JSON-RPC over stdin/stdout.
- Valid path: `initialize → didOpen → didSave → assert SVG file present, empty diagnostics`.
- Error path: `didSave with invalid syntax → assert diagnostics published, SVG file unchanged`.

### Manual tests (Zed extension)
- Install via `zed: install dev extension`.
- Open `examples/basic.d2`, save, verify `.basic.d2.svg` appears.
- Open the SVG, place in split pane, edit source, save, verify split-pane reload.
- Introduce a syntax error, verify diagnostic squiggle, fix, verify squiggle clears.
- Primary platform: macOS Apple Silicon. Other platforms covered by release CI.

No automated end-to-end harness exists in Zed today. Manual smoke test steps documented in `CONTRIBUTING.md`.

## License

Apache-2.0 at the repository root (required by Zed for publishing as of 2025-10-01). The license applies to extension code (`src/`, `lsp/`); third-party dependencies retain their own licenses.

## Open items deferred to implementation planning

- Pick exact tree-sitter-d2 grammar fork and pin commit.
- Pick exact Go LSP framework (`tliron/glsp` vs `go.lsp.dev/protocol`).
- Release tag/version scheme that the WASM extension uses to look up matching `d2-lsp` binary.
- Whether to publish to Zed's extension registry in this milestone or after the project has shaken out as a dev extension.
