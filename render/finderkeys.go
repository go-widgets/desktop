// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"path/filepath"
	"strings"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/toolkit"
)

// finderShortcut is a keyboard file-operation the Finder recognises.
type finderShortcut int

const (
	scNone      finderShortcut = iota
	scCopy                     // ⌘C / Ctrl+C
	scCut                      // ⌘X / Ctrl+X
	scPaste                    // ⌘V / Ctrl+V
	scPasteMove                // ⌘⌥V — "Move Item Here" (macOS only)
)

// classifyShortcut maps an EventKeyDown to the file operation it requests, or
// scNone. The primary accelerator is ⌘ on macOS and Ctrl elsewhere; because the
// Cocoa backend folds ⌘ into Ctrl (while also setting Meta), reading Meta||Ctrl
// recognises the accelerator on every platform. Paste-as-move is the macOS
// ⌘⌥V chord (Meta+Alt): on Linux/Windows a Ctrl+Alt+V never sets Meta, so it is
// naturally macOS-only and does not shadow a plain paste.
func classifyShortcut(ev toolkit.Event) finderShortcut {
	if ev.Kind != toolkit.EventKeyDown {
		return scNone
	}
	code := strings.ToLower(ev.Code)
	if ev.Meta && ev.Alt && code == "v" {
		return scPasteMove
	}
	accel := ev.Meta || ev.Ctrl
	if !accel || ev.Alt {
		// A bare key, or an Alt chord that is not the ⌘⌥V paste-as-move combo.
		return scNone
	}
	switch code {
	case "c":
		return scCopy
	case "x":
		return scCut
	case "v":
		return scPaste
	}
	return scNone
}

// onShortcut handles a keyboard file-operation event, reporting whether it
// consumed it (so the caller does not also route it to the view tree).
func (f *FinderPane) onShortcut(ev toolkit.Event) bool {
	switch classifyShortcut(ev) {
	case scCopy:
		f.markClipboard(false)
	case scCut:
		f.markClipboard(true)
	case scPaste:
		f.pasteInto(false)
	case scPasteMove:
		f.pasteInto(true)
	default:
		return false
	}
	return true
}

// markClipboard records the current selection on the file clipboard for a copy
// (cut=false) or a cut (cut=true). With no selection it is a no-op. A fresh mark
// repaints so a newly-cut item shows dimmed at once.
func (f *FinderPane) markClipboard(cut bool) {
	it, ok := f.selectedItem()
	if !ok {
		return
	}
	if cut {
		f.clip.SetCut(it.Path)
	} else {
		f.clip.SetCopy(it.Path)
	}
	f.relayout()
}

// pasteInto applies the clipboard into the current directory. forceMove is the
// ⌘⌥V "move regardless of the clipboard op" request. It classifies the paste
// (via the shell state machine) and either proceeds silently (a plain copy),
// pops a move/overwrite confirmation, or does nothing (empty clipboard, or the
// source already lives here).
func (f *FinderPane) pasteInto(forceMove bool) {
	dir := f.cwd.Get()
	if dir == "" || f.clip.Empty() {
		return
	}
	src := f.clip.Paths()[0]
	move := forceMove || f.clip.Op() == shell.ClipCut
	switch f.clip.PasteKindFor(src, dir, forceMove) {
	case shell.PasteNoop:
		return
	case shell.PasteCopyPlain, shell.PasteCopySuffix:
		// macOS proceeds without a modal for a plain copy; the brief result is
		// the pasted item appearing selected in the refreshed view.
		f.applyPaste(pendingPaste{src: src, dir: dir})
	case shell.PasteMoveConfirm:
		f.confirmPaste("Déplacer", "Déplacer « "+filepath.Base(src)+" » ici ?",
			pendingPaste{src: src, dir: dir, move: true})
	case shell.PasteOverwriteConfirm:
		f.confirmPaste("Remplacer", "Remplacer « "+filepath.Base(src)+" » ?",
			pendingPaste{src: src, dir: dir, move: move, overwrite: true})
	}
}

// pendingPaste is the paste a confirmation dialog will apply on confirm, held
// while the modal is up so the real filesystem is mutated ONLY on confirm.
type pendingPaste struct {
	src, dir        string
	move, overwrite bool
}

// confirmPaste pops the modal confirmation for a move-paste or an overwrite and
// records the paste it will apply. action labels the confirm button; body is the
// question. The filesystem is untouched until confirmPendingPaste runs.
func (f *FinderPane) confirmPaste(action, body string, p pendingPaste) {
	f.pending = &p
	cancel := toolkit.NewButton("Annuler", f.cancelPaste)
	confirm := toolkit.NewButton(action, f.confirmPendingPaste)
	f.dialog = toolkit.NewDialog(action, centeredLabel(body), cancel, confirm)
	f.dialog.OnClose = f.cancelPaste
	f.layoutDialog()
}

// confirmPendingPaste is the confirm-button handler: it dismisses the dialog and
// applies the recorded paste (the one point a confirmed move/overwrite mutates
// the filesystem).
func (f *FinderPane) confirmPendingPaste() {
	p := f.pending
	f.pending = nil
	f.closeDialog()
	if p != nil {
		f.applyPaste(*p)
	}
}

