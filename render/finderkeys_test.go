// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-widgets/toolkit"
)

// realFinder builds a FinderPane over a REAL temp directory (the default
// NativeLister), laid out at a realistic size, navigated to dir, so a keyboard
// file operation mutates actual files on disk. It returns the pane and the
// temp root.
func kbFinder(t *testing.T, dir string) *FinderPane {
	t.Helper()
	f := NewFinderPane(FinderConfig{Theme: toolkit.DefaultDark()})
	f.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: 900, H: 600})
	f.Navigate(dir)
	return f
}

func mkfile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// key builds a synthetic EventKeyDown carrying the given code + modifiers, as
// the windowed backend would deliver it.
func key(code string, ctrl, meta, alt bool) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code, Ctrl: ctrl, Meta: meta, Alt: alt}
}

func TestClassifyShortcut(t *testing.T) {
	cases := []struct {
		name string
		ev   toolkit.Event
		want finderShortcut
	}{
		{"cmd+c (mac: meta+ctrl fold)", key("c", true, true, false), scCopy},
		{"ctrl+c (linux)", key("c", true, false, false), scCopy},
		{"cmd+x", key("x", true, true, false), scCut},
		{"ctrl+v", key("v", true, false, false), scPaste},
		{"cmd+opt+v paste-as-move", key("v", true, true, true), scPasteMove},
		{"ctrl+alt+v is NOT paste-as-move (no meta)", key("v", true, false, true), scNone},
		{"cmd+opt+c is not a shortcut", key("c", true, true, true), scNone},
		{"bare c", key("c", false, false, false), scNone},
		{"cmd+z (unmapped)", key("z", true, true, false), scNone},
		{"uppercase code still matches", key("C", true, false, false), scCopy},
		{"not a keydown", toolkit.Event{Kind: toolkit.EventChar, Code: "c", Ctrl: true}, scNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyShortcut(c.ev); got != c.want {
				t.Fatalf("classifyShortcut = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCopyPasteToOtherDirOnDisk(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mkfile(t, filepath.Join(root, "f.txt"), "hello")

	f := kbFinder(t, root)
	f.SelectByPath(filepath.Join(root, "f.txt"))
	if !f.HandleKey(key("c", false, true, false)) { // ⌘C
		t.Fatal("⌘C should be consumed")
	}
	f.Navigate(sub)
	if !f.HandleKey(key("v", false, true, false)) { // ⌘V
		t.Fatal("⌘V should be consumed")
	}
	if f.DialogActive() {
		t.Fatal("a plain copy shows no modal")
	}
	if !exists(filepath.Join(sub, "f.txt")) {
		t.Fatal("the file was not copied into sub/")
	}
	if !exists(filepath.Join(root, "f.txt")) {
		t.Fatal("a copy must leave the original")
	}
	if got := readBody(t, filepath.Join(sub, "f.txt")); got != "hello" {
		t.Fatalf("copied body = %q", got)
	}
}

func TestCutPasteMovesOnDisk(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "m.txt")
	mkfile(t, src, "move me")

	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("x", false, true, false)) // ⌘X
	if !f.Clipboard().IsCut(src) {
		t.Fatal("⌘X should mark the file cut")
	}
	f.Navigate(sub)
	f.HandleKey(key("v", false, true, false)) // ⌘V
	if !f.DialogActive() {
		t.Fatal("a move-paste must confirm")
	}
	f.confirmPendingPaste()
	if exists(src) {
		t.Fatal("source should be gone after a move")
	}
	if !exists(filepath.Join(sub, "m.txt")) {
		t.Fatal("the file was not moved into sub/")
	}
	if !f.Clipboard().Empty() {
		t.Fatal("a move consumes the clipboard")
	}
}

func TestPasteAsMoveCmdOptV(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "p.txt")
	mkfile(t, src, "x")

	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("c", false, true, false)) // ⌘C — a COPY on the clipboard
	f.Navigate(sub)
	f.HandleKey(key("v", true, true, true)) // ⌘⌥V — move regardless
	if !f.DialogActive() {
		t.Fatal("paste-as-move must confirm")
	}
	f.confirmPendingPaste()
	if exists(src) {
		t.Fatal("⌘⌥V should MOVE, leaving no original")
	}
	if !exists(filepath.Join(sub, "p.txt")) {
		t.Fatal("the file was not moved by ⌘⌥V")
	}
}

func TestCopySameDirSuffixNoModal(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "photo.png")
	mkfile(t, src, "x")

	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("c", true, false, false)) // Ctrl+C
	f.HandleKey(key("v", true, false, false)) // Ctrl+V, same dir
	if f.DialogActive() {
		t.Fatal("a same-dir copy shows no modal")
	}
	if !exists(filepath.Join(root, "photo copie.png")) {
		t.Fatal("same-dir copy should create a suffixed duplicate")
	}
}

