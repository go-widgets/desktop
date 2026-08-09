// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Command desktop (js/wasm) is the go-widgets desktop shell running as a
// wasmdesk external client. It composes the exact same shell/render tree the
// native binary does, but from the embedded (embed.FS) app source instead of a
// real XDG filesystem — the browser has no filesystem — and presents it to the
// wasmbox compositor.
//
// This is the browser arm of the "same source, two targets" design, and it runs
// through the SAME code path as the native binary: github.com/go-widgets/window's
// Open() auto-selects the backend for the environment — X11/Wayland on Linux,
// the wasmbox client backend on js/wasm — and Backend.Run drives the identical
// widget tree. There is no desktop-local windowing backend anymore: swap
// source.NewEmbedded for source.NewXDG and the identical Scene runs on a real
// Linux desktop. The worker.js beside this file boots the wasm; the wasmbox
// client wire (hello/welcome/commit/input over the compositor's MessagePort +
// SharedArrayBuffer) lives entirely inside window's backend.
//
//go:build js && wasm

package main

import (
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/source"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/window"
)

// The desktop-shell surface size requested from the compositor (the same
// dimensions the native binary renders at by default).
const surfaceW, surfaceH = 960, 600

func main() {
	// The curated, filesystem-free data source: a populated dock, launcher,
	// application menu and file grid, all served from the binary's embed.FS.
	src := source.NewEmbedded()

	// The identical composition the native shell uses. A light theme blends
	// with the wasmdesk (Adwaita/WhiteSur) desktop.
	theme := toolkit.DefaultLight()
	sc := render.New(render.Config{
		Source: src,
		Theme:  theme,
		Width:  surfaceW,
		Height: surfaceH,
	})

	// Open the backend for this environment. On js/wasm this is window's wasmbox
	// client backend (the browser IS the wasmdesk compositor); the same call on
	// Linux returns the X11/Wayland backend. One path, native and browser.
	be, err := window.Open(window.Config{
		Title:  "Desktop",
		Width:  surfaceW,
		Height: surfaceH,
		Theme:  theme,
	})
	if err != nil {
		println("desktop: " + err.Error())
		return
	}
	defer be.Close()

	// Drive the shell's incremental-present root (scene.HostRoot) through the
	// backend: window type-asserts DamageRenderer and commits only the damaged
	// rectangles each frame. Run blocks (hands control to the browser event
	// loop) until the surface closes.
	if err := be.Run(sc.HostRoot()); err != nil {
		println("desktop: " + err.Error())
	}
}
