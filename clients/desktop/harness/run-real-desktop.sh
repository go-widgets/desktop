#!/usr/bin/env bash
# PRIMARY browser proof for the go-widgets/desktop shell: run it as a genuine
# EXTERNAL client of the REAL wasmdesk/wasmbox Ruby desktop compositor.
#
# It clones wasmbox READ-ONLY, builds the real compositor (wasmbox.wasm) + dev
# server (cmd/serve) with its own Taskfile (GOWORK=off), builds this repo's
# desktop client to js/wasm, assembles a SAME-ORIGIN serving tree by SYMLINKING
# the unmodified wasmbox tree plus this client dir (NO writes into the wasmbox
# checkout — `git status` of the clone stays clean), serves it with wasmbox's own
# COOP/COEP server, and runs the Playwright probe (probe-real-desktop.mjs), which
# spawns the shell via globalThis.wasmboxSpawnExternal and captures the real
# composited desktop. Screenshot -> $OUT (default: docs/).
#
# Usage:  clients/desktop/harness/run-real-desktop.sh
# Env:    OUT (screenshot dir), PORT (default 8100), CHROMIUM_PATH,
#         WASMBOX_DIR (existing read-only wasmbox checkout; else it is cloned).
#
# Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
# SPDX-License-Identifier: BSD-3-Clause
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="${OUT:-$REPO/docs}"
PORT="${PORT:-8100}"
STAMP="2026-08-09"
WORK="$(mktemp -d)"

echo "== read-only wasmbox clone + real compositor/serve build (GOWORK=off) =="
if [ -z "${WASMBOX_DIR:-}" ]; then
  WASMBOX_DIR="$WORK/wasmbox-ro"
  git clone --depth 1 git@github.com:wasmdesk/wasmbox.git "$WASMBOX_DIR"
fi
( cd "$WASMBOX_DIR" && GOWORK=off task build:compositor build:serve )
test -f "$WASMBOX_DIR/wasmbox.wasm" && test -x "$WASMBOX_DIR/bin/wasmbox-serve"

echo "== build the desktop client (CGO=0 js/wasm) =="
STAGE="$WORK/client/clients/desktop"
mkdir -p "$STAGE"
cp "$REPO/clients/desktop/worker.js" "$STAGE/worker.js"
( cd "$REPO" && GOWORK=off GOOS=js GOARCH=wasm CGO_ENABLED=0 \
    go build -trimpath -ldflags '-s -w' -o "$STAGE/desktop.wasm" ./clients/desktop/ )

echo "== same-origin symlink overlay (no writes into the wasmbox checkout) =="
ROOT="$WORK/serveroot"
mkdir -p "$ROOT/clients"
for e in "$WASMBOX_DIR"/*; do
  [ "$(basename "$e")" = clients ] || ln -s "$e" "$ROOT/"
done
for c in "$WASMBOX_DIR"/clients/*; do ln -s "$c" "$ROOT/clients/"; done
ln -s "$STAGE" "$ROOT/clients/desktop"   # the go-widgets desktop shell client

echo "== wasmbox checkout still clean? =="
( cd "$WASMBOX_DIR" && git status --short )

echo "== serve with wasmbox's own COOP/COEP server + run the probe =="
"$WASMBOX_DIR/bin/wasmbox-serve" -addr "127.0.0.1:$PORT" -dir "$ROOT" >"$WORK/serve.log" 2>&1 &
SERVE_PID=$!
trap 'kill "$SERVE_PID" 2>/dev/null || true' EXIT
sleep 1

mkdir -p "$OUT"
WASMBOX_BASE_URL="http://127.0.0.1:$PORT" \
  WASMBOX_SHOT="$OUT/browser-wasmdesk-real-desktop-$STAMP.png" \
  node "$REPO/clients/desktop/harness/probe-real-desktop.mjs"
echo "== screenshot written to $OUT =="
