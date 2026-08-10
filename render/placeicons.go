// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// This file rasterises the file manager's sidebar glyphs — one small,
// monochrome, accent-tinted symbol per shell.PlaceKind, in the SF-Symbols
// "source-list" spirit — so each Favoris / Emplacements row reads as an icon +
// label like a real Finder sidebar rather than a bare column of names. The
// glyphs are drawn once with rounded-rect / scanline primitives (no font, no
// asset), so they render crisply at the sidebar's ~16px size on any theme.

// placeGlyphInk is the sidebar icon tint: a muted blend of the theme accent
// toward the surface text colour, so the glyphs sit quietly beside their labels
// on both light and dark themes.
func placeGlyphInk(th *toolkit.Theme) toolkit.RGBA {
	a := th.Accent
	if a.A == 0 {
		a = toolkit.RGBA{R: 0x5B, G: 0x7C, B: 0xE6, A: 0xFF}
	}
	return toolkit.RGBA{
		R: blend(a.R, th.OnSurface.R, 0.35),
		G: blend(a.G, th.OnSurface.G, 0.35),
		B: blend(a.B, th.OnSurface.B, 0.35),
		A: 0xFF,
	}
}

// buildPlaceGlyphs rasterises every sidebar glyph at size, tinted with ink,
// returning them keyed by kind. Kinds with no dedicated glyph fall back to the
// generic folder symbol via placeGlyph.
func buildPlaceGlyphs(size int, ink toolkit.RGBA) map[shell.PlaceKind]*toolkit.Image {
	m := map[shell.PlaceKind]*toolkit.Image{}
	kinds := []shell.PlaceKind{
		shell.PlaceFolder, shell.PlaceApplications, shell.PlacePictures,
		shell.PlaceMovies, shell.PlaceMusic, shell.PlaceDownloads,
		shell.PlaceDesktop, shell.PlaceDocuments, shell.PlaceHome,
		shell.PlaceVolume, shell.PlaceNetwork, shell.PlaceTrash,
	}
	for _, k := range kinds {
		m[k] = glyphImage(size, func(p *painter.PixelPainter) { drawPlaceGlyph(p, size, k, ink) })
	}
	return m
}

// placeGlyph returns the rasterised glyph for kind from a prebuilt map, falling
// back to the folder glyph for any kind not present.
func placeGlyph(m map[shell.PlaceKind]*toolkit.Image, kind shell.PlaceKind) *toolkit.Image {
	if img, ok := m[kind]; ok {
		return img
	}
	return m[shell.PlaceFolder]
}

// drawPlaceGlyph paints the symbol for kind into a size×size painter in ink.
func drawPlaceGlyph(p *painter.PixelPainter, size int, kind shell.PlaceKind, ink toolkit.RGBA) {
	switch kind {
	case shell.PlaceHome:
		drawHouse(p, size, ink)
	case shell.PlaceDesktop:
		drawMonitor(p, size, ink)
	case shell.PlaceApplications:
		drawAppGrid(p, size, ink)
	case shell.PlacePictures:
		drawPhoto(p, size, ink)
	case shell.PlaceMovies:
		drawFilm(p, size, ink)
	case shell.PlaceMusic:
		drawNote(p, size, ink)
	case shell.PlaceDownloads:
		drawDownload(p, size, ink)
	case shell.PlaceVolume:
		drawDisk(p, size, ink)
	case shell.PlaceNetwork:
		drawGlobe(p, size, ink)
	case shell.PlaceTrash:
		drawTrash(p, size, ink)
	default: // PlaceFolder, PlaceDocuments
		drawFolder(p, size, ink)
	}
}

// stroke draws a 1px-ish outlined rounded rect at the glyph scale.
func drawFolder(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 8
	top := s/3 - s/16
	rad := s / 12
	tabW := (s - 2*m) / 2
	p.FillRoundRect(toolkit.Rect{X: m, Y: top - s/9, W: tabW, H: s / 6}, rad, ink)
	p.FillRoundRect(toolkit.Rect{X: m, Y: top, W: s - 2*m, H: s - top - m}, rad, ink)
}

func drawHouse(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	cx := s / 2
	roofY := s / 5
	roofH := s / 3
	// Roof: a filled triangle grown as a scanline stack from the apex.
	for dy := 0; dy < roofH; dy++ {
		half := (dy * (s/2 - s/8)) / roofH
		p.FillRect(toolkit.Rect{X: cx - half, Y: roofY + dy, W: 2*half + 1, H: 1}, ink)
	}
	// Body.
	bw := s/2 - s/8
	p.FillRoundRect(toolkit.Rect{X: cx - bw/2, Y: roofY + roofH, W: bw, H: s - (roofY + roofH) - s/6}, s/16, ink)
}

