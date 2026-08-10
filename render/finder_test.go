// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"errors"
	"testing"
	"time"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// fakeFS is an in-memory directory tree the finder navigates in tests, so no
// real filesystem (or OS) is touched.
type fakeFS map[string][]shell.FileItem

func (f fakeFS) list(path string) (*shell.Dir, error) {
	items, ok := f[path]
	if !ok {
		return nil, errors.New("no such dir")
	}
	d := &shell.Dir{Path: path, Items: append([]shell.FileItem(nil), items...)}
	d.ClassifyLite()
	return d, nil
}

func dirItem(name, path string) shell.FileItem {
	return shell.FileItem{Name: name, Path: path, IsDir: true}
}
func fileItem(name, path string, size int64) shell.FileItem {
	return shell.FileItem{Name: name, Path: path, Size: size, ModTime: time.Now()}
}

// testFinder builds a FinderPane over a small fake tree, laid out at a realistic
// size so every view has room to draw.
func testFinder(t *testing.T) (*FinderPane, fakeFS) {
	t.Helper()
	fs := fakeFS{
		"/root": {
			dirItem("alpha", "/root/alpha"),
			dirItem("beta", "/root/beta"),
			fileItem("z.txt", "/root/z.txt", 1200),
			fileItem("pic.png", "/root/pic.png", 3400),
			fileItem("a.pdf", "/root/a.pdf", 50),
		},
		"/root/alpha": {
			dirItem("deep", "/root/alpha/deep"),
			fileItem("one.txt", "/root/alpha/one.txt", 10),
		},
		"/root/alpha/deep": {
			fileItem("leaf.md", "/root/alpha/deep/leaf.md", 5),
		},
		"/root/beta": {},
	}
	places := &shell.Places{
		Favorites: []shell.Place{
			{Label: "Bureau", Path: "/root/alpha", Kind: shell.PlaceDesktop},
			{Label: "Docs", Path: "/root/beta", Kind: shell.PlaceDocuments},
			{Label: "Images", Path: "/root", Kind: shell.PlacePictures},
		},
		Locations: []shell.Place{
			{Label: "Accueil", Path: "/root", Kind: shell.PlaceHome},
			{Label: "Réseau", Kind: shell.PlaceNetwork},
			{Label: "Corbeille", Path: "/root/beta", Kind: shell.PlaceTrash},
		},
	}
	init, _ := fs.list("/root")
	f := NewFinderPane(FinderConfig{
		Theme:      toolkit.DefaultDark(),
		Icons:      NewIconLoader("", 32),
		Places:     places,
		Lister:     fs.list,
		InitialDir: init,
	})
	f.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: 900, H: 600})
	return f, fs
}

// drawFinder renders the pane into a fresh buffer, exercising the visible view's
// Draw plus the sidebar and toolbar.
func drawFinder(f *FinderPane) {
	const W, H = 900, 600
	buf := make([]byte, 4*W*H)
	p := painter.NewPixelPainter(buf, W, H)
	f.Root().Draw(p, f.theme)
}

func TestFinderViewsRenderAndNavigate(t *testing.T) {
	f, _ := testFinder(t)
	if f.FileCount() != 5 {
		t.Fatalf("initial FileCount = %d, want 5", f.FileCount())
	}
	for _, mode := range []int{ViewList, ViewIcons, ViewColumns} {
		f.SetView(mode)
		if f.ViewMode() != mode {
			t.Errorf("ViewMode = %d, want %d", f.ViewMode(), mode)
		}
		drawFinder(f)
	}
	// Out-of-range view is ignored.
	f.SetView(99)
	if f.ViewMode() == 99 {
		t.Error("out-of-range view was accepted")
	}

	// Navigate into a subdirectory, then to a missing one (error branch).
	f.Navigate("/root/alpha")
	if f.CurrentDir() != "/root/alpha" || f.FileCount() != 2 {
		t.Errorf("after nav: dir=%q count=%d", f.CurrentDir(), f.FileCount())
	}
	f.Navigate("/does/not/exist")
	if f.FileCount() != 0 || f.emptyMessage() == "" {
		t.Errorf("missing dir should empty with a message, got count=%d msg=%q", f.FileCount(), f.emptyMessage())
	}
	drawFinder(f)
}