func TestPasteOverwriteCancelThenConfirm(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "dup.txt")
	mkfile(t, src, "NEW")
	mkfile(t, filepath.Join(sub, "dup.txt"), "OLD")

	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("c", false, true, false))
	f.Navigate(sub)
	f.HandleKey(key("v", false, true, false))
	if !f.DialogActive() {
		t.Fatal("an overwrite must confirm")
	}
	// Cancel leaves the existing file untouched.
	f.cancelPaste()
	if got := readBody(t, filepath.Join(sub, "dup.txt")); got != "OLD" {
		t.Fatalf("cancel replaced the file: %q", got)
	}
	// Paste again and confirm → the existing file is replaced.
	f.Navigate(sub)
	f.HandleKey(key("v", false, true, false))
	f.confirmPendingPaste()
	if got := readBody(t, filepath.Join(sub, "dup.txt")); got != "NEW" {
		t.Fatalf("confirm did not overwrite: %q", got)
	}
}

func TestPasteEmptyClipboardIsNoop(t *testing.T) {
	root := t.TempDir()
	f := kbFinder(t, root)
	if f.HandleKey(key("v", false, true, false)); f.DialogActive() {
		t.Fatal("pasting an empty clipboard must do nothing")
	}
}

func TestCopyWithoutSelectionIsNoop(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "a.txt"), "x")
	f := kbFinder(t, root)
	f.iconView.clearSelection()
	f.HandleKey(key("c", false, true, false))
	if !f.Clipboard().Empty() {
		t.Fatal("⌘C with no selection should not fill the clipboard")
	}
}

func TestPasteIntoNetworkEmptyStateIsNoop(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "s.txt")
	mkfile(t, src, "x")
	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("c", false, true, false))
	f.showNetwork() // cwd becomes "" (no directory)
	f.HandleKey(key("v", false, true, false))
	if f.DialogActive() {
		t.Fatal("pasting with no current directory must do nothing")
	}
}

func TestPasteErrorShowsDialogNoCrash(t *testing.T) {
	if os.Geteuid() <= 0 {
		t.Skip("permission-denial branch needs a non-root POSIX host")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "f.txt")
	mkfile(t, src, "x")

	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("c", false, true, false))
	f.Navigate(sub)
	if err := os.Chmod(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })
	f.HandleKey(key("v", false, true, false)) // plain copy into a read-only dir
	if !f.DialogActive() {
		t.Fatal("a failed paste should surface a dismissable error dialog")
	}
	f.closeDialog()
}

func TestHandleKeyWhileDialogUpRoutesToOverlay(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "m.txt")
	mkfile(t, src, "x")
	f := kbFinder(t, root)
	f.SelectByPath(src)
	f.HandleKey(key("x", false, true, false))
	f.Navigate(sub)
	f.HandleKey(key("v", false, true, false)) // opens the confirm dialog
	if !f.DialogActive() {
		t.Fatal("expected a dialog")
	}
	// A further key with a dialog up is consumed by the overlay, not a shortcut.
	if !f.HandleKey(key("c", false, true, false)) {
		t.Fatal("HandleKey with a dialog up should report consumed")
	}
	f.cancelPaste()
}

func TestCutDimsIcon(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "d.txt")
	mkfile(t, src, "x")
	f := kbFinder(t, root)

	plain, _ := f.cellImage(f.fileModel.At(0))
	f.SelectByPath(src)
	f.HandleKey(key("x", false, true, false)) // cut
	dimmed, _ := f.cellImage(f.fileModel.At(0))
	if plain == nil || dimmed == nil {
		t.Fatal("expected icons")
	}
	// The dimmed icon is a distinct, more-transparent buffer.
	if maxAlpha(dimmed) >= maxAlpha(plain) {
		t.Fatalf("cut icon not dimmed: dimmed maxA=%d plain maxA=%d", maxAlpha(dimmed), maxAlpha(plain))
	}
}

func TestDimImageEdgeCases(t *testing.T) {
	if got := dimImage(nil); got != nil {
		t.Fatal("dimImage(nil) should be nil")
	}
	empty := toolkit.NewImage(nil, 0, 0)
	if got := dimImage(empty); got != empty {
		t.Fatal("dimImage of an empty image returns it unchanged")
	}
}

func TestSelectedItemInListView(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "l.txt"), "x")
	f := kbFinder(t, root)
	f.SetView(ViewList)
	f.SelectByPath(filepath.Join(root, "l.txt"))
	it, ok := f.selectedItem()
	if !ok || it.Name != "l.txt" {
		t.Fatalf("list selection = %+v ok=%v", it, ok)
	}
}

func TestSelectedItemColumnViewNone(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "c.txt"), "x")
	f := kbFinder(t, root)
	f.SetView(ViewColumns)
	if _, ok := f.selectedItem(); ok {
		t.Fatal("column view has no single-selection here")
	}
	// A shortcut in column view is a harmless no-op.
	f.HandleKey(key("c", false, true, false))
	if !f.Clipboard().Empty() {
		t.Fatal("no selection → empty clipboard")
	}
}

func readBody(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// maxAlpha returns the largest alpha byte in the image's pixels (0 for none),
// so a test can assert a dimmed icon is strictly more transparent.
func maxAlpha(img *toolkit.Image) int {
	m := 0
	for i := 3; i < len(img.Pixels); i += 4 {
		if int(img.Pixels[i]) > m {
			m = int(img.Pixels[i])
		}
	}
	return m
}
