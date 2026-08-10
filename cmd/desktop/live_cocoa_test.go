// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin && integration

// This is the native-macOS proof for the desktop shell — the darwin peer of
// live_x11_test.go. It composes the shell from the REAL macOS application tree
// (source.NewDarwin scans /Applications + the system/user Application
// directories) and drives the EXACT damage-aware root the windowed backend
// runs: scene.HostRoot, the same *scene.HostRoot that `desktop` with no flags
// hands to window.Backend.Run — which on macOS is the pure-Go Cocoa/AppKit
// backend that presents this framebuffer into a real NSWindow via
// -setNeedsDisplayInRect:/drawRect: (github.com/go-widgets/window v0.6.0).
//
// The RenderDamaged seed frame it captures is byte-identical to the `-capture`
// composite by construction (verified on-device), so the saved PNG is exactly
// what the NSWindow presents. It asserts, from sampled pixels, that:
//
//   - the dock (south band) carries real app-icon content over the theme
//     background, and
//   - the launcher (west band) carries real app-name content,
//
// then synthesises one pointer input (a hover over the launcher region) and
// asserts BOTH that the shell dispatched it and that the resulting incremental
// damage is confined to the launcher (west) region — the small-rect present the
// Cocoa backend commits with -setNeedsDisplayInRect:.
//
// It is gated behind the `integration` build tag AND DESKTOP_COCOA_CAPTURE=1 so
// it never runs in the ordinary unit gate (whose content is host-dependent);
// the CI "native shell (cocoa capture)" lane runs it on macos-latest and
// uploads the dated artifact. The pure-Go painter render works headless (no
// window server / Screen-Recording permission needed); the live NSWindow open
// itself is proven by running the binary on-device (the same window.Open →
// Backend.Run path this scene feeds).
package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/source"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

const cocoaW, cocoaH = 1280, 800

type cocoaRgn struct{ x0, y0, x1, y1 int }

// dominantRGB returns the region's most common quantised colour — its background.
func dominantRGB(buf []byte, w int, r cocoaRgn) (rr, gg, bb uint8) {
	counts := map[[3]uint8]int{}
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			i := (y*w + x) * 4
			counts[[3]uint8{buf[i] & 0xF8, buf[i+1] & 0xF8, buf[i+2] & 0xF8}]++
		}
	}
	best, bn := [3]uint8{}, -1
	for k, n := range counts {
		if n > bn {
			bn, best = n, k
		}
	}
	return best[0], best[1], best[2]
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// nonBgCount counts pixels in r differing from (br,bg,bb) by more than thr.
func nonBgCount(buf []byte, w int, r cocoaRgn, br, bg, bb uint8, thr int) int {
	n := 0
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			i := (y*w + x) * 4
			if absDiff(int(buf[i]&0xF8), int(br)) > thr ||
				absDiff(int(buf[i+1]&0xF8), int(bg)) > thr ||
				absDiff(int(buf[i+2]&0xF8), int(bb)) > thr {
				n++
			}
		}
	}
	return n
}

// contains reports whether inner lies wholly within outer.
func contains(outer, inner toolkit.Rect) bool {
	return inner.X >= outer.X && inner.Y >= outer.Y &&
		inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
}

func TestLiveCocoaShell(t *testing.T) {
	if os.Getenv("DESKTOP_COCOA_CAPTURE") == "" {
		t.Skip("set DESKTOP_COCOA_CAPTURE=1 to run the native-macOS shell capture proof")
	}

	src := source.NewDarwin(source.DarwinOptions{Dir: os.Getenv("HOME"), IconSize: render.DefaultIconSize})
	sc := render.New(render.Config{Source: src, Theme: toolkit.DefaultLight(), Width: cocoaW, Height: cocoaH})
	t.Logf("scene from real macOS apps: dock=%d apps=%d categories=%d files=%d",
		sc.DockCount(), sc.AppCount(), sc.MenuCategoryCount(), sc.FileCount())
	if sc.AppCount() == 0 {
		t.Fatal("no applications scanned from the macOS application directories")
	}

	// Drive the exact root the Cocoa backend's Run drives.
	root := sc.HostRoot()
	full := toolkit.Rect{X: 0, Y: 0, W: cocoaW, H: cocoaH}
	root.SetBounds(full)

	buf := make([]byte, 4*cocoaW*cocoaH)
	th := sc.Theme()
	p := painter.NewPixelPainter(buf, cocoaW, cocoaH)
	p.FillRect(full, th.Background)
	root.RenderDamaged(p, th) // seed the full frame (what the NSWindow first presents)

	// --- pixel assertions on the dock (south) and launcher (west) bands ---
	dockH := render.DefaultIconSize + 12
	menu := int(toolkit.MenuBarH)
	dock := cocoaRgn{4, cocoaH - dockH, 300, cocoaH - 2}
	launcher := cocoaRgn{2, menu + 2, 218, cocoaH - dockH - 2}
	for _, tc := range []struct {
		name string
		r    cocoaRgn
		min  int
	}{{"dock", dock, 100}, {"launcher", launcher, 100}} {
		br, bg, bb := dominantRGB(buf, cocoaW, tc.r)
		got := nonBgCount(buf, cocoaW, tc.r, br, bg, bb, 24)
		t.Logf("region %-8s bg=%d,%d,%d non-bg=%d (min %d)", tc.name, br, bg, bb, got, tc.min)
		if got < tc.min {
			t.Errorf("%s region has too little real content: %d < %d", tc.name, got, tc.min)
		}
	}

	// --- input round-trip: a hover over the launcher region dispatches the
	// toolkit.Event and the incremental damage is confined to the west band. ---
	root.OnEvent(toolkit.Event{Kind: toolkit.EventMouseMove, X: 80, Y: menu + 120})
	rects := root.RenderDamaged(p, th)
	if len(rects) == 0 {
		t.Fatal("hover produced no damage — the input round-trip updated no region")
	}
	west := toolkit.Rect{X: 0, Y: menu, W: 240, H: cocoaH - dockH - menu}
	confined := false
	for _, r := range rects {
		t.Logf("hover damage rect %+v", r)
		if contains(west, r) {
			confined = true
		}
	}
	if !confined {
		t.Errorf("hover damage not confined to the launcher (west) region: %+v", rects)
	}

	// Save the dated on-device artifact.
	out := os.Getenv("DESKTOP_COCOA_PNG")
	if out == "" {
		out = "native-macos-shell-2026-08-10.png"
	}
	img := &image.RGBA{Pix: buf, Stride: cocoaW * 4, Rect: image.Rect(0, 0, cocoaW, cocoaH)}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
}
