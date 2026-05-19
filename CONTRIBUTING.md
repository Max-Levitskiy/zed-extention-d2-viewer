# Contributing

## Running the smoke test inside Zed

1. Build the LSP locally if you don't want to wait on the release pipeline:
   `cd lsp && go build -o bin/d2-lsp .` (writes to `lsp/bin/d2-lsp`, which is already in `.gitignore`).
   Then point Zed at it by editing your project's `.zed/settings.json`:
   ```json
   { "lsp": { "d2-lsp": { "binary": { "path": "<absolute-path>/lsp/bin/d2-lsp" } } } }
   ```
2. In Zed, run `zed: install dev extension` and pick this repo's root.
3. Open `examples/basic.d2`. Confirm syntax highlighting renders.
4. Save the file (no edits needed). Confirm `.basic.d2.svg` appears alongside.
5. Open `.basic.d2.svg` via Cmd-P, drag it to a split pane.
6. Edit `examples/basic.d2` (e.g. add `cache -> db`), save. Confirm the split pane SVG refreshes.
7. Introduce a syntax error (e.g. delete the target of an arrow). Save. Confirm a red squiggle in the source pane and that the SVG in the split pane is unchanged from the previous good render.
8. Fix the syntax error. Save. Confirm the squiggle clears and the SVG updates.

If any step fails, capture `zed: open log` output and attach it to the issue you file.
