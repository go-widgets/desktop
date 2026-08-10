// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// iconView is the file manager's "Vignettes" (icon) mode: a reflowing grid of
// thumbnails / icons at the slider-chosen size, each with its name centred
// below. It improves on a plain gengrid in three ways the requirements call
// for: the thumbnail fills and is centred in its cell inside a subtle rounded
// frame; a real raster thumbnail sits on a light chip so a dark image stays
// visible on a dark theme; and the size is driven live by the toolbar slider.
//
// It is a shell-level widget (the improved cell + light-chip treatment is app
// policy, not a toolkit primitive) and composes only public toolkit calls.
type iconView struct {
	toolkit.Base

	model   *mvvm.ObservableList[shell.FileItem]
	iconPx  int // icon square side in pixels (from the size slider)
	scroll  int // vertical scroll offset in pixels
	sel     int // selected index, or -1
	imageFn func(shell.FileItem) (*toolkit.Image, bool)
	openFn  func(shell.FileItem)
	emptyFn func() string
	label   *toolkit.Label
}

// Icon-cell layout constants (padding around the icon and label band).
const (
	ivPadX      = 14
	ivPadTop    = 16
	ivLabelH    = 20
	ivIconLabel = 8 // gap between icon and label
	ivCellGapX  = 10
)

// newIconView builds an icon view over model. imageFn resolves an item to its
// Image plus whether that Image is a real raster thumbnail (so the cell can put
// a light chip behind it); openFn opens an item (navigate a folder / activate a
// file); emptyFn supplies the centred message shown when the model is empty.
func newIconView(model *mvvm.ObservableList[shell.FileItem], iconPx int,
	imageFn func(shell.FileItem) (*toolkit.Image, bool),
	openFn func(shell.FileItem), emptyFn func() string) *iconView {
	lbl := toolkit.NewLabel("")
	lbl.Align = toolkit.AlignCenter
	return &iconView{
		model:   model,
		iconPx:  iconPx,
		sel:     -1,
		imageFn: imageFn,
		openFn:  openFn,
		emptyFn: emptyFn,
		label:   lbl,
	}
}

// SetIconSize changes the icon square side and resets the scroll so the reflow
// stays anchored at the top.
func (v *iconView) SetIconSize(px int) {
	if px < 24 {
		px = 24
	}
	v.iconPx = px
}

func (v *iconView) cellW() int { return v.iconPx + 2*ivPadX }
func (v *iconView) cellH() int { return ivPadTop + v.iconPx + ivIconLabel + ivLabelH }

// cols is the number of columns that fit in the current width (at least one).
func (v *iconView) cols() int {
	w := v.Bounds().W
	if w <= 0 {
		return 1
	}
	c := w / v.cellW()
	if c < 1 {
		c = 1
	}
	return c
}

// Draw paints the visible cells (or the empty-state message), clipped to the
// view bounds so a scrolled row never bleeds into the toolbar above or the dock
// below.
func (v *iconView) Draw(p painter.Painter, th *toolkit.Theme) {
	b := v.Bounds()
	p.FillRect(b, th.Surface)

	n := v.model.Len()
	if n == 0 {
		v.drawEmpty(p, th)
		return
	}

	cols := v.cols()
	cw, ch := v.cellW(), v.cellH()
	// Centre the grid horizontally within any left-over pixels.
	gridW := cols * cw
	x0 := b.X + (b.W-gridW)/2
	if x0 < b.X {
		x0 = b.X
	}

	clipTo(p, b, func() {
		first := 0
		if ch > 0 {
			first = (v.scroll / ch) * cols
		}
		for i := first; i < n; i++ {
			col := i % cols
			rowTop := (i/cols)*ch - v.scroll
			if rowTop >= b.H {
				break
			}
			r := toolkit.Rect{X: x0 + col*cw, Y: b.Y + rowTop, W: cw, H: ch}
			if r.Y+ch < b.Y {
				continue
			}
			v.drawCell(p, th, r, i)
		}
	})
}

// drawEmpty centres the empty-state message (e.g. "Aucun partage" for the
// network location, "Dossier vide" for an empty folder).
func (v *iconView) drawEmpty(p painter.Painter, th *toolkit.Theme) {
	b := v.Bounds()
	msg := "Dossier vide"
	if v.emptyFn != nil {
		if m := v.emptyFn(); m != "" {
			msg = m
		}
	}
	ink := mutedInk(th, 0.4)
	tw := toolkit.TextWidth(msg)
	toolkit.DrawText(p, b.X+(b.W-tw)/2, b.Y+b.H/2-toolkit.GlyphHeight()/2, msg, ink)
}

