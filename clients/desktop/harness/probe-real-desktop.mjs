// Primary browser proof for the go-widgets/desktop shell, running as a genuine
// external client of the REAL wasmdesk/wasmbox Ruby desktop compositor.
//
// Horodate: 2026-08-09 22:43 CEST
//
// Unlike clients/desktop/harness/drive.js (which drives index.html, a
// protocol-faithful compositor stand-in — kept as the deterministic CI floor),
// this probe drives the ACTUAL wasmbox desktop: the pure-Go (CGO=0) rbgo
// interpreter running the Ruby compositor (compositor/*.rb) baked into
// wasmbox.wasm, served by wasmbox's own COOP/COEP cmd/serve. The desktop shell
// client is spawned as a genuine EXTERNAL client through the documented
// `globalThis.wasmboxSpawnExternal("clients/desktop/worker.js")` hook — a
// separate Web Worker + wasm instance talking to the compositor over the
// step-C.1 MessagePort + SharedArrayBuffer surface. Its wasmbox wire lives
// entirely in go-widgets/window (internal/wasmbox), the SAME backend the native
// binary reaches through window.Open on X11/Wayland. The wasmbox repository is
// UNMODIFIED; the client files are served same-origin via a symlink overlay (see
// clients/desktop/harness/README-real-desktop.md).
//
// Strategy (mirrors go-widgets/window's probe-wasmbox-real.mjs):
//   1. Boot the real compositor (window.wasmboxReady === true + the Ruby
//      "rbgo compositor: started with N windows" startup line).
//   2. Spawn the desktop shell client and locate it dynamically via the real
//      compositor's live focused-window rect (__wasmboxFocusedRect — no
//      hardcoded slot). The shell requests a large 960x600 surface, so it is
//      unambiguously distinguishable from the boot's auto-spawned clients
//      (clients/hello + the dock).
//   3. Assert the shell (dock, launcher, application menu, file grid — the
//      embedded source) actually RENDERED into the composited desktop at that
//      rect: near-full coverage (the light shell theme) + real structure
//      (non-trivial luma variance, i.e. widgets, not a flat fill).
//   4. Inject a REAL page.mouse.click on the menubar the compositor routes to the
//      focused window and assert the composited pixels change (a menu opened /
//      the shell repainted): input round-tripped to a toolkit.Event THROUGH the
//      real compositor input routing.
//   5. Save a screenshot of the real composited desktop with the shell window.
//
// Exit non-zero on any failed assertion.

import { chromium } from "playwright";
import { writeFileSync } from "node:fs";

const base = (process.env.WASMBOX_BASE_URL || "http://127.0.0.1:8100").replace(/\/+$/, "");
const out = process.env.WASMBOX_SHOT || "docs/browser-wasmdesk-real-desktop-2026-08-09.png";
const executablePath = process.env.CHROMIUM_PATH || undefined;
const BOOT_TIMEOUT_MS = 60000;

// The desktop shell requests a 960x600 surface. We do NOT hardcode where the
// compositor cascade-places it (boot auto-spawns clients/hello + the dock, so
// the exact slot depends on the build) NOR assume it is granted verbatim (the
// compositor may clamp to fit the workarea); instead we read the real
// compositor's live geometry for the freshly-spawned, raised-and-focused window
// via __wasmboxFocusedRect() and use whatever large rect it reports.
const MIN_W = 600, MIN_H = 400; // the shell is the only window this large
// Menubar click target in surface-local coordinates (north border, first menu).
const MENU_LOCAL = { x: 40, y: 12 };

let failed = false;
const check = (cond, msg) => {
  if (cond) console.log(`ok   ${msg}`);
  else { console.error(`FAIL ${msg}`); failed = true; }
};

// Find the compositor worker (the Web Worker exposing __wasmboxReadRegion — the
// compositor moved off the main thread in step C, so its OffscreenCanvas pixels
// are only reachable from inside its worker).
async function compositorWorker(page) {
  for (let i = 0; i < 100; i++) {
    for (const w of page.workers()) {
      try {
        if (await w.evaluate(() => typeof globalThis.__wasmboxReadRegion === "function")) return w;
      } catch { /* worker navigating */ }
    }
    await page.waitForTimeout(200);
  }
  return null;
}
const readRegion = (cw, r) => cw.evaluate(({ x, y, w, h }) => globalThis.__wasmboxReadRegion(x, y, w, h), r);
const focusedRect = (cw) => cw.evaluate(() => globalThis.__wasmboxFocusedRect());

