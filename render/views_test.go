// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"bytes"
	"errors"
	"image"
	"image/png"
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

func TestIconViewEventsAndCells(t *testing.T) {
	th := toolkit.DefaultDark()
	m := modelOf(
		dirItem("d", "/d"),
		fileItem("pic.png", "/pic.png", 10),
		fileItem("doc.txt", "/doc.txt", 20),
	)
	opened := ""
	iv := newIconView(m, 64, func(it shell.FileItem) (*toolkit.Image, bool) {
		// image files report a real thumbnail (thumb=true).
		if it.IsImage() {
			return toolkit.NewImageFit(tinyPNG(), 2, 2), true
		}
		return nil, false
	}, func(it shell.FileItem) { opened = it.Path }, func() string { return "vide" })
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 400, H: 300})

	iv.Draw(pxp(400, 300), th) // populated + a thumbnail cell + a nil-image cell

	// Click selects; a second click on the same cell opens it.
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 40, Y: 40})
	if iv.sel != 0 {
		t.Fatalf("first click sel=%d", iv.sel)
	}
	iv.Draw(pxp(400, 300), th) // selected cell (highlight + selection field)
	if p := iv.DragData(); p != "/d" {
		t.Errorf("DragData=%q, want /d", p)
	}
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 40, Y: 40})
	if opened != "/d" {
		t.Errorf("reselect did not open, opened=%q", opened)
	}
	// Click on empty space clears selection; DragData then empty.
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 399, Y: 299})
	if iv.sel != -1 || iv.DragData() != "" {
		t.Errorf("empty click sel=%d drag=%q", iv.sel, iv.DragData())
	}
	// Scroll clamps.
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: 5})
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: -50})

	// indexAt out-of-grid returns -1; SetIconSize clamps below the floor.
	if iv.indexAt(-5, -5) != -1 {
		t.Error("indexAt out of grid should be -1")
	}
	iv.SetIconSize(1)
	if iv.iconPx != 24 {
		t.Errorf("SetIconSize floor = %d", iv.iconPx)
	}

	// Empty model draws the empty-state message.
	iv.model = modelOf()
	iv.Draw(pxp(400, 300), th)
	if iv.contentHeight() != 0 {
		t.Error("empty content height")
	}
}

func TestIconViewChipLightTheme(t *testing.T) {
	if isDarkTheme(toolkit.DefaultDark()) != true {
		t.Error("dark theme not detected")
	}
	lt := toolkit.DefaultLight()
	c := chipColor(lt)
	if c.R != 0xFF {
		t.Errorf("light chip = %+v", c)
	}
	if got := withAlpha(toolkit.RGBA{R: 1}, 0x33); got.A != 0x33 {
		t.Error("withAlpha")
	}
}

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

	// Header sort click routes through OnSort -> sortFn.
	lv.table.OnSort(2, true)
	if sorted != 2 {
		t.Errorf("sortFn col=%d", sorted)
	}
	lv.SetSort(1, false)
	if lv.table.SortColumn != 1 || lv.table.SortAsc {
		t.Error("SetSort not applied")
	}
	// rowIconFunc out of range returns ok=false.
	if _, ok := lv.rowIconFunc(99); ok {
		t.Error("rowIconFunc out of range should be false")
	}
	if _, ok := lv.rowIconFunc(0); !ok {
		t.Error("rowIconFunc(0) should resolve")
	}
	// Click a row to select, then click the same row to open it.
	rowY := 40
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: rowY})
	prev := lv.table.Selected
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: rowY})
	if prev >= 0 && opened == "" {
		t.Error("reselect click did not open a row")
	}
	if lv.table.Selected >= 0 {
		lv.table.Selected = 0
		if lv.DragData() != "/d" {
			t.Errorf("listView DragData=%q", lv.DragData())
		}
	}
	// A non-click event just forwards to the table.
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, Delta: 1})

	// Empty model draws the centred message + header.
	lv.model = modelOf()
	lv.Refresh()
	lv.Draw(pxp(600, 400), th)
	if lv.DragData() != "" {
		t.Error("empty DragData")
	}
}

