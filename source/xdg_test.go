// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package source

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-thumbnail/thumbnail"
	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
)

// xdgFixture points the XDG env at temp dirs holding a couple of launchable
// apps and a directory of files, then returns the files dir.
func xdgFixture(t *testing.T) (files string) {
	t.Helper()
	data := t.TempDir()
	appsDir := filepath.Join(data, "applications")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(appsDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("org.x.Editor.desktop", "[Desktop Entry]\nType=Application\nName=Editor\nExec=false %f\nIcon=text-editor\n")
	write("org.x.Browser.desktop", "[Desktop Entry]\nType=Application\nName=Browser\nExec=false %u\nIcon=web-browser\n")

	files = t.TempDir()
	if err := os.WriteFile(filepath.Join(files, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(files, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_DATA_DIRS", data)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return files
}

// TestXDGScansFilesystem asserts the native source scans real apps + files and
// composes the same Scene the embedded source does.
func TestXDGScansFilesystem(t *testing.T) {
	files := xdgFixture(t)
	x := NewXDG(XDGOptions{Dir: files, IconSize: 32})

	if x.Apps().Len() < 2 {
		t.Errorf("Apps().Len() = %d, want >= 2 (scanned .desktop files)", x.Apps().Len())
	}
	if x.Menu() == nil {
		t.Error("Menu() is nil")
	}
	if x.Resolver() == nil {
		t.Error("Resolver() is nil")
	}
	d := x.Dir()
	if d == nil || len(d.Items) != 2 {
		t.Fatalf("Dir items = %v, want 2 (a.txt + sub)", d)
	}

	// The same render.New call the embedded/browser path uses.
	sc := render.New(render.Config{Source: x, Width: 400, Height: 300})
	if sc.DockCount() == 0 || sc.AppCount() < 2 {
		t.Errorf("scene dock=%d apps=%d, want populated", sc.DockCount(), sc.AppCount())
	}
	if sc.FileCount() != 2 {
		t.Errorf("file grid = %d, want 2", sc.FileCount())
	}
}

// TestXDGResolve asserts name+content classification runs through the real MIME
// resolver (a directory default when no other data is available; the call must
// not panic and must return a type).
func TestXDGResolve(t *testing.T) {
	xdgFixture(t)
	x := NewXDG(XDGOptions{})
	ow := x.Resolve("a.txt", []byte("hello"))
	if ow.MimeType == "" {
		t.Error("Resolve returned an empty MIME type")
	}
}

// TestXDGIconBytes exercises the icon-bytes seam: an absolute path is read
// directly; a missing path and an unresolvable theme name both report ok=false.
func TestXDGIconBytes(t *testing.T) {
	xdgFixture(t)
	x := NewXDG(XDGOptions{})

	// Absolute path -> read directly.
	dir := t.TempDir()
	p := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(p, pngBytes(t, color.RGBA{1, 2, 3, 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, ok := x.IconBytes(p); !ok || len(b) == 0 {
		t.Errorf("IconBytes(abs) = (%d bytes, %v), want ok", len(b), ok)
	}
	// Missing absolute path -> not ok.
	if _, ok := x.IconBytes(filepath.Join(dir, "nope.png")); ok {
		t.Error("IconBytes(missing abs) = ok, want false")
	}
	// Unresolvable theme name (no icon theme installed under the temp XDG) ->
	// not ok.
	if _, ok := x.IconBytes("definitely-not-an-icon-name-xyz"); ok {
		t.Error("IconBytes(bad name) = ok, want false")
	}
}

// TestXDGThumbKey asserts the thumbnail-key seam: an image whose cache file
// exists yields that path; a non-image, and an image with no cached thumbnail,
// yield "".
func TestXDGThumbKey(t *testing.T) {
	xdgFixture(t)
	x := NewXDG(XDGOptions{})

	imgItem := shell.FileItem{Name: "p.png", Path: "/tmp/p.png", Mime: "image/png"}
	txtItem := shell.FileItem{Name: "a.txt", Path: "/tmp/a.txt", Mime: "text/plain"}

	// Non-image -> "".
	if got := x.ThumbKey(txtItem); got != "" {
		t.Errorf("ThumbKey(text) = %q, want empty", got)
	}
	// Image with no cached thumbnail -> "".
	if got := x.ThumbKey(imgItem); got != "" {
		t.Errorf("ThumbKey(uncached image) = %q, want empty", got)
	}
	// Materialise the expected cache file, then it resolves.
	want := shell.NewThumbnailer(thumbnail.Normal).Path(imgItem)
	if want == "" {
		t.Fatal("thumbnailer produced no path for an image item")
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, pngBytes(t, color.RGBA{9, 9, 9, 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := x.ThumbKey(imgItem); got != want {
		t.Errorf("ThumbKey(cached image) = %q, want %q", got, want)
	}
}

// TestXDGDefaults covers the empty-option defaults (icon size, theme name, dir).
func TestXDGDefaults(t *testing.T) {
	xdgFixture(t)
	x := NewXDG(XDGOptions{}) // no Dir/IconTheme/IconSize -> home/hicolor/48
	if x.size != 48 {
		t.Errorf("default size = %d, want 48", x.size)
	}
	if x.Dir() == nil {
		t.Error("default Dir (home) did not list")
	}
}

// pngBytes encodes a 2×2 solid PNG.
func pngBytes(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.SetRGBA(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
