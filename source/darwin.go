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

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/mime"
	"github.com/go-freedesktop/mimeapps"
	"github.com/go-thumbnail/thumbnail"
	"github.com/go-widgets/desktop/shell"
)

// Darwin is the native macOS shell.AppSource: the darwin peer of the XDG source.
// It enumerates the .app bundles under the standard application directories,
// reads each bundle's Contents/Info.plist (binary or XML), decodes its .icns
// icon to a PNG, groups the apps by their LSApplicationCategoryType into the
// application menu, and lists the user's home directory into the file grid —
// exposing all of it through the identical AppSource seam, so the exact same
// shell/render composition renders a populated desktop of REAL macOS apps.
//
// It only builds the app model; launching an app (the `open -a` action captured
// in each entry's Exec) is the shell's native-only job, never performed here.
type Darwin struct {
	apps     *shell.AppIndex
	menu     *shell.MenuModel
	dir      *shell.Dir
	resolver *shell.Resolver
	thumb    *shell.Thumbnailer

	icons map[string][]byte // icon key (bundle id or path) -> encoded PNG
}

// compile-time assurance the darwin source satisfies the seam.
var _ shell.AppSource = (*Darwin)(nil)

// DarwinOptions parametrises the macOS scan.
type DarwinOptions struct {
	// Dir is the directory shown in the file grid (empty -> the user home).
	Dir string
	// IconSize is the nominal pixel size the shell displays app icons at. It is
	// passed to codec.DecodeBest, which selects the .icns representation best
	// matching it (the smallest at least that large, else the largest). Zero
	// selects the largest representation.
	IconSize int
	// appDirs overrides the scanned application directories (tests only).
	appDirs []string
}

// bundleInfo is the distilled Info.plist of one .app bundle.
type bundleInfo struct {
	path     string
	name     string
	id       string
	iconKey  string
	iconPNG  []byte
	category string
}

// NewDarwin scans the macOS application directories and builds the native
// source. Like NewXDG it never fails: an unreadable directory, a bundle with no
// Info.plist and a missing or non-PNG icon each degrade gracefully rather than
// aborting the scan.
func NewDarwin(o DarwinOptions) *Darwin {
	d := &Darwin{
		icons: map[string][]byte{},
		thumb: shell.NewThumbnailer(thumbnail.Normal),
	}

	bundles := scanBundles(o.appDirsOrDefault(), o.IconSize)
	entries := make([]*desktopentry.Entry, 0, len(bundles))
	byCat := map[string][]*desktopentry.Entry{}
	for _, b := range bundles {
		e := entryForBundle(b)
		entries = append(entries, e)
		if b.iconPNG != nil {
			d.icons[b.iconKey] = b.iconPNG
		}
		byCat[b.category] = append(byCat[b.category], e)
	}
	d.apps = shell.NewAppIndex(entries)
	d.menu = shell.NewMenuModel(menuTree(byCat))

	db, err := mimeLoad()
	if err != nil {
		db = mime.New()
	}
	d.resolver = shell.NewResolver(db, mimeapps.Load())
	if dd, err := shell.ListDir(dirOrDefault(o.Dir)); err == nil {
		dd.Classify(d.resolver)
		d.dir = dd
		warmThumbs(d.thumb, dd)
	}
	return d
}

