// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

package main

import (
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/desktop/source"
)

// nativeSource builds the Windows app source: it recursively scans the real
// Start Menu program trees (machine-wide + per-user) for .lnk shortcuts. This
// is the Win32 peer of the darwin /Applications scan — same AppSource seam, real
// Windows apps — so `desktop` with no flags opens a real Win32 window
// (window.Open) populated from the actual Start Menu.
func nativeSource(o options) shell.AppSource {
	return source.NewWindows(source.WindowsOptions{
		Dir:      o.dir,
		IconSize: render.DefaultIconSize,
	})
}
