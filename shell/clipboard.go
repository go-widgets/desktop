// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
)

// ClipOp is the pending file-clipboard operation: nothing, a copy (⌘C/Ctrl+C)
// or a cut/move (⌘X/Ctrl+X). A Paste applies whichever op the clipboard holds.
type ClipOp int

const (
	// ClipNone is an empty clipboard: a paste is a no-op.
	ClipNone ClipOp = iota
	// ClipCopy marks the paths to be COPIED on paste (the originals stay).
	ClipCopy
	// ClipCut marks the paths to be MOVED on paste (the originals disappear).
	// Cut paths are drawn dimmed in the view (IsCut) until pasted or cleared.
	ClipCut
)

// Clipboard is the Finder's file clipboard: a small, filesystem-free record of
// which path(s) the user marked with ⌘C / ⌘X (Ctrl on Linux/Windows) and which
// operation to apply when they ⌘V. It is the pure state half of the copy/paste
// feature — the actual bytes move through CopyFile / MoveFile once a paste is
// classified by PasteKindFor — so the whole state machine is unit-testable
// without a window.
type Clipboard struct {
	op    ClipOp
	paths []string
}

// SetCopy marks paths for copy-on-paste (⌘C). Blank paths are dropped; an
// all-blank/empty set clears the clipboard instead.
func (c *Clipboard) SetCopy(paths ...string) { c.set(ClipCopy, paths) }

// SetCut marks paths for move-on-paste (⌘X). Blank paths are dropped; an
// all-blank/empty set clears the clipboard instead.
func (c *Clipboard) SetCut(paths ...string) { c.set(ClipCut, paths) }

// set records op over the cleaned, non-blank subset of paths, or clears the
// clipboard when that subset is empty (so "copy nothing" is an empty clipboard,
// not a copy of nothing).
func (c *Clipboard) set(op ClipOp, paths []string) {
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != "" {
			cleaned = append(cleaned, filepath.Clean(p))
		}
	}
	if len(cleaned) == 0 {
		c.Clear()
		return
	}
	c.op = op
	c.paths = cleaned
}

// Op is the pending operation (ClipNone when empty).
func (c *Clipboard) Op() ClipOp { return c.op }

// Paths is a copy of the marked paths (nil when empty), safe for the caller to
// keep or mutate.
func (c *Clipboard) Paths() []string {
	if len(c.paths) == 0 {
		return nil
	}
	return append([]string(nil), c.paths...)
}

// Empty reports whether a paste would do nothing (no op, or no paths).
func (c *Clipboard) Empty() bool { return c.op == ClipNone || len(c.paths) == 0 }

// Clear empties the clipboard (after a move-paste consumes it, or on Escape).
func (c *Clipboard) Clear() {
	c.op = ClipNone
	c.paths = nil
}

// IsCut reports whether path is currently marked for a MOVE (so the view draws
// it dimmed). It is false for a copy mark and for any path not on the clipboard.
func (c *Clipboard) IsCut(path string) bool {
	if c.op != ClipCut {
		return false
	}
	path = filepath.Clean(path)
	for _, p := range c.paths {
		if p == path {
			return true
		}
	}
	return false
}

// PasteKind classifies how a single clipboard entry will paste into a target
// directory, so the UI knows whether to proceed silently, confirm a move, or
// confirm an overwrite — WITHOUT mutating the filesystem.
type PasteKind int

const (
	// PasteNoop: nothing to do (empty clipboard, or a move whose source is
	// already in the target directory).
	PasteNoop PasteKind = iota
	// PasteCopyPlain: a copy into another directory with no name collision —
	// proceed silently (macOS copies without a modal), then show a brief result.
	PasteCopyPlain
	// PasteCopySuffix: a copy back into the source's own directory — write a
	// " copie"-suffixed duplicate, no modal.
	PasteCopySuffix
	// PasteMoveConfirm: a move into another directory with no collision —
	// confirm "Déplacer « X » ici ?" before mutating.
	PasteMoveConfirm
	// PasteOverwriteConfirm: the target directory already holds an entry of the
	// source's name — confirm "Remplacer « X » ?" before overwriting.
	PasteOverwriteConfirm
)

// PasteKindFor classifies pasting src into destDir under the current clipboard
// op. forceMove requests a move regardless of the op (macOS ⌘⌥V, "Move Item
// Here"). It reads the filesystem only to test for a name collision; it writes
// nothing.
func (c *Clipboard) PasteKindFor(src, destDir string, forceMove bool) PasteKind {
	if c.Empty() {
		return PasteNoop
	}
	src = filepath.Clean(src)
	destDir = filepath.Clean(destDir)
	move := forceMove || c.op == ClipCut
	sameDir := filepath.Dir(src) == destDir
	_, statErr := os.Lstat(filepath.Join(destDir, filepath.Base(src)))
	collision := statErr == nil

	if move {
		if sameDir {
			return PasteNoop
		}
		if collision {
			return PasteOverwriteConfirm
		}
		return PasteMoveConfirm
	}
	if sameDir {
		return PasteCopySuffix
	}
	if collision {
		return PasteOverwriteConfirm
	}
	return PasteCopyPlain
}