// cancelPaste is the cancel-button / dialog-close handler: it drops the pending
// paste and dismisses the dialog, leaving the filesystem untouched.
func (f *FinderPane) cancelPaste() {
	f.pending = nil
	f.closeDialog()
}

// applyPaste performs the (already-classified, already-confirmed) paste: it
// mutates the filesystem via the shell copy/move primitives, clears the
// clipboard after a move (a cut is consumed; a copy stays, so ⌘V can repeat),
// refreshes the current directory and selects the pasted item. On failure it
// shows a dismissable error rather than crashing.
func (f *FinderPane) applyPaste(p pendingPaste) {
	var (
		dest string
		err  error
	)
	switch {
	case p.move && p.overwrite:
		dest, err = shell.MoveFileReplace(p.src, p.dir)
	case p.move:
		dest, err = shell.MoveFile(p.src, p.dir)
	case p.overwrite:
		dest, err = shell.CopyFileReplace(p.src, p.dir)
	default:
		dest, err = shell.CopyFile(p.src, p.dir)
	}
	if err != nil {
		f.showMoveError(err)
		return
	}
	if p.move {
		f.clip.Clear()
	}
	f.refreshAndSelect(p.dir, dest)
}

// refreshAndSelect re-lists dir and selects the entry at path (the pasted item),
// so the result is visible without a manual click.
func (f *FinderPane) refreshAndSelect(dir, path string) {
	f.Navigate(dir)
	f.selectPath(path)
	f.relayout()
}

// selectPath selects the model row whose path matches, in whichever view is
// active (list, icon or gallery). A path not present (or the column view, which
// has no single-selection model here) leaves the selection unchanged.
func (f *FinderPane) selectPath(path string) {
	path = filepath.Clean(path)
	idx := -1
	for i := 0; i < f.fileModel.Len(); i++ {
		if filepath.Clean(f.fileModel.At(i).Path) == path {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	switch f.viewMode.Get() {
	case ViewList:
		f.listView.table.Selected = idx
	case ViewIcons:
		// The grid validates a selection against its cell count, which is filled
		// from the model at Draw time; sync it first so a programmatic select
		// (before the first paint) is not clamped away.
		f.iconView.syncCells()
		f.iconView.grid.SetSelected(idx)
	case ViewGallery:
		// The gallery validates a selection against its item count, filled from the
		// model at Draw time; sync it first so a programmatic select (before the
		// first paint) is not clamped away.
		f.galleryView.syncItems()
		f.galleryView.SetSelected(idx)
	}
}

// selectedItem returns the file item selected in the active view (list, icon or
// gallery), or false when nothing is selected (or the column view is active,
// which has no single-selection model here).
func (f *FinderPane) selectedItem() (shell.FileItem, bool) {
	idx := -1
	switch f.viewMode.Get() {
	case ViewList:
		idx = f.listView.table.Selected
	case ViewIcons:
		idx = f.iconView.grid.Selected()
	case ViewGallery:
		idx = f.galleryView.Selected()
	}
	if idx < 0 || idx >= f.fileModel.Len() {
		return shell.FileItem{}, false
	}
	return f.fileModel.At(idx), true
}

// Clipboard exposes the finder's file clipboard for the capture CLI and tests.
func (f *FinderPane) Clipboard() *shell.Clipboard { return &f.clip }

// ConfirmDialog accepts the active paste confirmation (the peer of clicking its
// confirm button), applying the pending move/overwrite — the programmatic hook
// the capture CLI and the on-device proof use to drive the modal without a live
// pointer. It is a no-op when no paste is pending.
func (f *FinderPane) ConfirmDialog() {
	if f.pending != nil {
		f.confirmPendingPaste()
	}
}

// SelectByPath selects the model row at path in the active view — the
// programmatic peer of clicking it, used by the capture CLI and tests to set a
// selection before firing a keyboard shortcut.
func (f *FinderPane) SelectByPath(path string) { f.selectPath(path) }

// HandleKey dispatches a synthetic key event to the finder exactly as the
// windowed backend would, so the capture CLI and tests can drive the keyboard
// file operations without a live compositor. It returns whether the event was
// consumed as a shortcut.
func (f *FinderPane) HandleKey(ev toolkit.Event) bool {
	if f.dialog != nil {
		f.overlay.OnEvent(ev)
		return true
	}
	return f.onShortcut(ev)
}

// dimAlphaNum is the alpha multiplier applied to a cut item's icon (a
// translucent "ghost" ~40% opaque), the Finder/Explorer cut cue.
const dimAlphaNum = 40

// dimImage returns a translucent copy of img with its per-pixel alpha scaled to
// dimAlphaNum% — the dimmed look of an item marked for a cut. The source pixels
// are left untouched (a fresh buffer is returned), so the un-cut icon is
// unaffected. A nil or empty image is returned unchanged.
func dimImage(img *toolkit.Image) *toolkit.Image {
	if img == nil || img.W <= 0 || img.H <= 0 || len(img.Pixels) < img.W*img.H*4 {
		return img
	}
	out := make([]byte, len(img.Pixels))
	copy(out, img.Pixels)
	for i := 3; i < len(out); i += 4 {
		out[i] = byte(int(out[i]) * dimAlphaNum / 100)
	}
	dim := toolkit.NewImage(out, img.W, img.H)
	dim.Scale = img.Scale
	return dim
}
