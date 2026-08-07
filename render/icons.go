// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package render turns the shell package's composition/model values into a
// live go-widgets widget tree and captures it to an image. It is the raw
// rendering layer: it imports the toolkit, the icon theme and the notification
// toast bridge, so it is exercised by the runtime smoke rather than the model
// coverage gate.
package render

import (
	"image"
	"image/draw"
	_ "image/gif"  // register GIF decoder for icon/thumbnail loading
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"
	"path/filepath"

	"github.com/go-freedesktop/icontheme"
	"github.com/go-widgets/toolkit"
)

// DefaultIconSize is the nominal pixel size requested from the icon theme.
const DefaultIconSize = 48

// IconLoader resolves icon names (or absolute paths) to go-widgets Image
// widgets through an XDG icon theme, rasterizing PNG/JPEG/GIF icons and falling
// back to a solid placeholder when an icon is missing or cannot be decoded
// (e.g. an SVG-only icon). Results are cached per name.
type IconLoader struct {
	theme       *icontheme.Theme
	size, scale int
	cache       map[string]*toolkit.Image
}

// NewIconLoader builds a loader for the named icon theme (defaulting to
// hicolor) at the given nominal size.
func NewIconLoader(themeName string, size int) *IconLoader {
	if themeName == "" {
		themeName = icontheme.HicolorTheme
	}
	if size <= 0 {
		size = DefaultIconSize
	}
	return &IconLoader{
		theme: icontheme.New(themeName),
		size:  size,
		scale: 1,
		cache: map[string]*toolkit.Image{},
	}
}

// Image returns a cached Image for an icon name or absolute path. It never
// returns nil: an unresolved / undecodable icon yields a placeholder swatch.
func (l *IconLoader) Image(name string) *toolkit.Image {
	if img, ok := l.cache[name]; ok {
		return img
	}
	img := l.load(name)
	l.cache[name] = img
	return img
}

// load resolves name to pixels or a placeholder.
func (l *IconLoader) load(name string) *toolkit.Image {
	path := name
	if name == "" {
		return placeholder(l.size)
	}
	if !filepath.IsAbs(name) {
		p, err := l.theme.FindIcon([]string{name, "application-x-executable"}, l.size, l.scale)
		if err != nil {
			return placeholder(l.size)
		}
		path = p
	}
	pix, w, h, err := decodeRGBA(path)
	if err != nil {
		return placeholder(l.size)
	}
	return toolkit.NewImageFit(pix, w, h)
}

// decodeRGBA decodes an image file into tightly packed RGBA pixels.
func decodeRGBA(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}
	b := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	return rgba.Pix, b.Dx(), b.Dy(), nil
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// placeholder builds a solid slate-grey square Image of the given size.
func placeholder(size int) *toolkit.Image {
	if size <= 0 {
		size = DefaultIconSize
	}
	pix := make([]byte, size*size*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = 0x5a, 0x60, 0x74, 0xff
	}
	return toolkit.NewImageFit(pix, size, size)
}
