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
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"  // register GIF decoder for icon/thumbnail loading
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"os"
	"path/filepath"

	"github.com/go-freedesktop/icontheme"
	"github.com/go-widgets/toolkit"
)

// DefaultIconSize is the nominal pixel size requested from the icon theme.
const DefaultIconSize = 48

// IconLoader resolves icon names (or absolute paths) to go-widgets Image
// widgets, rasterizing PNG/JPEG/GIF icons and falling back to a solid
// placeholder when an icon is missing or cannot be decoded (e.g. an SVG-only
// icon). Results are cached per name.
//
// It has two interchangeable resolution modes: the native mode resolves names
// through an XDG icon theme and reads the file (NewIconLoader), while the
// portable mode resolves the encoded bytes through a supplied function
// (NewIconLoaderFunc) — the seam an embed.FS-backed shell.AppSource plugs into,
// so the exact same rendering path runs in a browser with no filesystem.
type IconLoader struct {
	theme       *icontheme.Theme                 // native mode (nil in bytes mode)
	bytesFn     func(name string) ([]byte, bool) // portable mode (nil in theme mode)
	size, scale int
	cache       map[string]*toolkit.Image
}

// NewIconLoader builds a loader for the named icon theme (defaulting to
// hicolor) at the given nominal size — the native, filesystem-backed mode.
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

// NewIconLoaderFunc builds a loader that resolves an icon name (or absolute
// path / virtual key) to encoded image bytes through fn — the portable mode
// used by an embed.FS-backed source in the browser, where there is no XDG icon
// theme to scan. A nil result from fn yields the placeholder swatch, exactly as
// a theme miss does in native mode.
func NewIconLoaderFunc(fn func(name string) ([]byte, bool), size int) *IconLoader {
	if size <= 0 {
		size = DefaultIconSize
	}
	return &IconLoader{
		bytesFn: fn,
		size:    size,
		scale:   1,
		cache:   map[string]*toolkit.Image{},
	}
}

// Image returns a cached Image for an icon name or absolute path. It never
// returns nil: an unresolved / undecodable icon yields a placeholder swatch.
func (l *IconLoader) Image(name string) *toolkit.Image {
	if img, ok := l.TryImage(name); ok {
		return img
	}
	return placeholder(l.size)
}

// TryImage returns a cached Image for an icon name or absolute path and ok=true
// when it resolved to real pixels, or (nil, false) on a miss (an empty name, a
// theme/asset lookup that found nothing, or an undecodable file) WITHOUT
// substituting a placeholder. It is the seam the file grid and dock use to fall
// back to a tasteful drawn glyph (folder / document / app tile) instead of a
// blank grey square when there is no real icon — the whole reason the native
// macOS shell, which has no XDG icon theme, need never show an empty placeholder.
func (l *IconLoader) TryImage(name string) (*toolkit.Image, bool) {
	if img, ok := l.cache[name]; ok {
		return img, img != nil
	}
	img := l.load(name)
	l.cache[name] = img // cache misses (nil) too, so a repeat lookup is O(1)
	return img, img != nil
}

// load resolves name to real pixels, or nil on a miss (the caller decides the
// fallback — a placeholder swatch for Image, a drawn glyph for the grid/dock).
func (l *IconLoader) load(name string) *toolkit.Image {
	if name == "" {
		return nil
	}
	if l.bytesFn != nil {
		b, ok := l.bytesFn(name)
		if !ok {
			return nil
		}
		pix, w, h, err := decodeRGBABytes(b)
		if err != nil {
			return nil
		}
		return toolkit.NewImageFit(pix, w, h)
	}
	path := name
	if !filepath.IsAbs(name) {
		p, err := l.theme.FindIcon([]string{name, "application-x-executable"}, l.size, l.scale)
		if err != nil {
			return nil
		}
		path = p
	}
	pix, w, h, err := decodeRGBA(path)
	if err != nil {
		return nil
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
	return decodeReader(f)
}

// decodeRGBABytes decodes encoded image bytes into tightly packed RGBA pixels —
// the portable counterpart of decodeRGBA for a source that serves icon bytes
// directly (embed.FS) rather than a filesystem path.
func decodeRGBABytes(b []byte) ([]byte, int, int, error) {
	return decodeReader(bytes.NewReader(b))
}

// decodeReader decodes an image stream into tightly packed RGBA pixels.
func decodeReader(r io.Reader) ([]byte, int, int, error) {
	src, _, err := image.Decode(r)
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
