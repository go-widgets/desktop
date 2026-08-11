// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"path/filepath"
	"testing"
)

func TestClipboardEmptyByDefault(t *testing.T) {
	var c Clipboard
	if !c.Empty() {
		t.Fatal("a fresh clipboard is empty")
	}
	if c.Op() != ClipNone {
		t.Fatalf("op = %v", c.Op())
	}
	if c.Paths() != nil {
		t.Fatalf("paths = %v", c.Paths())
	}
}

func TestClipboardSetCopyAndCut(t *testing.T) {
	var c Clipboard
	c.SetCopy("/a/x.txt")
	if c.Empty() || c.Op() != ClipCopy {
		t.Fatalf("copy op = %v", c.Op())
	}
	if got := c.Paths(); len(got) != 1 || got[0] != filepath.Clean("/a/x.txt") {
		t.Fatalf("paths = %v", got)
	}
	if c.IsCut("/a/x.txt") {
		t.Fatal("a copied path is not cut")
	}

	c.SetCut("/a/y.txt", "/a/z.txt")
	if c.Op() != ClipCut || len(c.Paths()) != 2 {
		t.Fatalf("cut op = %v paths = %v", c.Op(), c.Paths())
	}
	if !c.IsCut("/a/y.txt") || !c.IsCut("/a/z.txt") {
		t.Fatal("cut paths should report IsCut")
	}
	if c.IsCut("/a/other.txt") {
		t.Fatal("an unmarked path is not cut")
	}
}

func TestClipboardBlankPathsDropped(t *testing.T) {
	var c Clipboard
	c.SetCopy("", "/a/keep")
	if len(c.Paths()) != 1 {
		t.Fatalf("blank not dropped: %v", c.Paths())
	}
}

func TestClipboardAllBlankClears(t *testing.T) {
	var c Clipboard
	c.SetCopy("/a/x")
	c.SetCopy("") // an all-blank set clears rather than "copy of nothing"
	if !c.Empty() {
		t.Fatal("all-blank SetCopy should clear")
	}
}

func TestClipboardClear(t *testing.T) {
	var c Clipboard
	c.SetCut("/a/x")
	c.Clear()
	if !c.Empty() || c.Op() != ClipNone {
		t.Fatal("Clear should empty the clipboard")
	}
}

func TestClipboardIsCutFalseWhenCopy(t *testing.T) {
	var c Clipboard
	c.SetCopy("/a/x")
	if c.IsCut("/a/x") {
		t.Fatal("IsCut is false for a copy mark")
	}
}

// --- PasteKindFor: the state machine -----------------------------------------

func TestPasteKindEmptyClipboardIsNoop(t *testing.T) {
	var c Clipboard
	if k := c.PasteKindFor("/a/x", "/b", false); k != PasteNoop {
		t.Fatalf("empty clipboard should be PasteNoop, got %v", k)
	}
}

func TestPasteKindCopyPlainAndSuffix(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.txt")
	writeFile(t, src, "x")
	var c Clipboard
	c.SetCopy(src)

	other := t.TempDir()
	if k := c.PasteKindFor(src, other, false); k != PasteCopyPlain {
		t.Fatalf("copy into empty other dir = %v, want PasteCopyPlain", k)
	}
	// Back into its own directory: a suffixed duplicate, no modal.
	if k := c.PasteKindFor(src, dir, false); k != PasteCopySuffix {
		t.Fatalf("copy into own dir = %v, want PasteCopySuffix", k)
	}
}

func TestPasteKindCopyOverwriteConfirm(t *testing.T) {
	src := filepath.Join(t.TempDir(), "x.txt")
	writeFile(t, src, "x")
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "x.txt"), "old")
	var c Clipboard
	c.SetCopy(src)
	if k := c.PasteKindFor(src, other, false); k != PasteOverwriteConfirm {
		t.Fatalf("copy over existing = %v, want PasteOverwriteConfirm", k)
	}
}

func TestPasteKindMoveConfirmAndNoop(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.txt")
	writeFile(t, src, "x")
	var c Clipboard
	c.SetCut(src)

	other := t.TempDir()
	if k := c.PasteKindFor(src, other, false); k != PasteMoveConfirm {
		t.Fatalf("move into other dir = %v, want PasteMoveConfirm", k)
	}
	// Moving into its own directory is a no-op.
	if k := c.PasteKindFor(src, dir, false); k != PasteNoop {
		t.Fatalf("move into own dir = %v, want PasteNoop", k)
	}
}

func TestPasteKindMoveOverwriteConfirm(t *testing.T) {
	src := filepath.Join(t.TempDir(), "x.txt")
	writeFile(t, src, "x")
	other := t.TempDir()
	writeFile(t, filepath.Join(other, "x.txt"), "old")
	var c Clipboard
	c.SetCut(src)
	if k := c.PasteKindFor(src, other, false); k != PasteOverwriteConfirm {
		t.Fatalf("move over existing = %v, want PasteOverwriteConfirm", k)
	}
}

func TestPasteKindForceMoveOverridesCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.txt")
	writeFile(t, src, "x")
	var c Clipboard
	c.SetCopy(src) // op is copy, but forceMove (Cmd+Opt+V) makes it a move

	other := t.TempDir()
	if k := c.PasteKindFor(src, other, true); k != PasteMoveConfirm {
		t.Fatalf("forceMove copy-op = %v, want PasteMoveConfirm", k)
	}
	if k := c.PasteKindFor(src, dir, true); k != PasteNoop {
		t.Fatalf("forceMove into own dir = %v, want PasteNoop", k)
	}
}