const browser = await chromium.launch({ headless: true, executablePath });
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  const consoleLines = [];
  const pageErrors = [];
  page.on("console", (m) => consoleLines.push(m.text()));
  page.on("pageerror", (e) => pageErrors.push(String(e)));

  await page.goto(`${base}/`, { waitUntil: "load" });

  // 1. Real compositor boots.
  await page.waitForFunction(() => {
    if (globalThis.wasmboxError) throw new Error(String(globalThis.wasmboxError));
    return globalThis.wasmboxReady === true;
  }, { timeout: BOOT_TIMEOUT_MS });
  check(await page.evaluate(() => self.crossOriginIsolated === true),
    "page is cross-origin isolated (COOP/COEP via wasmbox cmd/serve)");
  const startup = consoleLines.find((l) => /rbgo compositor: started with \d+ windows/.test(l));
  check(!!startup, `real Ruby compositor booted (${startup || "no startup line"})`);

  const cw = await compositorWorker(page);
  check(!!cw, "found the compositor Web Worker (real step-C off-main-thread compositor)");
  if (!cw) throw new Error("no compositor worker");

  // 2. Spawn the desktop shell as a real external client, then locate it: the
  //    compositor raises+focuses a freshly-registered external window, so its
  //    live focused-window rect resolves to the shell surface once its first
  //    frame is published.
  await page.evaluate(() => globalThis.wasmboxSpawnExternal("clients/desktop/worker.js"));

  let fr = null;
  for (let i = 0; i < 300; i++) {
    await page.waitForTimeout(150);
    fr = await focusedRect(cw);
    if (fr && fr.w >= MIN_W && fr.h >= MIN_H) break;
  }
  check(!!fr && fr.w >= MIN_W && fr.h >= MIN_H,
    `real compositor focused the desktop shell window (rect=${fr ? `${fr.x},${fr.y} ${fr.w}x${fr.h}` : "null"})`);
  if (!fr || fr.w < MIN_W || fr.h < MIN_H) throw new Error("desktop shell window never became focused");

  const BODY = { x: fr.x, y: fr.y, w: fr.w, h: fr.h };

  // 3. Assert the shell tree actually rendered into the composited desktop at the
  //    located rect: near-full coverage (the light shell theme) + structure
  //    (dock/launcher/menubar/file-grid — a flat fill would have ~0 variance).
  let body = await readRegion(cw, BODY);
  for (let i = 0; i < 40 && !(body.nonblackPct > 90 && body.variance > 150); i++) {
    await page.waitForTimeout(150);
    body = await readRegion(cw, BODY);
  }
  check(body.nonblackPct > 90 && body.variance > 150,
    `desktop shell (dock/launcher/menu/file-grid) rendered at (${BODY.x},${BODY.y}) (nonblackPct=${body.nonblackPct}, variance=${body.variance})`);

  // 4. Read the window hash, then inject a REAL click on the menubar through the
  //    real compositor input routing (DOM event -> page relay -> compositor
  //    worker -> focused external window -> toolkit.Event -> repaint).
  const h0 = await readRegion(cw, BODY);
  await page.mouse.click(BODY.x + MENU_LOCAL.x, BODY.y + MENU_LOCAL.y);

  let h1 = h0;
  for (let i = 0; i < 60; i++) {
    await page.waitForTimeout(200);
    h1 = await readRegion(cw, BODY);
    if (h1.hash !== h0.hash) break;
  }
  check(h1.hash !== h0.hash,
    "click round-tripped through the REAL compositor: composited pixels changed — input reached a toolkit.Event");

  // 5. Save the real composited desktop screenshot.
  const shot = await page.screenshot({ type: "png" });
  writeFileSync(out, shot);
  console.log(`saved ${out} (${shot.length} bytes)`);

  const relevant = pageErrors.filter((e) => /desktop|gowidgets/i.test(e));
  check(relevant.length === 0, `no desktop client page errors${pageErrors.length ? ` (${pageErrors.length} unrelated ignored)` : ""}`);
} finally {
  await browser.close();
}

console.log(failed ? "\nRESULT: FAIL" : "\nRESULT: PASS");
process.exit(failed ? 1 : 0);
