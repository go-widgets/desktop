// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build darwin

package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-freedesktop/mime"
	"github.com/go-widgets/desktop/shell"
)

// xmlPlistDoc builds a minimal XML Info.plist mapping each key to a string.
func xmlPlistDoc(kv map[string]string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><plist version="1.0"><dict>`)
	for k, v := range kv {
		fmt.Fprintf(&b, "<key>%s</key><string>%s</string>", k, v)
	}
	b.WriteString(`</dict></plist>`)
	return []byte(b.String())
}

// makeApp writes an .app bundle under dir: name.app/Contents/Info.plist and,
// when iconFile is non-empty, Contents/Resources/<iconFile> holding iconData.
func makeApp(t *testing.T, dir, name string, plist []byte, iconFile string, iconData []byte) {
	t.Helper()
	base := filepath.Join(dir, name+".app", "Contents")
	if plist != nil {
		if err := os.MkdirAll(base, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "Info.plist"), plist, 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		// A bundle with no Info.plist at all.
		if err := os.MkdirAll(filepath.Join(dir, name+".app"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if iconFile != "" {
		res := filepath.Join(base, "Resources")
		if err := os.MkdirAll(res, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(res, iconFile), iconData, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// findApp returns the indexed app with the given label.
func findApp(t *testing.T, d *Darwin, label string) shell.App {
	t.Helper()
	for _, a := range d.Apps().All() {
		if a.Label() == label {
			return a
		}
	}
	t.Fatalf("app %q not found", label)
	return shell.App{}
}

func TestNewDarwinScan(t *testing.T) {
	appDir := t.TempDir()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	goodIcns := makeICNS(chunk{"ic13", pngOf(t, 128, 128)})
	badIcns := []byte("not an icns at all")

	// A full spread of bundle shapes covering every readBundle/readIcon branch.
	makeApp(t, appDir, "XMLApp",
		xmlPlistDoc(map[string]string{
			"CFBundleDisplayName":       "XML Display",
			"CFBundleName":              "XML Name",
			"CFBundleIdentifier":        "com.example.xml",
			"CFBundleIconFile":          "Icon", // no extension -> ".icns" appended
			"LSApplicationCategoryType": "public.app-category.utilities",
		}), "Icon.icns", goodIcns)

	// Real macOS binary plist (id com.example.fixture, "Fixture Display",
	// developer-tools, icon "AppIcon").
	makeApp(t, appDir, "BinApp", mustBase64(t, realBinaryPlist), "AppIcon.icns", goodIcns)

	// No Info.plist: name falls back to the bundle base name.
	makeApp(t, appDir, "NoPlist", nil, "", nil)

	// Plist parses but carries no name keys: name stays the base name; a games
	// sub-genre category folds to "Games"; no icon file.
	makeApp(t, appDir, "NoName",
		xmlPlistDoc(map[string]string{
			"LSApplicationCategoryType": "public.app-category.games.arcade",
		}), "", nil)

	// Icon file referenced but absent -> no icon.
	makeApp(t, appDir, "BadRef",
		xmlPlistDoc(map[string]string{"CFBundleIconFile": "Missing"}), "", nil)

	// Icon file present but not a valid icns -> no icon; explicit .icns keeps
	// its extension (EqualFold branch).
	makeApp(t, appDir, "BadIcns",
		xmlPlistDoc(map[string]string{"CFBundleIconFile": "Bad.icns"}), "Bad.icns", badIcns)

	// Corrupt Info.plist bytes -> parse error -> base name, "Other".
	makeApp(t, appDir, "Corrupt", []byte("\x00\x01garbage"), "", nil)

	// A non-.app directory entry that must be skipped.
	if err := os.MkdirAll(filepath.Join(appDir, "NotAnApp"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Include an unreadable (non-existent) application directory: scanBundles
	// skips it rather than aborting the scan.
	d := NewDarwin(DarwinOptions{Dir: home, appDirs: []string{appDir, filepath.Join(appDir, "missing-dir")}})

	// Every bundle is indexed (7 .app dirs, the plain directory skipped).
	if got := d.Apps().Len(); got != 7 {
		t.Fatalf("Apps().Len() = %d, want 7", got)
	}

	// Names: display name preferred, then CFBundleName, then base name.
	xml := findApp(t, d, "XML Display")
	if xml.ID != "com.example.xml" {
		t.Errorf("XMLApp ID = %q", xml.ID)
	}
	findApp(t, d, "Fixture Display")
	noPlist := findApp(t, d, "NoPlist")
	if noPlist.ID == "" || !filepath.IsAbs(noPlist.ID) {
		t.Errorf("NoPlist ID should fall back to the bundle path, got %q", noPlist.ID)
	}
	findApp(t, d, "NoName")

	// Icons: the two good icns decode to PNG through IconBytes; the icon-less
	// and bad-icon apps resolve to no bytes.
	for _, label := range []string{"XML Display", "Fixture Display"} {
		a := findApp(t, d, label)
		b, ok := d.IconBytes(a.Icon)
		if !ok || len(b) < 8 || string(b[1:4]) != "PNG" {
			t.Errorf("%s icon did not decode to PNG (ok=%v len=%d)", label, ok, len(b))
		}
	}
	for _, label := range []string{"NoPlist", "NoName", "BadRef", "BadIcns", "Corrupt"} {
		a := findApp(t, d, label)
		if _, ok := d.IconBytes(a.Icon); ok {
			t.Errorf("%s unexpectedly has icon bytes", label)
		}
	}

	// Categories flatten into the menu; assert the expected branches appear.
	cats := map[string]bool{}
	for _, c := range d.Menu().Categories {
		cats[c.Name] = true
	}
	for _, want := range []string{"Developer", "Utilities", "Games", "Other"} {
		if !cats[want] {
			t.Errorf("menu missing category %q (have %v)", want, cats)
		}
	}

	// Directory listing is populated and classified.
	dir := d.Dir()
	if dir == nil || dir.Path != home {
		t.Fatalf("Dir() = %v", dir)
	}
	var sawReadme bool
	for _, it := range dir.Items {
		if it.Name == "readme.txt" {
			sawReadme = true
		}
	}
	if !sawReadme {
		t.Errorf("readme.txt not listed")
	}

	// Resolve runs the MIME resolver seam without error.
	_ = d.Resolve("readme.txt", []byte("hi"))
	if d.Resolver() == nil {
		t.Errorf("Resolver() is nil")
	}
}

func TestDarwinIconBytesPaths(t *testing.T) {
	d := NewDarwin(DarwinOptions{Dir: t.TempDir(), appDirs: []string{t.TempDir()}})

	// An absolute path to a real file is read directly (the thumbnail path).
	f := filepath.Join(t.TempDir(), "thumb.png")
	want := pngOf(t, 8, 8)
	if err := os.WriteFile(f, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, ok := d.IconBytes(f); !ok || len(b) != len(want) {
		t.Errorf("abs read: ok=%v len=%d", ok, len(b))
	}
	// An absolute path to a missing file, and a bare unknown name, both miss.
	if _, ok := d.IconBytes(filepath.Join(t.TempDir(), "absent.png")); ok {
		t.Errorf("absent abs path should miss")
	}
	if _, ok := d.IconBytes("unknown-name"); ok {
		t.Errorf("unknown bare name should miss")
	}
}

func TestDarwinThumbKey(t *testing.T) {
	d := NewDarwin(DarwinOptions{Dir: t.TempDir(), appDirs: []string{t.TempDir()}})

	// A non-image item is never thumbnailed.
	if k := d.ThumbKey(shell.FileItem{Name: "a.txt", Path: "/x/a.txt", Mime: "text/plain"}); k != "" {
		t.Errorf("text ThumbKey = %q, want empty", k)
	}

	img := shell.FileItem{Name: "p.png", Path: "/x/p.png", Mime: "image/png"}
	// With no cached thumbnail on disk, ThumbKey is empty.
	if k := d.ThumbKey(img); k != "" {
		t.Errorf("uncached ThumbKey = %q, want empty", k)
	}
	// Materialise the thumbnail at its cache path and it is returned.
	p := d.thumb.Path(img)
	if p == "" {
		t.Fatal("thumb path empty")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, pngOf(t, 4, 4), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(p) })
	if k := d.ThumbKey(img); k != p {
		t.Errorf("cached ThumbKey = %q, want %q", k, p)
	}
}

func TestDarwinDirUnreadable(t *testing.T) {
	// A non-existent file-grid directory leaves Dir() nil rather than aborting.
	d := NewDarwin(DarwinOptions{Dir: filepath.Join(t.TempDir(), "does-not-exist"), appDirs: []string{t.TempDir()}})
	if d.Dir() != nil {
		t.Errorf("Dir() = %v, want nil", d.Dir())
	}
}

func TestNewDarwinDefaultDirs(t *testing.T) {
	// Exercise the default application-directory + home resolution against the
	// real machine: it must not panic and must build valid (non-nil) models.
	d := NewDarwin(DarwinOptions{})
	if d.Apps() == nil || d.Menu() == nil {
		t.Fatal("default scan produced nil models")
	}
	t.Logf("real /Applications scan: apps=%d categories=%d", d.Apps().Len(), d.Menu().Len())
}

func TestDarwinMimeLoadFallback(t *testing.T) {
	// When no shared MIME database is available, NewDarwin degrades to an empty
	// database rather than failing.
	orig := mimeLoad
	mimeLoad = func() (*mime.Database, error) { return nil, fmt.Errorf("no mime db") }
	t.Cleanup(func() { mimeLoad = orig })

	d := NewDarwin(DarwinOptions{Dir: t.TempDir(), appDirs: []string{t.TempDir()}})
	if d.Resolver() == nil {
		t.Fatal("resolver nil after mime fallback")
	}
	// The resolver still answers (with an empty classification).
	_ = d.Resolve("x.txt", nil)
}

func TestCategoryFor(t *testing.T) {
	cases := map[string]string{
		"":                                       "Other",
		"public.app-category.developer-tools":    "Developer",
		"public.app-category.utilities":          "Utilities",
		"public.app-category.games":              "Games",
		"public.app-category.games.role-playing": "Games",
		"public.app-category.unheard-of":         "Other",
	}
	for in, want := range cases {
		if got := categoryFor(in); got != want {
			t.Errorf("categoryFor(%q) = %q, want %q", in, got, want)
		}
	}
}
