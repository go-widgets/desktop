// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"path/filepath"
	"testing"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/toolkit"
)

// galleryStripY is a widget-local Y that lands inside the filmstrip band of a
// gallery laid out at WxH, and galleryThumbX the X centre of thumbnail i, so a
// synthetic click targets a real thumbnail (mirrors the toolkit's own strip
// geometry: 30% bottom band, gvStripPad=10, gvThumbGap=12).
func galleryThumbXY(v *galleryView, i int) (int, int) {
	sr := v.gv.StripRect()
	r, _ := v.gv.ThumbRect(i)
	return r.X + r.W/2, sr.Y + sr.H/2
}

// --- galleryView (toolkit.GalleryView adapter) ---------------------------

func TestGalleryViewAdapter(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(
		dirItem("d", "/d"),
		fileItem("pic.png", "/pic.png", 10),
		fileItem("doc.txt", "/doc.txt", 20),
	)
	opened := ""
	gv := newGalleryView(m, func(it shell.FileItem) (*toolkit.Image, bool) {
		if it.IsImage() {
			return toolkit.NewImageFit(tinyPNG(), 2, 2), true // real raster thumbnail
		}
		return nil, false
	}, func(it shell.FileItem) { opened = it.Path }, func() string { return "vide" })
	gv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 480, H: 320})
	gv.Draw(pxp(480, 320), th) // populated: preview of item 0 + filmstrip

	// A gallery always shows a current item: the first is selected after the sync.
	if gv.Selected() != 0 {
		t.Fatalf("first item should be selected, got %d", gv.Selected())
	}

	// Click a DIFFERENT thumbnail (index 1) -> selection moves there; DragData then
	// carries its path.
	x1, y1 := galleryThumbXY(gv, 1)
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: x1, Y: y1})
	if gv.Selected() != 1 {
		t.Fatalf("click thumb 1 selected %d", gv.Selected())
	}
	if p := gv.DragData(); p != "/pic.png" {
		t.Errorf("DragData after select = %q, want /pic.png", p)
	}
	gv.Draw(pxp(480, 320), th) // selected thumbnail: accent field + ring, preview swap

	// A second click on the already-selected thumbnail activates it (opens).
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: x1, Y: y1})
	if opened != "/pic.png" {
		t.Errorf("reselect did not open, opened=%q", opened)
	}

	// Keyboard: ArrowRight moves the selection, End jumps to the last item, Enter
	// activates the current one.
	opened = ""
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "ArrowRight"})
	if gv.Selected() != 2 {
		t.Errorf("ArrowRight from 1 -> %d, want 2", gv.Selected())
	}
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Home"})
	if gv.Selected() != 0 {
		t.Errorf("Home -> %d, want 0", gv.Selected())
	}
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if opened != "/d" {
		t.Errorf("Enter did not activate current item, opened=%q", opened)
	}

	// clearSelection blanks the selection; the next sync re-selects the first item
	// (a populated gallery is never left blank).
	gv.clearSelection()
	if gv.Selected() != -1 {
		t.Errorf("clearSelection -> %d, want -1", gv.Selected())
	}
	gv.Draw(pxp(480, 320), th)
	if gv.Selected() != 0 {
		t.Errorf("sync after clear should re-select 0, got %d", gv.Selected())
	}

	// Empty model draws the empty-state message from emptyFn.
	gv.model = modelOf()
	gv.Draw(pxp(480, 320), th)
	if gv.DragData() != "" {
		t.Errorf("empty DragData = %q", gv.DragData())
	}
	// A blank emptyFn falls back to the default label; a nil emptyFn is safe.
	gv.emptyFn = func() string { return "" }
	gv.Draw(pxp(480, 320), th)
	gv.emptyFn = nil
	gv.Draw(pxp(480, 320), th)
}

