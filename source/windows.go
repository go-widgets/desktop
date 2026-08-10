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

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/mime"
	"github.com/go-freedesktop/mimeapps"
	"github.com/go-thumbnail/thumbnail"
	"github.com/go-widgets/desktop/shell"
)

// Windows is the native Windows shell.AppSource: the Win32 peer of the macOS
// (Darwin) and XDG sources. It recursively scans the machine-wide and per-user
// Start Menu program trees for .lnk shortcuts, parses each shortcut's Shell Link
// binary (lnk.go) for its launch target, arguments and icon location, decodes
// that icon from its .ico container (ico.go) or from an .exe/.dll resource
// section (peicon.go), groups the shortcuts by their Start Menu subfolder (the
// natural Windows taxonomy) into the application menu, and lists the user
// profile directory into the file grid — all through the identical AppSource
// seam, so the exact same shell/render composition renders a populated desktop
// of REAL Windows apps.
//
// It only builds the app model; launching an app (the resolved target captured
// in each entry's Exec) is the shell's native-only job, never performed here.
type Windows struct {
	apps     *shell.AppIndex
	menu     *shell.MenuModel
	dir      *shell.Dir
	resolver *shell.Resolver
	thumb    *shell.Thumbnailer

	icons map[string][]byte // icon key (target path or .lnk path) -> encoded PNG
}

// compile-time assurance the windows source satisfies the seam.
var _ shell.AppSource = (*Windows)(nil)

// WindowsOptions parametrises the Start Menu scan.
type WindowsOptions struct {
	// Dir is the directory shown in the file grid (empty -> the user profile).
	Dir string
	// IconSize is retained for parity with the other sources; the .ico/PE
	// readers already pick the largest embedded representation, so it currently
	// only documents intent.
	IconSize int
	// startDirs overrides the scanned Start Menu directories (tests only).
	startDirs []string
}

// shortcut is the distilled content of one Start Menu .lnk.
type shortcut struct {
	path     string // the .lnk path
	name     string // display name = shortcut file name without .lnk
	target   string // resolved launch target
	args     string // command-line arguments
	category string // Start Menu subfolder (the taxonomy)
	iconKey  string
	iconPNG  []byte
}

// NewWindows scans the Start Menu program directories and builds the native
// source. Like NewDarwin it never fails: an unreadable directory, a malformed
// .lnk and a missing or undecodable icon each degrade gracefully rather than
// aborting the scan.
func NewWindows(o WindowsOptions) *Windows {
	w := &Windows{
		icons: map[string][]byte{},
		thumb: shell.NewThumbnailer(thumbnail.Normal),
	}

	shortcuts := scanShortcuts(o.startDirsOrDefault())
	entries := make([]*desktopentry.Entry, 0, len(shortcuts))
	byCat := map[string][]*desktopentry.Entry{}
	for _, s := range shortcuts {
		e := entryForShortcut(s)
		entries = append(entries, e)
		if s.iconPNG != nil {
			w.icons[s.iconKey] = s.iconPNG
		}
		byCat[s.category] = append(byCat[s.category], e)
	}
	w.apps = shell.NewAppIndex(entries)
	w.menu = shell.NewMenuModel(menuTree(byCat))

	db, err := mimeLoad()
	if err != nil {
		db = mime.New()
	}
	w.resolver = shell.NewResolver(db, mimeapps.Load())
	if dd, err := shell.ListDir(dirOrDefault(o.Dir)); err == nil {
		dd.Classify(w.resolver)
		w.dir = dd
		warmThumbs(w.thumb, dd)
	}
	return w
}

// startDirsOrDefault returns the configured Start Menu directories, or the two
// standard locations (the all-users program tree and the per-user one) when
// none are set. An unset environment variable simply drops that root.
func (o WindowsOptions) startDirsOrDefault() []string {
	if len(o.startDirs) > 0 {
		return o.startDirs
	}
	var dirs []string
	for _, env := range []string{"ProgramData", "AppData"} {
		if base := os.Getenv(env); base != "" {
			dirs = append(dirs, filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs"))
		}
	}
	return dirs
}

// scanShortcuts walks each Start Menu root recursively (in root order), distils
// every .lnk it finds, and returns the collected shortcuts. Unreadable roots and
// unreadable subtrees are skipped; the category is the top-level subfolder under
// the root ("Other" for a shortcut sitting directly in the root).
func scanShortcuts(roots []string) []shortcut {
	var out []shortcut
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".lnk") {
				return nil
			}
			out = append(out, readShortcut(path, categoryOf(root, path)))
			return nil
		})
	}
	return out
}

