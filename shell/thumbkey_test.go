// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
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
		{FileItem{Mime: "image/png"}, true},
		{FileItem{Mime: "image/jpeg"}, true},
		{FileItem{Mime: "text/plain"}, false},
		{FileItem{Mime: ""}, false},
	}
	for _, c := range cases {
		if got := Thumbnailable(c.it); got != c.want {
			t.Errorf("Thumbnailable(%+v) = %v, want %v", c.it, got, c.want)
		}
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
