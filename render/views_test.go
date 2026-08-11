// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// pxp builds a pixel painter over a fresh WxH buffer.
func pxp(w, h int) painter.Painter { return painter.NewPixelPainter(make([]byte, 4*w*h), w, h) }

// tinyPNG is a 2x2 PNG the icon loader can decode, so a thumbnail key resolves
// to a real Image (thumb == true) in cellImage/iconView tests.
func tinyPNG() []byte {
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range m.Pix {
		m.Pix[i] = 0x20
	}
	var b bytes.Buffer
	_ = png.Encode(&b, m)
	return b.Bytes()
}

func modelOf(items ...shell.FileItem) *mvvm.ObservableList[shell.FileItem] {
	m := mvvm.NewObservableList[shell.FileItem]()
	m.Append(items...)
	return m
}

// --- iconView (toolkit.IconGrid adapter) ---------------------------------

func TestIconViewAdapter(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(
		dirItem("d", "/d"),
		fileItem("pic.png", "/pic.png", 10),
		fileItem("doc.txt", "/doc.txt", 20),
	)
	opened := ""
	iv := newIconView(m, 64, func(it shell.FileItem) (*toolkit.Image, bool) {
		if it.IsImage() {
			return toolkit.NewImageFit(tinyPNG(), 2, 2), true // real raster thumbnail
		}
		return nil, false
	}, func(it shell.FileItem) { opened = it.Path }, func() string { return "vide" })
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 400, H: 300})
	iv.Draw(pxp(400, 300), th) // populated: a thumbnail cell + a nil-image cell

	// Click selects cell 0; DragData then carries its path; a second click opens.
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 40, Y: 40})
	if p := iv.DragData(); p != "/d" {
		t.Errorf("DragData after select = %q, want /d", p)
	}
	iv.Draw(pxp(400, 300), th) // selected cell (selection field + label highlight)
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 40, Y: 40})
	if opened != "/d" {
		t.Errorf("reselect did not open, opened=%q", opened)
	}
	// clearSelection drops the drag payload.
	iv.clearSelection()
	if iv.DragData() != "" {
		t.Errorf("DragData after clearSelection = %q", iv.DragData())
	}
	// SetIconSize clamps below the floor (the grid enforces a minimum).
	iv.SetIconSize(1)
	iv.Draw(pxp(400, 300), th)

	// Empty model draws the empty-state message from emptyFn.
	iv.model = modelOf()
	iv.Draw(pxp(400, 300), th)
	// A blank emptyFn falls back to the default label.
	iv.emptyFn = func() string { return "" }
	iv.Draw(pxp(400, 300), th)
	iv.emptyFn = nil
	iv.Draw(pxp(400, 300), th)
}

func TestIconViewFileDropOntoFolder(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(dirItem("dst", "/dst"), fileItem("f.txt", "/f.txt", 3))
	var gotDir, gotSrc string
	iv := newIconView(m, 64, func(shell.FileItem) (*toolkit.Image, bool) { return nil, false }, nil, nil)
	iv.onFileDrop = func(dstDir, src string) { gotDir, gotSrc = dstDir, src }
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 400, H: 300})
	iv.Draw(pxp(400, 300), th)

	// Drop the file onto the folder cell (index 0) -> a move request.
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 40, Y: 40, Code: "/f.txt"})
	if gotDir != "/dst" || gotSrc != "/f.txt" {
		t.Errorf("drop onto folder = (%q,%q)", gotDir, gotSrc)
	}
	// Dropping onto empty space is a no-op.
	gotDir, gotSrc = "", ""
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 399, Y: 299, Code: "/f.txt"})
	if gotDir != "" {
		t.Error("drop on empty space should not move")
	}
	// Dropping a file onto its OWN cell is refused (dropping onto index 1, which
	// is the dragged file itself).
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 140, Y: 40, Code: "/f.txt"})
	if gotDir != "" {
		t.Error("drop onto self should not move")
	}
	// AcceptsDrop discriminates a file payload from a reorder token / empty.
	if !iv.AcceptsDrop("/some/path") || iv.AcceptsDrop("") || iv.AcceptsDrop(toolkit.SourceRowDragPrefix+"0:1") {
		t.Error("iconView AcceptsDrop wrong")
	}
	// A non-drop event still routes to the grid (scroll, no panic).
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: 1})
	// A nil onFileDrop callback must not panic.
	iv.onFileDrop = nil
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 40, Y: 40, Code: "/f.txt"})
}

