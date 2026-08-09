// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package wasmhost is the desktop shell's wasmbox (wasmdesk) backend: it drives
// a go-widgets widget tree as an external wasmbox client, painting into the
// compositor's shared surface and translating wasmbox input messages into
// toolkit events.
//
// It is the browser analogue of github.com/go-widgets/window's native X11 /
// Wayland backend. window v0.4.0 has no js/wasm backend yet (its Open returns
// ErrUnsupported off Linux), so the desktop shell supplies its own wasmbox host
// here; the platform split mirrors window's exactly — Run on js/wasm, an
// ErrUnsupported stub elsewhere — so the same clients/desktop entry point
// cross-builds on every target.
//
// The rendering core (Host) is platform-independent and fully unit-tested on
// native: only the thin syscall/js glue in host_js.go is browser-only. Host
// reuses the shell's incremental-present root (scene.HostRoot): each frame it
// repaints just the damaged rectangles and reports their bounding box as the
// wasmbox commit{damage}, so the small-rect present the native window backend
// does flows through the compositor's blit too.
package wasmhost

import (
	"errors"
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// ErrUnsupported is returned by Open on platforms other than js/wasm, mirroring
// window.ErrUnsupported so a cross-build stays green everywhere.
var ErrUnsupported = errors.New("wasmhost: the wasmbox backend is only available on js/wasm")

// damageRenderer is the incremental-present seam: a root (scene.HostRoot) that
// repaints only the rectangles it reports changed. Host detects it exactly as
// the native window backend detects window.DamageRenderer.
type damageRenderer interface {
	RenderDamaged(p painter.Painter, th *toolkit.Theme) []toolkit.Rect
}

// InputEvent is one decoded wasmbox input message (the fields the compositor
// puts on event objects it posts to a client: kind, surface-local x/y, the
// keyboard key/code, and the wheel deltaY). host_js.go decodes the js value
// into this struct so the mapping logic stays pure and testable.
type InputEvent struct {
	Kind   string
	X, Y   int
	Key    string
	Code   string
	DeltaY int
}

// Host binds a go-widgets root widget to a persistent RGBA framebuffer and
// drives it: Frame renders the next frame (incrementally when the root opts in)
// and returns the damage to present; Input dispatches a decoded wasmbox event
// into the tree. It owns no js state, so it is exercised end-to-end on native.
type Host struct {
	root  toolkit.Widget
	dmg   damageRenderer
	theme *toolkit.Theme
	w, h  int
	buf   []byte
	first bool
}

// NewHost builds a Host over a fresh w×h framebuffer bound to root. When root
// implements the incremental seam (RenderDamaged) frames present only the
// damaged rectangles; otherwise every frame is a full repaint.
func NewHost(root toolkit.Widget, theme *toolkit.Theme, w, h int) *Host {
	if theme == nil {
		theme = toolkit.DefaultDark()
	}
	if w <= 0 {
		w = 960
	}
	if h <= 0 {
		h = 600
	}
	host := &Host{
		root:  root,
		theme: theme,
		w:     w,
		h:     h,
		buf:   make([]byte, 4*w*h),
		first: true,
	}
	host.dmg, _ = root.(damageRenderer)
	return host
}

// Buffer is the RGBA framebuffer the compositor's shared surface mirrors.
func (h *Host) Buffer() []byte { return h.buf }

// Size returns the framebuffer dimensions.
func (h *Host) Size() (int, int) { return h.w, h.h }

// Frame renders the next frame into the framebuffer and returns the rectangle
// to present. The first frame (the initial map) always reports the whole
// surface; afterwards an incremental root reports the bounding box of the
// rectangles it repainted (empty when nothing changed), and a plain root always
// repaints and reports the whole surface.
func (h *Host) Frame() toolkit.Rect {
	p := painter.NewPixelPainter(h.buf, h.w, h.h)
	full := toolkit.Rect{X: 0, Y: 0, W: h.w, H: h.h}
	h.root.SetBounds(full)
	if h.dmg != nil {
		rects := h.dmg.RenderDamaged(p, h.theme)
		if h.first {
			h.first = false
			return full
		}
		return unionRect(rects)
	}
	p.FillRect(full, h.theme.Background)
	h.root.Draw(p, h.theme)
	h.first = false
	return full
}

// Input dispatches a decoded wasmbox event into the widget tree, returning
// whether it produced any toolkit event (so the js glue can skip a needless
// frame for an event the shell ignores).
func (h *Host) Input(in InputEvent) bool {
	evs := toolkitEvents(in)
	for _, ev := range evs {
		h.root.OnEvent(ev)
	}
	return len(evs) > 0
}

// toolkitEvents maps one wasmbox input message to the toolkit events it drives.
// A press is delivered as EventClick (the toolkit's action event); a key press
// as EventKeyDown, plus an EventChar when the key is a single printable rune (so
// a launcher search field receives typed characters); a wheel tick as an
// EventScroll of ±1 row. An unknown kind maps to nothing.
func toolkitEvents(in InputEvent) []toolkit.Event {
	switch in.Kind {
	case "mousedown":
		return []toolkit.Event{{Kind: toolkit.EventClick, X: in.X, Y: in.Y}}
	case "mouseup":
		return []toolkit.Event{{Kind: toolkit.EventMouseUp, X: in.X, Y: in.Y}}
	case "mousemove":
		return []toolkit.Event{{Kind: toolkit.EventMouseMove, X: in.X, Y: in.Y}}
	case "wheel":
		d := 1
		if in.DeltaY < 0 {
			d = -1
		}
		return []toolkit.Event{{Kind: toolkit.EventScroll, X: in.X, Y: in.Y, Delta: d}}
	case "keydown":
		out := []toolkit.Event{{Kind: toolkit.EventKeyDown, Code: in.Key}}
		if isPrintableKey(in.Key) {
			out = append(out, toolkit.Event{Kind: toolkit.EventChar, Code: in.Key})
		}
		return out
	case "keyup":
		return []toolkit.Event{{Kind: toolkit.EventKeyUp, Code: in.Key}}
	case "char":
		if in.Code == "" {
			return nil
		}
		return []toolkit.Event{{Kind: toolkit.EventChar, Code: in.Code}}
	default:
		return nil
	}
}

// isPrintableKey reports whether a browser KeyboardEvent.key value is a single
// printable character (so it should also produce an EventChar). Named keys like
// "Enter", "ArrowDown" or "Backspace" are multi-rune and excluded.
func isPrintableKey(key string) bool {
	if key == "" {
		return false
	}
	if utf8.RuneCountInString(key) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(key)
	return r >= 0x20 && r != utf8.RuneError
}

// unionRect returns the smallest rectangle covering every rect (the zero Rect
// when the slice is empty), so a multi-region incremental frame commits one
// bounding-box damage to the compositor.
func unionRect(rects []toolkit.Rect) toolkit.Rect {
	if len(rects) == 0 {
		return toolkit.Rect{}
	}
	x0, y0 := rects[0].X, rects[0].Y
	x1, y1 := rects[0].X+rects[0].W, rects[0].Y+rects[0].H
	for _, r := range rects[1:] {
		if r.X < x0 {
			x0 = r.X
		}
		if r.Y < y0 {
			y0 = r.Y
		}
		if r.X+r.W > x1 {
			x1 = r.X + r.W
		}
		if r.Y+r.H > y1 {
			y1 = r.Y + r.H
		}
	}
	return toolkit.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}