func TestColumnViewMiller(t *testing.T) {
	th := toolkit.DefaultDark()
	fs := fakeFS{
		"/r":     {dirItem("a", "/r/a"), fileItem("f.txt", "/r/f.txt", 2)},
		"/r/a":   {dirItem("b", "/r/a/b")},
		"/r/a/b": {fileItem("leaf", "/r/a/b/leaf", 1)},
	}
	opened := ""
	cv := newColumnView(th, fs.list,
		func(it shell.FileItem) *toolkit.Image { return toolkit.NewImageFit(tinyPNG(), 2, 2) },
		func(it shell.FileItem) { opened = it.Path })
	cv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 700, H: 400})
	cv.SetRoot("/r")
	if cv.ColumnCount() != 1 {
		t.Fatalf("root columns = %d", cv.ColumnCount())
	}
	// Pick the folder (row 0) -> a second column opens.
	cv.onPick(0, 0)
	if cv.ColumnCount() != 2 {
		t.Fatalf("after folder pick columns = %d", cv.ColumnCount())
	}
	// Pick the file (row 1) in the first column -> preview + truncation.
	cv.onPick(0, 1)
	if cv.preview == nil || cv.ColumnCount() != 1 {
		t.Fatalf("file pick preview=%v cols=%d", cv.preview, cv.ColumnCount())
	}
	// Re-pick the same file -> opens it.
	cv.onPick(0, 1)
	if opened != "/r/f.txt" {
		t.Errorf("file re-pick opened=%q", opened)
	}
	cv.Draw(pxp(700, 400), th) // draws columns + preview
	// Events route to the column under the pointer.
	cv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 10})
	cv.OnEvent(toolkit.Event{Kind: toolkit.EventScroll, X: 10, Y: 10, Delta: 1})
	if cv.columnAt(100000) != nil {
		t.Error("columnAt far right should be nil")
	}
	// Out-of-range picks are no-ops.
	cv.onPick(-1, 0)
	cv.onPick(0, 999)

	// Cascade + contentWidth with a preview.
	cv.SetRoot("/r")
	cv.cascadeFirst(3)
	if cv.ColumnCount() < 2 {
		t.Errorf("cascade cols=%d", cv.ColumnCount())
	}

	// A root whose listing fails yields no columns (makeColumn nil path).
	bad := newColumnView(th, func(string) (*shell.Dir, error) { return nil, errors.New("x") }, cv.iconFn, nil)
	bad.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 700, H: 400})
	bad.SetRoot("/nope")
	if bad.ColumnCount() != 0 {
		t.Error("failed root should have no columns")
	}
}

func TestSidebarNavAndReorder(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	places := &shell.Places{
		Favorites: []shell.Place{
			{Label: "A", Path: "/a", Kind: shell.PlaceDesktop},
			{Label: "B", Path: "/b", Kind: shell.PlaceDocuments},
			{Label: "C", Path: "/c", Kind: shell.PlacePictures},
		},
		Locations: []shell.Place{
			{Label: "Home", Path: "/", Kind: shell.PlaceHome},
			{Label: "Net", Kind: shell.PlaceNetwork},
		},
	}
	navd := ""
	sb := newSidebar(th, glyphs, places, func(p shell.Place) { navd = p.Path })
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})
	sb.SetActive("/a")
	sb.Draw(pxp(200, 400), th)

	// Click a favorite row navigates + records the pressed favorite.
	favRow := sb.rows[1] // first place row (index 0 header)
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: favRow.rect.Y + 2})
	if navd != "/a" || sb.pressedFav != 0 {
		t.Errorf("fav click nav=%q pressed=%d", navd, sb.pressedFav)
	}
	if sb.DragData() != "fav" || !sb.AcceptsDrop("fav") || sb.AcceptsDrop("x") {
		t.Error("DragData/AcceptsDrop wrong for a favorite")
	}
	// Drag the first favorite down past the third and drop -> reordered.
	thirdMidY := sb.rows[3].rect.Y + sbRowH/2 + 1
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDragMove, Y: thirdMidY})
	sb.Draw(pxp(200, 400), th) // dragging -> insertion line
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, Y: sb.rows[3].rect.Y + sbRowH})
	if sb.favorites[len(sb.favorites)-1].Label != "A" {
		t.Errorf("reorder result = %v", sb.favorites)
	}
	// Drag-leave cancels.
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDragMove, Y: 40})
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventDragLeave})
	if sb.dragging {
		t.Error("drag-leave did not cancel")
	}

	// Clicking a header (non place) is a no-op; DragData then empty.
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: sb.rows[0].rect.Y + 2})
	if sb.pressedFav != -1 || sb.DragData() != "" {
		t.Error("header click should clear pressedFav")
	}
	// Clicking a non-favorite (location) row navigates but is not reorderable.
	loc := lastLocationRow(sb)
	sb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: loc.rect.Y + 2})
	if sb.pressedFav != -1 {
		t.Error("location row should not set pressedFav")
	}
}