// --- listView (toolkit.Table adapter) ------------------------------------

func TestListViewEvents(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(dirItem("d", "/d"), fileItem("f.txt", "/f.txt", 3))
	opened := ""
	sorted := -1
	lv := newListView(m, th,
		func(it shell.FileItem) *toolkit.Image { return toolkit.NewImageFit(tinyPNG(), 2, 2) },
		func(it shell.FileItem) { opened = it.Path },
		func(col int, asc bool) { sorted = col },
		func() string { return "vide" })
	lv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 600, H: 400})
	lv.Draw(pxp(600, 400), th)

	lv.table.OnSort(2, true)
	if sorted != 2 {
		t.Errorf("sortFn col=%d", sorted)
	}
	lv.SetSort(1, false)
	if lv.table.SortColumn != 1 || lv.table.SortAsc {
		t.Error("SetSort not applied")
	}
	if _, ok := lv.rowIconFunc(99); ok {
		t.Error("rowIconFunc out of range should be false")
	}
	if _, ok := lv.rowIconFunc(0); !ok {
		t.Error("rowIconFunc(0) should resolve")
	}
	// Click a row to select, then click the same row to open it.
	rowY := firstTableRowY(lv, 0)
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: rowY})
	prev := lv.table.Selected
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: rowY})
	if prev >= 0 && opened == "" {
		t.Error("reselect click did not open a row")
	}
	lv.table.Selected = 0
	if lv.DragData() != "/d" {
		t.Errorf("listView DragData=%q", lv.DragData())
	}
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: 1})

	// Empty model draws the centred message + header.
	lv.model = modelOf()
	lv.Refresh()
	lv.Draw(pxp(600, 400), th)
	if lv.DragData() != "" {
		t.Error("empty DragData")
	}
}

func TestListViewFileDropOntoFolder(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(dirItem("dst", "/dst"), fileItem("f.txt", "/f.txt", 3))
	var gotDir, gotSrc string
	lv := newListView(m, th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil, nil)
	lv.onFileDrop = func(dstDir, src string) { gotDir, gotSrc = dstDir, src }
	lv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 600, H: 400})
	lv.Draw(pxp(600, 400), th)

	folderY := firstTableRowY(lv, 0) // the /dst folder row
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 20, Y: folderY, Code: "/f.txt"})
	if gotDir != "/dst" || gotSrc != "/f.txt" {
		t.Errorf("list drop onto folder = (%q,%q)", gotDir, gotSrc)
	}
	// Drop on a non-folder / out-of-range row is a no-op.
	gotDir = ""
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 20, Y: 100000, Code: "/f.txt"})
	if gotDir != "" {
		t.Error("list drop out of range should not move")
	}
	if !lv.AcceptsDrop("/p") || lv.AcceptsDrop("") {
		t.Error("listView AcceptsDrop wrong")
	}
	lv.onFileDrop = nil
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: 20, Y: folderY, Code: "/f.txt"})
}

// firstTableRowY finds a widget-local Y that the table maps to data row idx.
func firstTableRowY(lv *listView, idx int) int {
	for y := 0; y < 400; y++ {
		if lv.table.RowAt(0, y) == idx {
			return y
		}
	}
	return 40
}

// --- columnView (toolkit.ColumnBrowser adapter) --------------------------

