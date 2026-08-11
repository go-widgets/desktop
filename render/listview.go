// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// listView is the file manager's "Liste" mode: a sortable table with the four
// Finder columns — Nom, Taille, Type, Date de modification — reusing the
// toolkit's Table (which already owns columns, header-click sort surfacing,
// selection and a per-row leading icon). listView is the thin host that owns
// the data model: it projects the file items into Table.Rows, paints each row's
// type icon through Table.RowIcon, re-sorts on a header click, and opens a row
// on a second click (single-click selects, a click on the already-selected row
// activates — the pointer-only stand-in for a double click).
type listView struct {
	toolkit.Base

	table   *toolkit.Table
	model   *mvvm.ObservableList[shell.FileItem]
	theme   *toolkit.Theme
	iconFn  func(shell.FileItem) *toolkit.Image
	openFn  func(shell.FileItem)
	sortFn  func(col int, asc bool)
	emptyFn func() string

	// onFileDrop reports a file dropped onto a folder row as a move into it.
	onFileDrop func(dstDir, srcPath string)

	// reusable per-row icon Image, bounds mutated per row.
	rowIcon *toolkit.Image
}

// listColumns are the four Finder columns; Nom takes the auto (remaining) width.
func listColumns() []toolkit.TableColumn {
	return []toolkit.TableColumn{
		{Title: "Nom", Width: 0, Sortable: true},
		{Title: "Taille", Width: 90, Align: toolkit.AlignRight, Sortable: true},
		{Title: "Type", Width: 150, Sortable: true},
		{Title: "Date de modification", Width: 170, Sortable: true},
	}
}

// newListView builds a list view over model. iconFn resolves an item to its
// small leading type icon; openFn opens an item; sortFn re-sorts the shared
// model by a column (the finder owns the sort so every view stays in step);
// emptyFn supplies the empty-state message.
func newListView(model *mvvm.ObservableList[shell.FileItem], th *toolkit.Theme,
	iconFn func(shell.FileItem) *toolkit.Image, openFn func(shell.FileItem),
	sortFn func(col int, asc bool), emptyFn func() string) *listView {
	lv := &listView{
		model:   model,
		theme:   th,
		iconFn:  iconFn,
		openFn:  openFn,
		sortFn:  sortFn,
		emptyFn: emptyFn,
		rowIcon: toolkit.NewImageFit(nil, 0, 0),
	}
	lv.table = toolkit.NewTable(listColumns(), nil)
	lv.table.SortColumn = 0
	lv.table.SortAsc = true
	lv.table.RowIcon = lv.rowIconFunc
	lv.table.OnSort = func(col int, asc bool) {
		if lv.sortFn != nil {
			lv.sortFn(col, asc)
		}
	}
	lv.Refresh()
	return lv
}

// Refresh reprojects the model into the table rows (call after a navigation or
// a re-sort).
func (lv *listView) Refresh() {
	rows := make([][]string, lv.model.Len())
	for i := 0; i < lv.model.Len(); i++ {
		it := lv.model.At(i)
		rows[i] = []string{it.Name, it.HumanSize(), it.TypeLabel(), it.ModTimeString()}
	}
	lv.table.Rows = rows
}

// SetSort mirrors the finder's sort state onto the table header indicator.
func (lv *listView) SetSort(col int, asc bool) {
	lv.table.SortColumn = col
	lv.table.SortAsc = asc
}

// rowIconFunc supplies the table's per-row leading type icon.
func (lv *listView) rowIconFunc(row int) (toolkit.TableIconFunc, bool) {
	if row < 0 || row >= lv.model.Len() {
		return nil, false
	}
	img := lv.iconFn(lv.model.At(row))
	if img == nil {
		return nil, false
	}
	return func(p painter.Painter, r toolkit.Rect, _ toolkit.RGBA) {
		img.SetBounds(r)
		img.Draw(p, lv.theme)
	}, true
}

// SetBounds lays the table into the view bounds.
func (lv *listView) SetBounds(r toolkit.Rect) {
	lv.Base.SetBounds(r)
	lv.table.SetBounds(r)
}

// Draw paints the table (or the empty-state message).
func (lv *listView) Draw(p painter.Painter, th *toolkit.Theme) {
	b := lv.Bounds()
	p.FillRect(b, th.Surface)
	if lv.model.Len() == 0 {
		drawCentredMessage(p, th, b, lv.emptyFn)
		// Still draw the header row for context.
		lv.table.Draw(p, th)
		return
	}
	lv.table.Draw(p, th)
}

// OnEvent forwards to the table, opens the row when a click lands on the
// already-selected row (the pointer stand-in for a double click), and maps a
// file dropped on a folder row to a move into that folder.
func (lv *listView) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventDrop && isFilePayload(ev.Code) {
		lv.handleFileDrop(ev)
		return
	}
	if ev.Kind == toolkit.EventClick {
		prev := lv.table.Selected
		row := lv.table.RowAt(ev.X, ev.Y)
		lv.table.OnEvent(ev)
		if row >= 0 && row == prev && row < lv.model.Len() && lv.openFn != nil {
			lv.openFn(lv.model.At(row))
		}
		return
	}
	lv.table.OnEvent(ev)
}

// handleFileDrop resolves the row under the drop point and, when it is a folder
// (and not the dragged file's own row), reports a move of the dropped file into
// it.
func (lv *listView) handleFileDrop(ev toolkit.Event) {
	row := lv.table.RowAt(ev.X, ev.Y)
	if row < 0 || row >= lv.model.Len() {
		return
	}
	it := lv.model.At(row)
	if !it.IsDir || it.Path == ev.Code {
		return
	}
	if lv.onFileDrop != nil {
		lv.onFileDrop(it.Path, ev.Code)
	}
}

// DragData makes the list view a toolkit DragSource: a drag started on the
// selected row carries that file's path, so the host shows a real file drag and
// can route a drop back to the finder as a move.
func (lv *listView) DragData() string {
	row := lv.table.Selected
	if row < 0 || row >= lv.model.Len() {
		return ""
	}
	return lv.model.At(row).Path
}

// AcceptsDrop accepts any file-path payload so the host routes a file drag onto
// the list here (where handleFileDrop decides whether it landed on a folder).
func (lv *listView) AcceptsDrop(payload string) bool { return isFilePayload(payload) }

// listView is a DragSource + DropTarget.
var (
	_ toolkit.DragSource = (*listView)(nil)
	_ toolkit.DropTarget = (*listView)(nil)
)

// drawCentredMessage centres the empty-state message within b.
func drawCentredMessage(p painter.Painter, th *toolkit.Theme, b toolkit.Rect, emptyFn func() string) {
	msg := "Dossier vide"
	if emptyFn != nil {
		if m := emptyFn(); m != "" {
			msg = m
		}
	}
	ink := mutedInk(th, 0.4)
	tw := toolkit.TextWidth(msg)
	toolkit.DrawText(p, b.X+(b.W-tw)/2, b.Y+b.H/2-toolkit.GlyphHeight()/2, msg, ink)
}
