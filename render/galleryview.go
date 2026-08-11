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

// galleryView is a thin shell adapter over toolkit.GalleryView (the big-preview
// + filmstrip browser the toolkit now ships — macOS Finder's "Galerie" view). It
// projects the shared directory model into the widget's items — resolving each
// item's image (a real raster thumbnail sits on a light chip, a folder/document
// glyph does not, exactly as the Vignettes grid does), and the large centred
// preview of the current item, the horizontally-scrolling filmstrip, the accent
// selection ring and arrow-key navigation all come from the toolkit widget — and
// wires activation (double-click / Enter on a folder) back to the shell's open
// handler.
//
// Beyond the toolkit widget, the adapter accepts a file dragged onto a folder
// thumbnail and reports it as a move request via onFileDrop (the Finder's
// drag-to-move), and exposes the selected item's path as the drag payload, so
// ⌘C/⌘X/⌘V and drag-to-move keep working in the gallery's selection too.
type galleryView struct {
	toolkit.Base

	gv      *toolkit.GalleryView
	model   *mvvm.ObservableList[shell.FileItem]
	imageFn func(shell.FileItem) (*toolkit.Image, bool)
	openFn  func(shell.FileItem)
	emptyFn func() string

	onFileDrop func(dstDir, srcPath string)
}

// newGalleryView builds a gallery view over model. imageFn resolves an item to
// its Image plus whether that Image is a real raster thumbnail (so the preview /
// thumbnail backs a dark photo with a light chip); openFn opens an item
// (navigate a folder / activate a file); emptyFn supplies the centred message
// shown when the model is empty.
func newGalleryView(model *mvvm.ObservableList[shell.FileItem],
	imageFn func(shell.FileItem) (*toolkit.Image, bool),
	openFn func(shell.FileItem), emptyFn func() string) *galleryView {
	v := &galleryView{
		gv:      toolkit.NewGalleryView(),
		model:   model,
		imageFn: imageFn,
		openFn:  openFn,
		emptyFn: emptyFn,
	}
	v.gv.OnActivate = v.activate
	return v
}

// activate opens the model item behind gallery index (the GalleryView fires this
// on Enter/Space or a second click of the already-selected thumbnail).
func (v *galleryView) activate(index int) {
	if v.openFn != nil && index >= 0 && index < v.model.Len() {
		v.openFn(v.model.At(index))
	}
}

// clearSelection drops the gallery selection (after a navigation); the next
// syncItems re-selects the new directory's first item so the preview is never
// blank while the directory has content.
func (v *galleryView) clearSelection() { v.gv.SetSelected(-1) }

// Selected is the current gallery selection index (-1 when nothing is selected).
func (v *galleryView) Selected() int { return v.gv.Selected() }

// SetSelected drives the gallery selection programmatically (the paste-select /
// keyboard-shortcut peer of clicking a thumbnail).
func (v *galleryView) SetSelected(i int) { v.gv.SetSelected(i) }

// syncItems reprojects the current model into the widget's items, resolving each
// item's image and empty-state message, and — because a gallery always shows a
// current item — selects the first item when the widget has no selection but the
// directory has content. It runs at Draw time (and before an event is routed),
// so a freshly-generated thumbnail or a navigation is reflected on the next
// frame, mirroring the icon view's syncCells.
func (v *galleryView) syncItems() {
	items := make([]toolkit.GalleryItem, v.model.Len())
	for i := 0; i < v.model.Len(); i++ {
		it := v.model.At(i)
		img, raster := v.imageFn(it)
		items[i] = toolkit.GalleryItem{Image: img, Label: it.Name, Key: it.Path, Raster: raster}
	}
	v.gv.Items = items
	v.gv.Empty = "Dossier vide"
	if v.emptyFn != nil {
		if m := v.emptyFn(); m != "" {
			v.gv.Empty = m
		}
	}
	if v.gv.Selected() < 0 && len(items) > 0 {
		v.gv.SetSelected(0)
	}
}

// SetBounds records the view bounds and forwards them to the gallery.
func (v *galleryView) SetBounds(r toolkit.Rect) {
	v.Base.SetBounds(r)
	v.gv.SetBounds(r)
}

// Draw syncs the items and paints the gallery (or its empty-state message).
func (v *galleryView) Draw(p painter.Painter, th *toolkit.Theme) {
	v.syncItems()
	v.gv.SetBounds(v.Bounds())
	v.gv.Draw(p, th)
}

// OnEvent forwards to the gallery, except an external file drop (an EventDrop
// carrying a file-path payload) onto a folder thumbnail, which the adapter
// reports as a move into that folder.
func (v *galleryView) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventDrop && isFilePayload(ev.Code) {
		v.handleFileDrop(ev)
		return
	}
	v.syncItems()
	v.gv.OnEvent(ev)
}

// handleFileDrop resolves the filmstrip thumbnail under the drop point and, when
// it is a folder (and not the dragged file's own thumbnail), reports a move of
// the dropped file into it.
func (v *galleryView) handleFileDrop(ev toolkit.Event) {
	v.syncItems()
	idx := v.gv.ThumbAt(ev.X, ev.Y)
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

// DragData makes the gallery view a toolkit DragSource: a drag started on the
// selected thumbnail carries that file's path, so the host shows a real file
// drag and can route a drop back to the finder as a move.
func (v *galleryView) DragData() string {
	v.syncItems()
	idx := v.gv.Selected()
	if idx < 0 || idx >= v.model.Len() {
		return ""
	}
	return v.model.At(idx).Path
}

// AcceptsDrop accepts any file-path payload so the host routes a file drag onto
// the gallery here (where handleFileDrop decides whether it landed on a folder).
func (v *galleryView) AcceptsDrop(payload string) bool { return isFilePayload(payload) }

// galleryView is a DragSource + DropTarget.
var (
	_ toolkit.DragSource = (*galleryView)(nil)
	_ toolkit.DropTarget = (*galleryView)(nil)
)