func TestColumnViewAdapter(t *testing.T) {
	th := toolkit.DefaultDark()
	fs := fakeFS{
		"/r":     {dirItem("a", "/r/a"), fileItem("f.txt", "/r/f.txt", 2)},
		"/r/a":   {dirItem("b", "/r/a/b")},
		"/r/a/b": {fileItem("leaf", "/r/a/b/leaf", 1)},
	}
	opened := ""
	cv := newColumnView(fs.list,
		func(it shell.FileItem) *toolkit.Image { return toolkit.NewImageFit(tinyPNG(), 2, 2) },
		func(it shell.FileItem) { opened = it.Path })
	cv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 700, H: 400})
	cv.SetRoot("/r")
	if cv.ColumnCount() != 1 {
		t.Fatalf("root columns = %d", cv.ColumnCount())
	}
	cv.Draw(pxp(700, 400), th)

	// Provider surface: Children lists a dir; Preview describes a leaf; an
	// unknown provider key is rejected.
	if nodes, ok := cv.Children("/r"); !ok || len(nodes) != 2 || !nodes[0].Container {
		t.Errorf("Children(/r) = %v ok=%v", nodes, ok)
	}
	if _, ok := cv.Children("/nope"); ok {
		t.Error("Children of a missing dir should be ok=false")
	}
	if lines := cv.Preview(toolkit.ColumnNode{Key: "/r/f.txt"}); len(lines) != 2 {
		t.Errorf("Preview lines = %v", lines)
	}
	if lines := cv.Preview(toolkit.ColumnNode{Key: "/unknown"}); lines != nil {
		t.Errorf("Preview of an unknown node = %v", lines)
	}

	// Cascade opens the first-folder chain, and re-picking a leaf activates it.
	cv.SetRoot("/r")
	cv.cascadeFirst(3)
	if cv.ColumnCount() < 2 {
		t.Errorf("cascade cols=%d, want >= 2", cv.ColumnCount())
	}
	cv.activate(toolkit.ColumnNode{Key: "/r/f.txt"})
	if opened != "/r/f.txt" {
		t.Errorf("activate opened=%q", opened)
	}
	// activate on an unknown node / nil openFn is safe.
	cv.activate(toolkit.ColumnNode{Key: "/unknown"})

	// Events route to the browser (click/scroll), and the view relays out.
	cv.SetBounds(toolkit.Rect{X: 0, Y: 44, W: 700, H: 400})
	cv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 13})
	cv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, X: 10, Y: 13, Delta: 1})
	cv.Draw(pxp(700, 500), th)

	// A root whose listing fails yields no columns; cascade then no-ops.
	bad := newColumnView(func(string) (*shell.Dir, error) { return nil, errors.New("x") },
		func(shell.FileItem) *toolkit.Image { return nil }, nil)
	bad.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 700, H: 400})
	bad.SetRoot("/nope")
	if bad.ColumnCount() != 0 {
		t.Error("failed root should have no columns")
	}
	bad.cascadeFirst(3)

	// firstContainer: a dir with no sub-directory reports ok=false.
	if _, _, ok := firstContainer(&shell.Dir{Items: []shell.FileItem{fileItem("x", "/x", 1)}}); ok {
		t.Error("firstContainer with no folder should be ok=false")
	}
}

// --- sidebar (toolkit.SourceList adapter) --------------------------------

func sidebarPlaces() *shell.Places {
	return &shell.Places{
		Favorites: []shell.Place{
			{Label: "A", Path: "/a", Kind: shell.PlaceDesktop},
			{Label: "B", Path: "/b", Kind: shell.PlaceDocuments},
			{Label: "C", Path: "/c", Kind: shell.PlacePictures},
		},
		Locations: []shell.Place{
			{Label: "Home", Path: "/root", Kind: shell.PlaceHome},
			{Label: "Net", Kind: shell.PlaceNetwork},
		},
	}
}

