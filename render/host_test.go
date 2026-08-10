// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"bytes"
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// TestHostRootPixelIdentityAndDamage is the correctness oracle for the
// incremental-present path: over a scripted interaction it drives the shell's
// scene.HostRoot (the exact root the windowed backend runs) into a PERSISTENT
// framebuffer via RenderDamaged, and after every step asserts the persistent
// buffer is byte-identical to an independent FULL composite of the same state
// (HostRoot.Draw — the -capture path's compositor). A missed invalidation would
// leave a stale region and fail the byte-equality immediately, so this proves
// the incremental path never drops a pixel.
//
// It also measures the presented damage per step (bytes = Σ rect area × 4) and
// asserts the small interactions damage exactly the region that changed (a
// launcher keystroke → the west region; a grid scroll → the centre region), by
// bounds-containment and exact-rectangle equality — not "something painted".
func TestHostRootPixelIdentityAndDamage(t *testing.T) {
	const W, H = 640, 480
	bounds := toolkit.Rect{X: 0, Y: 0, W: W, H: H}

	sc := New(testConfig())
	root := sc.HostRoot()
	root.SetBounds(bounds)
	th := sc.Theme()

	// Persistent incremental framebuffer, updated only within reported damage.
	inc := make([]byte, 4*W*H)
	pinc := painter.NewPixelPainter(inc, W, H)

	// full paints an independent full composite of the CURRENT state into a
	// fresh buffer — the oracle each incremental frame must equal.
	full := func() []byte {
		b := make([]byte, 4*W*H)
		root.Draw(painter.NewPixelPainter(b, W, H), th)
		return b
	}

	firstDiff := func(a, b []byte) (x, y int) {
		for i := 0; i+4 <= len(a); i += 4 {
			if a[i] != b[i] || a[i+1] != b[i+1] || a[i+2] != b[i+2] || a[i+3] != b[i+3] {
				px := (i / 4) % W
				return px, (i / 4) / W
			}
		}
		return -1, -1
	}

	area := func(rects []toolkit.Rect) int {
		a := 0
		for _, r := range rects {
			a += r.W * r.H
		}
		return a
	}
	// bounds of the coalesced damage (single bounding box).
	dmgBounds := func(rects []toolkit.Rect) toolkit.Rect {
		if len(rects) == 0 {
			return toolkit.Rect{}
		}
		x0, y0 := rects[0].X, rects[0].Y
		x1, y1 := rects[0].X+rects[0].W, rects[0].Y+rects[0].H
		for _, r := range rects[1:] {
			x0, y0 = min(x0, r.X), min(y0, r.Y)
			x1, y1 = max(x1, r.X+r.W), max(y1, r.Y+r.H)
		}
		return toolkit.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	}
	contains := func(outer, inner toolkit.Rect) bool {
		return inner.X >= outer.X && inner.Y >= outer.Y &&
			inner.X+inner.W <= outer.X+outer.W && inner.Y+inner.H <= outer.Y+outer.H
	}

	const fullBytes = W * H * 4

	// step mutates state, renders one incremental frame, asserts byte-identity
	// with the full oracle, and returns the presented damage.
	step := func(name string, mutate func()) []toolkit.Rect {
		t.Helper()
		mutate()
		rects := root.RenderDamaged(pinc, th)
		// Copy: the slice is owned by the scene and reused next frame.
		got := make([]toolkit.Rect, len(rects))
		copy(got, rects)
		if want := full(); !bytes.Equal(inc, want) {
			x, y := firstDiff(inc, want)
			t.Fatalf("step %q: incremental != full repaint (first diff at %d,%d) — a stale region was left", name, x, y)
		}
		t.Logf("step %-16s damage=%d rect(s) bounds=%v bytes=%d (%.1f%% of full %d)",
			name, len(got), dmgBounds(got), area(got)*4, 100*float64(area(got)*4)/float64(fullBytes), fullBytes)
		return got
	}

	// 1) First frame: the scene seeds full-surface damage, so it paints
	//    everything — and must equal the full composite.
	if r := step("initial", func() {}); area(r)*4 < fullBytes {
		t.Errorf("initial frame presented %d bytes, want the full surface %d", area(r)*4, fullBytes)
	}

	// 2) Launcher keystroke: the search results change. Only the west region may
	//    repaint — assert the damage is EXACTLY the launcher frame's bounds.
	west := sc.launcherFrame.Bounds()
	if r := step("launcher-query", func() { sc.SetQuery("brow") }); true {
		db := dmgBounds(r)
		if db != west {
			t.Errorf("launcher damage = %v, want exactly the west region %v", db, west)
		}
		if !(area(r)*4 < fullBytes) {
			t.Errorf("launcher damage %d bytes is not smaller than a full repaint %d", area(r)*4, fullBytes)
		}
	}

	// 3) Hover into the dock (south): only the dock region repaints.
	dock := sc.dock.Bounds()
	if r := step("hover-dock", func() {
		root.OnEvent(toolkit.Event{Kind: toolkit.EventMouseMove, X: dock.X + 4, Y: dock.Y + 4})
	}); true {
		if db := dmgBounds(r); !contains(dock, db) {
			t.Errorf("hover-dock damage %v not contained in dock region %v", db, dock)
		}
	}

	// 4) Hover from the dock into the launcher: BOTH the region left and the
	//    region entered repaint (so a stale hover highlight cannot linger).
	if r := step("hover-move", func() {
		root.OnEvent(toolkit.Event{Kind: toolkit.EventMouseMove, X: west.X + 4, Y: west.Y + 4})
	}); true {
		db := dmgBounds(r)
		wantUnion := toolkit.Rect{
			X: min(dock.X, west.X), Y: min(dock.Y, west.Y),
			W: max(dock.X+dock.W, west.X+west.W) - min(dock.X, west.X),
			H: max(dock.Y+dock.H, west.Y+west.H) - min(dock.Y, west.Y),
		}
		if db != wantUnion {
			t.Errorf("hover-move damage = %v, want dock∪launcher %v", db, wantUnion)
		}
	}

	// 5) Grid scroll: only the centre region repaints.
	center := sc.finder.Root().Bounds()
	if r := step("grid-scroll", func() {
		root.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, X: center.X + 10, Y: center.Y + 10, Delta: 1})
	}); true {
		if db := dmgBounds(r); !contains(center, db) {
			t.Errorf("grid-scroll damage %v not contained in centre region %v", db, center)
		}
	}

	// 6) A click (may open a menu popup spanning regions) full-damages — correct
	//    by construction, verified byte-identical.
	step("click", func() {
		root.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 10})
	})

	// 7) Notification appears: the toast overlay is (conservatively) repainted;
	//    it must be byte-identical to the composite with the toast.
	step("toast-appear", func() {
		sc.ShowToast(toolkit.NewToast("Build finished", toolkit.ToastSuccess))
	})

	// 8) A second toast stacks, then one dismisses (reflow) — both byte-identical.
	step("toast-stack", func() {
		sc.ShowToast(toolkit.NewToast("Deploy queued", toolkit.ToastInfo))
	})
	step("toast-dismiss", func() {
		sc.SetToasts(sc.toasts[:1])
	})

	// 9) A no-op frame presents nothing (no damage).
	if r := step("idle", func() {}); len(r) != 0 {
		t.Errorf("idle frame presented %d rect(s), want none", len(r))
	}
}
