<!-- Copyright (c) 2026 the go-widgets/desktop authors. SPDX-License-Identifier: BSD-3-Clause -->
# Real-desktop proof — the shell on the actual wasmdesk compositor

`probe-real-desktop.mjs` (driven by `run-real-desktop.sh`) is the **primary**
browser proof: it runs the go-widgets desktop shell as a genuine EXTERNAL client
of the **real [wasmdesk/wasmbox](https://github.com/wasmdesk/wasmbox) Ruby
desktop** — the pure-Go (CGO=0)
[`rbgo`](https://github.com/go-embedded-ruby/ruby) interpreter running the Ruby
compositor (`compositor/*.rb`, baked into `wasmbox.wasm`), served by wasmbox's
own COOP/COEP `cmd/serve` — not the `index.html` stand-in.

The shell's wasmbox client wire lives entirely in
[`go-widgets/window`](https://github.com/go-widgets/window) (`internal/wasmbox`),
the SAME backend the native binary reaches through `window.Open` on X11/Wayland:
`clients/desktop` just calls `window.Open()` + `Backend.Run(sc.HostRoot())`.

**The wasmbox repository is never modified.** It is cloned read-only and built
and run as-is; the shell client is made reachable *same-origin* through a symlink
overlay directory, so `cmd/serve` serves both the unmodified compositor and the
client without a single write into the wasmbox checkout (`git status` of the
clone stays clean).

## What the probe asserts

1. The **real Ruby compositor boots** (`window.wasmboxReady`, the
   `rbgo compositor: started with N windows` startup line, cross-origin
   isolation active).
2. The compositor runs **off the main thread** (step C) — its OffscreenCanvas
   pixels are reachable only from the compositor Web Worker, via the
   `__wasmboxReadRegion` test hook.
3. Spawning `clients/desktop/worker.js` through the documented
   `globalThis.wasmboxSpawnExternal(...)` hook makes the compositor **focus** the
   shell window; its live rect is read with `__wasmboxFocusedRect` (no hardcoded
   position) — the shell's large 960×600 surface is unambiguous against the
   boot's auto-spawned `clients/hello` + dock.
4. The **shell rendered** into the composited desktop at that rect — dock,
   launcher, application menu and file grid from the embedded source (near-full
   coverage + non-trivial luma variance = real widget structure, not a flat
   fill).
5. A **real `page.mouse.click`** on the menubar — a genuine DOM event the page
   relays to the compositor worker, which routes it to the focused window —
   round-trips to a `toolkit.Event`: the composited pixels change (a menu opens /
   the shell repaints). This exercises the **real compositor input routing**.
6. A screenshot of the composited desktop with the shell window is saved to
   `docs/browser-wasmdesk-real-desktop-2026-08-09.png`.

## Reproduce

```sh
# One command does it all (clone read-only, build compositor+serve+client,
# overlay, serve, probe). Needs: go, task, node, a Playwright Chromium.
export CHROMIUM_PATH=/path/to/chromium        # a Playwright-installed build
clients/desktop/harness/run-real-desktop.sh   # -> docs/browser-wasmdesk-real-desktop-*.png
```

Expected tail:

```
ok   real Ruby compositor booted (rbgo compositor: started with 3 windows)
ok   real compositor focused the desktop shell window (rect=172,172 960x600)
ok   desktop shell (dock/launcher/menu/file-grid) rendered at (172,172) (nonblackPct=100, variance=1064)
ok   click round-tripped through the REAL compositor: composited pixels changed ...
RESULT: PASS
```

The `run.sh` + `index.html` + `drive.js` trio is kept as the deterministic CI
floor: it runs the identical wire/SAB/input assertions without building the Ruby
compositor, so CI (`browser-wasmdesk` lane) can gate on it anywhere.