func TestSidebarNavigateAndActive(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	navd := ""
	sb := newSidebar(th, glyphs, sidebarPlaces(), func(p shell.Place) { navd = p.Path })
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})

	// SetActive highlights the matching row; an unknown / empty path clears it.
	sb.SetActive("/b")
	if s, r := sb.list.Selected(); s != 0 || r != 1 {
		t.Errorf("SetActive(/b) selected = (%d,%d)", s, r)
	}
	sb.SetActive("/root")
	if s, r := sb.list.Selected(); s != 1 || r != 0 {
		t.Errorf("SetActive(/root) selected = (%d,%d)", s, r)
	}
	sb.SetActive("")
	if s, r := sb.list.Selected(); s != -1 || r != -1 {
		t.Errorf("SetActive(\"\") should clear, got (%d,%d)", s, r)
	}
	sb.Draw(pxp(200, 400), th)

	// selectRow (the SourceList OnSelect handler) navigates; an out-of-range
	// (section,row) is a no-op.
	sb.selectRow(0, 0)
	if navd != "/a" {
		t.Errorf("selectRow(0,0) navd=%q", navd)
	}
	navd = ""
	sb.selectRow(9, 9)
	sb.selectRow(0, 99)
	if navd != "" {
		t.Error("out-of-range selectRow navigated")
	}
	if _, ok := sb.placeAt(2, 0); ok {
		t.Error("placeAt on an unknown section should be false")
	}
}

func TestSidebarReorderMirror(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	sb := newSidebar(th, glyphs, sidebarPlaces(), nil)
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})

	// reordered mirrors the SourceList's favourite move onto the adapter slice:
	// move favourite 0 to the end.
	sb.reordered(0, 0, 2)
	if sb.favorites[2].Label != "A" {
		t.Errorf("after reorder favorites = %v", sb.favorites)
	}
	// A non-favourite section reorder is ignored; an out-of-range from is a no-op.
	before := append([]shell.Place(nil), sb.favorites...)
	sb.reordered(1, 0, 1)
	sb.reordered(0, 99, 0)
	if len(sb.favorites) != len(before) {
		t.Error("favorites length changed unexpectedly")
	}
	// movePlace clamps a negative / oversized destination.
	_ = movePlace(sb.favorites, 0, -5)
	_ = movePlace(sb.favorites, 0, 99)
}

func TestSidebarFileDrop(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	var gotDest shell.Place
	var gotSrc string
	sb := newSidebar(th, glyphs, sidebarPlaces(), nil)
	sb.onFileDrop = func(dest shell.Place, src string) { gotDest, gotSrc = dest, src }
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})

	// rowAt: locate the first favourite row (section 0, row 0), a real directory.
	favY := sbTopPad + sbHeaderH + sbRowH/2
	if s, r, ok := sb.rowAt(favY); !ok || s != 0 || r != 0 {
		t.Fatalf("rowAt(fav0) = (%d,%d,%v)", s, r, ok)
	}
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: favY, Code: "/file.txt"})
	if gotDest.Path != "/a" || gotSrc != "/file.txt" {
		t.Errorf("file drop on fav0 = (%q,%q)", gotDest.Path, gotSrc)
	}

	// A drop on the network row (not navigable) is refused.
	gotSrc = ""
	netY := sbTopPad + sbHeaderH + 3*sbRowH + sbSectGap + sbHeaderH + sbRowH + sbRowH/2
	if s, r, ok := sb.rowAt(netY); !ok || s != 1 || r != 1 {
		t.Fatalf("rowAt(net) = (%d,%d,%v)", s, r, ok)
	}
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: netY, Code: "/file.txt"})
	if gotSrc != "" {
		t.Error("drop on the non-navigable network row should be refused")
	}

	// A drop on a header / gap (no row) is refused.
	gotSrc = ""
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: sbTopPad + 2, Code: "/file.txt"})
	if gotSrc != "" {
		t.Error("drop on a header should be refused")
	}
	// rowAt outside every band returns ok=false.
	if _, _, ok := sb.rowAt(100000); ok {
		t.Error("rowAt far below should be false")
	}
	// A nil onFileDrop must not panic.
	sb.onFileDrop = nil
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: favY, Code: "/file.txt"})
}

