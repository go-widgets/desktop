// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin && integration

// This is the on-device, end-to-end drag-and-drop proof for the desktop shell,
// running against github.com/go-widgets/window v0.11.0 — the release whose
// shared, backend-agnostic DnD state machine makes a real pointer drag in a
// native window synthesise the toolkit DnD lifecycle (EventDragStart/DragMove/
// Drop) the shell already listens for. window's own on-device Cocoa test proves
// that raw NSEvents through the real view handlers produce that lifecycle and
// deliver EventDrop with the payload; this test proves the OTHER half: that the
// shell, driven by exactly that lifecycle, moves files and reorders favourites.
//
// It (1) opens a REAL Cocoa NSWindow for the shell via window.Open on this Mac
// (proving the pure-Go Cocoa backend brings the shell up on-device), then, on a
// throwaway temp directory — never real user files — (2) selects a file and
// drops it onto a folder through the shell's real DragSource/DropTarget widgets,
// asserts the « Déplacer … ? » confirmation dialog appears, captures that
// drag→dialog frame as a dated PNG, clicks the real « Déplacer » button and
// asserts the file ACTUALLY MOVED on disk, and (3) drag-reorders the Favoris and
// asserts the order changed.
//
// The Cocoa window work runs on the process main OS thread (TestMain reserves it
// and callOnMain funnels window calls there), as NSApplication/NSWindow require.
package render

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/window"
)

// --- main-thread plumbing (AppKit requires the main OS thread) --------------

var mainfuncs = make(chan func())

func TestMain(m *testing.M) {
	runtime.LockOSThread()
	go func() { os.Exit(m.Run()) }()
	for f := range mainfuncs {
		f()
	}
}

func callOnMain(f func()) {
	done := make(chan struct{})
	mainfuncs <- func() { f(); close(done) }
	<-done
}

// --- capture helper ---------------------------------------------------------