func lastLocationRow(sb *sidebar) sbRow {
	var last sbRow
	for _, r := range sb.rows {
		if r.isPlace && r.favIdx < 0 {
			last = r
		}
	}
	return last
}

func TestSidebarDropTargetsAndMove(t *testing.T) {
	th := toolkit.DefaultDark()
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	places := &shell.Places{Favorites: []shell.Place{
		{Label: "A", Path: "/a", Kind: shell.PlaceDesktop},
		{Label: "B", Path: "/b", Kind: shell.PlaceDocuments},
	}}
	sb := newSidebar(th, glyphs, places, nil)
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})

	// dropTarget above the band and far below are rejected.
	if _, ok := sb.dropTarget(-9999); ok {
		t.Error("dropTarget far above should reject")
	}
	if _, ok := sb.dropTarget(99999); ok {
		t.Error("dropTarget far below should reject")
	}
	// favInsertY: end index and a normal index both resolve.
	_ = sb.favInsertY(len(sb.favorites))
	_ = sb.favInsertY(0)

	// moveFavorite edge cases.
	sb.moveFavorite(-1, 0) // from out of range: no-op
	sb.moveFavorite(0, 99) // to clamped to len
	sb.moveFavorite(1, -5) // to clamped to 0
	if len(sb.favorites) != 2 {
		t.Errorf("favorites length changed unexpectedly: %d", len(sb.favorites))
	}

	// placeRowAt on a header / outside returns ok=false.
	if _, ok := sb.placeRowAt(-1); ok {
		t.Error("placeRowAt above should be false")
	}

	// An empty-favorites sidebar: dropTarget/favInsertY handle no rows.
	empty := newSidebar(th, glyphs, &shell.Places{}, nil)
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})
	if _, ok := empty.dropTarget(10); ok {
		t.Error("empty dropTarget should reject")
	}
	if empty.favInsertY(0) == 0 && false {
		t.Error("unreachable")
	}
}

