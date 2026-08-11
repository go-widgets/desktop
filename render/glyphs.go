// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// This file draws the shell's own tasteful fallback icons — a folder, a
// document and a generic app tile — as small anti-aliased vector glyphs, so a
// file grid or dock item with no real theme icon or thumbnail shows a clean
// symbol rather than a blank grey placeholder square. The glyphs are rasterised
// once (into a fixed-size RGBA buffer via the pure-Go painter) and cached on the
// Scene; they are drawn with rounded rectangles only, so they read crisply at
// the shell's icon size on both light and dark themes.

// glyphImage rasterises draw into a size×size RGBA Image (fully transparent
// where draw paints nothing, so the icon composits cleanly over any cell).
func glyphImage(size int, draw func(p *painter.PixelPainter)) *toolkit.Image {
	if size <= 0 {
		size = DefaultIconSize
	}
	buf := make([]byte, size*size*4)
	p := painter.NewPixelPainter(buf, size, size)
	draw(p)
	return toolkit.NewImageFit(buf, size, size)
}

// folderIcon draws a friendly rounded folder: a darker back tab, the body, and
// a lighter top lip for a hint of depth. Folder blue reads as "folder"
// universally, independent of the theme accent.
func folderIcon(size int) *toolkit.Image {
	var (
		body = toolkit.RGBA{R: 0x54, G: 0x8C, B: 0xE6, A: 0xFF}
		tab  = toolkit.RGBA{R: 0x3E, G: 0x74, B: 0xC8, A: 0xFF}
		lip  = toolkit.RGBA{R: 0x6F, G: 0xA2, B: 0xF0, A: 0xFF}
	)
	return glyphImage(size, func(p *painter.PixelPainter) {
		s := size
		m := s / 7        // outer margin
		top := s/3 - s/16 // folder top (body) line
		rad := s / 12     // corner radius
		tabW := (s - 2*m) / 2
		// Back tab, peeking above the body.
		p.FillRoundRect(toolkit.Rect{X: m, Y: top - s/9, W: tabW, H: s / 5}, rad, tab)
		// Body.
		p.FillRoundRect(toolkit.Rect{X: m, Y: top, W: s - 2*m, H: s - top - m}, rad, body)
		// Top lip highlight.
		p.FillRoundRect(toolkit.Rect{X: m, Y: top, W: s - 2*m, H: s / 9}, rad, lip)
	})
}

// fileIcon draws a clean document page: a light rounded sheet with a folded
// top-right corner and a few muted text lines. The near-white page and its
// border read as a document over either theme background.
func fileIcon(size int) *toolkit.Image {
	var (
		page   = toolkit.RGBA{R: 0xF6, G: 0xF7, B: 0xFA, A: 0xFF}
		border = toolkit.RGBA{R: 0xC2, G: 0xC8, B: 0xD2, A: 0xFF}
		fold   = toolkit.RGBA{R: 0xD8, G: 0xDD, B: 0xE6, A: 0xFF}
		line   = toolkit.RGBA{R: 0xB6, G: 0xBD, B: 0xCA, A: 0xFF}
	)
	return glyphImage(size, func(p *painter.PixelPainter) {
		s := size
		mx := s / 4 // side margin (page is portrait, narrower than tall)
		my := s / 8 // top/bottom margin
		rad := s / 14
		w := s - 2*mx
		h := s - 2*my
		foldSz := w / 3
		// Sheet.
		p.FillRoundRect(toolkit.Rect{X: mx, Y: my, W: w, H: h}, rad, page)
		p.StrokeRoundRect(toolkit.Rect{X: mx, Y: my, W: w, H: h}, rad, border, 1)
		// Folded top-right corner (a small triangle, drawn as a shrinking scanline
		// stack so it needs no path primitive).
		for dy := 0; dy < foldSz; dy++ {
			p.FillRect(toolkit.Rect{X: mx + w - foldSz + dy, Y: my + dy, W: foldSz - dy, H: 1}, fold)
		}
		// Text lines.
		lx := mx + w/6
		lw := w - w/3
		ly := my + h/3
		gap := h / 6
		for i := 0; i < 3; i++ {
			ww := lw
			if i == 2 {
				ww = lw / 2 // a short last line
			}
			p.FillRect(toolkit.Rect{X: lx, Y: ly + i*gap, W: ww, H: max1(s / 24)}, line)
		}
	})
}