func TestSidebarDragAndAccepts(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	sb := newSidebar(th, glyphs, sidebarPlaces(), func(shell.Place) {})
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})

	// A click on a favourite arms the reorder drag; DragData then carries the
	// source list's reorder token.
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: sbTopPad + sbHeaderH + sbRowH/2})
	if got := sb.DragData(); got == "" || isFilePayload(got) {
		t.Errorf("armed DragData = %q, want a reorder token", got)
	}
	// AcceptsDrop accepts both a reorder token and a file path, but not empty.
	if !sb.AcceptsDrop(toolkit.SourceRowDragPrefix+"0:0") || !sb.AcceptsDrop("/a/file") || sb.AcceptsDrop("") {
		t.Error("sidebar AcceptsDrop wrong")
	}
	// A reorder-token EventDrop is forwarded to the source list (not treated as a
	// file move); it must not panic.
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: sbTopPad + sbHeaderH + sbRowH/2, Code: toolkit.SourceRowDragPrefix + "0:0"})
}

func TestIsFilePayload(t *testing.T) {
	cases := map[string]bool{
		"":                                  false,
		"/home/user/file.txt":               true,
		toolkit.SourceRowDragPrefix + "0:1": false,
		"C:\\Users\\file":                   true,
	}
	for payload, want := range cases {
		if got := isFilePayload(payload); got != want {
			t.Errorf("isFilePayload(%q) = %v, want %v", payload, got, want)
		}
	}
}

// --- FinderPane: drag-to-move with confirmation --------------------------

// realFinder builds a FinderPane over a real temp directory holding a sub-folder
// and a file, so the move logic mutates real (throwaway) files.
func realFinder(t *testing.T) (*FinderPane, string, string, string) {
	t.Helper()
	root := t.TempDir()
	sub := filepath.Join(root, "Sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "note.txt")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	init, err := NativeLister(root)
	if err != nil {
		t.Fatal(err)
	}
	f := NewFinderPane(FinderConfig{
		Theme:      toolkit.DefaultDark(),
		Icons:      NewIconLoader("", 32),
		Places:     &shell.Places{Favorites: []shell.Place{{Label: "Sub", Path: sub, Kind: shell.PlaceFolder}}},
		Lister:     NativeLister,
		InitialDir: init,
	})
	f.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: 900, H: 600})
	return f, root, sub, file
}

func TestFinderMoveConfirmAccepts(t *testing.T) {
	f, _, sub, file := realFinder(t)

	f.requestMove(file, sub, "Sub")
	if !f.DialogActive() {
		t.Fatal("requestMove should show the confirmation dialog")
	}
	drawFinder(f) // paints the scrim + dialog

	// Confirm -> the file is really moved into the sub-folder.
	f.confirmMove(file, sub)
	if f.DialogActive() {
		t.Error("dialog should close after confirming")
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Error("source file should be gone after the move")
	}
	if _, err := os.Stat(filepath.Join(sub, "note.txt")); err != nil {
		t.Errorf("moved file not at destination: %v", err)
	}
}