// appDirsOrDefault returns the configured application directories, or the four
// standard macOS locations when none are set.
func (o DarwinOptions) appDirsOrDefault() []string {
	if len(o.appDirs) > 0 {
		return o.appDirs
	}
	dirs := []string{"/Applications", "/System/Applications", "/System/Applications/Utilities"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// scanBundles enumerates the *.app bundles in each directory (in directory
// order), reads and distils each bundle's Info.plist, and returns the collected
// bundle infos. Unreadable directories are skipped.
func scanBundles(dirs []string, iconSize int) []bundleInfo {
	var out []bundleInfo
	for _, dir := range dirs {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range ents {
			if !strings.HasSuffix(ent.Name(), ".app") {
				continue
			}
			out = append(out, readBundle(filepath.Join(dir, ent.Name()), iconSize))
		}
	}
	return out
}

// readBundle distils one .app bundle into a bundleInfo. A missing or malformed
// Info.plist degrades to the bundle's base name with no id, icon or category; a
// missing or non-PNG icon degrades to no icon.
func readBundle(path string, iconSize int) bundleInfo {
	b := bundleInfo{path: path, name: bundleBaseName(path), category: "Other"}
	data, err := os.ReadFile(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		b.iconKey = path
		return b
	}
	info, err := parsePlist(data)
	if err != nil {
		b.iconKey = path
		return b
	}
	if n := firstNonEmpty(plistString(info, "CFBundleDisplayName"), plistString(info, "CFBundleName")); n != "" {
		b.name = n
	}
	b.id = plistString(info, "CFBundleIdentifier")
	b.category = categoryFor(plistString(info, "LSApplicationCategoryType"))
	b.iconKey = firstNonEmpty(b.id, path)
	b.iconPNG = readIcon(path, plistString(info, "CFBundleIconFile"), iconSize)
	return b
}

// readIcon loads the bundle's .icns icon (Contents/Resources/<iconFile>) and
// decodes the representation best matching iconSize to PNG bytes. An empty icon
// name, an unreadable file or an icns with no decodable variant yields nil (the
// app renders with a placeholder).
func readIcon(bundlePath, iconFile string, iconSize int) []byte {
	if iconFile == "" {
		return nil
	}
	if !strings.EqualFold(filepath.Ext(iconFile), ".icns") {
		iconFile += ".icns"
	}
	data, err := os.ReadFile(filepath.Join(bundlePath, "Contents", "Resources", iconFile))
	if err != nil {
		return nil
	}
	png, err := iconPNG(data, iconSize)
	if err != nil {
		return nil
	}
	return png
}

// entryForBundle projects a bundle onto a desktop entry, exactly as the embedded
// source does for its curated apps, so the identical shell.App path builds the
// index and menu. Exec is the macOS launch action (`open -a <bundle>`); it is
// non-blank so the app is launchable, but the darwin source never executes it.
func entryForBundle(b bundleInfo) *desktopentry.Entry {
	return &desktopentry.Entry{
		ID:         firstNonEmpty(b.id, b.path),
		Type:       "Application",
		Name:       b.name,
		Icon:       b.iconKey,
		Exec:       fmt.Sprintf("open -a %q", b.path),
		Categories: []string{b.category},
	}
}

// bundleBaseName is the bundle directory name without its .app suffix.
func bundleBaseName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".app")
}

// lsCategories maps Apple's LSApplicationCategoryType uniform-type identifiers
// onto user-visible menu categories. An unknown or absent value falls through to
// "Other".
var lsCategories = map[string]string{
	"public.app-category.developer-tools":    "Developer",
	"public.app-category.business":           "Business",
	"public.app-category.education":          "Education",
	"public.app-category.entertainment":      "Entertainment",
	"public.app-category.finance":            "Finance",
	"public.app-category.games":              "Games",
	"public.app-category.graphics-design":    "Graphics",
	"public.app-category.healthcare-fitness": "Health",
	"public.app-category.lifestyle":          "Lifestyle",
	"public.app-category.medical":            "Medical",
	"public.app-category.music":              "Music",
	"public.app-category.news":               "News",
	"public.app-category.photography":        "Graphics",
	"public.app-category.productivity":       "Productivity",
	"public.app-category.reference":          "Reference",
	"public.app-category.social-networking":  "Social",
	"public.app-category.sports":             "Sports",
	"public.app-category.travel":             "Travel",
	"public.app-category.utilities":          "Utilities",
	"public.app-category.video":              "Video",
	"public.app-category.weather":            "Weather",
}

// categoryFor maps an LSApplicationCategoryType to a menu category. Games have
// per-genre identifiers (public.app-category.games.*); any such prefix folds to
// "Games". Everything else falls through the table to "Other".
func categoryFor(ls string) string {
	if ls == "" {
		return "Other"
	}
	if c, ok := lsCategories[ls]; ok {
		return c
	}
	if strings.HasPrefix(ls, "public.app-category.games.") {
		return "Games"
	}
	return "Other"
}

// Apps returns the scanned app index.
func (d *Darwin) Apps() *shell.AppIndex { return d.apps }

// Menu returns the category-grouped application menu.
func (d *Darwin) Menu() *shell.MenuModel { return d.menu }

// Dir returns the classified home-directory listing (nil when unreadable).
func (d *Darwin) Dir() *shell.Dir { return d.dir }

// Resolver returns the MIME/association resolver (so the native binary can
// answer "open with" for an arbitrary path).
func (d *Darwin) Resolver() *shell.Resolver { return d.resolver }

// Resolve answers "open with" for a bare name plus content bytes.
func (d *Darwin) Resolve(name string, content []byte) shell.OpenWith {
	return d.resolver.ResolveName(name, content)
}

// IconBytes returns the PNG bytes for an app's icon key, or reads an absolute
// path (a thumbnail cache file) directly, and ok=false when neither resolves.
func (d *Darwin) IconBytes(name string) ([]byte, bool) {
	if b, ok := d.icons[name]; ok {
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
// thumbnail already exists, or "" otherwise (mirrors the XDG policy).
func (d *Darwin) ThumbKey(it shell.FileItem) string {
	p := d.thumb.Path(it)
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