// pictureIcon draws a clean photo icon: a light framed tile with a small sun
// and a mountain range — the fallback for an image file with no cached
// thumbnail, so a picture reads as a picture rather than a generic document.
func pictureIcon(size int) *toolkit.Image {
	var (
		frame  = toolkit.RGBA{R: 0xF6, G: 0xF7, B: 0xFA, A: 0xFF}
		border = toolkit.RGBA{R: 0xC2, G: 0xC8, B: 0xD2, A: 0xFF}
		sky    = toolkit.RGBA{R: 0xCF, G: 0xE4, B: 0xF7, A: 0xFF}
		sun    = toolkit.RGBA{R: 0xF2, G: 0xC9, B: 0x4C, A: 0xFF}
		hill   = toolkit.RGBA{R: 0x6D, G: 0xB4, B: 0x7C, A: 0xFF}
	)
	return glyphImage(size, func(p *painter.PixelPainter) {
		s := size
		m := s / 6
		rad := s / 12
		w := s - 2*m
		h := s - 2*m
		// Frame + sky.
		p.FillRoundRect(toolkit.Rect{X: m, Y: m, W: w, H: h}, rad, frame)
		inset := max1(s / 24)
		p.FillRoundRect(toolkit.Rect{X: m + inset, Y: m + inset, W: w - 2*inset, H: h - 2*inset}, rad, sky)
		p.StrokeRoundRect(toolkit.Rect{X: m, Y: m, W: w, H: h}, rad, border, 1)
		// Sun.
		sr := s / 9
		p.FillRoundRect(toolkit.Rect{X: m + w*2/3, Y: m + h/4, W: sr, H: sr}, sr, sun)
		// Mountain (a triangle grown as a scanline stack from its apex).
		peakX := m + w/3
		baseY := m + h - inset - 1
		mh := h / 2
		for dy := 0; dy < mh; dy++ {
			half := dy
			y := m + h/3 + dy
			if y > baseY {
				break
			}
			p.FillRect(toolkit.Rect{X: peakX - half, Y: y, W: 2*half + 1, H: 1}, hill)
		}
	})
}

// appTileIcon draws a generic rounded app tile in the theme accent with a top
// highlight — the dock/launcher fallback when an application has no resolvable
// icon.
func appTileIcon(size int, th *toolkit.Theme) *toolkit.Image {
	accent := th.Accent
	if accent.A == 0 {
		accent = toolkit.RGBA{R: 0x5B, G: 0x7C, B: 0xE6, A: 0xFF}
	}
	hi := lighten(accent, 0.18)
	return glyphImage(size, func(p *painter.PixelPainter) {
		s := size
		m := s / 8
		rad := s / 5
		p.FillRoundRect(toolkit.Rect{X: m, Y: m, W: s - 2*m, H: s - 2*m}, rad, accent)
		p.FillRoundRect(toolkit.Rect{X: m, Y: m, W: s - 2*m, H: (s - 2*m) / 2}, rad, hi)
	})
}

// lighten blends c toward white by t in [0,1].
func lighten(c toolkit.RGBA, t float64) toolkit.RGBA {
	return toolkit.RGBA{
		R: blend(c.R, 0xFF, t),
		G: blend(c.G, 0xFF, t),
		B: blend(c.B, 0xFF, t),
		A: c.A,
	}
}

// blend interpolates from a to b by t in [0,1].
func blend(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

// mutedInk blends the theme's surface text toward its background by t, for the
// quiet secondary labels (list-view empty state, section headers).
func mutedInk(th *toolkit.Theme, t float64) toolkit.RGBA {
	return toolkit.RGBA{
		R: blend(th.OnSurface.R, th.SurfaceAlt.R, t),
		G: blend(th.OnSurface.G, th.SurfaceAlt.G, t),
		B: blend(th.OnSurface.B, th.SurfaceAlt.B, t),
		A: 0xFF,
	}
}

// max1 clamps n to at least 1 (so a thin feature never vanishes at small sizes).
func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
