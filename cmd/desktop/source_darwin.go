// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package main

import (
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/desktop/source"
)

// nativeSource builds the macOS app source: it scans the real /Applications
// tree (and the system + user application directories) for .app bundles. This
// is the darwin peer of the XDG scan — same AppSource seam, real macOS apps.
func nativeSource(o options) shell.AppSource {
	return source.NewDarwin(source.DarwinOptions{
		Dir:      o.dir,
		IconSize: render.DefaultIconSize,
	})
}
