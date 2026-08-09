// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build integration && linux

// This is the live windowed proof for the desktop shell. It builds the desktop
// binary and runs it exactly as a user would with no flags — the default path
// opens a REAL X11 window via github.com/go-widgets/window and drives the shell
// through its Run loop — then captures the X server's framebuffer with
// ImageMagick's `import` and asserts, from sampled pixels, that:
//
//   - the shell rendered: the dock (south) and launcher (west) regions carry
//     widget content over the theme background, and
//   - a notification produced a visible Toast: the with-notify capture differs
//     from the no-notify capture in the top-right toast anchor region.
//
// The binary is spawned as a subprocess (rather than opening the window
// in-process) so the fixture's XDG_DATA_HOME/DIRS are read at the child's
// startup — the faithful `desktop`-with-no-flags path — and so the whole
// launch→map→present→toast pipeline is exercised end to end.
//
// It is gated behind the `integration` build tag AND DESKTOP_X11_INTEGRATION=1
// so it never runs in the ordinary unit gate; the CI "live shell (windowed)"
// lane runs it under `xvfb-run` and `dbus-run-session` (the notifications
// daemon needs a session bus). It matches the live-only precedent of
// go-widgets/window's TestLiveX11: covered by the live run, not the unit gate.
package main

import (
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-widgets/toolkit"
)

const liveW, liveH = 900, 600

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("required tool %q not found: %v", name, err)
	}
}

