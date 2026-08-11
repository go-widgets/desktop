// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin && integration

// On-device macOS proof for the Finder's KEYBOARD file operations (⌘C / ⌘X /
// ⌘V and ⌘⌥V "Move Item Here"). It drives the EXACT damage-aware root the
// Cocoa/AppKit backend runs (scene.HostRoot — what `desktop` hands to
// window.Backend.Run on darwin) with synthetic key events carrying the real
// modifier flags the window v0.12.0 Cocoa backend now decodes (⌘ ⇒ Meta+Ctrl,
// ⌥ ⇒ Alt), over a REAL throwaway temp directory, and asserts the files are
// actually copied / moved ON DISK. It then renders the paste-confirmation frame
// with the real Inter UI font and saves the dated PNG under docs/.
//
// Gated behind `integration` + DESKTOP_COCOA_KEYS=1 so it never runs in the
// ordinary unit gate. Run on this Mac with:
//
//	DESKTOP_COCOA_KEYS=1 CGO_ENABLED=0 go test -tags=integration \
//	  -run TestCocoaKeyboardFileOps -v ./cmd/desktop/
package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

const keysW, keysH = 1280, 800

// keyEvent builds the toolkit.Event the Cocoa backend emits for a modified key:
// ⌘ sets Meta AND Ctrl (the backend folds Command into Ctrl for platform-neutral
// shortcuts while also reporting the real ⌘ as Meta); ⌥ sets Alt.
func cmdKey(code string, opt bool) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code, Meta: true, Ctrl: true, Alt: opt}
}

func onDisk(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

func TestCocoaKeyboardFileOps(t *testing.T) {
	if os.Getenv("DESKTOP_COCOA_KEYS") == "" {
		t.Skip("set DESKTOP_COCOA_KEYS=1 to run the on-device keyboard file-ops proof")
	}
	render.UseUIFont() // the real Inter proportional face, as the live window uses

	// A real throwaway working directory — NOT the user's files.
	root := t.TempDir()
	docs := filepath.Join(root, "Documents")
	pics := filepath.Join(root, "Images")
	for _, d := range []string{docs, pics} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	report := filepath.Join(docs, "rapport.txt")
	if err := os.WriteFile(report, []byte("chiffres 2026"), 0o644); err != nil {
		t.Fatal(err)
	}

	init, err := render.NativeLister(docs)
	if err != nil {
		t.Fatal(err)
	}
	sc := render.New(render.Config{
		Dir:    init,
		Lister: render.NativeLister,
		Theme:  toolkit.DefaultLight(),
		Width:  keysW,
		Height: keysH,
	})
	rootW := sc.HostRoot()
	full := toolkit.Rect{X: 0, Y: 0, W: keysW, H: keysH}
	rootW.SetBounds(full)
	fp := sc.Finder()

	// --- 1) ⌘C then navigate + ⌘V: the file is COPIED on disk ---------------
	fp.SelectByPath(report)
	rootW.OnEvent(cmdKey("c", false)) // ⌘C, dispatched through the real root
	fp.Navigate(pics)
	rootW.OnEvent(cmdKey("v", false)) // ⌘V
	copied := filepath.Join(pics, "rapport.txt")
	if !onDisk(t, copied) {
		t.Fatal("⌘C/⌘V did not copy the file into Images/")
	}
	if !onDisk(t, report) {
		t.Fatal("a copy must leave the original in Documents/")
	}
	t.Logf("copy OK: %s -> %s (original kept)", report, copied)

	// --- 2) ⌘X then navigate + ⌘V: the file is MOVED on disk (with confirm) --
	moveSrc := filepath.Join(pics, "rapport.txt")
	fp.SelectByPath(moveSrc)
	rootW.OnEvent(cmdKey("x", false)) // ⌘X — cut (drawn dimmed)
	// Capture the CUT state: viewing Images, the cut rapport.txt shows dimmed.
	saveKeysFrameAs(t, rootW, sc.Theme(), "DESKTOP_COCOA_CUT_PNG", "native-macos-cut-dimmed-2026-08-11.png")
	fp.Navigate(docs)                 // Documents already holds rapport.txt → overwrite
	rootW.OnEvent(cmdKey("v", false)) // ⌘V → confirm "Remplacer"
	if !fp.DialogActive() {
		t.Fatal("a cut-paste over an existing name must confirm")
	}

	// Capture THIS confirmation frame with the real Inter font, under docs/.
	saveKeysFrame(t, rootW, sc.Theme())

	fp.ConfirmDialog()
	if !onDisk(t, filepath.Join(docs, "rapport.txt")) {
		t.Fatal("the moved file is not in Documents/")
	}
	if onDisk(t, moveSrc) {
		t.Fatal("the cut source should be gone after the move")
	}
	t.Logf("move-by-cut OK: gone from Images/, present in Documents/")

	// --- 3) ⌘⌥V paste-as-move: a COPY on the clipboard, moved by the chord ---
	fp.Navigate(docs)
	fp.SelectByPath(filepath.Join(docs, "rapport.txt"))
	rootW.OnEvent(cmdKey("c", false)) // ⌘C — clipboard op is COPY
	fp.Navigate(pics)
	rootW.OnEvent(cmdKey("v", true)) // ⌘⌥V — move regardless
	if !fp.DialogActive() {
		t.Fatal("⌘⌥V must confirm the move")
	}
	fp.ConfirmDialog()
	if !onDisk(t, filepath.Join(pics, "rapport.txt")) {
		t.Fatal("⌘⌥V did not move the file into Images/")
	}
	if onDisk(t, filepath.Join(docs, "rapport.txt")) {
		t.Fatal("⌘⌥V should MOVE, leaving no original in Documents/")
	}
	t.Logf("paste-as-move (⌘⌥V) OK")
}

type damageRoot interface {
	RenderDamaged(painter.Painter, *toolkit.Theme) []toolkit.Rect
}

// saveKeysFrame renders the current shell frame (dialog included) with the real
// font and writes the dated on-device artifact under docs/.
func saveKeysFrame(t *testing.T, rootW damageRoot, th *toolkit.Theme) {
	saveKeysFrameAs(t, rootW, th, "DESKTOP_COCOA_KEYS_PNG", "native-macos-keyboard-fileops-2026-08-11.png")
}

// saveKeysFrameAs renders the current frame and writes it to the path in env
// (or fallback), so a test can capture several dated frames.
func saveKeysFrameAs(t *testing.T, rootW damageRoot, th *toolkit.Theme, env, fallback string) {
	t.Helper()
	buf := make([]byte, 4*keysW*keysH)
	p := painter.NewPixelPainter(buf, keysW, keysH)
	p.FillRect(toolkit.Rect{W: keysW, H: keysH}, th.Background)
	rootW.RenderDamaged(p, th)

	out := os.Getenv(env)
	if out == "" {
		out = fallback
	}
	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	img := &image.RGBA{Pix: buf, Stride: keysW * 4, Rect: image.Rect(0, 0, keysW, keysH)}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote paste-confirm capture %s", out)
}
