// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package wasmhost

import (
	"testing"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/source"
	"github.com/go-widgets/toolkit"
)

func TestToolkitEvents(t *testing.T) {
	cases := []struct {
		name  string
		in    InputEvent
		kinds []toolkit.EventKind
	}{
		{"mousedown->click", InputEvent{Kind: "mousedown", X: 5, Y: 6}, []toolkit.EventKind{toolkit.EventClick}},
		{"mouseup", InputEvent{Kind: "mouseup"}, []toolkit.EventKind{toolkit.EventMouseUp}},
		{"mousemove", InputEvent{Kind: "mousemove"}, []toolkit.EventKind{toolkit.EventMouseMove}},
		{"wheel", InputEvent{Kind: "wheel", DeltaY: 3}, []toolkit.EventKind{toolkit.EventScroll}},
		{"printable key adds char", InputEvent{Kind: "keydown", Key: "a"}, []toolkit.EventKind{toolkit.EventKeyDown, toolkit.EventChar}},
		{"named key no char", InputEvent{Kind: "keydown", Key: "Enter"}, []toolkit.EventKind{toolkit.EventKeyDown}},
		{"keyup", InputEvent{Kind: "keyup", Key: "a"}, []toolkit.EventKind{toolkit.EventKeyUp}},
		{"char", InputEvent{Kind: "char", Code: "z"}, []toolkit.EventKind{toolkit.EventChar}},
		{"empty char", InputEvent{Kind: "char", Code: ""}, nil},
		{"unknown", InputEvent{Kind: "focus"}, nil},
	}
	for _, c := range cases {
		got := toolkitEvents(c.in)
		if len(got) != len(c.kinds) {
			t.Errorf("%s: %d events, want %d (%v)", c.name, len(got), len(c.kinds), got)
			continue
		}
		for i, k := range c.kinds {
			if got[i].Kind != k {
				t.Errorf("%s: event %d = %v, want %v", c.name, i, got[i].Kind, k)
			}
		}
	}
}

func TestWheelDirection(t *testing.T) {
	if d := toolkitEvents(InputEvent{Kind: "wheel", DeltaY: 5})[0].Delta; d != 1 {
		t.Errorf("wheel down delta = %d, want 1", d)
	}
	if d := toolkitEvents(InputEvent{Kind: "wheel", DeltaY: -5})[0].Delta; d != -1 {
		t.Errorf("wheel up delta = %d, want -1", d)
	}
}

func TestKeyPayloads(t *testing.T) {
	ev := toolkitEvents(InputEvent{Kind: "keydown", Key: "x"})
	if ev[0].Code != "x" || ev[1].Code != "x" {
		t.Errorf("keydown codes = %q,%q, want x,x", ev[0].Code, ev[1].Code)
	}
	// A space (0x20) is printable; a control char is not.
	if !isPrintableKey(" ") {
		t.Error("space should be printable")
	}
	if isPrintableKey("\t") {
		t.Error("tab should not be printable")
	}
	if isPrintableKey("") {
		t.Error("empty should not be printable")
	}
}

func TestUnionRect(t *testing.T) {
	if r := unionRect(nil); r != (toolkit.Rect{}) {
		t.Errorf("union(nil) = %v, want zero", r)
	}
	one := toolkit.Rect{X: 3, Y: 4, W: 5, H: 6}
	if r := unionRect([]toolkit.Rect{one}); r != one {
		t.Errorf("union(single) = %v, want %v", r, one)
	}
	got := unionRect([]toolkit.Rect{
		{X: 10, Y: 10, W: 10, H: 10}, // -> covers 10..20
		{X: 0, Y: 5, W: 5, H: 5},     // -> covers x 0..5, y 5..10
		{X: 18, Y: 0, W: 4, H: 4},    // -> covers x 18..22, y 0..4
	})
	want := toolkit.Rect{X: 0, Y: 0, W: 22, H: 20}
	if got != want {
		t.Errorf("union(multi) = %v, want %v", got, want)
	}
}

// TestHostFullPath drives a plain (non-incremental) root: every frame reports
// the whole surface and paints a non-empty framebuffer.
func TestHostFullPath(t *testing.T) {
	root := toolkit.NewLabel("hi")
	h := NewHost(root, nil, 100, 80)
	if w, hh := h.Size(); w != 100 || hh != 80 {
		t.Fatalf("Size = %d,%d", w, hh)
	}
	dmg := h.Frame()
	if dmg != (toolkit.Rect{X: 0, Y: 0, W: 100, H: 80}) {
		t.Errorf("full-path frame damage = %v, want whole surface", dmg)
	}
	nonzero := false
	for _, b := range h.Buffer() {
		if b != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Error("framebuffer is all zero after a full repaint")
	}
}

// TestHostIncrementalPath drives the real shell root (scene.HostRoot): the first
// frame is the whole surface, a quiescent frame reports no damage, and an input
// produces a bounded, non-empty damage rectangle — the incremental present the
// wasmbox commit{damage} carries.
func TestHostIncrementalPath(t *testing.T) {
	const w, hh = 960, 600
	sc := render.New(render.Config{Source: source.NewEmbedded(), Width: w, Height: hh})
	h := NewHost(sc.HostRoot(), sc.Theme(), w, hh)

	if first := h.Frame(); first != (toolkit.Rect{X: 0, Y: 0, W: w, H: hh}) {
		t.Errorf("first frame damage = %v, want whole surface", first)
	}
	if quiet := h.Frame(); quiet.W != 0 || quiet.H != 0 {
		t.Errorf("quiescent frame damage = %v, want empty", quiet)
	}

	// A pointer move over the launcher (west strip) invalidates a region.
	if !h.Input(InputEvent{Kind: "mousemove", X: 40, Y: 120}) {
		t.Fatal("mousemove produced no toolkit event")
	}
	dmg := h.Frame()
	if dmg.W <= 0 || dmg.H <= 0 {
		t.Fatalf("post-move damage = %v, want non-empty", dmg)
	}
	// Damage is contained within the surface (never a stale/out-of-bounds rect).
	if dmg.X < 0 || dmg.Y < 0 || dmg.X+dmg.W > w || dmg.Y+dmg.H > hh {
		t.Errorf("damage %v escapes the %dx%d surface", dmg, w, hh)
	}

	// An unknown event kind dispatches nothing.
	if h.Input(InputEvent{Kind: "blur"}) {
		t.Error("unknown event should dispatch no toolkit event")
	}
}

// TestRunUnsupportedOffBrowser asserts the non-js Run stub returns
// ErrUnsupported (this test itself runs on native).
func TestRunUnsupportedOffBrowser(t *testing.T) {
	if err := Run(toolkit.NewLabel("x"), nil); err != ErrUnsupported {
		t.Errorf("Run off-browser = %v, want ErrUnsupported", err)
	}
}
