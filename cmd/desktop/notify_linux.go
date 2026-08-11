// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build linux

package main

import (
	"fmt"

	"github.com/go-freedesktop/notifications"
	ntoast "github.com/go-freedesktop/notifications/toast"
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/toolkit"
	"github.com/godbus/dbus/v5"
)

// runNotify runs a real org.freedesktop.Notifications daemon on the session
// bus, sends the requested notification through D-Bus (exercising the whole
// godbus round trip), and hands the resulting go-widgets Toasts to the
// scene. Run it under dbus-run-session so a private session bus exists.
func runNotify(sc *render.Scene, spec string) error {
	summary, body := splitNotify(spec)

	srvConn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}
	defer srvConn.Close()

	daemon := ntoast.NewDaemon(nil, toolkit.DefaultDark(), nil)
	server := notifications.NewServer(srvConn, daemon)
	daemon.SetEmitter(server) // break the daemon<->server construction cycle
	if err := server.Export(); err != nil {
		return fmt.Errorf("export service: %w", err)
	}

	// A separate client connection sends the notification, so the call really
	// travels over the bus to the daemon's exported object.
	cliConn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect client: %w", err)
	}
	defer cliConn.Close()

	obj := cliConn.Object(notifications.BusName, dbus.ObjectPath(notifications.ObjectPath))
	call := obj.Call(notifications.Interface+".Notify", 0,
		"desktop-demo", uint32(0), "dialog-information", summary, body,
		[]string{}, map[string]dbus.Variant{}, int32(-1))
	if call.Err != nil {
		return fmt.Errorf("Notify call: %w", call.Err)
	}
	var id uint32
	if err := call.Store(&id); err != nil {
		return fmt.Errorf("Notify reply: %w", err)
	}

	sc.SetToasts(daemon.Toasts())
	return nil
}
