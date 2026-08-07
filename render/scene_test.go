// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"testing"

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/menu"
	"github.com/go-widgets/desktop/shell"
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
