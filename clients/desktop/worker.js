// go-widgets desktop-shell wasmbox external client (worker entry).
//
// A wasmdesk/wasmbox compositor spawns this script as a dedicated Web Worker
// (wasmboxSpawnExternal). It does NOT import the wasmbox client SDK: the
// go-widgets `window` backend (github.com/go-widgets/window internal/wasmbox,
// //go:build js && wasm) implements the wasmbox client wire protocol itself — it
// allocates the surface SharedArrayBuffer, posts `hello` over the per-client
// MessagePort, awaits `welcome`, paints the shell's widget tree into the SAB and
// posts `commit`. This is the identical backend the native binary reaches
// through window.Open on X11/Wayland (open_linux.go); on js/wasm Open returns the
// wasmbox client (open_js.go). So this worker only needs to:
//
//   1. load Go's wasm_exec.js runtime shim,
//   2. buffer any messages the compositor posts before the wasm boots — the port
//      handoff (`{type:"__wasmbox_port", port}`) can beat go.run — and replay
//      them once the Go backend installs its handler via
//      globalThis.__gowidgetsInstall (window's fixed client hook), and
//   3. instantiate + run desktop.wasm (clients/desktop/main.go).
//
// wasm_exec.js and desktop.wasm are expected alongside/above this file (see the
// harness assembler and the real-desktop overlay in clients/desktop/harness/).
// SharedArrayBuffer requires the page to be served cross-origin isolated
// (COOP/COEP); wasmbox's cmd/serve does that.

"use strict";

importScripts("../../wasm_exec.js");

// Buffer `self` messages until the Go backend installs its handler. The
// compositor's one-shot port handoff is the first message it sends after spawn
// and may arrive before go.run() starts, so it must not be dropped.
let _sink = [];
let _handler = null;
self.onmessage = (e) => {
  if (_handler) _handler(e.data);
  else _sink.push(e.data);
};

// The Go backend (window internal/wasmbox.Dial) calls this to take over message
// delivery; we replay whatever was buffered, in order.
globalThis.__gowidgetsInstall = (fn) => {
  _handler = fn;
  const queued = _sink;
  _sink = [];
  for (const m of queued) fn(m);
};

const go = new Go();
WebAssembly.instantiateStreaming(fetch("./desktop.wasm"), go.importObject)
  .then((result) => go.run(result.instance))
  .catch((err) => {
    // Surface boot failures so a Playwright probe can assert on them.
    self.wasmboxError = String(err);
    throw err;
  });
