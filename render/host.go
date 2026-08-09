// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/toolkit/scene"
)

// HostRoot builds a damage-aware root over the shell's widget tree for the
// windowed backend's incremental-present path (github.com/go-widgets/window
// v0.4.0 presents only the rectangles a scene.HostRoot reports, instead of the
// whole surface, every frame).
//
// The returned *scene.HostRoot is a drop-in toolkit.Widget: handed to
// window.Backend.Run it drives RenderDamaged (small-rect present); handed to a
// damage-unaware host it still full-repaints correctly through Draw. It owns a
// Scene mirroring a hostShell — a thin composite of the shell's five border
// regions plus the floating toast stack — and repaints a widget's VACATED
// background as scene chrome, so an incremental frame is pixel-identical to the
// full composite by construction.
//
// HostRoot also installs the Scene's incremental hooks: a launcher result-list
// change invalidates just the launcher (west) region, and any toast-stack
// mutation re-anchors and (conservatively) full-damages the surface. Pointer
// hover/scroll invalidate the region under the pointer; every other input event
// (click, key, drag) full-damages, because the region it affects is uncertain
// (a click may open a menu popup that spans regions). Correctness first: an
// uncertain region is always over-invalidated, never under-invalidated, so no
// frame can leave a stale pixel — the win is that the common high-frequency
// interactions (hover, a launcher keystroke, a grid scroll) present a small
// rect instead of the whole window.
func (s *Scene) HostRoot() *scene.HostRoot {
	hs := &hostShell{sc: s}
	root := scene.NewHostRoot(hs)
	hs.host = root

	// A launcher result change repaints exactly the west region.
	s.onLauncherChanged = func() { root.Invalidate(s.launcherFrame) }
	// A toast change re-anchors the stack and full-damages: a freshly appended
	// toast is not yet in the retained scene (nodes are added on the next sync,
	// so Invalidate(newToast) would be a no-op) and a dismissal reflows the
	// column, so damaging from the always-present shell root is the correct,
	// race-free choice for the notification path.
	s.onToastsChanged = func() {
		s.anchorToasts(hs.Bounds())
		hs.fullDamage()
	}
	return root
}

// hostShell is the single application widget the HostRoot's scene mirrors: it
// exposes the shell's border regions and toast stack as its scene children (so
// each is invalidated and repainted independently) and translates input events
// into damage. It has no chrome of its own — the HostRoot's background container
// fills the window background — so DrawSelf is a no-op and the scene descends
// straight into the regions.
type hostShell struct {
	toolkit.Base
	sc   *Scene
	host *scene.HostRoot

	// lastRegion is the border region the pointer was last over, so a hover
	// move can repaint both the region left (clearing its hover state) and the
	// region entered.
	lastRegion toolkit.Widget
}

// SetBounds lays the shell root out to the client area and anchors the toast
// stack, so every child's Bounds() is current before the first frame or after a
// resize. The scene reads those bounds to size its damage.
func (h *hostShell) SetBounds(r toolkit.Rect) {
	h.Base.SetBounds(r)
	h.sc.root.SetBounds(r)
	h.sc.anchorToasts(r)
}

// Children exposes the shell's border regions (in the border's own draw order)
// followed by the toast stack (drawn last, on top). This is the seam the scene
// walks to build its retained mirror; per-region invalidation prunes to a single
// region, and a toast is tracked as its own leaf.
func (h *hostShell) Children() []toolkit.Widget {
	regions := h.sc.regions()
	out := make([]toolkit.Widget, 0, len(regions)+len(h.sc.toasts))
	out = append(out, regions...)
	for _, t := range h.sc.toasts {
		out = append(out, t)
	}
	return out
}

// DrawSelf paints nothing: the shell has no chrome under its regions (the
// HostRoot's background container owns the window fill). Implementing SelfDrawer
// is what makes the scene descend into the regions individually instead of
// drawing the whole shell wholesale on any damage.
func (h *hostShell) DrawSelf(painter.Painter, *toolkit.Theme) {}

// Draw is the full-surface fallback (a damage-unaware host, the first seed
// frame, a resize, an Expose): it paints the same composite the -capture path
// and the plain-Widget adapter paint, so the incremental and full paths cannot
// drift.
func (h *hostShell) Draw(p painter.Painter, th *toolkit.Theme) {
	h.sc.drawComposited(p, th, h.Bounds())
}

// HitTest reports whether the point falls within the shell (its whole client
// area).
func (h *hostShell) HitTest(px, py int) bool { return h.Bounds().Contains(px, py) }

// OnEvent forwards the event to the shell root (the toast overlay is passive,
// exactly as the plain-Widget adapter) and then records the damage the event
// produced.
func (h *hostShell) OnEvent(ev toolkit.Event) {
	h.sc.root.OnEvent(ev)
	h.invalidateFor(ev)
}

// invalidateFor records the damage an already-dispatched event produced. Hover
// and scroll change only the region under the pointer, so they invalidate that
// region (hover additionally repaints the region just left, to clear its hover
// state). Every other event's affected region is uncertain, so it full-damages —
// correctness over optimisation.
func (h *hostShell) invalidateFor(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventMouseMove:
		reg := h.regionAt(ev.X, ev.Y)
		if h.lastRegion != nil && h.lastRegion != reg {
			h.host.Invalidate(h.lastRegion)
		}
		if reg != nil {
			h.host.Invalidate(reg)
		}
		h.lastRegion = reg
	case toolkit.EventScroll:
		if reg := h.regionAt(ev.X, ev.Y); reg != nil {
			h.host.Invalidate(reg)
		} else {
			h.fullDamage()
		}
	default:
		// EventClick / EventMouseUp / EventMouseDrag / EventKeyDown /
		// EventKeyUp / EventChar: a click may open a menu popup that spills out
		// of the menubar region, a drag may resize a splitter, a key may
		// retarget focus — the changed area is not knowable from the event, so
		// damage the whole surface. Always correct, never a stale pixel.
		h.fullDamage()
	}
}

// regionAt returns the border region whose bounds contain the surface point, or
// nil when the point is outside every region. The shell fills the window, so a
// point inside it always hits a region; nil only guards a stray off-surface
// coordinate.
func (h *hostShell) regionAt(x, y int) toolkit.Widget {
	for _, w := range h.sc.regions() {
		if w.Bounds().Contains(x, y) {
			return w
		}
	}
	return nil
}

// fullDamage invalidates the shell root itself, damaging the whole client area —
// the safe fallback for any change whose region is uncertain.
func (h *hostShell) fullDamage() { h.host.Invalidate(h) }
