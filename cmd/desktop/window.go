// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/window"
)

// runWindow opens a real window on the running display server via the pure-Go
// github.com/go-widgets/window backend — which auto-selects the Wayland
// backend when $WAYLAND_DISPLAY is set (the modern default) and otherwise the
// X11 backend driven by $DISPLAY — and runs the shell's root widget live: the
// dock, launcher, application menu, file grid and the floating
// notification-toast overlay (see render.Scene.Widget), until the window is
// closed. Resize and close are handled by the backend's Run loop
// (ConfigureNotify → relayout, WM_DELETE_WINDOW / xdg close → quit).
//
// It is the interactive analogue of the -capture screenshot path: the same
// backend-agnostic widget tree, presented on a live compositor instead of
// blitted to a PNG. This closes the "a live compositor backend would be a
// follow-up" gap the shell previously flagged.
//
// When no display server is reachable (headless CI, or a platform with no
// windowing backend) it does not fail: it reports the composed shell and
// returns nil, so `desktop` with no flags still succeeds everywhere and
// -capture remains the headless screenshot path.
func runWindow(o options, sc *render.Scene, errw io.Writer) error {
	if !displayAvailable() {
		reportComposed(errw, sc)
		return nil
	}
	be, err := window.Open(window.Config{
		Title:    "go-widgets desktop shell",
		Instance: "desktop",
		Class:    "go-widgets-desktop",
		Width:    o.width,
		Height:   o.height,
		Theme:    sc.Theme(),
	})
	if errors.Is(err, window.ErrUnsupported) {
		reportComposed(errw, sc)
		return nil
	}
	if err != nil {
		return err
	}
	defer be.Close()
	// Hand the backend the shell's damage-aware root (scene.HostRoot): window
	// type-asserts it for RenderDamaged and presents only the rectangles
	// the shell reports changed each frame, instead of blitting the whole
	// surface. It is pixel-identical to the full composite by construction (and
	// still full-repaints correctly on a damage-unaware host), so the live
	// window shows exactly what -capture writes — now incrementally.
	return be.Run(sc.HostRoot())
}

// displayAvailable reports whether a display server is named in the
// environment — Wayland preferred, else X11 — i.e. whether window.Open can dial
// one. Checking here keeps the headless fallback deterministic instead of
// depending on the exact dial error.
func displayAvailable() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != ""
}

// reportComposed prints the composed-shell summary used on headless hosts.
func reportComposed(errw io.Writer, sc *render.Scene) {
	fmt.Fprintf(errw, "desktop: composed shell — dock=%d apps=%d categories=%d files=%d (headless; use -capture for a PNG)\n",
		sc.DockCount(), sc.AppCount(), sc.MenuCategoryCount(), sc.FileCount())
}
