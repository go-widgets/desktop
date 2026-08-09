// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !(js && wasm)

package wasmhost

import "github.com/go-widgets/toolkit"

// Run is unavailable off js/wasm: there is no wasmbox compositor to attach to,
// so it returns ErrUnsupported. The rendering core (Host) and the input mapping
// stay compiled and unit-tested on every platform; only this entry point is
// gated, mirroring window.Open's Linux-only gating so cross-builds stay green.
func Run(_ toolkit.Widget, _ *toolkit.Theme) error {
	return ErrUnsupported
}