func TestFinderMoveConfirmCancels(t *testing.T) {
	f, _, sub, file := realFinder(t)
	f.requestMove(file, sub, "Sub")
	f.closeDialog() // the "Annuler" path
	if f.DialogActive() {
		t.Error("cancel should dismiss the dialog")
	}
	if _, err := os.Stat(file); err != nil {
		t.Error("cancel must leave the file untouched")
	}
	if _, err := os.Stat(filepath.Join(sub, "note.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("cancel must not move the file")
	}
}

func TestFinderMoveNoopRefused(t *testing.T) {
	f, root, _, file := realFinder(t)
	// Dropping the file back into its own directory pops no dialog.
	f.requestMove(file, root, "root")
	if f.DialogActive() {
		t.Error("a no-op move should not show a dialog")
	}
	// Empty operands are ignored too.
	f.requestMove("", root, "root")
	f.requestMove(file, "", "")
	if f.DialogActive() {
		t.Error("empty-operand move should not show a dialog")
	}
}

func TestFinderMoveErrorDialog(t *testing.T) {
	f, _, sub, file := realFinder(t)
	// Pre-create a colliding file in the destination so the move fails.
	if err := os.WriteFile(filepath.Join(sub, "note.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.confirmMove(file, sub)
	if !f.DialogActive() {
		t.Error("a failed move should show the error dialog")
	}
	drawFinder(f) // paints the error dialog
	f.closeDialog()

	// moveErrorMessage maps each sentinel + a generic error.
	for _, err := range []error{shell.ErrMoveExists, shell.ErrMoveIntoSelf, shell.ErrMoveNotDir, errors.New("boom")} {
		if moveErrorMessage(err) == "" {
			t.Errorf("moveErrorMessage(%v) empty", err)
		}
	}
}

func TestFinderMoveDialogEventRouting(t *testing.T) {
	f, _, sub, file := realFinder(t)
	f.requestMove(file, sub, "Sub")

	// While the dialog is up, an overlay event is forwarded to the dialog. Click
	// the "Déplacer" button (rightmost in the strip): translate a surface click
	// on it back through the overlay so the router lands on the button.
	dlg := f.dialog
	confirm := dlg.Buttons[len(dlg.Buttons)-1]
	bb := confirm.Bounds()
	// The overlay receives overlay-local coords; the button bounds are surface
	// coords, and the overlay fills the surface from (0,0) here.
	f.overlay.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: bb.X + bb.W/2, Y: bb.Y + bb.H/2})
	if f.DialogActive() {
		t.Error("clicking Déplacer should confirm + close the dialog")
	}
	if _, err := os.Stat(filepath.Join(sub, "note.txt")); err != nil {
		t.Errorf("routed confirm did not move the file: %v", err)
	}
}

func TestFinderDemoMoveConfirm(t *testing.T) {
	f, _, _, _ := realFinder(t)
	f.DemoMoveConfirm() // first file (note.txt) over first folder (Sub)
	if !f.DialogActive() {
		t.Error("DemoMoveConfirm should pop the confirmation dialog")
	}
	f.closeDialog()

	// With no file+folder pair, it is a no-op.
	empty := NewFinderPane(FinderConfig{
		Theme:  toolkit.DefaultDark(),
		Icons:  NewIconLoader("", 32),
		Places: &shell.Places{},
		Lister: func(string) (*shell.Dir, error) { return &shell.Dir{}, nil },
	})
	empty.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: 800, H: 500})
	empty.DemoMoveConfirm()
	if empty.DialogActive() {
		t.Error("DemoMoveConfirm with no pair should be a no-op")
	}
}

func TestFinderSidebarDropTriggersMove(t *testing.T) {
	f, _, sub, file := realFinder(t)
	// The sidebar's onFileDrop (wired in build) requests a move; drop the file on
	// the "Sub" favourite row.
	favY := sbTopPad + sbHeaderH + sbRowH/2
	f.sidebar.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 194, H: 560})
	f.sidebar.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: favY, Code: file})
	if !f.DialogActive() {
		t.Fatal("dropping a file on a sidebar folder should pop the confirmation")
	}
	f.confirmMove(file, sub)
	if _, err := os.Stat(filepath.Join(sub, "note.txt")); err != nil {
		t.Errorf("sidebar-drop move did not land: %v", err)
	}
}

// --- shared helpers / remaining coverage ---------------------------------

