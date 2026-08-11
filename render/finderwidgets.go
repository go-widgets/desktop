// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// insetWidget lays its child with a left inset, so a toolbar label does not hug
// the window edge. It is a passive layout wrapper — Draw and OnEvent forward to
// the child, whose bounds are already offset, so input routing needs no
// coordinate translation.
type insetWidget struct {
	toolkit.Base
	w    toolkit.Widget
	left int
}

func (i *insetWidget) SetBounds(r toolkit.Rect) {
	i.Base.SetBounds(r)
	i.w.SetBounds(toolkit.Rect{X: r.X + i.left, Y: r.Y, W: r.W - i.left, H: r.H})
}

func (i *insetWidget) Draw(p painter.Painter, th *toolkit.Theme) { i.w.Draw(p, th) }
func (i *insetWidget) OnEvent(ev toolkit.Event)                  { i.w.OnEvent(ev) }

// vcenterWidget centres its child vertically at a fixed content height within
// its bounds — a short control (view switcher, size slider) seated in the taller
// toolbar. Draw and OnEvent forward to the child at its centred bounds.
type vcenterWidget struct {
	toolkit.Base
	w toolkit.Widget
	h int
}

func (c *vcenterWidget) SetBounds(r toolkit.Rect) {
	c.Base.SetBounds(r)
	h := c.h
	if h > r.H || h <= 0 {
		h = r.H
	}
	c.w.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h})
}

func (c *vcenterWidget) Draw(p painter.Painter, th *toolkit.Theme) { c.w.Draw(p, th) }
func (c *vcenterWidget) OnEvent(ev toolkit.Event)                  { c.w.OnEvent(ev) }
