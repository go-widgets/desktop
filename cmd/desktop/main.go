// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Command desktop is a native go-widgets desktop-shell demo. It composes the
// whole go-freedesktop + go-widgets stack against a real Linux filesystem:
//
//   - desktopentry.Scan   -> the dock + launcher app index
//   - icontheme.FindIcon  -> dock / launcher / file-grid icons
//   - menu.Load           -> the categorized application menu
//   - mime + mimeapps     -> file-grid MIME classification ("open with")
//   - go-thumbnail        -> file-grid thumbnail cache keys
//   - notifications       -> a live org.freedesktop.Notifications daemon whose
//     incoming notifications render as go-widgets Toasts (Linux only)
//
// go-widgets is a pure pixel-blitting toolkit, so the shell renders into an
// offscreen framebuffer; -capture writes that framebuffer to a PNG (the
// headless "screenshot" path used for autonomous visual verification), and a
// host compositor would otherwise present it.
//
// This is the NATIVE binary (X11/Wayland via go-widgets/window, real XDG scan).
// The browser (wasmdesk) build lives in ../../clients/desktop, so this command
// is excluded from js/wasm.
//
//go:build !js

package main

import (
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/desktop/source"
	"github.com/go-widgets/toolkit"
)

// osExit is a seam over os.Exit so run's exit codes can be asserted in tests.
var osExit = os.Exit

func main() {
	// Pin the main goroutine to the process main OS thread. The macOS Cocoa/
	// AppKit windowing backend (window.Open → cocoa.New/Run) requires all
	// window and run-loop work on the main thread; locking here before any
	// goroutine work keeps the interactive window path on it. It is harmless on
	// X11/Wayland (they do not care which thread they run on).
	runtime.LockOSThread()
	osExit(run(os.Args[1:], os.Stderr))
}

// options are the parsed command-line flags.
type options struct {
	dir       string
	capture   string
	query     string
	launch    string
	notify    string
	iconTheme string
	width     int
	height    int
	light     bool
	embedded  bool
	view      string
	iconSize  int
	place     string
}

// run parses args, builds the shell scene and performs the requested action,
// returning the process exit code. It writes diagnostics to errw.
func run(args []string, errw io.Writer) int {
	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o options
	fs.StringVar(&o.dir, "dir", defaultDir(), "directory to show in the file grid")
	fs.StringVar(&o.capture, "capture", "", "render the shell to this PNG path and exit")
	fs.StringVar(&o.query, "query", "", "seed the launcher search query")
	fs.StringVar(&o.launch, "launch", "", "launch the app with this desktop-file id, then exit")
	fs.StringVar(&o.notify, "notify", "", `send a notification "Summary|Body" through the daemon (Linux)`)
	fs.StringVar(&o.iconTheme, "icon-theme", "", "icon theme name (default hicolor)")
	fs.IntVar(&o.width, "w", 960, "render width")
	fs.IntVar(&o.height, "h", 600, "render height")
	fs.BoolVar(&o.light, "light", false, "use the light theme")
	fs.BoolVar(&o.embedded, "embedded", false, "use the embedded (browser) app source instead of scanning the filesystem — renders the exact scene the wasmdesk client shows")
	fs.StringVar(&o.view, "view", "", "file-manager view mode: liste | vignettes | colonnes")
	fs.IntVar(&o.iconSize, "icon-size", 0, "Vignettes icon size in pixels (32..128)")
	fs.StringVar(&o.place, "place", "", "navigate the finder to a sidebar place: reseau | corbeille | accueil | volume")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	sc, apps := buildScene(o)
	applyFinderOptions(sc, o)

	if o.launch != "" {
		if err := launchByID(apps, o.launch); err != nil {
			fmt.Fprintf(errw, "desktop: launch %q: %v\n", o.launch, err)
			return 1
		}
		return 0
	}

	if o.notify != "" {
		if err := runNotify(sc, o.notify); err != nil {
			fmt.Fprintf(errw, "desktop: notify: %v\n", err)
			return 1
		}
	}

	if o.capture != "" {
		if err := capture(sc, o.capture); err != nil {
			fmt.Fprintf(errw, "desktop: capture: %v\n", err)
			return 1
		}
		fmt.Fprintf(errw, "desktop: dock=%d apps=%d categories=%d files=%d toasts=%d -> %s\n",
			sc.DockCount(), sc.AppCount(), sc.MenuCategoryCount(), sc.FileCount(), sc.ToastCount(), o.capture)
		return 0
	}

	// No capture / launch: open a real window on the running display server and
	// run the shell interactively until it is closed. On a headless host (no
	// DISPLAY / WAYLAND_DISPLAY) or a platform with no windowing backend,
	// runWindow falls back to reporting the composed shell — use -capture for a
	// screenshot there.
	if err := runWindow(o, sc, errw); err != nil {
		fmt.Fprintf(errw, "desktop: window: %v\n", err)
		return 1
	}
	return 0
}