// drawCell paints one icon cell: the selection highlight, the icon (on a light
// chip + hairline frame when it is a real raster thumbnail), then the centred
// name.
func (v *iconView) drawCell(p painter.Painter, th *toolkit.Theme, r toolkit.Rect, i int) {
	item := v.model.At(i)
	selected := i == v.sel

	iconBox := toolkit.Rect{X: r.X + ivPadX, Y: r.Y + ivPadTop, W: v.iconPx, H: v.iconPx}

	if selected {
		// Subtle rounded selection field behind the icon.
		field := toolkit.Rect{X: iconBox.X - 6, Y: iconBox.Y - 6, W: iconBox.W + 12, H: iconBox.H + 12}
		p.FillRoundRect(field, 10, withAlpha(th.Accent, 0x33))
	}

	img, isThumb := v.imageFn(item)
	if isThumb {
		// Light chip + hairline frame so a dark photo stays visible.
		chip := toolkit.Rect{X: iconBox.X - 2, Y: iconBox.Y - 2, W: iconBox.W + 4, H: iconBox.H + 4}
		p.FillRoundRect(chip, 8, chipColor(th))
		p.StrokeRoundRect(chip, 8, th.Border, 1)
	}
	if img != nil {
		img.SetBounds(iconBox)
		img.Draw(p, th)
	}

	// Centred label under the icon, elided to the cell width (measured, so a
	// proportional font never spills a long name into the neighbouring cell),
	// with a rounded highlight when selected.
	label := elideToWidth(item.Name, r.W-10)
	tw := toolkit.TextWidth(label)
	lx := r.X + (r.W-tw)/2
	ly := r.Y + ivPadTop + v.iconPx + ivIconLabel
	ink := th.OnSurface
	if selected {
		hl := toolkit.Rect{X: lx - 6, Y: ly - 2, W: tw + 12, H: toolkit.GlyphHeight() + 4}
		p.FillRoundRect(hl, 6, th.Accent)
		ink = th.Background
	}
	toolkit.DrawText(p, lx, ly, label, ink)
}

// contentHeight is the total pixel height of all rows at the current width.
func (v *iconView) contentHeight() int {
	n := v.model.Len()
	if n == 0 {
		return 0
	}
	cols := v.cols()
	rows := (n + cols - 1) / cols
	return rows * v.cellH()
}

// clampScroll keeps the scroll offset within [0, max].
func (v *iconView) clampScroll(off int) int {
	max := v.contentHeight() - v.Bounds().H
	if max < 0 {
		max = 0
	}
	if off < 0 {
		return 0
	}
	if off > max {
		return max
	}
	return off
}

// indexAt maps a widget-local point to a model index, or -1 for empty space.
func (v *iconView) indexAt(x, y int) int {
	b := v.Bounds()
	cols := v.cols()
	cw, ch := v.cellW(), v.cellH()
	gridW := cols * cw
	x0 := b.X + (b.W-gridW)/2
	if x0 < b.X {
		x0 = b.X
	}
	if x < x0 || x >= x0+gridW || ch <= 0 {
		return -1
	}
	col := (x - x0) / cw
	row := (y - b.Y + v.scroll) / ch
	idx := row*cols + col
	if idx < 0 || idx >= v.model.Len() {
		return -1
	}
	return idx
}

// OnEvent selects on a click, opens on a second click of the same cell, and
// scrolls on the wheel.
func (v *iconView) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventScroll:
		v.scroll = v.clampScroll(v.scroll + ev.Delta*v.cellH()/2)
	case toolkit.EventClick:
		idx := v.indexAt(ev.X, ev.Y)
		if idx < 0 {
			v.sel = -1
			return
		}
		if idx == v.sel && v.openFn != nil {
			v.openFn(v.model.At(idx))
			return
		}
		v.sel = idx
	}
}

// DragData makes the icon view a toolkit DragSource: a drag started on the
// selected cell carries that file's path, so the host shows a real file drag.
// (Dropping onto a folder to MOVE the file is intentionally NOT wired — it would
// mutate the real filesystem; see the PR notes.)
func (v *iconView) DragData() string {
	if v.sel < 0 || v.sel >= v.model.Len() {
		return ""
	}
	return v.model.At(v.sel).Path
}

// withAlpha returns c with its alpha replaced.
func withAlpha(c toolkit.RGBA, a uint8) toolkit.RGBA {
	c.A = a
	return c
}

// chipColor is the light backing chip behind a raster thumbnail: near-white on
// a dark theme (so a dark photo reads), a hair off the surface on a light one.
func chipColor(th *toolkit.Theme) toolkit.RGBA {
	if isDarkTheme(th) {
		return toolkit.RGBA{R: 0xEC, G: 0xEE, B: 0xF2, A: 0xFF}
	}
	return toolkit.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
}

// isDarkTheme reports whether the theme background is dark (luma-based).
func isDarkTheme(th *toolkit.Theme) bool {
	c := th.Background
	luma := int(c.R)*299 + int(c.G)*587 + int(c.B)*114
	return luma < 128*1000
}
