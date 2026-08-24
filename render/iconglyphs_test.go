// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-freedesktop/icontheme"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/toolkit"
)

// tempIconTheme writes a minimal hicolor icon theme with a 48×48 Places icon per
// name in names, and returns an IconLoader whose theme searches it. It is the
// seam that exercises the themed (icontheme) resolution path deterministically,
// without depending on whatever icons the host machine happens to have installed.
func tempIconTheme(t *testing.T, names ...string) *IconLoader {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "hicolor", "48x48", "places")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "[Icon Theme]\nName=Hicolor\nDirectories=48x48/places\n\n" +
		"[48x48/places]\nSize=48\nContext=Places\nType=Threshold\n"
	if err := os.WriteFile(filepath.Join(base, "hicolor", "index.theme"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n+".png"), tinyPNG(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &IconLoader{
		theme: icontheme.NewWithBaseDirs("hicolor", []string{base}),
		size:  48,
		scale: 1,
		cache: map[string]*toolkit.Image{},
	}
}

func TestTryThemeIconResolvesCachesAndMisses(t *testing.T) {
	loader := tempIconTheme(t, "user-home")

	img, ok := loader.TryThemeIcon([]string{"user-home"})
	if !ok || img == nil {
		t.Fatalf("TryThemeIcon(user-home) = (%v, %v), want a real image", img, ok)
	}
	// A second lookup is served from the cache as the very same Image.
	if again, ok2 := loader.TryThemeIcon([]string{"user-home"}); !ok2 || again != img {
		t.Errorf("cache hit = (%v, %v), want same image %v", again, ok2, img)
	}
	// A name the theme does not provide misses (and caches the miss).
	if _, ok := loader.TryThemeIcon([]string{"no-such-icon"}); ok {
		t.Error("TryThemeIcon(no-such-icon) resolved, want miss")
	}
	if _, ok := loader.TryThemeIcon([]string{"no-such-icon"}); ok {
		t.Error("cached miss re-resolved, want miss")
	}
	// Empty name list, a nil loader, and a bytes-mode (themeless) loader all miss.
	if _, ok := loader.TryThemeIcon(nil); ok {
		t.Error("TryThemeIcon(nil) resolved, want miss")
	}
	var nilLoader *IconLoader
	if _, ok := nilLoader.TryThemeIcon([]string{"user-home"}); ok {
		t.Error("nil-loader TryThemeIcon resolved, want miss")
	}
	bytesMode := NewIconLoaderFunc(func(string) ([]byte, bool) { return tinyPNG(), true }, 48)
	if _, ok := bytesMode.TryThemeIcon([]string{"user-home"}); ok {
		t.Error("bytes-mode TryThemeIcon resolved, want miss")
	}
}

func TestBuildPlaceGlyphsThemedAndFallback(t *testing.T) {
	th := toolkit.DefaultDark()
	// The theme provides a real user-home (PlaceHome); every other kind must fall
	// back to its go-iconoir glyph.
	loader := tempIconTheme(t, "user-home")
	glyphs := buildPlaceGlyphs(loader, sbIconPx, placeGlyphInk(th))

	themed, ok := loader.TryThemeIcon([]string{"user-home", "go-home", "folder-home"})
	if !ok || themed == nil {
		t.Fatal("expected themed user-home to resolve")
	}
	if glyphs[shell.PlaceHome] != themed {
		t.Error("PlaceHome glyph should be the themed system icon")
	}
	// A kind with no themed icon still gets a (non-nil) iconoir fallback glyph.
	if glyphs[shell.PlaceTrash] == nil {
		t.Error("PlaceTrash fallback glyph is nil")
	}
	if glyphs[shell.PlaceHome] == glyphs[shell.PlaceTrash] {
		t.Error("themed home and fallback trash should differ")
	}
}

func TestFallbackGlyphsRenderFromIconoir(t *testing.T) {
	th := toolkit.DefaultDark()
	// Each fallback glyph resolves to a real, non-nil Image from the iconoir set.
	for name, img := range map[string]*toolkit.Image{
		"folder":  folderIcon(DefaultIconSize, th),
		"file":    fileIcon(DefaultIconSize, th),
		"picture": pictureIcon(DefaultIconSize, th),
		"app":     appTileIcon(DefaultIconSize, th),
	} {
		if img == nil {
			t.Errorf("%s glyph is nil", name)
		}
	}
	// A theme with no accent falls back to the built-in blue, and a non-positive
	// size falls back to the default icon size — both glyphs still resolve.
	if appTileIcon(DefaultIconSize, &toolkit.Theme{}) == nil {
		t.Error("zero-accent app tile is nil")
	}
	if iconGlyph(0, "folder", placeGlyphInk(th)) == nil {
		t.Error("zero-size glyph is nil")
	}
}

func TestListViewEmptyMessage(t *testing.T) {
	th := toolkit.DefaultDark()
	// nil emptyFn -> default.
	lv := newListView(modelOf(), th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil, nil)
	if got := lv.emptyMessage(); got != "Dossier vide" {
		t.Errorf("nil emptyFn message = %q, want default", got)
	}
	// emptyFn returning "" -> default.
	lv2 := newListView(modelOf(), th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil,
		func() string { return "" })
	if got := lv2.emptyMessage(); got != "Dossier vide" {
		t.Errorf("empty emptyFn message = %q, want default", got)
	}
	// emptyFn returning a message -> that message, and Draw paints it via the widget.
	lv3 := newListView(modelOf(), th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil,
		func() string { return "Aucun résultat" })
	if got := lv3.emptyMessage(); got != "Aucun résultat" {
		t.Errorf("emptyFn message = %q, want custom", got)
	}
	lv3.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 300, H: 200})
	lv3.Draw(pxp(300, 200), th)
	if got := lv3.empty.Message().Get(); got != "Aucun résultat" {
		t.Errorf("empty-state widget message = %q after Draw", got)
	}
}