func TestCellImageMimeIconBranch(t *testing.T) {
	loader := NewIconLoaderFunc(func(string) ([]byte, bool) { return tinyPNG(), true }, 32)
	f := NewFinderPane(FinderConfig{
		Theme:  toolkit.DefaultDark(),
		Icons:  loader,
		Places: &shell.Places{},
		Lister: func(string) (*shell.Dir, error) { return &shell.Dir{}, nil },
	})
	it := shell.FileItem{Name: "p.png", Path: "/p.png", Mime: "image/png"}
	if img, thumb := f.cellImage(it); img == nil || thumb {
		t.Errorf("mime-icon branch img=%v thumb=%v", img, thumb)
	}
	if img, _ := f.cellImage(shell.FileItem{Name: "x", Path: "/x"}); img == nil {
		t.Error("empty-mime resolve")
	}
}

func TestFinderWidgetsAndHelpers(t *testing.T) {
	th := toolkit.DefaultDark()
	lbl := toolkit.NewLabel("x")
	iw := &insetWidget{w: lbl, left: 14}
	iw.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 30})
	iw.Draw(pxp(100, 30), th)
	iw.OnEvent(toolkit.Event{Kind: toolkit.EventClick})

	sw := toolkit.NewViewSwitcher([]string{"a"}, 0)
	cw := &vcenterWidget{w: sw, h: 20}
	cw.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 40})
	cw.Draw(pxp(100, 40), th)
	cw.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	cw.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10}) // h > bounds -> clamps

	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	if placeGlyph(glyphs, shell.PlaceKind(999)) != glyphs[shell.PlaceFolder] {
		t.Error("placeGlyph fallback")
	}
	zero := &toolkit.Theme{}
	if placeGlyphInk(zero).A != 0xFF {
		t.Error("placeGlyphInk zero accent")
	}
	// mutedInk stays opaque.
	if mutedInk(th, 0.4).A != 0xFF {
		t.Error("mutedInk alpha")
	}
}

func TestFinderCellImageThumbAndGlyphs(t *testing.T) {
	loader := NewIconLoaderFunc(func(string) ([]byte, bool) { return tinyPNG(), true }, 32)
	f := NewFinderPane(FinderConfig{
		Theme:    toolkit.DefaultDark(),
		Icons:    loader,
		Places:   &shell.Places{},
		Lister:   func(string) (*shell.Dir, error) { return &shell.Dir{}, nil },
		ThumbKey: func(it shell.FileItem) string { return it.Path },
	})
	if img, thumb := f.cellImage(fileItem("p.png", "/p.png", 1)); img == nil || !thumb {
		t.Errorf("thumb cellImage img=%v thumb=%v", img, thumb)
	}
	if img, thumb := f.cellImage(dirItem("d", "/d")); img == nil || thumb {
		t.Errorf("dir cellImage img=%v thumb=%v", img, thumb)
	}
	if f.listIcon(dirItem("d", "/d")) == nil {
		t.Error("listIcon nil")
	}

	f2 := NewFinderPane(FinderConfig{
		Theme:  toolkit.DefaultDark(),
		Icons:  NewIconLoader("", 32),
		Places: &shell.Places{},
		Lister: func(string) (*shell.Dir, error) { return &shell.Dir{}, nil },
	})
	if img, _ := f2.cellImage(dirItem("d", "/d")); img != f2.folderGlyph {
		t.Error("dir fallback glyph")
	}
	if img, _ := f2.cellImage(fileItem("p.png", "/p.png", 1)); img != f2.pictureGlyph {
		t.Error("image fallback glyph")
	}
	if img, _ := f2.cellImage(fileItem("x.bin", "/x.bin", 1)); img != f2.fileGlyph {
		t.Error("file fallback glyph")
	}
}

func TestFinderAccessor(t *testing.T) {
	sc := New(testConfig())
	if sc.Finder() == nil {
		t.Error("Scene.Finder nil")
	}
}
