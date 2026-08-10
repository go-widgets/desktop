// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:             "0 o",
		512:           "512 o",
		1500:          "1.5 Ko",
		2_500_000:     "2.5 Mo",
		3_200_000_000: "3.2 Go",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
	// A petabyte-scale value pins to the last unit rather than running off.
	if got := HumanBytes(5_000_000_000_000_000_000); got == "" {
		t.Error("HumanBytes overflow returned empty")
	}
}

func TestHumanSizeAndType(t *testing.T) {
	dir := FileItem{IsDir: true}
	if dir.HumanSize() != "--" {
		t.Errorf("dir size = %q", dir.HumanSize())
	}
	if dir.TypeLabel() != "Dossier" {
		t.Errorf("dir type = %q", dir.TypeLabel())
	}
	f := FileItem{Name: "a.txt", Size: 10, Mime: "text/plain"}
	if f.HumanSize() != "10 o" {
		t.Errorf("file size = %q", f.HumanSize())
	}
	if f.TypeLabel() != "Texte" {
		t.Errorf("txt type = %q", f.TypeLabel())
	}
}

func TestTypeLabelVariants(t *testing.T) {
	cases := []struct{ name, mime, want string }{
		{"p.png", "image/png", "Image PNG"},
		{"m.mp4", "video/mp4", "Vidéo MP4"},
		{"s.mp3", "audio/mpeg", "Audio MPEG"},
		{"d.pdf", "application/pdf", "Document PDF"},
		{"weird.xyz", "application/x-thing", "Document XYZ"},
		{"noext", "", "Document"},
		{"data.bin", "", "Document BIN"},
	}
	for _, c := range cases {
		it := FileItem{Name: c.name, Mime: c.mime}
		if got := it.TypeLabel(); got != c.want {
			t.Errorf("TypeLabel(%q,%q) = %q, want %q", c.name, c.mime, got, c.want)
		}
	}
}

func TestModTimeString(t *testing.T) {
	if (FileItem{}).ModTimeString() != "--" {
		t.Error("zero modtime should be --")
	}
	tm := time.Date(2026, 8, 10, 21, 4, 0, 0, time.UTC)
	if got := (FileItem{ModTime: tm}).ModTimeString(); got != "10/08/2026 21:04" {
		t.Errorf("modtime = %q", got)
	}
}

func TestMimeByExtAndIsImage(t *testing.T) {
	cases := map[string]string{
		"a.PNG": "image/png", "b.jpeg": "image/jpeg", "c.gif": "image/gif",
		"d.webp": "image/webp", "e.svg": "image/svg+xml", "f.tiff": "image/tiff",
		"g.mp4": "video/mp4", "h.flac": "audio/flac", "i.md": "text/plain",
		"j.pdf": "application/pdf", "k.unknown": "",
	}
	for name, want := range cases {
		if got := MimeByExt(name); got != want {
			t.Errorf("MimeByExt(%q) = %q, want %q", name, got, want)
		}
	}
	if !(FileItem{Mime: "image/png"}).IsImage() {
		t.Error("png should be image")
	}
	if (FileItem{Mime: "image/svg+xml"}).IsImage() {
		t.Error("svg should not be a raster image")
	}
	if !(FileItem{Name: "x.jpg"}).IsImage() {
		t.Error("jpg by name should be image")
	}
	if (FileItem{Name: "x.txt"}).IsImage() {
		t.Error("txt should not be image")
	}
}

func TestListDirAndClassifyLite(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "photo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".hidden"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := ListDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 2 { // .hidden filtered
		t.Fatalf("items = %d, want 2", len(d.Items))
	}
	// Directories first; the file carries a non-zero size + modtime.
	if !d.Items[0].IsDir || d.Items[0].Name != "sub" {
		t.Errorf("first item = %+v, want dir sub", d.Items[0])
	}
	if d.Items[1].Size != 1 || d.Items[1].ModTime.IsZero() {
		t.Errorf("file stat not populated: %+v", d.Items[1])
	}
	d.ClassifyLite()
	if d.Items[0].Mime != MimeDirectory {
		t.Errorf("dir mime = %q", d.Items[0].Mime)
	}
	if d.Items[1].Mime != "image/png" {
		t.Errorf("png mime = %q", d.Items[1].Mime)
	}
	if _, err := ListDir(filepath.Join(tmp, "nope")); err == nil {
		t.Error("listing a missing dir should error")
	}
}
