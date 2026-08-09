// go-widgets desktop-shell wasmbox external client (worker entry). Mirrors the
// wasmbox clients/*/worker.js convention: import the client SDK, construct a
// WasmboxClient, boot the Go/wasm program (main.go) which paints the whole
// shell composition (dock, launcher, application menu, file grid) and handles
// input. Static spawn loads ./desktop.wasm + ../../wasm_exec.js; an OCI spawn
// pulls the same assets from the envelope the compositor sends.
//
// Copyright (c) 2026 the go-widgets/desktop authors. BSD-3-Clause.

"use strict";

const isOCI = self.location.protocol === "blob:";
if (isOCI) {
  importScripts(self.location.origin + "/clients/sdk/sdk.js");
} else {
  importScripts("../sdk/sdk.js");
  importScripts("../../wasm_exec.js");
}

const client = new WasmboxClient({ title: "Desktop", w: 960, h: 600 });
self.wasmboxClient = client;

client.start().then(async () => {
  const assets = isOCI
    ? await WasmboxClient.bootFromOCIAssets({ fallbackMs: 2000 })
    : null;
  if (isOCI) {
    if (!assets) throw new Error("desktop: no OCI assets envelope");
    importScripts(assets.wasm_exec_url);
  }
  const go = new Go();
  const wasmURL = assets ? assets.wasm_url : "./desktop.wasm";
  const instance = await WasmboxClient.bootWasm(wasmURL, go.importObject, {
    bg:    [245, 245, 245],
    track: [218, 220, 224],
    fill:  [ 53, 132, 228],
  });
  go.run(instance);
});
