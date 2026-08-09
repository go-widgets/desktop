// Playwright driver for the desktop shell's wasmbox (wasmdesk) browser proof.
//
// It loads the protocol-faithful harness (index.html) — which runs the REAL
// wasmbox client SDK (clients/sdk/sdk.js) in a REAL Web Worker hosting the Go/
// wasm desktop client — in headless Chromium, waits for the client to paint its
// scene from the embedded source, screenshots the composited canvas, drives one
// input round-trip over the wire (a menubar click) and screenshots again,
// asserting cross-origin isolation (real SharedArrayBuffer), a rich frame and a
// visible post-input change.
//
// Env: BASE (default http://localhost:8099), OUT (screenshot dir, default ".").
"use strict";
const { chromium } = require("playwright");
const fs = require("fs");

const BASE = process.env.BASE || "http://localhost:8099";
const OUT = process.env.OUT || ".";
const STAMP = process.env.STAMP || "2026-08-09";

(async () => {
  const browser = await chromium.launch({ args: ["--no-sandbox"] });
  const page = await browser.newPage({ viewport: { width: 1000, height: 720 } });
  page.on("console", (m) => console.log("[page]", m.text()));
  page.on("pageerror", (e) => console.log("[pageerror]", e.message));

  await page.goto(BASE + "/index.html", { waitUntil: "load" });
  const isolated = await page.evaluate(() => self.crossOriginIsolated);
  console.log("crossOriginIsolated =", isolated);

  await page.waitForFunction(() => window.__hstate && window.__hstate.lastColors > 24, null, { timeout: 30000 });
  await page.waitForTimeout(800);
  const s1 = await page.evaluate(() => ({ ...window.__hstate, sab: undefined }));
  console.log("initial render:", JSON.stringify(s1));
  await page.locator("#screen").screenshot({ path: `${OUT}/browser-wasmdesk-initial-${STAMP}.png` });

  const before = s1.commits;
  await page.evaluate(() => {
    window.__input({ kind: "mousemove", x: 48, y: 12 });
    window.__input({ kind: "mousedown", x: 48, y: 12, button: 0 });
    window.__input({ kind: "mouseup", x: 48, y: 12, button: 0 });
  });
  await page.waitForFunction((b) => window.__hstate.commits > b, before, { timeout: 15000 });
  await page.waitForTimeout(600);
  const s2 = await page.evaluate(() => ({ ...window.__hstate, sab: undefined }));
  console.log("after input round-trip:", JSON.stringify(s2));
  await page.locator("#screen").screenshot({ path: `${OUT}/browser-wasmdesk-after-input-${STAMP}.png` });

  await browser.close();

  let ok = true;
  const fail = (m) => { console.error("FAIL:", m); ok = false; };
  if (!isolated) fail("not crossOriginIsolated (no SharedArrayBuffer)");
  if (s1.commits < 1) fail("no commit from the client");
  if (s1.lastColors <= 24) fail("client frame not rich (scene did not paint)");
  if (s2.commits <= before) fail("input did not drive a repaint");
  for (const f of [`browser-wasmdesk-initial-${STAMP}.png`, `browser-wasmdesk-after-input-${STAMP}.png`]) {
    const sz = fs.statSync(`${OUT}/${f}`).size;
    console.log(`${f}: ${sz} bytes`);
    if (sz < 1000) fail(`screenshot too small: ${f}`);
  }
  console.log(ok ? "BROWSER PROOF: PASS" : "BROWSER PROOF: FAIL");
  process.exit(ok ? 0 : 1);
})().catch((e) => { console.error("DRIVER ERROR", e); process.exit(2); });
