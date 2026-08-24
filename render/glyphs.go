// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-iconoir/iconoir"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// This file builds the shell's fallback icons — a folder, a document, a picture
// and a generic app tile — from the Iconoir set (go-iconoir) rather than
// hand-rasterised primitives, so a file grid or dock item with no real theme
// icon or thumbnail shows a real, consistent symbol instead of a blank grey
// placeholder square. Each glyph is rasterised once (into a fixed-size RGBA
// buffer via the pure-Go painter) and cached on the Scene / FinderPane; the
// Iconoir artwork is monochrome, so it is tinted with a theme-derived ink and
// reads crisply at the shell's icon size on both light and dark themes.

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

// iconGlyph rasterises the named Iconoir (Regular) glyph, fitted into a size×size
// transparent buffer and tinted with ink, and returns it as a cached Image. It is
// the single seam every fallback glyph (and every sidebar place glyph) goes
// through. Callers pass only stems verified against iconoir.Names(); an unknown
// stem would draw nothing (a transparent image) rather than a wrong symbol.
func iconGlyph(size int, stem string, ink toolkit.RGBA) *toolkit.Image {
	return glyphImage(size, func(p *painter.PixelPainter) {
		iconoir.Draw(p, toolkit.Rect{X: 0, Y: 0, W: size, H: size}, stem, ink)
	})
}

// folderIcon draws the generic folder fallback (Iconoir "folder"), tinted with
// the theme's sidebar-glyph ink so a directory cell with no real icon reads as a
// folder on either theme.
func folderIcon(size int, th *toolkit.Theme) *toolkit.Image {
	return iconGlyph(size, "folder", placeGlyphInk(th))
}

// fileIcon draws the generic document fallback (Iconoir "page") for a file with
// no real theme icon or thumbnail.
func fileIcon(size int, th *toolkit.Theme) *toolkit.Image {
	return iconGlyph(size, "page", placeGlyphInk(th))
}

// pictureIcon draws the image fallback (Iconoir "media-image") — the fallback for
// an image file with no cached thumbnail, so a picture reads as a picture rather
// than a generic document.
func pictureIcon(size int, th *toolkit.Theme) *toolkit.Image {
	return iconGlyph(size, "media-image", placeGlyphInk(th))
}

// appTileIcon draws the generic application fallback (Iconoir "app-window"),
// tinted with the theme accent — the dock/launcher fallback when an application
// has no resolvable icon.
func appTileIcon(size int, th *toolkit.Theme) *toolkit.Image {
	accent := th.Accent
	if accent.A == 0 {
		accent = toolkit.RGBA{R: 0x5B, G: 0x7C, B: 0xE6, A: 0xFF}
	}
	return iconGlyph(size, "app-window", accent)
}

// blend interpolates from a to b by t in [0,1].
func blend(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}
