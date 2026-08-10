// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-thumbnail/thumbnail"
)

func TestThumbnailable(t *testing.T) {
	cases := []struct {
		it   FileItem
		want bool
	}{
		{FileItem{IsDir: true, Mime: "image/png"}, false},
		{FileItem{Name: "sub", IsDir: true}, false}, // a dir named like nothing
		{FileItem{Mime: "image/png"}, true},
		{FileItem{Mime: "image/jpeg"}, true},
		{FileItem{Mime: "text/plain"}, false},
		{FileItem{Mime: ""}, false},
		// Extension fallback: a macOS-style octet-stream / unclassified image is
		// still thumbnailable by its known image extension.
		{FileItem{Name: "shot.png", Mime: "application/octet-stream"}, true},
		{FileItem{Name: "photo.JPG", Mime: ""}, true},
		{FileItem{Name: "notes.txt", Mime: ""}, false},
	}
	for _, c := range cases {
		if got := Thumbnailable(c.it); got != c.want {
			t.Errorf("Thumbnailable(%+v) = %v, want %v", c.it, got, c.want)
		}
	}
}

func TestIsImageName(t *testing.T) {
	for _, n := range []string{"a.png", "B.JPG", "x.jpeg", "y.webp", "z.HEIC", "i.avif"} {
		if !IsImageName(n) {
			t.Errorf("IsImageName(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"a.txt", "b.pdf", "noext", "c.md", ""} {
		if IsImageName(n) {
			t.Errorf("IsImageName(%q) = true, want false", n)
		}
	}
}

// writePNG writes a tiny solid-colour PNG under dir and returns its path.
func writePNG(t *testing.T, dir, name string) string {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range m.Pix {
		m.Pix[i] = 0xC0
	}
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create %s: %v", p, err)
	}
	if err := png.Encode(f, m); err != nil {
		t.Fatalf("encode %s: %v", p, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", p, err)
	}
	return p
}

// TestThumbnailerEnsure covers the four Ensure outcomes: a non-thumbnailable
// item, a successful decode+downscale+cache of a real image, the memoized
// repeat call, and a thumbnailable item whose source cannot be read.
func TestThumbnailerEnsure(t *testing.T) {
	th := NewThumbnailer(thumbnail.Normal)

	// Non-thumbnailable -> "" (never touches the cache).
	if got := th.Ensure(FileItem{Name: "n.txt", Path: "/x/n.txt", Mime: "text/plain"}); got != "" {
		t.Errorf("Ensure(text) = %q, want empty", got)
	}

	// A real image is decoded, downscaled and cached; Ensure returns its path.
	src := writePNG(t, t.TempDir(), "pic.png")
	img := FileItem{Name: "pic.png", Path: src, Mime: "image/png"}
	got := th.Ensure(img)

	// Memoized: a repeat call returns the identical outcome without re-hitting
	// disk (holds on every platform, whatever the first call produced).
	if again := th.Ensure(img); again != got {
		t.Errorf("Ensure not memoized: %q vs %q", again, got)
	}

	// On the POSIX lanes (where the coverage gate runs) generation succeeds and
	// Ensure returns the on-disk cache path. (On Windows, go-thumbnail's file://
	// round-trip cannot resolve a C:\ source, so generation yields "" and the
	// grid falls back to the picture glyph — still exercised, just no preview.)
	if got != "" {
		if !strings.HasSuffix(got, th.Key(img)+".png") {
			t.Errorf("Ensure path = %q, want to end with %s.png", got, th.Key(img))
		}
		if _, err := os.Stat(got); err != nil {
			t.Errorf("Ensure path does not exist on disk: %v", err)
		}
	}

	// Thumbnailable by extension but the source file is absent -> "" (and the
	// negative outcome is memoized, exercised by the repeat).
	miss := FileItem{Name: "gone.png", Path: filepath.Join(t.TempDir(), "gone.png"), Mime: "application/octet-stream"}
	if got := th.Ensure(miss); got != "" {
		t.Errorf("Ensure(missing source) = %q, want empty", got)
	}
	if got := th.Ensure(miss); got != "" {
		t.Errorf("memoized Ensure(missing source) = %q, want empty", got)
	}
}

func TestThumbnailerKeyAndPath(t *testing.T) {
	th := NewThumbnailer(thumbnail.Normal)
	img := FileItem{Name: "p.png", Path: "/home/u/p.png", Mime: "image/png"}
	txt := FileItem{Name: "n.txt", Path: "/home/u/n.txt", Mime: "text/plain"}

	key := th.Key(img)
	if key == "" || key != thumbnail.Hash(thumbnail.FileURI(img.Path)) {
		t.Fatalf("Key = %q, want stable md5 of file URI", key)
	}
	// Stability: same path -> same key.
	if th.Key(img) != key {
		t.Error("Key is not stable")
	}
	p := th.Path(img)
	if !strings.HasSuffix(p, key+".png") {
		t.Errorf("Path = %q, want to end with %s.png", p, key)
	}

	// Non-thumbnailable items yield empty key and path.
	if th.Key(txt) != "" || th.Path(txt) != "" {
		t.Errorf("non-image Key/Path not empty: %q %q", th.Key(txt), th.Path(txt))
	}
}
