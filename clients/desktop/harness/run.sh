#!/usr/bin/env bash
# Reproducible wasmbox (wasmdesk) browser proof for the go-widgets/desktop
# client — the DETERMINISTIC CI FLOOR. Assembles a serving tree from the REAL
# wasmbox dev server (a read-only clone of wasmdesk/wasmbox, for the COOP/COEP
# headers a SharedArrayBuffer page needs) and this repo's Go/wasm client, serves
# it, and drives the protocol-faithful harness (index.html) with Playwright
# (clients/desktop/harness/drive.js). The client's wasmbox wire lives entirely in
# go-widgets/window, so this needs no wasmbox client SDK. Screenshots land in
# $OUT (default: docs/). The PRIMARY proof against the real Ruby desktop is
# clients/desktop/harness/probe-real-desktop.mjs.
#
# Usage:  clients/desktop/harness/run.sh
# Env:    OUT (screenshot dir), PORT (default 8099), STAMP (date stamp),
#         WASMBOX_DIR (existing wasmbox checkout; else it is cloned).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="${OUT:-$REPO/docs}"
PORT="${PORT:-8099}"
STAMP="${STAMP:-$(date +%Y-%m-%d)}"
WORK="$(mktemp -d)"
HARNESS="$WORK/harness"

echo "== assembling harness tree in $WORK =="
mkdir -p "$HARNESS/clients/desktop"

# The wasmbox read-only clone (dev server for COOP/COEP). Never modified.
if [ -z "${WASMBOX_DIR:-}" ]; then
  WASMBOX_DIR="$WORK/wasmbox"
  git clone --depth 1 https://github.com/wasmdesk/wasmbox.git "$WASMBOX_DIR"
fi

# Go wasm_exec.js + the compiled desktop client + its worker.
GOROOT="$(go env GOROOT)"
if [ -f "$GOROOT/lib/wasm/wasm_exec.js" ]; then cp "$GOROOT/lib/wasm/wasm_exec.js" "$HARNESS/wasm_exec.js";
else cp "$GOROOT/misc/wasm/wasm_exec.js" "$HARNESS/wasm_exec.js"; fi
cp "$REPO/clients/desktop/worker.js" "$HARNESS/clients/desktop/worker.js"
cp "$REPO/clients/desktop/harness/index.html" "$HARNESS/index.html"
( cd "$REPO" && GOOS=js GOARCH=wasm CGO_ENABLED=0 \
    go build -trimpath -ldflags '-s -w' -o "$HARNESS/clients/desktop/desktop.wasm" ./clients/desktop/ )

echo "== building + starting the real wasmbox dev server (cmd/serve) =="
( cd "$WASMBOX_DIR" && GOWORK=off CGO_ENABLED=0 go build -o "$WORK/wasmbox-serve" ./cmd/serve )
"$WORK/wasmbox-serve" -addr ":$PORT" -dir "$HARNESS" >"$WORK/serve.log" 2>&1 &
SERVE_PID=$!
trap 'kill "$SERVE_PID" 2>/dev/null || true' EXIT
sleep 1

echo "== driving the browser proof with Playwright =="
mkdir -p "$OUT"
OUT="$OUT" BASE="http://localhost:$PORT" STAMP="$STAMP" node "$REPO/clients/desktop/harness/drive.js"
echo "== screenshots written to $OUT =="
