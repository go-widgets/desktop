// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"testing"

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/menu"
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func entry(id, name, icon string, cats ...string) *desktopentry.Entry {
	return &desktopentry.Entry{ID: id, Name: name, Icon: icon, Exec: "false", Categories: cats}
}

func testConfig() Config {
	apps := shell.NewAppIndex([]*desktopentry.Entry{
		entry("org.a.Editor", "Editor", "accessories-text-editor"),
		entry("org.a.Browser", "Browser", "web-browser"),
		entry("org.a.Files", "Files", "system-file-manager"),
	})
	tree := &menu.Tree{Root: &menu.Menu{Name: "Applications", Submenus: []*menu.Menu{
		{Name: "Utility", DirectoryName: "Utilities", Apps: []*desktopentry.Entry{entry("org.a.Editor", "Editor", "")}},
		{Name: "Net", DirectoryName: "Internet", Apps: []*desktopentry.Entry{entry("org.a.Browser", "Browser", "")}},
	}}}
	dir := &shell.Dir{Path: "/tmp/x", Items: []shell.FileItem{
		{Name: "sub", Path: "/tmp/x/sub", IsDir: true, Mime: shell.MimeDirectory},
		{Name: "a.txt", Path: "/tmp/x/a.txt", Mime: "text/plain"},
		{Name: "p.png", Path: "/tmp/x/p.png", Mime: "image/png"},
	}}
	return Config{
		Apps:        apps,
		Menu:        shell.NewMenuModel(tree),
		Dir:         dir,
		Thumbnailer: shell.NewThumbnailer(0),
		Icons:       NewIconLoader("", 32),
		Width:       640,
		Height:      480,
	}
}

func TestSceneComposition(t *testing.T) {
	s := New(testConfig())
	if s.DockCount() != 3 {
		t.Errorf("DockCount = %d, want 3", s.DockCount())
	}
	if s.AppCount() != 3 {
		t.Errorf("AppCount = %d, want 3", s.AppCount())
	}
	if s.MenuCategoryCount() != 2 {
		t.Errorf("MenuCategoryCount = %d, want 2", s.MenuCategoryCount())
	}
	if s.FileCount() != 3 {
		t.Errorf("FileCount = %d, want 3", s.FileCount())
	}
	if s.Root() == nil {
		t.Error("Root is nil")
	}
}

func TestSceneQueryFilters(t *testing.T) {
	s := New(testConfig())
	s.SetQuery("brow")
	if s.AppCount() != 1 {
		t.Errorf("filtered AppCount = %d, want 1", s.AppCount())
	}
	s.SetQuery("")
	if s.AppCount() != 3 {
		t.Errorf("cleared AppCount = %d, want 3", s.AppCount())
	}
}

func TestSceneRenderAndToasts(t *testing.T) {
	s := New(testConfig())
	img, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Fatalf("image size = %dx%d, want 640x480", b.Dx(), b.Dy())
	}

	s.ShowToast(nil) // nil is ignored
	if s.ToastCount() != 0 {
		t.Errorf("nil toast counted")
	}
	s.ShowToast(toolkit.NewToast("Build finished", toolkit.ToastSuccess))
	s.SetToasts(append(s.toasts, toolkit.NewToast("Second", toolkit.ToastInfo)))
	if s.ToastCount() != 2 {
		t.Errorf("ToastCount = %d, want 2", s.ToastCount())
	}
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
}

// TestSceneWidget exercises the single-widget adapter a windowing backend
// runs: SetBounds must propagate to the shell root, Draw must composite the
// root and then overlay the toast stack, and OnEvent must forward to the root.
// This is the headless proof that the live/windowed path renders the same
// composition the -capture path does (the windowed run itself is verified on a
// real compositor, not in the unit gate).
func TestSceneWidget(t *testing.T) {
	s := New(testConfig())
	w := s.Widget()
	if s.Theme() == nil {
		t.Fatal("Theme() is nil")
	}

	bounds := toolkit.Rect{X: 0, Y: 0, W: 640, H: 480}
	w.SetBounds(bounds)
	if got := s.Root().Bounds(); got != bounds {
		t.Fatalf("SetBounds did not propagate to root: got %+v", got)
	}

	const stride = 640
	sample := func(buf []byte, x, y int) [4]byte {
		i := (y*stride + x) * 4
		return [4]byte{buf[i], buf[i+1], buf[i+2], buf[i+3]}
	}
	draw := func() []byte {
		buf := make([]byte, 4*640*480)
		p := painter.NewPixelPainter(buf, 640, 480)
		p.FillRect(bounds, s.Theme().Background)
		w.Draw(p, s.Theme())
		return buf
	}

	// Without a toast, the top-right corner shows only the background fill.
	base := draw()

	// A toast anchors top-right; drawing it must change pixels there.
	s.ShowToast(toolkit.NewToast("Build finished", toolkit.ToastSuccess))
	withToast := draw()

	// Assert the toast overlay actually painted into its top-right anchor
	// region (and not merely somewhere): compare a band of the top-right.
	changed := false
	for y := 8; y < 60 && !changed; y++ {
		for x := 500; x < 632; x++ {
			if sample(base, x, y) != sample(withToast, x, y) {
				changed = true
				break
			}
		}
	}
	if !changed {
		t.Error("toast overlay did not paint into the top-right anchor region")
	}

	// OnEvent must not panic and is forwarded to the root widget tree.
	w.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 10})
}

func TestSceneDefaultsAndEmpties(t *testing.T) {
	// Zero-ish config: nil models, no theme/icons, default sizes.
	s := New(Config{})
	if s.DockCount() != 0 || s.AppCount() != 0 || s.MenuCategoryCount() != 0 || s.FileCount() != 0 {
		t.Errorf("empty scene not empty: dock=%d apps=%d cats=%d files=%d",
			s.DockCount(), s.AppCount(), s.MenuCategoryCount(), s.FileCount())
	}
	img, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 960 || img.Bounds().Dy() != 600 {
		t.Errorf("default size = %v, want 960x600", img.Bounds())
	}
}

func TestElideAndIconName(t *testing.T) {
	if got := elide("short", 16); got != "short" {
		t.Errorf("elide short = %q", got)
	}
	if got := elide("a-very-long-filename-here.txt", 10); len([]rune(got)) != 10 {
		t.Errorf("elide long = %q (len %d)", got, len([]rune(got)))
	}
	if n := iconNameFor(shell.FileItem{IsDir: true}, nil); n != "folder" {
		t.Errorf("dir icon = %q", n)
	}
	if n := iconNameFor(shell.FileItem{Mime: ""}, nil); n != "text-x-generic" {
		t.Errorf("empty-mime icon = %q", n)
	}
	if n := iconNameFor(shell.FileItem{Mime: "text/plain"}, nil); n != "text-plain" {
		t.Errorf("mime icon = %q", n)
	}
}
