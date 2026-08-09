// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package source

import (
	"os"
	"path/filepath"

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/icontheme"
	"github.com/go-freedesktop/menu"
	"github.com/go-freedesktop/mime"
	"github.com/go-freedesktop/mimeapps"
	"github.com/go-thumbnail/thumbnail"
	"github.com/go-widgets/desktop/shell"
)

// XDGOptions parametrises the native filesystem scan.
type XDGOptions struct {
	// Dir is the directory shown in the file grid (empty -> the user home,
	// then the working directory, then the filesystem root).
	Dir string
	// IconTheme is the icon-theme name (empty -> hicolor).
	IconTheme string
	// IconSize is the nominal icon size requested from the theme (0 ->
	// render.DefaultIconSize's 48).
	IconSize int
}

// XDG is the native shell.AppSource: it scans a real XDG filesystem exactly as
// the shell always has — desktopentry.Scan for the app index, menu.Load for the
// categorized menu, mime + mimeapps for "open with", go-thumbnail for
// thumbnails and an icontheme.Theme for icons — and exposes it through the
// portable AppSource seam so the identical shell/render composition runs on a
// real Linux desktop and in the browser alike.
type XDG struct {
	apps     *shell.AppIndex
	menu     *shell.MenuModel
	dir      *shell.Dir
	resolver *shell.Resolver
	thumb    *shell.Thumbnailer

	theme *icontheme.Theme
	size  int
	scale int
}

// compile-time assurance the native source satisfies the seam.
var _ shell.AppSource = (*XDG)(nil)

// NewXDG scans the filesystem and builds the native source. It never fails: a
// missing MIME database, unreadable menu or unreadable directory each degrade
// to an empty-but-valid model, exactly as the shell's original buildScene did.
func NewXDG(o XDGOptions) *XDG {
	size := o.IconSize
	if size <= 0 {
		size = 48
	}
	x := &XDG{
		apps:  shell.NewAppIndex(desktopentry.Scan()),
		thumb: shell.NewThumbnailer(thumbnail.Normal),
		theme: icontheme.New(themeName(o.IconTheme)),
		size:  size,
		scale: 1,
	}

	if tree, err := menu.Load(); err == nil {
		x.menu = shell.NewMenuModel(tree)
	} else {
		x.menu = shell.NewMenuModel(nil)
	}

	db, err := mime.Load()
	if err != nil {
		db = mime.New()
	}
	x.resolver = shell.NewResolver(db, mimeapps.Load())

	if d, err := shell.ListDir(dirOrDefault(o.Dir)); err == nil {
		d.Classify(x.resolver)
		x.dir = d
	}
	return x
}

// themeName defaults an empty icon-theme name to hicolor.
func themeName(name string) string {
	if name == "" {
		return icontheme.HicolorTheme
	}
	return name
}

// dirOrDefault resolves the file-grid directory: the given path, else the user
// home, else the working directory, else the filesystem root.
func dirOrDefault(dir string) string {
	if dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return string(filepath.Separator)
}

// Apps returns the scanned app index.
func (x *XDG) Apps() *shell.AppIndex { return x.apps }

// Menu returns the loaded application menu.
func (x *XDG) Menu() *shell.MenuModel { return x.menu }

// Dir returns the classified directory listing (nil when unreadable).
func (x *XDG) Dir() *shell.Dir { return x.dir }

// Resolver returns the MIME/association resolver (so the native binary can
// answer "open with" for an arbitrary path).
func (x *XDG) Resolver() *shell.Resolver { return x.resolver }

// Resolve answers "open with" for a bare name plus content bytes.
func (x *XDG) Resolve(name string, content []byte) shell.OpenWith {
	return x.resolver.ResolveName(name, content)
}

// IconBytes resolves an icon name (or absolute path) to its encoded file bytes
// through the icon theme, and ok=false when it cannot be found or read.
func (x *XDG) IconBytes(name string) ([]byte, bool) {
	path := name
	if !filepath.IsAbs(name) {
		p, err := x.theme.FindIcon([]string{name, "application-x-executable"}, x.size, x.scale)
		if err != nil {
			return nil, false
		}
		path = p
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return b, true
}

// ThumbKey returns the on-disk thumbnail cache path for an image item when the
// thumbnail has already been generated, or "" otherwise (a directory, a non-
// image, or a not-yet-thumbnailed file).
func (x *XDG) ThumbKey(it shell.FileItem) string {
	p := x.thumb.Path(it)
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
