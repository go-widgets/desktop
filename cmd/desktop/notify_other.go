// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !linux && !js

package main

import (
	"errors"

	"github.com/go-widgets/desktop/render"
)

// runNotify is unavailable off Linux: the notification daemon owns a D-Bus name
// on the session bus, which is a Linux desktop facility. The rest of the shell
// (dock, launcher, menu, file grid, capture) works on every platform.
func runNotify(_ *render.Scene, _ string) error {
	return errors.New("notifications require a Linux session bus (build/run on Linux, under dbus-run-session)")
}
