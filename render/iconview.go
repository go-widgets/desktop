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

// iconView is a thin shell adapter over toolkit.IconGrid (the size-driven,
// framed/chip-backed icon grid the toolkit now ships). It projects the shared
// directory model into the grid's cells — resolving each item's icon (a real
// raster thumbnail sits on a light chip, a folder/document glyph does not),
// size-driven cell footprint, centred elided label and selection all come from
// the toolkit widget — and wires activation back to the shell's open handler.
//
// Beyond the toolkit widget, the adapter accepts a file dragged onto a folder
// cell and reports it as a move request via onFileDrop (the Finder's
// drag-to-move), and exposes the selected item's path as the drag payload.
type iconView struct {
	toolkit.Base

	grid    *toolkit.IconGrid
	model   *mvvm.ObservableList[shell.FileItem]
	imageFn func(shell.FileItem) (*toolkit.Image, bool)
	openFn  func(shell.FileItem)
	emptyFn func() string

	onFileDrop func(dstDir, srcPath string)
}

// newIconView builds an icon view over model. imageFn resolves an item to its
// Image plus whether that Image is a real raster thumbnail (so the cell backs it
// with a light chip); openFn opens an item (navigate a folder / activate a
// file); emptyFn supplies the centred message shown when the model is empty.
func newIconView(model *mvvm.ObservableList[shell.FileItem], iconPx int,
	imageFn func(shell.FileItem) (*toolkit.Image, bool),
	openFn func(shell.FileItem), emptyFn func() string) *iconView {
	v := &iconView{
		grid:    toolkit.NewIconGrid(),
		model:   model,
		imageFn: imageFn,
		openFn:  openFn,
		emptyFn: emptyFn,
	}
	v.grid.SetIconSize(iconPx)
	v.grid.OnActivate = v.activate
	return v
}

// activate opens the model item behind grid cell index (the IconGrid fires this
// on a second click of the already-selected cell).
func (v *iconView) activate(index int) {
	if v.openFn != nil && index >= 0 && index < v.model.Len() {
		v.openFn(v.model.At(index))
	}
}

// SetIconSize changes the icon square side (the grid clamps it to a readable
// minimum and re-anchors the reflow at the top).
func (v *iconView) SetIconSize(px int) { v.grid.SetIconSize(px) }

// clearSelection drops the grid selection (after a navigation).
func (v *iconView) clearSelection() { v.grid.SetSelected(-1) }

// syncCells reprojects the current model into the grid's cells, resolving each
// item's icon and empty-state message. It runs at Draw time (and before an event
// is routed), so a freshly-generated thumbnail or a navigation is reflected on
// the next frame.
func (v *iconView) syncCells() {
	cells := make([]toolkit.IconCell, v.model.Len())
	for i := 0; i < v.model.Len(); i++ {
		it := v.model.At(i)
		img, raster := v.imageFn(it)
		cells[i] = toolkit.IconCell{Image: img, Label: it.Name, Key: it.Path, Raster: raster}
	}
	v.grid.Cells = cells
	v.grid.Empty = "Dossier vide"
	if v.emptyFn != nil {
		if m := v.emptyFn(); m != "" {
			v.grid.Empty = m
		}
	}
}

// SetBounds records the view bounds and forwards them to the grid.
func (v *iconView) SetBounds(r toolkit.Rect) {
	v.Base.SetBounds(r)
	v.grid.SetBounds(r)
}

// Draw syncs the cells and paints the grid (or its empty-state message).
func (v *iconView) Draw(p painter.Painter, th *toolkit.Theme) {
	v.syncCells()
	v.grid.SetBounds(v.Bounds())
	v.grid.Draw(p, th)
}

// OnEvent forwards to the grid, except an external file drop (an EventDrop
// carrying a file-path payload) onto a folder cell, which the adapter reports as
// a move into that folder.
func (v *iconView) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventDrop && isFilePayload(ev.Code) {
		v.handleFileDrop(ev)
		return
	}
	v.syncCells()
	v.grid.OnEvent(ev)
}

// handleFileDrop resolves the cell under the drop point and, when it is a folder
// (and not the dragged file's own row), reports a move of the dropped file into
// it.
func (v *iconView) handleFileDrop(ev toolkit.Event) {
	v.syncCells()
	idx := v.grid.IndexAt(ev.X, ev.Y)
	if idx < 0 || idx >= v.model.Len() {
		return
	}
	it := v.model.At(idx)
	if !it.IsDir || it.Path == ev.Code {
		return
	}
	if v.onFileDrop != nil {
		v.onFileDrop(it.Path, ev.Code)
	}
}

// DragData makes the icon view a toolkit DragSource: a drag started on the
// selected cell carries that file's path (the IconGrid's own DragData), so the
// host shows a real file drag and can route a drop back as a move.
func (v *iconView) DragData() string {
	v.syncCells()
	return v.grid.DragData()
}

// AcceptsDrop accepts any file-path payload so the host routes a file drag onto
// the grid here (where handleFileDrop decides whether it landed on a folder).
func (v *iconView) AcceptsDrop(payload string) bool { return isFilePayload(payload) }

// iconView is a DragSource + DropTarget.
var (
	_ toolkit.DragSource = (*iconView)(nil)
	_ toolkit.DropTarget = (*iconView)(nil)
)
