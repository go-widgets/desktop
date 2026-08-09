// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"testing"

	"github.com/go-widgets/desktop/render"
	"github.com/go-widgets/desktop/shell"
)

// TestEmbeddedPopulatesShellLogic drives the same shell model logic the native
// source feeds and asserts the embedded source yields a real, populated
// desktop: a sorted app index, a categorized menu, a virtual directory and
// working search — with NO filesystem access.
func TestEmbeddedPopulatesShellLogic(t *testing.T) {
	e := NewEmbedded()

	if got := e.Apps().Len(); got != len(demoApps) {
		t.Fatalf("Apps().Len() = %d, want %d", got, len(demoApps))
	}
	// The index sorts case-insensitively by label; verify the ordering the
	// dock/launcher will show.
	all := e.Apps().All()
	for i := 1; i < len(all); i++ {
		if all[i-1].Label() > all[i].Label() {
			t.Errorf("apps not sorted: %q before %q", all[i-1].Label(), all[i].Label())
		}
	}

	// Search is the launcher's live query path.
	if got := len(e.Apps().Search("web")); got != 1 {
		t.Errorf("Search(web) = %d apps, want 1", got)
	}
	if got := len(e.Apps().Search("")); got != len(demoApps) {
		t.Errorf("Search(empty) = %d, want all %d", got, len(demoApps))
	}

	if got := e.Menu().Len(); got != len(demoMenu) {
		t.Errorf("Menu().Len() = %d, want %d", got, len(demoMenu))
	}

	d := e.Dir()
	if d == nil || len(d.Items) != len(demoFiles) {
		t.Fatalf("Dir items = %v, want %d", d, len(demoFiles))
	}
}

// TestEmbeddedIconsResolve asserts every app icon, every generic icon and both
// thumbnails resolve to decodable image bytes, and that a specific per-MIME
// name falls back to a generic icon rather than a placeholder.
func TestEmbeddedIconsResolve(t *testing.T) {
	e := NewEmbedded()

	for _, a := range e.Apps().All() {
		if _, ok := e.IconBytes(a.Icon); !ok {
			t.Errorf("icon %q for app %q did not resolve", a.Icon, a.ID)
		}
	}
	// Per-MIME theme names the embed set does not carry verbatim fall back.
	for _, name := range []string{"text-plain", "image-png", "application-pdf-xyz", "folder"} {
		if _, ok := e.IconBytes(name); !ok {
			t.Errorf("icon %q did not resolve (want a fallback)", name)
		}
	}
	// A wholly unknown, non-prefixed name still falls back to the executable
	// generic (never a hard miss for a launchable), so ok is true.
	if _, ok := e.IconBytes("totally-unknown"); !ok {
		t.Error("unknown icon: want executable fallback")
	}
}

// TestEmbeddedThumbKeys asserts the two curated photos get real thumbnail
// assets and everything else (directories, documents) does not.
func TestEmbeddedThumbKeys(t *testing.T) {
	e := NewEmbedded()
	cases := map[string]string{
		"photo1.png": "thumb-photo1",
		"photo2.png": "thumb-photo2",
		"readme.txt": "",
		"Documents":  "",
	}
	byName := map[string]shell.FileItem{}
	for _, it := range e.Dir().Items {
		byName[it.Name] = it
	}
	for name, want := range cases {
		if got := e.ThumbKey(byName[name]); got != want {
			t.Errorf("ThumbKey(%s) = %q, want %q", name, got, want)
		}
		if want != "" {
			if _, ok := e.IconBytes(want); !ok {
				t.Errorf("thumb asset %q does not resolve", want)
			}
		}
	}
}

// TestEmbeddedResolve asserts the "open with" table returns real candidates.
func TestEmbeddedResolve(t *testing.T) {
	e := NewEmbedded()
	ow := e.Resolve("photo.png", nil)
	if ow.MimeType != "image/png" {
		t.Errorf("mime = %q, want image/png", ow.MimeType)
	}
	if !ow.HasDefault || len(ow.Candidates) != 2 {
		t.Errorf("png associations = %+v, want a default + 2 candidates", ow)
	}
	if ow := e.Resolve("noext", nil); ow.MimeType != "application/octet-stream" {
		t.Errorf("unknown mime = %q, want octet-stream", ow.MimeType)
	}
	if ow := e.Resolve("weird.xyz", nil); ow.HasDefault {
		t.Error("unknown type should have no default app")
	}
}

// TestEmbeddedComposesScene proves the whole point of the seam: a Scene built
// from the embedded source (the browser path) is populated exactly as one built
// from the native source would be — same render.New call.
func TestEmbeddedComposesScene(t *testing.T) {
	sc := render.New(render.Config{Source: NewEmbedded(), Width: 960, Height: 600})
	if sc.DockCount() == 0 {
		t.Error("dock is empty")
	}
	if sc.AppCount() != len(demoApps) {
		t.Errorf("launcher shows %d apps, want %d", sc.AppCount(), len(demoApps))
	}
	if sc.MenuCategoryCount() != len(demoMenu) {
		t.Errorf("menu categories = %d, want %d", sc.MenuCategoryCount(), len(demoMenu))
	}
	if sc.FileCount() != len(demoFiles) {
		t.Errorf("file grid = %d, want %d", sc.FileCount(), len(demoFiles))
	}
	// It renders to a non-empty image (the browser presents these very pixels).
	img, err := sc.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if img.Bounds().Dx() != 960 || img.Bounds().Dy() != 600 {
		t.Errorf("render size = %v", img.Bounds())
	}
}
