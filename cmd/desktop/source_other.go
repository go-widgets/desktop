// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js && !darwin && !windows

package main

import (
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/desktop/source"
)

// nativeSource builds the native XDG app source on the non-darwin unixes: it
// scans a real XDG filesystem (desktopentry + icontheme + menu + mime).
func nativeSource(o options) shell.AppSource {
	return source.NewXDG(source.XDGOptions{
		Dir:       o.dir,
		IconTheme: o.iconTheme,
		IconSize:  render.DefaultIconSize,
	})
}