// buildScene scans the filesystem through the native XDG source and composes
// the shell Scene, returning it alongside the source (whose app index -launch
// needs). The source is the portable seam: swapping source.NewXDG for
// source.NewEmbedded is the whole difference between the native desktop and the
// browser (wasmdesk) client — the render.New call below is identical.
func buildScene(o options) (*render.Scene, *shell.AppIndex) {
	var src shell.AppSource
	if o.embedded {
		src = source.NewEmbedded()
	} else {
		src = nativeSource(o)
	}

	theme := toolkit.DefaultDark()
	if o.light {
		theme = toolkit.DefaultLight()
	}

	sc := render.New(render.Config{
		Source: src,
		Theme:  theme,
		Width:  o.width,
		Height: o.height,
	})
	if o.query != "" {
		sc.SetQuery(o.query)
	}
	return sc, src.Apps()
}

// applyFinderOptions applies the -view / -icon-size flags to the finder pane
// before a capture or an interactive run, so a screenshot can target a specific
// Finder view mode and icon size.
func applyFinderOptions(sc *render.Scene, o options) {
	switch o.view {
	case "liste", "list":
		sc.Finder().SetView(render.ViewList)
	case "vignettes", "icons", "icones":
		sc.Finder().SetView(render.ViewIcons)
	case "colonnes", "columns":
		sc.Finder().SetView(render.ViewColumns)
		// For a screenshot, open a few levels so the Miller cascade is visible.
		if o.capture != "" {
			sc.Finder().CascadeColumns(4)
		}
	}
	if o.iconSize > 0 {
		sc.Finder().SetIconSize(o.iconSize)
	}
	switch o.place {
	case "reseau", "network":
		sc.Finder().GoToPlace(shell.PlaceNetwork)
	case "corbeille", "trash":
		sc.Finder().GoToPlace(shell.PlaceTrash)
	case "accueil", "home":
		sc.Finder().GoToPlace(shell.PlaceHome)
	case "volume", "macintosh-hd":
		sc.Finder().GoToPlace(shell.PlaceVolume)
	}
}

// launchByID resolves a desktop-file id in the index and starts it. This is the
// one legitimate exec of an external command in the shell: launching a
// user-chosen application is the launcher's whole purpose. It is reached only
// via the explicit -launch flag.
func launchByID(apps *shell.AppIndex, id string) error {
	for _, a := range apps.All() {
		if a.ID == id {
			return launch(a)
		}
	}
	return fmt.Errorf("no app with id %q", id)
}

// launch expands an app's Exec line and starts the resulting argv detached.
func launch(a shell.App) error {
	e := a.Entry()
	if e == nil {
		return fmt.Errorf("app %q has no desktop entry", a.ID)
	}
	argv, err := e.ExpandExec(nil, "")
	if err != nil {
		return err
	}
	if len(argv) == 0 {
		return fmt.Errorf("app %q expanded to an empty command", a.ID)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	return cmd.Start()
}

// capture renders the scene to a PNG file.
func capture(sc *render.Scene, path string) error {
	img, err := sc.Render()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// defaultDir is the directory the file grid shows when -dir is not given: a
// sensible user-facing folder — Desktop, else Documents, else the home
// directory (whose dotfiles ListDir filters out), else the working directory,
// else the filesystem root.
func defaultDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"Desktop", "Documents"} {
			if p := filepath.Join(home, name); isDir(p) {
				return p
			}
		}
		return home
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return string(filepath.Separator)
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// splitNotify splits a "Summary|Body" spec into its two parts.
func splitNotify(spec string) (summary, body string) {
	if i := strings.IndexByte(spec, '|'); i >= 0 {
		return spec[:i], spec[i+1:]
	}
	return spec, ""
}