func drawMonitor(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 7
	h := s / 2
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m, W: s - 2*m, H: h}, s/12, ink, max1(s/16))
	// Stand.
	p.FillRect(toolkit.Rect{X: s/2 - s/16, Y: m + h, W: s / 8, H: s / 8}, ink)
	p.FillRoundRect(toolkit.Rect{X: s/2 - s/5, Y: m + h + s/8, W: 2 * (s / 5), H: max1(s / 14)}, s/20, ink)
}

func drawAppGrid(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 6
	cell := (s - 2*m - s/12) / 2
	gap := s / 12
	for _, gy := range []int{m, m + cell + gap} {
		for _, gx := range []int{m, m + cell + gap} {
			p.FillRoundRect(toolkit.Rect{X: gx, Y: gy, W: cell, H: cell}, s/16, ink)
		}
	}
}

func drawPhoto(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 6
	w, h := s-2*m, s-2*m
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m, W: w, H: h}, s/12, ink, max1(s/16))
	// Sun.
	sr := s / 10
	p.FillRoundRect(toolkit.Rect{X: m + w - w/3, Y: m + h/4, W: sr, H: sr}, sr, ink)
	// Mountain (scanline triangle from the apex).
	peakX := m + w/3
	mh := h / 2
	for dy := 0; dy < mh; dy++ {
		y := m + h - mh + dy
		half := mh - dy
		p.FillRect(toolkit.Rect{X: peakX - half, Y: y, W: 2*half + 1, H: 1}, ink)
	}
}

func drawFilm(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 6
	w, h := s-2*m, s-2*m
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m, W: w, H: h}, s/12, ink, max1(s/16))
	// Play triangle.
	tw := w / 3
	x0 := m + w/2 - tw/3
	for dx := 0; dx < tw; dx++ {
		half := (tw - dx) * (h / 4) / tw
		p.FillRect(toolkit.Rect{X: x0 + dx, Y: s/2 - half, W: 1, H: 2*half + 1}, ink)
	}
}

func drawNote(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	stemX := s - s/3
	p.FillRect(toolkit.Rect{X: stemX, Y: s / 5, W: max1(s / 16), H: s/2 + s/12}, ink)
	// Note heads.
	r := s / 6
	p.FillRoundRect(toolkit.Rect{X: stemX - r, Y: s/2 + s/12 - r/2, W: r, H: r}, r, ink)
	// Beam.
	p.FillRect(toolkit.Rect{X: stemX, Y: s / 5, W: s / 5, H: max1(s / 16)}, ink)
}

func drawDownload(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	cx := s / 2
	// Arrow shaft.
	p.FillRect(toolkit.Rect{X: cx - s/16, Y: s / 6, W: max1(s / 8), H: s / 3}, ink)
	// Arrow head (downward triangle).
	head := s / 4
	for dy := 0; dy < head; dy++ {
		half := head - dy
		p.FillRect(toolkit.Rect{X: cx - half, Y: s/2 + dy, W: 2*half + 1, H: 1}, ink)
	}
	// Tray.
	p.FillRoundRect(toolkit.Rect{X: s / 5, Y: s - s/4, W: s - 2*(s/5), H: max1(s / 12)}, s/20, ink)
}

func drawDisk(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 6
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m + s/8, W: s - 2*m, H: s - 2*m - s/4}, s/10, ink, max1(s/16))
	// Slot.
	p.FillRoundRect(toolkit.Rect{X: s/2 - s/8, Y: s/2 - s/16, W: s / 4, H: max1(s / 12)}, s/20, ink)
}

func drawGlobe(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 6
	d := s - 2*m
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m, W: d, H: d}, d/2, ink, max1(s/16))
	// Equator + meridian.
	p.FillRect(toolkit.Rect{X: m, Y: s/2 - max1(s/32), W: d, H: max1(s / 16)}, ink)
	p.FillRect(toolkit.Rect{X: s/2 - max1(s/32), Y: m, W: max1(s / 16), H: d}, ink)
}

func drawTrash(p *painter.PixelPainter, s int, ink toolkit.RGBA) {
	m := s / 5
	w := s - 2*m
	// Lid.
	p.FillRoundRect(toolkit.Rect{X: m - s/16, Y: m, W: w + s/8, H: max1(s / 12)}, s/20, ink)
	// Handle.
	p.FillRoundRect(toolkit.Rect{X: s/2 - s/10, Y: m - s/16, W: s / 5, H: max1(s / 14)}, s/20, ink)
	// Can body (outline).
	p.StrokeRoundRect(toolkit.Rect{X: m, Y: m + s/8, W: w, H: s - m - (m + s/8)}, s/16, ink, max1(s/16))
}