func TestFinderIconSizeSlider(t *testing.T) {
	f, _ := testFinder(t)
	f.SetView(ViewIcons)
	f.SetIconSize(120)
	if f.IconSize() != 120 {
		t.Errorf("IconSize = %d, want 120", f.IconSize())
	}
	// Clamped to the range at both ends.
	f.SetIconSize(5)
	if f.IconSize() != int(iconSizeMin) {
		t.Errorf("min clamp = %d", f.IconSize())
	}
	f.SetIconSize(9999)
	if f.IconSize() != int(iconSizeMax) {
		t.Errorf("max clamp = %d", f.IconSize())
	}
	// Slider OnChange drives SetIconSize.
	f.slider.OnChange(72)
	if f.IconSize() != 72 {
		t.Errorf("slider OnChange size = %d", f.IconSize())
	}
	drawFinder(f)
}

func TestFinderSort(t *testing.T) {
	f, _ := testFinder(t)
	f.SetView(ViewList)
	for col := 0; col < 4; col++ {
		f.sortBy(col, true)
		f.sortBy(col, false)
	}
	// Directories always sort first regardless of column/direction.
	if !f.fileModel.At(0).IsDir {
		t.Error("directories should sort first")
	}
	// The switcher's OnChange path.
	f.switcher.OnChange(ViewList)
	drawFinder(f)
}

func TestFinderPlacesAndNetwork(t *testing.T) {
	f, _ := testFinder(t)
	f.GoToPlace(shell.PlaceNetwork)
	if f.CurrentDir() != "" || f.emptyMessage() != "Aucun partage" {
		t.Errorf("network state: dir=%q msg=%q", f.CurrentDir(), f.emptyMessage())
	}
	drawFinder(f) // empty-state message path
	f.GoToPlace(shell.PlaceHome)
	if f.CurrentDir() != "/root" {
		t.Errorf("home nav dir=%q", f.CurrentDir())
	}
	// A place kind absent from the model is a no-op.
	before := f.CurrentDir()
	f.GoToPlace(shell.PlaceMusic)
	if f.CurrentDir() != before {
		t.Error("navigating an absent place kind changed the dir")
	}
	// navigatePlace on a non-navigable (empty-path) place is a no-op.
	f.navigatePlace(shell.Place{Kind: shell.PlaceHome, Path: ""})
}

func TestFinderOpenAndCascade(t *testing.T) {
	f, _ := testFinder(t)
	// open a directory navigates; open a file is a no-op.
	f.open(dirItem("alpha", "/root/alpha"))
	if f.CurrentDir() != "/root/alpha" {
		t.Errorf("open dir dir=%q", f.CurrentDir())
	}
	f.open(fileItem("one.txt", "/root/alpha/one.txt", 1))
	if f.CurrentDir() != "/root/alpha" {
		t.Error("opening a file should not navigate")
	}
	// Column cascade opens several levels.
	f.Navigate("/root")
	f.CascadeColumns(3)
	if n := f.columnView.ColumnCount(); n < 2 {
		t.Errorf("cascade opened %d columns, want >= 2", n)
	}
	drawFinder(f)
}

func TestFinderTitleAndPageName(t *testing.T) {
	if pageName(ViewList) != "liste" || pageName(ViewColumns) != "colonnes" || pageName(ViewIcons) != "icones" {
		t.Error("pageName mapping wrong")
	}
	if titleFor("") != "Fichiers" {
		t.Error("empty title")
	}
	if titleFor("/") != "/" {
		t.Errorf("root title = %q", titleFor("/"))
	}
	if titleFor("/a/b") != "b" {
		t.Errorf("base title = %q", titleFor("/a/b"))
	}
}

func TestFinderDefaultsAndNativeLister(t *testing.T) {
	// Nil config fields fall back to defaults (DefaultPlaces + NativeLister).
	f := NewFinderPane(FinderConfig{})
	f.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: 800, H: 500})
	drawFinder(f)
	// NativeLister on a missing path errors; on the temp dir it lists.
	if _, err := NativeLister("/definitely/not/here/xyz"); err == nil {
		t.Error("NativeLister missing path should error")
	}
	tmp := t.TempDir()
	if d, err := NativeLister(tmp); err != nil || d == nil {
		t.Errorf("NativeLister temp dir: %v", err)
	}
}