// categoryOf derives a shortcut's menu category from its path relative to the
// scanned root: the first path segment (the top-level Start Menu folder), or
// "Other" when the shortcut lives directly in the root.
func categoryOf(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "Other"
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" {
		return "Other"
	}
	return parts[0]
}

// readShortcut distils one .lnk into a shortcut. A missing/short/corrupt .lnk
// degrades to the shortcut base name with no target, icon or arguments; an icon
// that cannot be read or decoded degrades to no icon. iconKey is set only when
// an icon actually decoded, so an icon-less app's Entry.Icon stays empty and
// IconBytes misses cleanly instead of blitting the .lnk file's own bytes.
func readShortcut(path, category string) shortcut {
	s := shortcut{path: path, name: shortcutName(path), category: category}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	sl, err := parseLink(data)
	if err != nil {
		return s
	}
	s.target = sl.target
	s.args = sl.arguments
	if png := readWindowsIcon(firstNonEmpty(sl.iconLocation, sl.target)); png != nil {
		s.iconKey = firstNonEmpty(sl.target, path)
		s.iconPNG = png
	}
	return s
}

// readWindowsIcon loads and decodes a shortcut's icon to PNG bytes. The location
// may be an .ico file or an .exe/.dll whose resource section carries the icon;
// %VAR% environment references are expanded first. Anything unreadable or
// undecodable yields nil (the app renders with a placeholder).
func readWindowsIcon(loc string) []byte {
	loc = strings.Trim(loc, `"`)
	if loc == "" {
		return nil
	}
	loc = expandWinEnv(loc)
	data, err := os.ReadFile(loc)
	if err != nil {
		return nil
	}
	var png []byte
	if strings.EqualFold(filepath.Ext(loc), ".ico") {
		png, err = icoBestPNG(data)
	} else {
		png, err = peIconPNG(data)
	}
	if err != nil {
		return nil
	}
	return png
}

// expandWinEnv expands %VAR% references (the Windows form os.ExpandEnv does not
// understand) using the process environment; an undefined variable expands to
// the empty string, matching cmd.exe.
func expandWinEnv(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			b.WriteString(s)
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		b.WriteString(os.Getenv(s[i+1 : i+1+j]))
		s = s[i+1+j+1:]
	}
	return b.String()
}

// entryForShortcut projects a shortcut onto a desktop entry, exactly as the
// darwin and embedded sources do, so the identical shell.App path builds the
// index and menu. Exec is the resolved launch target (with arguments); it is
// non-blank so the app is launchable, but the windows source never executes it.
func entryForShortcut(s shortcut) *desktopentry.Entry {
	exec := s.target
	if exec == "" {
		exec = s.path
	}
	if s.args != "" {
		exec = fmt.Sprintf("%q %s", exec, s.args)
	} else {
		exec = fmt.Sprintf("%q", exec)
	}
	return &desktopentry.Entry{
		ID:         firstNonEmpty(s.target, s.path),
		Type:       "Application",
		Name:       s.name,
		Icon:       s.iconKey,
		Exec:       exec,
		Categories: []string{s.category},
	}
}

// shortcutName is the .lnk file name without its extension.
func shortcutName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// Apps returns the scanned app index.
func (w *Windows) Apps() *shell.AppIndex { return w.apps }

// Menu returns the subfolder-grouped application menu.
func (w *Windows) Menu() *shell.MenuModel { return w.menu }

// Dir returns the classified user-profile listing (nil when unreadable).
func (w *Windows) Dir() *shell.Dir { return w.dir }

// Resolver returns the MIME/association resolver (so the native binary can
// answer "open with" for an arbitrary path).
func (w *Windows) Resolver() *shell.Resolver { return w.resolver }

// Resolve answers "open with" for a bare name plus content bytes.
func (w *Windows) Resolve(name string, content []byte) shell.OpenWith {
	return w.resolver.ResolveName(name, content)
}

// IconBytes returns the PNG bytes for an app's icon key, or reads an absolute
// path (a thumbnail cache file) directly, and ok=false when neither resolves.
func (w *Windows) IconBytes(name string) ([]byte, bool) {
	if b, ok := w.icons[name]; ok {
		return b, true
	}
	if filepath.IsAbs(name) {
		if b, err := os.ReadFile(name); err == nil {
			return b, true
		}
	}
	return nil, false
}

// ThumbKey returns the on-disk thumbnail cache path for an image item when the
// thumbnail already exists, or "" otherwise (mirrors the other sources' policy).
func (w *Windows) ThumbKey(it shell.FileItem) string {
	p := w.thumb.Path(it)
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
