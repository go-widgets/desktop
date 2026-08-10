// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"sync"

	"github.com/go-opentype/fonts/inter"
	"github.com/go-widgets/toolkit"
)

// UIFontSizePx is the em size, in pixels, the shell renders its proportional UI
// text at. Inter reads cleanly at this size through the toolkit's pure-Go
// TrueType rasteriser (truetype.go), sitting comfortably inside the 18 px
// ListBox row and the 22 px menu/title bars.
const UIFontSizePx = 13

// uiFontOnce guards the one-time parse + install of the proportional UI face.
var uiFontOnce sync.Once

// UseUIFont installs a real proportional TrueType face (Inter, embedded via
// github.com/go-opentype/fonts) as the toolkit's active font, replacing the
// built-in 5x7 bitmap font for the whole shell. This is a pure app-level call:
// it only invokes the toolkit's public SetFont/NewTrueTypeFont seam (the toolkit
// itself is never modified), and swapping the active font rescales every
// widget's typography to crisp anti-aliased glyphs without touching a single
// widget — layout follows because widgets measure through the active font.
//
// It runs at most once (subsequent calls are no-ops) and is safe to call from
// every entry point — the native binary, the browser client and render.New — so
// any path that builds a Scene renders with the good font. A parse failure
// leaves the bitmap default in place rather than aborting the shell.
func UseUIFont() {
	uiFontOnce.Do(func() {
		f, err := toolkit.NewTrueTypeFont(inter.TTF, UIFontSizePx)
		if err != nil {
			return
		}
		toolkit.SetFont(f)
	})
}
