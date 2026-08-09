# wasmbox (wasmdesk) browser proof harness

This harness runs the go-widgets desktop shell as a **wasmbox external client**
in a real headless browser and captures the composited surface — the browser
half of the "same source, two targets" verification.

It is **protocol-faithful**: it uses the *unmodified* wasmbox client SDK
(`clients/sdk/sdk.js`) in a real Web Worker hosting the real Go/wasm client
(`clients/desktop`), served by the real wasmbox dev server (`cmd/serve`) with the
COOP/COEP headers a `SharedArrayBuffer` page requires. The one piece it stands in
for is the Ruby window-manager compositor: `index.html` is a minimal compositor
that speaks the exact wire the Ruby one does for a single client — the
`__wasmbox_port` handoff, `hello` → `welcome`, blitting each `commit`'s shared
surface onto a canvas, and forwarding `input` events.

The full Ruby desktop is stood in for because spawning a *newly added* client
inside it would require editing wasmbox's boot configuration, and wasmbox is used
strictly read-only here.

## Run

```sh
clients/desktop/harness/run.sh          # OUT=docs by default
```

`run.sh` assembles the serving tree (real SDK + `wasm_exec.js` + the compiled
`desktop.wasm` + `worker.js`), builds and starts `wasmbox/cmd/serve`, and drives
the page with Playwright (`drive.js`). It writes:

- `docs/browser-wasmdesk-initial-*.png` — the shell rendered from the embedded
  source, inside the browser, over the wasmbox wire.
- `docs/browser-wasmdesk-after-input-*.png` — after one input round-trip.

The driver asserts cross-origin isolation (a real `SharedArrayBuffer`), a rich
first frame, and that the input drove a repaint.
