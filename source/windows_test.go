// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build windows

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

// writeLnk writes a valid .lnk shortcut with the given target, icon location and
// arguments (each optional). StringData fields are appended in on-disk order.
func writeLnk(t *testing.T, path, target, iconLoc, args string) {
	t.Helper()
	b := &lnkBuilder{}
	b.flags |= flagHasLinkInfo
	b.linkInfo = ansiLinkInfo(target, "")
	if args != "" {
		b.addString(flagHasArguments, args)
	}
	if iconLoc != "" {
		b.addString(flagHasIconLocation, iconLoc)
	}
	mustWrite(t, path, b.bytes())
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func findWinApp(t *testing.T, w *Windows, label string) shell.App {
	t.Helper()
	for _, a := range w.Apps().All() {
		if a.Label() == label {
			return a
		}
	}
	t.Fatalf("app %q not found", label)
	return shell.App{}
}

func TestNewWindowsScan(t *testing.T) {
	root1 := filepath.Join(t.TempDir(), "Programs")
	root2 := filepath.Join(t.TempDir(), "Programs")
	iconDir := t.TempDir()

	// A real .ico and a real PE .exe to serve as icon sources.
	icoPath := filepath.Join(iconDir, "calc.ico")
	mustWrite(t, icoPath, buildICO(icoEntry{32, 32, dib32(32, 32)}))
	exePath := filepath.Join(iconDir, "editor.exe")
	mustWrite(t, exePath, buildPE(t, makeRsrc(0), ".rsrc"))

	// Machine-wide tree.
	writeLnk(t, filepath.Join(root1, "Accessories", "Calculator.lnk"), `C:\Windows\System32\calc.exe`, icoPath, "")
	writeLnk(t, filepath.Join(root1, "Dev Tools", "Editor.lnk"), `C:\Program Files\Editor\editor.exe`, exePath, "--fast")
	writeLnk(t, filepath.Join(root1, "Loose.lnk"), `C:\loose.exe`, "", "") // directly in root -> "Other"
	// A truncated .lnk degrades to its base name with no target/icon.
	mustWrite(t, filepath.Join(root1, "Broken.lnk"), []byte("nope"))
	// A non-.lnk file is ignored.
	mustWrite(t, filepath.Join(root1, "readme.txt"), []byte("hi"))

	// Per-user tree.
	writeLnk(t, filepath.Join(root2, "Games", "Solitaire.lnk"), `C:\Games\sol.exe`, "", "")

	home := t.TempDir()
	mustWrite(t, filepath.Join(home, "notes.txt"), []byte("hi"))

	// A non-existent root is skipped rather than aborting the scan.
	w := NewWindows(WindowsOptions{Dir: home, startDirs: []string{root1, root2, filepath.Join(t.TempDir(), "missing")}})

	if got := w.Apps().Len(); got != 5 {
		t.Fatalf("Apps().Len() = %d, want 5", got)
	}

	// Names come from the shortcut file name.
	calc := findWinApp(t, w, "Calculator")
	if calc.ID != `C:\Windows\System32\calc.exe` {
		t.Errorf("Calculator ID = %q", calc.ID)
	}
	editor := findWinApp(t, w, "Editor")
	if e := editor.Entry(); e == nil || !strings.Contains(e.Exec, "--fast") {
		t.Errorf("Editor Exec missing args: %+v", e)
	}
	broken := findWinApp(t, w, "Broken")
	if broken.ID == "" || !strings.HasSuffix(broken.ID, "Broken.lnk") {
		t.Errorf("Broken ID should fall back to the .lnk path, got %q", broken.ID)
	}
	findWinApp(t, w, "Loose")
	findWinApp(t, w, "Solitaire")

	// Icons: the .ico and PE both decode to PNG through IconBytes.
	for _, label := range []string{"Calculator", "Editor"} {
		a := findWinApp(t, w, label)
		b, ok := w.IconBytes(a.Icon)
		if !ok || len(b) < 8 || string(b[1:4]) != "PNG" {
			t.Errorf("%s icon did not decode to PNG (ok=%v len=%d)", label, ok, len(b))
		}
	}
	// The icon-less shortcuts resolve to no bytes.
	for _, label := range []string{"Loose", "Broken", "Solitaire"} {
		a := findWinApp(t, w, label)
		if _, ok := w.IconBytes(a.Icon); ok {
			t.Errorf("%s unexpectedly has icon bytes", label)
		}
	}

	// Categories: the Start Menu subfolders plus "Other" for the loose one.
	cats := map[string]bool{}
	for _, c := range w.Menu().Categories {
		cats[c.Name] = true
	}
	for _, want := range []string{"Accessories", "Dev Tools", "Games", "Other"} {
		if !cats[want] {
			t.Errorf("menu missing category %q (have %v)", want, cats)
		}
	}

	// Directory listing is populated and classified.
	dir := w.Dir()
	if dir == nil || dir.Path != home {
		t.Fatalf("Dir() = %v", dir)
	}
	var sawNotes bool
	for _, it := range dir.Items {
		if it.Name == "notes.txt" {
			sawNotes = true
		}
	}
	if !sawNotes {
		t.Errorf("notes.txt not listed")
	}

	// Resolve runs the MIME resolver seam without error.
	_ = w.Resolve("notes.txt", []byte("hi"))
	if w.Resolver() == nil {
		t.Errorf("Resolver() is nil")
	}
}

func TestWindowsStartDirsDefault(t *testing.T) {
	// startDirsOrDefault builds the two standard roots from the environment; an
	// unset variable simply drops that root.
	base := t.TempDir()
	t.Setenv("ProgramData", base)
	os.Unsetenv("AppData")
	dirs := WindowsOptions{}.startDirsOrDefault()
	if len(dirs) != 1 {
		t.Fatalf("dirs = %v, want 1 (AppData unset)", dirs)
	}
	want := filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs")
	if dirs[0] != want {
		t.Errorf("dirs[0] = %q, want %q", dirs[0], want)
	}

	// With both set, both roots appear.
	t.Setenv("AppData", base)
	if got := (WindowsOptions{}).startDirsOrDefault(); len(got) != 2 {
		t.Errorf("dirs = %v, want 2", got)
	}
}

func TestNewWindowsDefaultDirs(t *testing.T) {
	// Drive NewWindows through the env-derived default path (no startDirs).
	base := t.TempDir()
	t.Setenv("ProgramData", base)
	t.Setenv("AppData", base)
	prog := filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs")
	writeLnk(t, filepath.Join(prog, "Tools", "Thing.lnk"), `C:\thing.exe`, "", "")
	w := NewWindows(WindowsOptions{Dir: t.TempDir()})
	if w.Apps() == nil || w.Menu() == nil {
		t.Fatal("default scan produced nil models")
	}
	findWinApp(t, w, "Thing")
}

func TestCategoryOf(t *testing.T) {
	root := `C:\Start\Programs`
	if c := categoryOf(root, filepath.Join(root, "Accessories", "x.lnk")); c != "Accessories" {
		t.Errorf("subfolder category = %q", c)
	}
	if c := categoryOf(root, filepath.Join(root, "x.lnk")); c != "Other" {
		t.Errorf("root-level category = %q, want Other", c)
	}
	// A path on a different volume cannot be made relative -> "Other".
	if c := categoryOf(`C:\a`, `D:\b\x.lnk`); c != "Other" {
		t.Errorf("cross-volume category = %q, want Other", c)
	}
}

func TestReadShortcutMissingFile(t *testing.T) {
	// A path that cannot be read degrades to the base name with no target.
	s := readShortcut(filepath.Join(t.TempDir(), "Ghost.lnk"), "Cat", 0)
	if s.name != "Ghost" || s.target != "" {
		t.Errorf("degraded shortcut = %+v", s)
	}
	// No icon decoded, so iconKey stays empty (Entry.Icon then misses cleanly).
	if s.iconKey != "" || s.iconPNG != nil {
		t.Errorf("degraded shortcut should carry no icon, got key=%q png=%d", s.iconKey, len(s.iconPNG))
	}
}

func TestReadWindowsIcon(t *testing.T) {
	dir := t.TempDir()
	// Empty and quoted-empty locations yield nil.
	if readWindowsIcon("", 0) != nil || readWindowsIcon(`""`, 0) != nil {
		t.Error("empty icon location should be nil")
	}
	// Missing file yields nil.
	if readWindowsIcon(filepath.Join(dir, "absent.ico"), 0) != nil {
		t.Error("missing icon file should be nil")
	}
	// A valid .ico decodes.
	ico := filepath.Join(dir, "a.ico")
	mustWrite(t, ico, buildICO(icoEntry{16, 16, dib32(16, 16)}))
	if b := readWindowsIcon(`"`+ico+`"`, 0); b == nil || string(b[1:4]) != "PNG" {
		t.Errorf("ico did not decode: %v", b)
	}
	// A bad .ico yields nil.
	bad := filepath.Join(dir, "bad.ico")
	mustWrite(t, bad, []byte{0, 0, 9, 9})
	if readWindowsIcon(bad, 0) != nil {
		t.Error("bad ico should be nil")
	}
	// A valid PE (non-.ico extension routes to the PE path) decodes.
	exe := filepath.Join(dir, "a.exe")
	mustWrite(t, exe, buildPE(t, makeRsrc(0), ".rsrc"))
	if b := readWindowsIcon(exe, 0); b == nil || string(b[1:4]) != "PNG" {
		t.Errorf("pe did not decode: %v", b)
	}
	// A non-PE, non-.ico file yields nil (PE path errors).
	dll := filepath.Join(dir, "a.dll")
	mustWrite(t, dll, []byte("not a pe"))
	if readWindowsIcon(dll, 0) != nil {
		t.Error("bad pe should be nil")
	}
	// %VAR% expansion resolves to the real path.
	t.Setenv("ICONHOME", dir)
	if b := readWindowsIcon(`%ICONHOME%\a.ico`, 0); b == nil {
		t.Error("env-expanded icon should decode")
	}
}

func TestExpandWinEnv(t *testing.T) {
	t.Setenv("FOO", "X")
	t.Setenv("BAR", "Y")
	cases := map[string]string{
		"plain":         "plain",    // no percent
		"50% done":      "50% done", // single unmatched percent
		"%FOO%bar":      "Xbar",     // one variable
		"a%FOO%b%BAR%c": "aXbYc",    // chained
		"%MISSING%z":    "z",        // undefined -> empty
	}
	for in, want := range cases {
		if got := expandWinEnv(in); got != want {
			t.Errorf("expandWinEnv(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEntryForShortcut(t *testing.T) {
	// No target -> Exec uses the .lnk path; no args -> just a quoted target.
	e := entryForShortcut(shortcut{path: `C:\s.lnk`, name: "S"})
	if !strings.Contains(e.Exec, `s.lnk`) {
		t.Errorf("no-target Exec = %q", e.Exec)
	}
	// Target + args -> quoted target followed by args.
	e2 := entryForShortcut(shortcut{path: `C:\s.lnk`, name: "S", target: `C:\app.exe`, args: "--x"})
	if !strings.Contains(e2.Exec, `app.exe`) || !strings.HasSuffix(e2.Exec, "--x") {
		t.Errorf("target+args Exec = %q", e2.Exec)
	}
}

func TestWindowsIconBytesPaths(t *testing.T) {
	w := NewWindows(WindowsOptions{Dir: t.TempDir(), startDirs: []string{t.TempDir()}})
	f := filepath.Join(t.TempDir(), "thumb.png")
	want := pngOf(t, 8, 8)
	mustWrite(t, f, want)
	if b, ok := w.IconBytes(f); !ok || len(b) != len(want) {
		t.Errorf("abs read: ok=%v len=%d", ok, len(b))
	}
	if _, ok := w.IconBytes(filepath.Join(t.TempDir(), "absent.png")); ok {
		t.Error("absent abs path should miss")
	}
	if _, ok := w.IconBytes("unknown-name"); ok {
		t.Error("unknown bare name should miss")
	}
}

func TestWindowsThumbKey(t *testing.T) {
	w := NewWindows(WindowsOptions{Dir: t.TempDir(), startDirs: []string{t.TempDir()}})
	if k := w.ThumbKey(shell.FileItem{Name: "a.txt", Path: `C:\x\a.txt`, Mime: "text/plain"}); k != "" {
		t.Errorf("text ThumbKey = %q, want empty", k)
	}
	img := shell.FileItem{Name: "p.png", Path: `C:\x\p.png`, Mime: "image/png"}
	if k := w.ThumbKey(img); k != "" {
		t.Errorf("uncached ThumbKey = %q, want empty", k)
	}
	p := w.thumb.Path(img)
	if p == "" {
		t.Fatal("thumb path empty")
	}
	mustWrite(t, p, pngOf(t, 4, 4))
	t.Cleanup(func() { os.Remove(p) })
	if k := w.ThumbKey(img); k != p {
		t.Errorf("cached ThumbKey = %q, want %q", k, p)
	}
}

func TestWindowsDirUnreadable(t *testing.T) {
	w := NewWindows(WindowsOptions{Dir: filepath.Join(t.TempDir(), "does-not-exist"), startDirs: []string{t.TempDir()}})
	if w.Dir() != nil {
		t.Errorf("Dir() = %v, want nil", w.Dir())
	}
}

func TestWindowsMimeLoadFallback(t *testing.T) {
	orig := mimeLoad
	mimeLoad = func() (*mime.Database, error) { return nil, fmt.Errorf("no mime db") }
	t.Cleanup(func() { mimeLoad = orig })
	w := NewWindows(WindowsOptions{Dir: t.TempDir(), startDirs: []string{t.TempDir()}})
	if w.Resolver() == nil {
		t.Fatal("resolver nil after mime fallback")
	}
	_ = w.Resolve("x.txt", nil)
}