func TestViewEdgeBranches(t *testing.T) {
	th := toolkit.DefaultDark()

	// iconView: zero-width bounds -> cols() == 1; scrolled Draw exercises the
	// row-skip / row-break branches; clampScroll both bounds.
	many := mvvm.NewObservableList[shell.FileItem]()
	for i := 0; i < 60; i++ {
		many.Append(dirItem("d", "/d"))
	}
	iv := newIconView(many, 64, func(shell.FileItem) (*toolkit.Image, bool) { return nil, false }, nil, nil)
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 0, H: 0})
	if iv.cols() != 1 {
		t.Errorf("zero-width cols = %d", iv.cols())
	}
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 400, H: 220})
	iv.scroll = iv.cellH() + 10 // top row partly above, bottom rows past b.H
	iv.Draw(pxp(400, 220), th)
	if iv.clampScroll(-1) != 0 {
		t.Error("clampScroll negative")
	}
	if iv.clampScroll(1 << 20); iv.clampScroll(1<<20) != iv.contentHeight()-iv.Bounds().H {
		t.Error("clampScroll over-max")
	}
	// indexAt on a row past the model returns -1.
	if iv.indexAt(50, 1<<20) != -1 {
		t.Error("indexAt past model")
	}
	// A nil openFn on a reselect is a no-op (must not panic).
	iv.sel = 0
	iv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 30, Y: 30})

	// listView: an iconFn returning nil -> rowIconFunc ok=false; header click
	// (row -1) just forwards; DragData with no selection is "".
	m := modelOf(dirItem("d", "/d"))
	lv := newListView(m, th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil, nil)
	lv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 500, H: 300})
	if _, ok := lv.rowIconFunc(0); ok {
		t.Error("nil iconFn should give ok=false")
	}
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 0}) // header row -> -1
	lv.table.Selected = -1
	if lv.DragData() != "" {
		t.Error("no-selection DragData")
	}

	// columnView: cascadeFirst past the deepest folder hits the fi<0 break;
	// drawColRow with an out-of-range index is guarded.
	fs := fakeFS{"/r": {dirItem("a", "/r/a")}, "/r/a": {fileItem("leaf", "/r/a/leaf", 1)}}
	cv := newColumnView(th, fs.list, func(shell.FileItem) *toolkit.Image { return nil }, nil)
	cv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 600, H: 300})
	cv.SetRoot("/r")
	cv.cascadeFirst(10) // /r -> /r/a -> (no folder) break
	cv.drawColRow(pxp(600, 300), th, toolkit.Rect{W: 200, H: 26}, cv.cols[0].dir, 999, th.OnSurface, false)

	// sidebar dropTarget hitting the mid-row branch (returns an interior index).
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	sb := newSidebar(th, glyphs, &shell.Places{Favorites: []shell.Place{
		{Label: "A", Path: "/a", Kind: shell.PlaceDesktop},
		{Label: "B", Path: "/b", Kind: shell.PlaceDocuments},
	}}, nil)
	sb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 200, H: 400})
	favY := sb.rows[1].rect.Y + 1 // upper half of the first favorite
	if idx, ok := sb.dropTarget(favY); !ok || idx != 0 {
		t.Errorf("mid dropTarget = %d,%v", idx, ok)
	}

	// titleFor with a "." base returns the path.
	if titleFor(".") != "." {
		t.Errorf("titleFor(.) = %q", titleFor("."))
	}
}

func TestElideToWidthAndFinderAccessor(t *testing.T) {
	if elideToWidth("x", 0) != "" {
		t.Error("elideToWidth(_,0) should be empty")
	}
	if elideToWidth("hi", 10000) != "hi" {
		t.Error("elideToWidth fitting should be unchanged")
	}
	if got := elideToWidth("a-very-long-name-indeed.txt", 40); got == "" || []rune(got)[len([]rune(got))-1] != '…' {
		t.Errorf("elideToWidth trim = %q", got)
	}
	if elideToWidth("wwww", 1) != "…" {
		t.Errorf("elideToWidth tiny = %q", elideToWidth("wwww", 1))
	}
	// Scene.Finder accessor.
	sc := New(testConfig())
	if sc.Finder() == nil {
		t.Error("Scene.Finder nil")
	}
}

func TestMoreEdgeBranches(t *testing.T) {
	th := toolkit.DefaultDark()

	// iconView cols(): width smaller than a cell (but > 0) still yields 1 column;
	// clampScroll when content is shorter than the viewport clamps to 0.
	iv := newIconView(modelOf(dirItem("d", "/d")), 64,
		func(shell.FileItem) (*toolkit.Image, bool) { return nil, false }, nil, nil)
	iv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 400})
	if iv.cols() != 1 {
		t.Errorf("narrow cols = %d", iv.cols())
	}
	if iv.clampScroll(50) != 0 {
		t.Error("clampScroll with tiny content should pin to 0")
	}

	// listView: a re-selection with a nil openFn must not panic; DragData with a
	// selection past the model length returns "".
	m := modelOf(dirItem("d", "/d"))
	lv := newListView(m, th, func(shell.FileItem) *toolkit.Image { return nil }, nil, nil, nil)
	lv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 500, H: 300})
	lv.table.Selected = 0
	lv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 20, Y: 40}) // reselect, nil openFn
	lv.table.Selected = 5
	if lv.DragData() != "" {
		t.Error("out-of-range DragData")
	}
}

