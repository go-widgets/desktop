// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/window"
)

// fakeBackend is a window.Backend stand-in whose Run returns immediately, so
// the no-flags windowed path can be exercised in a unit test without opening a
// real (blocking) window. onRun records that Run was reached.
type fakeBackend struct {
	onRun  func(root toolkit.Widget)
	runErr error
}

func (f *fakeBackend) Run(root toolkit.Widget) error {
	if f.onRun != nil {
		f.onRun(root)
	}
	return f.runErr
}
func (f *fakeBackend) Close() error     { return nil }
func (f *fakeBackend) Size() (int, int) { return 0, 0 }
func (f *fakeBackend) String() string   { return "fakeBackend" }

// withDisplay makes displayAvailable() report true on non-darwin (darwin is
// always true) so the windowed path is reached.
func withDisplay(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Setenv("DISPLAY", ":0")
	}
}

// writeApp drops a minimal .desktop file into an applications dir.
func writeApp(t *testing.T, appsDir, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(appsDir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCapture(t *testing.T) {
	sb := t.TempDir()
	xdg := filepath.Join(sb, "xdg")
	appsDir := filepath.Join(xdg, "applications")
	files := filepath.Join(sb, "files")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(files, 0o755); err != nil {
		t.Fatal(err)
	}
	writeApp(t, appsDir, "org.x.Editor.desktop",
		"[Desktop Entry]\nType=Application\nName=Editor\nExec=false %f\nIcon=text-editor\n")
	writeApp(t, appsDir, "org.x.Browser.desktop",
		"[Desktop Entry]\nType=Application\nName=Browser\nExec=false %u\nIcon=web-browser\n")
	if err := os.WriteFile(filepath.Join(files, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("XDG_DATA_DIRS", xdg)

	out := filepath.Join(sb, "shell.png")
	var errb bytes.Buffer
	code := run([]string{"-dir", files, "-w", "400", "-h", "300", "-capture", out}, &errb)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s", code, errb.String())
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("capture PNG missing/empty: %v", err)
	}
}

func TestRunLaunchUnknownID(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"-launch", "no.such.App.definitely"}, &errb); code != 1 {
		t.Errorf("launch unknown exit = %d, want 1", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"-nope"}, &errb); code != 2 {
		t.Errorf("bad flag exit = %d, want 2", code)
	}
}

// TestRunNoAction drives the default (no-flags) path — which opens a real
// window and runs the shell interactively — through a fake backend so the unit
// test does not open (and block on) a real window. It asserts the shell's
// damage-aware root (scene.HostRoot) is handed to the backend's Run: the one
// backend-agnostic path shared by X11/Wayland/Cocoa/wasmbox.
func TestRunNoAction(t *testing.T) {
	withDisplay(t)
	orig := openWindow
	defer func() { openWindow = orig }()
	var gotRoot toolkit.Widget
	openWindow = func(cfg window.Config) (window.Backend, error) {
		return &fakeBackend{onRun: func(root toolkit.Widget) { gotRoot = root }}, nil
	}
	var errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, &errb); code != 0 {
		t.Errorf("no-action exit = %d, want 0; stderr=%s", code, errb.String())
	}
	if gotRoot == nil {
		t.Fatal("windowed path did not Run a root widget")
	}
	// The root must be the scene's damage-aware host root (incremental present).
	if _, ok := gotRoot.(window.DamageRenderer); !ok {
		t.Errorf("root %T does not implement window.DamageRenderer (no incremental present)", gotRoot)
	}
}

// TestRunWindowError propagates a backend open error as a non-zero exit.
func TestRunWindowError(t *testing.T) {
	withDisplay(t)
	orig := openWindow
	defer func() { openWindow = orig }()
	openWindow = func(cfg window.Config) (window.Backend, error) {
		return nil, fmt.Errorf("dial failed")
	}
	var errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, &errb); code != 1 {
		t.Errorf("window-error exit = %d, want 1", code)
	}
}

// TestRunWindowUnsupported falls back to the composed-shell report (exit 0)
// when the backend reports no windowing support.
func TestRunWindowUnsupported(t *testing.T) {
	withDisplay(t)
	orig := openWindow
	defer func() { openWindow = orig }()
	openWindow = func(cfg window.Config) (window.Backend, error) {
		return nil, window.ErrUnsupported
	}
	var errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, &errb); code != 0 {
		t.Errorf("unsupported exit = %d, want 0", code)
	}
}

// TestRunNoActionHeadless exercises the headless fallback (no display named,
// non-darwin): it reports the composed shell and returns 0 without opening a
// window. On darwin displayAvailable is always true, so this path is covered by
// the non-darwin unit lane.
func TestRunNoActionHeadless(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin always has a native windowing backend")
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	orig := openWindow
	defer func() { openWindow = orig }()
	openWindow = func(cfg window.Config) (window.Backend, error) {
		t.Fatal("headless path must not open a window")
		return nil, errors.New("unreachable")
	}
	var errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, &errb); code != 0 {
		t.Errorf("headless exit = %d, want 0", code)
	}
}

// TestDisplayAvailable checks the darwin-always-true rule and the env gate.
func TestDisplayAvailable(t *testing.T) {
	if runtime.GOOS == "darwin" {
		if !displayAvailable() {
			t.Fatal("darwin must always report a native windowing backend")
		}
		return
	}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	if displayAvailable() {
		t.Fatal("no display named: want unavailable")
	}
	t.Setenv("DISPLAY", ":0")
	if !displayAvailable() {
		t.Fatal("DISPLAY set: want available")
	}
}

func TestSplitNotify(t *testing.T) {
	if s, b := splitNotify("Summary|Body text"); s != "Summary" || b != "Body text" {
		t.Errorf("split = %q,%q", s, b)
	}
	if s, b := splitNotify("bare"); s != "bare" || b != "" {
		t.Errorf("split bare = %q,%q", s, b)
	}
}

func TestRunNotifyOffLinuxErrors(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("notify is supported on linux")
	}
	var errb bytes.Buffer
	if code := run([]string{"-notify", "Hi|there", "-dir", t.TempDir()}, &errb); code != 1 {
		t.Errorf("notify off-linux exit = %d, want 1", code)
	}
}
