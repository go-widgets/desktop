// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/toolkit"
)

// This file resolves the file manager's sidebar place glyphs — one small symbol
// per shell.PlaceKind (Home / Documents / Downloads / Applications / Network /
// Trash / Volume…), in the Finder "source-list" spirit — so each Favoris /
// Emplacements row reads as an icon + label rather than a bare column of names.
//
// The glyphs are REAL icons, not hand-rasterised primitives: each kind is first
// looked up against the XDG icon theme by its freedesktop-standard name
// (go-freedesktop/icontheme: user-home, folder-download, user-trash…), so a
// Linux host with an installed theme shows its own place icons; on a miss (a
// themeless / headless host, or the browser) it falls back to the matching
// Iconoir glyph, tinted with a theme-derived ink. Either way no symbol is
// drawn by hand.

// placeIcon pairs a place kind's freedesktop-standard theme names (tried in
// order against the icon theme) with the Iconoir stem used when no themed icon
// resolves. Every stem is verified against iconoir.Names().
type placeIcon struct {
	themeNames []string // freedesktop icon-naming-spec candidates, most-specific first
	iconoir    string   // Iconoir Regular stem fallback
}

// placeIcons maps every shell.PlaceKind to its themed-name candidates and iconoir
// fallback stem.
var placeIcons = map[shell.PlaceKind]placeIcon{
	shell.PlaceFolder:       {[]string{"folder"}, "folder"},
	shell.PlaceApplications: {[]string{"applications-other", "applications-system", "system-run"}, "app-window"},
	shell.PlacePictures:     {[]string{"folder-pictures"}, "media-image"},
	shell.PlaceMovies:       {[]string{"folder-videos"}, "movie"},
	shell.PlaceMusic:        {[]string{"folder-music"}, "music-note"},
	shell.PlaceDownloads:    {[]string{"folder-download", "folder-downloads"}, "download"},
	shell.PlaceDesktop:      {[]string{"user-desktop", "desktop"}, "computer"},
	shell.PlaceDocuments:    {[]string{"folder-documents"}, "page"},
	shell.PlaceHome:         {[]string{"user-home", "go-home", "folder-home"}, "home"},
	shell.PlaceVolume:       {[]string{"drive-harddisk", "drive-removable-media"}, "hard-drive"},
	shell.PlaceNetwork:      {[]string{"network-workgroup", "network-server", "folder-remote"}, "network"},
	shell.PlaceTrash:        {[]string{"user-trash", "user-trash-full"}, "trash"},
}

// placeGlyphInk is the sidebar icon tint used for the iconoir fallback glyphs: a
// muted blend of the theme accent toward the surface text colour, so the glyphs
// sit quietly beside their labels on both light and dark themes.
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

// buildPlaceGlyphs resolves every sidebar glyph, keyed by kind: it prefers a real
// themed icon from icons (an XDG icon theme) and falls back to the kind's
// Iconoir stem, tinted with ink, rendered at size. A nil icons loader (or one
// with no theme, e.g. the browser's bytes mode) simply uses the iconoir glyph for
// every kind. Kinds with no dedicated entry fall back to the generic folder glyph
// via placeGlyph.
func buildPlaceGlyphs(icons *IconLoader, size int, ink toolkit.RGBA) map[shell.PlaceKind]*toolkit.Image {
	m := map[shell.PlaceKind]*toolkit.Image{}
	for kind, spec := range placeIcons {
		if img, ok := icons.TryThemeIcon(spec.themeNames); ok {
			m[kind] = img
			continue
		}
		m[kind] = iconGlyph(size, spec.iconoir, ink)
	}
	return m
}

// placeGlyph returns the resolved glyph for kind from a prebuilt map, falling
// back to the folder glyph for any kind not present.
func placeGlyph(m map[shell.PlaceKind]*toolkit.Image, kind shell.PlaceKind) *toolkit.Image {
	if img, ok := m[kind]; ok {
		return img
	}
	return m[shell.PlaceFolder]
}
