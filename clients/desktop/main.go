// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Command desktop (js/wasm) is the go-widgets desktop shell running as a
// wasmdesk external client. It composes the exact same shell/render tree the
// native binary does, but from the embedded (embed.FS) app source instead of a
// real XDG filesystem — the browser has no filesystem — and presents it to the
// wasmbox compositor through the wasmhost backend.
//
// This is the browser arm of the "same source, two targets" design: swap
// source.NewEmbedded for source.NewXDG and wasmhost.Run for window.Open and the
// identical Scene runs on a real Linux desktop. The worker.js beside this file
// boots the wasm and wires the wasmbox client SDK.
//
//go:build js && wasm

package main

import (
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/source"
	"github.com/go-widgets/desktop/wasmhost"
	"github.com/go-widgets/toolkit"
)

func main() {
	// The curated, filesystem-free data source: a populated dock, launcher,
	// application menu and file grid, all served from the binary's embed.FS.
	src := source.NewEmbedded()

	// The identical composition the native shell uses. A light theme blends
	// with the wasmdesk (Adwaita/WhiteSur) desktop.
	sc := render.New(render.Config{
		Source: src,
		Theme:  toolkit.DefaultLight(),
	})

	// Drive the shell's incremental-present root through the wasmbox backend.
	// wasmhost.Run blocks (hands control to the browser event loop) on success;
	// it only returns on a setup error.
	if err := wasmhost.Run(sc.HostRoot(), sc.Theme()); err != nil {
		println("desktop: " + err.Error())
	}
}