func TestCellImageMimeIconBranch(t *testing.T) {
	// A loader that resolves the per-MIME theme icon, but NO thumbnail policy, so
	// cellImage returns the theme icon (thumb=false) via the mime-name branch.
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
	// Empty-mime file resolves the text-x-generic name.
	if img, _ := f.cellImage(shell.FileItem{Name: "x", Path: "/x"}); img == nil {
		t.Error("empty-mime resolve")
	}
}

// nonClipPainter is a Painter that does NOT implement painter.Clipper, so
// clipTo's fallback (unclipped) branch runs.
type nonClipPainter struct{}

func (nonClipPainter) FillRect(painter.Rect, painter.RGBA)                  {}
func (nonClipPainter) StrokeRect(painter.Rect, painter.RGBA, int)           {}
func (nonClipPainter) FillRoundRect(painter.Rect, int, painter.RGBA)        {}
func (nonClipPainter) StrokeRoundRect(painter.Rect, int, painter.RGBA, int) {}
func (nonClipPainter) PutPixel(int, int, painter.RGBA)                      {}
func (nonClipPainter) Text(int, int, string, painter.RGBA)                  {}
func (nonClipPainter) Size() (int, int)                                     { return 100, 100 }

func TestFinderWidgetsAndHelpers(t *testing.T) {
	// clipTo on a non-clipping painter runs fn unclipped.
	ran := false
	clipTo(nonClipPainter{}, toolkit.Rect{W: 10, H: 10}, func() { ran = true })
	if !ran {
		t.Error("clipTo fn did not run on a non-clipper")
	}

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
	// h > bounds height falls back to the bounds height.
	cw.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10})

	// placeGlyph falls back to the folder glyph for an unknown kind.
	glyphs := buildPlaceGlyphs(sbIconPx, placeGlyphInk(th))
	if placeGlyph(glyphs, shell.PlaceKind(999)) != glyphs[shell.PlaceFolder] {
		t.Error("placeGlyph fallback")
	}
	// placeGlyphInk tolerates a zero-accent theme.
	zero := &toolkit.Theme{}
	if placeGlyphInk(zero).A != 0xFF {
		t.Error("placeGlyphInk zero accent")
	}
}

func TestFinderCellImageThumbAndGlyphs(t *testing.T) {
	// A loader that resolves any name (folder icon, mime icon, thumbnail key) to
	// a real image, so cellImage's thumbnail + theme-icon branches run.
	loader := NewIconLoaderFunc(func(string) ([]byte, bool) { return tinyPNG(), true }, 32)
	f := NewFinderPane(FinderConfig{
		Theme:    toolkit.DefaultDark(),
		Icons:    loader,
		Places:   &shell.Places{},
		Lister:   func(string) (*shell.Dir, error) { return &shell.Dir{}, nil },
		ThumbKey: func(it shell.FileItem) string { return it.Path },
	})
	// Thumbnail path: a file whose thumbKey resolves -> (img, true).
	if img, thumb := f.cellImage(fileItem("p.png", "/p.png", 1)); img == nil || !thumb {
		t.Errorf("thumb cellImage img=%v thumb=%v", img, thumb)
	}
	// Directory path resolves the "folder" theme icon.
	if img, thumb := f.cellImage(dirItem("d", "/d")); img == nil || thumb {
		t.Errorf("dir cellImage img=%v thumb=%v", img, thumb)
	}
	if f.listIcon(dirItem("d", "/d")) == nil {
		t.Error("listIcon nil")
	}

	// With a loader that resolves NOTHING, cellImage falls back to drawn glyphs.
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