func TestGalleryViewFileDropOntoFolder(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(dirItem("dst", "/dst"), fileItem("f.txt", "/f.txt", 3))
	var gotDir, gotSrc string
	gv := newGalleryView(m, func(shell.FileItem) (*toolkit.Image, bool) { return nil, false }, nil, nil)
	gv.onFileDrop = func(dstDir, src string) { gotDir, gotSrc = dstDir, src }
	gv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 480, H: 320})
	gv.Draw(pxp(480, 320), th)

	// Drop the file onto the folder thumbnail (index 0) -> a move request.
	x0, y0 := galleryThumbXY(gv, 0)
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: x0, Y: y0, Code: "/f.txt"})
	if gotDir != "/dst" || gotSrc != "/f.txt" {
		t.Errorf("drop onto folder = (%q,%q)", gotDir, gotSrc)
	}
	// Dropping onto the caption/preview region (no thumbnail) is a no-op.
	gotDir, gotSrc = "", ""
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 240, Y: 20, Code: "/f.txt"})
	if gotDir != "" {
		t.Error("drop off the filmstrip should not move")
	}
	// Dropping a file onto its OWN thumbnail (index 1) is refused.
	x1, y1 := galleryThumbXY(gv, 1)
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: x1, Y: y1, Code: "/f.txt"})
	if gotDir != "" {
		t.Error("drop onto self should not move")
	}
	// AcceptsDrop discriminates a file payload from a reorder token / empty.
	if !gv.AcceptsDrop("/some/path") || gv.AcceptsDrop("") || gv.AcceptsDrop(toolkit.SourceRowDragPrefix+"0:1") {
		t.Error("galleryView AcceptsDrop wrong")
	}
	// A non-drop event still routes to the widget (scroll, no panic).
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: 1})
	// A nil onFileDrop callback must not panic.
	gv.onFileDrop = nil
	gv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: x0, Y: y0, Code: "/f.txt"})
}

// --- FinderPane: Galerie view wiring -------------------------------------

func TestFinderGalleryView(t *testing.T) {
	f, _ := testFinder(t)

	// pageName maps the gallery mode.
	if pageName(ViewGallery) != "galerie" {
		t.Errorf("pageName(ViewGallery) = %q", pageName(ViewGallery))
	}

	f.SetView(ViewGallery)
	if f.ViewMode() != ViewGallery {
		t.Fatalf("ViewMode = %d, want %d", f.ViewMode(), ViewGallery)
	}
	drawFinder(f)

	// FocusGalleryImage selects the directory's first image (pic.png), so the big
	// preview shows a real photo; selectedItem then reads that selection.
	f.FocusGalleryImage()
	it, ok := f.selectedItem()
	if !ok || it.Name != "pic.png" {
		t.Fatalf("gallery selection after focus = %+v ok=%v", it, ok)
	}

	// selectPath drives the gallery selection to a named row too.
	f.selectPath("/root/a.pdf")
	if it, ok := f.selectedItem(); !ok || it.Name != "a.pdf" {
		t.Fatalf("selectPath in gallery = %+v ok=%v", it, ok)
	}
	drawFinder(f)

	// Navigating clears the gallery selection; the next paint re-selects the new
	// directory's first item.
	f.Navigate("/root/alpha")
	if f.galleryView.Selected() != -1 {
		t.Errorf("navigation should clear the gallery selection, got %d", f.galleryView.Selected())
	}
	drawFinder(f)
	if f.galleryView.Selected() != 0 {
		t.Errorf("paint after navigation should re-select 0, got %d", f.galleryView.Selected())
	}
}

func TestFinderGalleryFocusNoImage(t *testing.T) {
	f, _ := testFinder(t)
	// /root/beta is an empty directory: FocusGalleryImage finds no image and leaves
	// the gallery unselected (a no-op, no panic).
	f.Navigate("/root/beta")
	f.FocusGalleryImage()
	if _, ok := f.selectedItem(); ok {
		t.Error("no image in the directory: gallery should have no selection")
	}
}

func TestFinderGalleryKeyboardCopy(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "photo.png"), "x")
	f := kbFinder(t, root)
	f.SetView(ViewGallery)
	f.SelectByPath(filepath.Join(root, "photo.png"))

	// ⌘C in the gallery view copies the selected item onto the file clipboard, so
	// the keyboard file operations keep working alongside the new view.
	f.HandleKey(key("c", false, true, false))
	if f.Clipboard().Empty() {
		t.Fatal("⌘C in the gallery view should populate the clipboard")
	}
	if got := f.Clipboard().Paths(); len(got) != 1 || filepath.Base(got[0]) != "photo.png" {
		t.Fatalf("gallery clipboard = %v", got)
	}
}
