// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build js && wasm

package wasmhost

import (
	"errors"
	"syscall/js"

	"github.com/go-widgets/toolkit"
)

// Run drives root as a wasmbox external client until the surface closes. It
// reaches the compositor through the wasmbox client SDK's globalThis.wasmboxClient
// (the worker constructs it and boots this wasm after its welcome resolves), so
// the granted surface size and the shared pixel buffer are already in place. It
// renders the initial frame, registers the input callback, then blocks — the
// browser event loop keeps servicing the js callbacks.
//
// It is the js/wasm arm of the desktop shell's backend, the analogue of
// window.(*Window).Run: same widget tree, presented to the wasmdesk compositor
// instead of an X11/Wayland server.
func Run(root toolkit.Widget, theme *toolkit.Theme) error {
	client := js.Global().Get("wasmboxClient")
	if client.IsUndefined() || client.IsNull() {
		return errors.New("wasmhost: globalThis.wasmboxClient missing (wasmbox SDK not loaded?)")
	}
	w := client.Get("w").Int()
	h := client.Get("h").Int()
	pixels := client.Get("pixels")
	if pixels.Get("length").Int() != 4*w*h {
		return errors.New("wasmhost: shared pixel buffer size mismatch")
	}

	host := NewHost(root, theme, w, h)

	// present copies the framebuffer into the shared surface and commits the
	// damage rectangle. beginFrame opens the seqlock window before the bulk copy
	// so the compositor never blits a half-written frame (tear-free); commit
	// closes it and tells the compositor which rectangle changed — the shell's
	// incremental damage flowing straight through wasmbox commit{damage}.
	present := func(dmg toolkit.Rect) {
		if dmg.W <= 0 || dmg.H <= 0 {
			return
		}
		client.Call("beginFrame")
		js.CopyBytesToJS(pixels, host.Buffer())
		d := js.Global().Get("Object").New()
		d.Set("x", dmg.X)
		d.Set("y", dmg.Y)
		d.Set("w", dmg.W)
		d.Set("h", dmg.H)
		client.Call("commit", d)
	}

	present(host.Frame()) // initial full-surface map

	cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		if host.Input(decodeInput(args[0])) {
			present(host.Frame())
		}
		return nil
	})
	client.Call("onInput", cb)

	select {} // hand control to the browser event loop
}

// decodeInput reads a wasmbox input-event js object into an InputEvent, guarding
// every field (a mouse event carries no key, a key event no x/y).
func decodeInput(ev js.Value) InputEvent {
	return InputEvent{
		Kind:   strField(ev, "kind"),
		X:      intField(ev, "x"),
		Y:      intField(ev, "y"),
		Key:    strField(ev, "key"),
		Code:   inputText(ev),
		DeltaY: intField(ev, "deltaY"),
	}
}

// inputText returns the character payload of a "char" event: the compositor
// puts it on "text" (or "code" for synthesized characters), so both are
// accepted, "text" first.
func inputText(ev js.Value) string {
	if s := strField(ev, "text"); s != "" {
		return s
	}
	return strField(ev, "code")
}

// intField reads an integer property, returning 0 when it is absent or not a
// number (so a mouse event's missing deltaY does not panic).
func intField(v js.Value, key string) int {
	p := v.Get(key)
	if p.Type() != js.TypeNumber {
		return 0
	}
	return p.Int()
}

// strField reads a string property, returning "" when it is absent or not a
// string.
func strField(v js.Value, key string) string {
	p := v.Get(key)
	if p.Type() != js.TypeString {
		return ""
	}
	return p.String()
}