// snapshot renders the shell root into an RGBA buffer and returns its PNG.
func snapshot(t *testing.T, root toolkit.Widget, th *toolkit.Theme, w, h int) []byte {
	t.Helper()
	buf := make([]byte, 4*w*h)
	root.Draw(painter.NewPixelPainter(buf, w, h), th)
	img := &image.RGBA{Pix: buf, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return b.Bytes()
}

// --- the proof --------------------------------------------------------------

func TestLiveShellDragAndDrop(t *testing.T) {
	if os.Getenv("DESKTOP_COCOA_INTEGRATION") == "" {
		t.Skip("set DESKTOP_COCOA_INTEGRATION=1 to run the on-device shell DnD proof")
	}
	th := toolkit.DefaultDark()
	const W, H = 900, 600

	// A throwaway working directory: a file to move, a folder to move it into,
	// and two favourites to reorder. NEVER real user files.
	work := t.TempDir()
	album := filepath.Join(work, "Album")
	mustMkdir(t, album)
	favA := filepath.Join(work, "Alpha")
	favB := filepath.Join(work, "Bravo")
	mustMkdir(t, favA)
	mustMkdir(t, favB)
	photo := filepath.Join(work, "photo.txt")
	if err := os.WriteFile(photo, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	init, err := NativeLister(work)
	if err != nil {
		t.Fatal(err)
	}
	f := NewFinderPane(FinderConfig{
		Theme:      th,
		Icons:      NewIconLoader("", 32),
		Places:     &shell.Places{Favorites: []shell.Place{{Label: "Alpha", Path: favA, Kind: shell.PlaceFolder}, {Label: "Bravo", Path: favB, Kind: shell.PlaceFolder}}},
		Lister:     NativeLister,
		InitialDir: init,
	})
	f.Root().SetBounds(toolkit.Rect{X: 0, Y: 0, W: W, H: H})
	// Seed a frame so the icon grid syncs its cells and every widget is laid out.
	_ = snapshot(t, f.Root(), th, W, H)

	// (1) Open a REAL Cocoa NSWindow for the shell on this Mac.
	callOnMain(func() {
		be, err := window.Open(window.Config{
			Title:  "go-widgets desktop shell — DnD proof",
			Width:  W,
			Height: H,
			Theme:  th,
		})
		if err != nil {
			t.Errorf("window.Open (real NSWindow) failed: %v", err)
			return
		}
		gw, gh := be.Size()
		if gw <= 0 || gh <= 0 {
			t.Errorf("real window size = %dx%d", gw, gh)
		}
		t.Logf("opened real Cocoa NSWindow for the shell: %s (%dx%d)", be.String(), gw, gh)
		_ = be.Close()
	})

	// (2) Drag a file onto a folder → confirmation dialog → confirmed move.
	folderIdx, fileIdx := -1, -1
	for i := 0; i < f.iconView.model.Len(); i++ {
		it := f.iconView.model.At(i)
		switch {
		case it.IsDir && it.Path == album:
			folderIdx = i
		case it.Path == photo:
			fileIdx = i
		}
	}
	if folderIdx < 0 || fileIdx < 0 {
		t.Fatalf("model missing folder/file: folderIdx=%d fileIdx=%d (len=%d)", folderIdx, fileIdx, f.iconView.model.Len())
	}
	fx, fy := cellPoint(t, f.iconView, fileIdx)
	dx, dy := cellPoint(t, f.iconView, folderIdx)

	// Select the file: the press-click is what makes DragData meaningful — the
	// icon grid selects the cell and DragData then returns its path (exactly the
	// window controller's press→read-payload ordering).
	f.iconView.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: fx, Y: fy})
	payload := f.iconView.DragData()
	if payload != photo {
		t.Fatalf("DragData after selecting the file = %q, want %q", payload, photo)
	}

	// Drive the DnD lifecycle onto the folder, as the window controller would.
	f.iconView.OnEvent(toolkit.Event{Kind: toolkit.EventDragStart, X: dx, Y: dy, Code: payload})
	f.iconView.OnEvent(toolkit.Event{Kind: toolkit.EventDragMove, X: dx, Y: dy, Code: payload})
	f.iconView.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: dx, Y: dy, Code: payload})

	if !f.DialogActive() {
		t.Fatalf("dropping the file onto the folder did not raise the « Déplacer » dialog")
	}
	// Capture the drag→dialog frame (the confirmation over the shell).
	writeArtifact(t, "desktop-dnd-move-dialog-2026-08-11.png", snapshot(t, f.Root(), th, W, H))

	// Click the real « Déplacer » confirm button through the overlay's routing.
	confirm := dialogButton(t, f, "Déplacer")
	cc := confirm.Bounds()
	f.Root().OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: cc.X + cc.W/2, Y: cc.Y + cc.H/2})

	if f.DialogActive() {
		t.Fatalf("dialog still up after clicking « Déplacer »")
	}
	if _, err := os.Stat(photo); !os.IsNotExist(err) {
		t.Fatalf("source file still present after the move: %v", err)
	}
	moved := filepath.Join(album, "photo.txt")
	if _, err := os.Stat(moved); err != nil {
		t.Fatalf("file was not moved into the folder: %v", err)
	}
	t.Logf("file moved on disk: %s → %s", photo, moved)

	// (3) Drag-reorder the Favoris: move row 0 (Alpha) below row 1 (Bravo).
	before := []string{f.sidebar.favorites[0].Label, f.sidebar.favorites[1].Label}
	sbX := f.sidebar.Bounds().X + 20
	rowY := func(row int) int {
		return f.sidebar.Bounds().Y + sbTopPad + sbHeaderH + row*sbRowH + sbRowH/2
	}
	f.sidebar.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: sbX, Y: rowY(0)})
	token := f.sidebar.DragData()
	if !strings.HasPrefix(token, toolkit.SourceRowDragPrefix) {
		t.Fatalf("Favoris drag payload = %q, want a %s… reorder token", token, toolkit.SourceRowDragPrefix)
	}
	f.sidebar.OnEvent(toolkit.Event{Kind: toolkit.EventDragStart, X: sbX, Y: rowY(1), Code: token})
	f.sidebar.OnEvent(toolkit.Event{Kind: toolkit.EventDragMove, X: sbX, Y: rowY(1), Code: token})
	f.sidebar.OnEvent(toolkit.Event{Kind: toolkit.EventDrop, X: sbX, Y: rowY(1), Code: token})

	after := []string{f.sidebar.favorites[0].Label, f.sidebar.favorites[1].Label}
	if after[0] != "Bravo" || after[1] != "Alpha" {
		t.Fatalf("Favoris reorder: before %v, after %v, want [Bravo Alpha]", before, after)
	}
	t.Logf("Favoris reordered by drag: %v → %v", before, after)
}

// --- helpers ----------------------------------------------------------------

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// cellPoint returns a widget-local point that the icon view's grid maps to the
// given cell index, found by probing IndexAt (robust to the grid's private cell
// metrics).
func cellPoint(t *testing.T, v *iconView, idx int) (int, int) {
	t.Helper()
	b := v.grid.Bounds()
	for y := 0; y < b.H; y += 3 {
		for x := 0; x < b.W; x += 3 {
			if v.grid.IndexAt(x, y) == idx {
				return x, y
			}
		}
	}
	t.Fatalf("no point maps to cell %d (grid bounds %v)", idx, b)
	return 0, 0
}

// dialogButton returns the modal dialog's button with the given label.
func dialogButton(t *testing.T, f *FinderPane, label string) *toolkit.Button {
	t.Helper()
	if f.dialog == nil {
		t.Fatal("no dialog is up")
	}
	for _, b := range f.dialog.Buttons {
		if b.Label == label {
			return b
		}
	}
	t.Fatalf("dialog has no %q button", label)
	return nil
}

// writeArtifact saves a capture, logging (not failing) on write error.
func writeArtifact(t *testing.T, name string, data []byte) {
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Logf("could not save %s: %v", name, err)
		return
	}
	t.Logf("saved %s (%d bytes)", name, len(data))
}