// liveFixture writes a handful of launchable apps and a directory of files, and
// returns the XDG data root (its applications/ subdir feeds the dock+launcher)
// and the files directory (feeds the file grid).
func liveFixture(t *testing.T) (root, files string) {
	t.Helper()
	root = t.TempDir()
	appsDir := filepath.Join(root, "applications")
	files = filepath.Join(root, "files")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(files, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	apps := []struct{ id, name, icon, cats string }{
		{"org.x.Editor", "Text Editor", "accessories-text-editor", "Utility;TextEditor;"},
		{"org.x.Browser", "Web Browser", "web-browser", "Network;WebBrowser;"},
		{"org.x.Files", "File Manager", "system-file-manager", "System;FileManager;"},
		{"org.x.Term", "Terminal", "utilities-terminal", "System;TerminalEmulator;"},
		{"org.x.Calc", "Calculator", "accessories-calculator", "Utility;Calculator;"},
		{"org.x.Image", "Image Viewer", "image-viewer", "Graphics;Viewer;"},
	}
	for _, a := range apps {
		body := "[Desktop Entry]\nType=Application\nName=" + a.name +
			"\nExec=false %f\nIcon=" + a.icon + "\nCategories=" + a.cats + "\n"
		if err := os.WriteFile(filepath.Join(appsDir, a.id+".desktop"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"alpha.txt", "beta.md", "gamma.log", "delta.conf"} {
		if err := os.WriteFile(filepath.Join(files, f), []byte("content of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, files
}

// buildBinary compiles the desktop binary from the package under test.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "desktop")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, out)
	}
	return bin
}

// captureWindow runs the desktop binary (opening a real window), lets it map +
// present, captures the root drawable to pngPath, then terminates it. extra
// carries optional flags such as -notify.
func captureWindow(t *testing.T, bin, root, files, pngPath string, extra ...string) {
	t.Helper()
	args := append([]string{"-dir", files, "-w", strconv.Itoa(liveW), "-h", strconv.Itoa(liveH)}, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+root,
		"XDG_DATA_DIRS="+root+":/usr/share",
	)
	logf, _ := os.CreateTemp(t.TempDir(), "desktop-*.log")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start desktop: %v", err)
	}
	time.Sleep(3 * time.Second) // launch + map + initial draw/present

	capCmd := exec.Command("import", "-display", os.Getenv("DISPLAY"), "-window", "root", pngPath)
	if out, err := capCmd.CombinedOutput(); err != nil {
		t.Fatalf("import: %v: %s", err, out)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	if data, err := os.ReadFile(logf.Name()); err == nil && len(data) > 0 {
		t.Logf("desktop output: %s", data)
	}
}

func decode(t *testing.T, path string) *image.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

type rgnRect struct{ x0, y0, x1, y1 int }

// dominant returns the most common quantised RGB in r — the region's background.
func dominant(img *image.RGBA, r rgnRect) (rr, gg, bb uint8) {
	counts := map[[3]uint8]int{}
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			c := img.RGBAAt(x, y)
			counts[[3]uint8{c.R & 0xF8, c.G & 0xF8, c.B & 0xF8}]++
		}
	}
	best, bn := [3]uint8{}, -1
	for k, n := range counts {
		if n > bn {
			bn, best = n, k
		}
	}
	return best[0], best[1], best[2]
}

func absi(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// nonBg counts pixels in r that differ from the background by > thr.
func nonBg(img *image.RGBA, r rgnRect, br, bg, bb uint8, thr int) int {
	n := 0
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			c := img.RGBAAt(x, y)
			if absi(int(c.R&0xF8)-int(br)) > thr || absi(int(c.G&0xF8)-int(bg)) > thr || absi(int(c.B&0xF8)-int(bb)) > thr {
				n++
			}
		}
	}
	return n
}

// diffCount counts pixels differing between a and b within r by > thr.
func diffCount(a, b *image.RGBA, r rgnRect, thr int) int {
	n := 0
	for y := r.y0; y < r.y1; y++ {
		for x := r.x0; x < r.x1; x++ {
			ca, cb := a.RGBAAt(x, y), b.RGBAAt(x, y)
			if absi(int(ca.R)-int(cb.R)) > thr || absi(int(ca.G)-int(cb.G)) > thr || absi(int(ca.B)-int(cb.B)) > thr {
				n++
			}
		}
	}
	return n
}

// TestLiveWindowedX11 is the real-window proof described in the file comment.
func TestLiveWindowedX11(t *testing.T) {
	if os.Getenv("DESKTOP_X11_INTEGRATION") == "" {
		t.Skip("set DESKTOP_X11_INTEGRATION=1 (and run under xvfb + dbus-run-session) to run the live windowed proof")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Fatal("DISPLAY is not set (run under xvfb-run)")
	}
	requireTool(t, "import")

	bin := buildBinary(t)
	root, files := liveFixture(t)
	dir := t.TempDir()
	basePNG := filepath.Join(dir, "base.png")
	toastPNG := filepath.Join(dir, "toast.png")

	captureWindow(t, bin, root, files, basePNG)
	captureWindow(t, bin, root, files, toastPNG, "-notify", "Build finished|All checks green")

	// Keep the with-notify capture as a build artifact.
	if data, err := os.ReadFile(toastPNG); err == nil {
		_ = os.WriteFile("live-shell-windowed.png", data, 0o644)
		t.Logf("saved capture to live-shell-windowed.png")
	}

	base := decode(t, basePNG)
	toast := decode(t, toastPNG)

	dockH := 48 + 12 // render.DefaultIconSize + 12 (the south band height)
	dock := rgnRect{4, liveH - dockH, 300, liveH - 2}
	launcher := rgnRect{2, int(toolkit.MenuBarH) + 2, 218, liveH - dockH - 2}
	toastR := rgnRect{liveW - 280, 6, liveW - 4, 90}

	// Shell rendered: dock + launcher carry widget content over the background.
	for _, tc := range []struct {
		name string
		r    rgnRect
		min  int
	}{
		{"dock", dock, 200},
		{"launcher", launcher, 150},
	} {
		br, bg, bb := dominant(base, tc.r)
		got := nonBg(base, tc.r, br, bg, bb, 24)
		t.Logf("region %-8s bg=%d,%d,%d non-bg=%d", tc.name, br, bg, bb, got)
		if got < tc.min {
			t.Errorf("%s region has too little widget content: %d (want >=%d)", tc.name, got, tc.min)
		}
	}

	// Notification produced a visible Toast in the window.
	d := diffCount(base, toast, toastR, 24)
	t.Logf("toast diff (with-notify vs no-notify) = %d px", d)
	if d < 300 {
		t.Errorf("no visible Toast appeared in the top-right (diff %d too small)", d)
	}
}
