# Zed D2 Viewer

Zed extension + Go language server that renders D2 diagrams to a sibling SVG file on save.

See `docs/superpowers/specs/2026-05-19-d2-viewer-design.md` for the design.

## Status

In development. Not yet published to Zed's extension registry.

## Local install (dev extension)

1. Build the LSP: `cd lsp && go build -o ../bin/d2-lsp .`
2. In Zed: `zed: install dev extension` and pick this repo's root.
3. Open a `.d2` file, edit, save. The sibling `.<filename>.svg` is rewritten.
4. Open the sibling once via Cmd-P and drag into a split pane. Subsequent saves auto-refresh it.
