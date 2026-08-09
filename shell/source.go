// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

// AppSource is the data source behind the desktop shell: everything the UI
// needs to render a populated desktop, abstracted away from where it comes
// from. It supplies the launchable-app index, the categorized application
// menu, the current directory listing (already MIME-classified), the "open
// with" resolution for a file, the encoded image bytes for an icon (by theme
// name or absolute path), and — for a file item — the icon-loader key of its
// thumbnail (or "" when it has none).
//
// The two implementations live in github.com/go-widgets/desktop/source:
//
//   - the native xdgSource scans a real XDG filesystem (desktopentry.Scan +
//     icontheme + mime/mimeapps + menu.Load + go-thumbnail), exactly the
//     behavior the shell has always had; and
//   - the embeddedSource serves a curated set from an embed.FS, so the shell
//     renders a real, populated desktop in the browser (js/wasm), where no
//     real filesystem exists.
//
// Both drive the identical shell/render composition logic — the whole point of
// the seam — so a Scene built from either is indistinguishable to the toolkit.
type AppSource interface {
	// Apps is the sorted, de-duplicated, searchable launchable-app index that
	// feeds the dock and launcher.
	Apps() *AppIndex
	// Menu is the flattened, ordered application menu.
	Menu() *MenuModel
	// Dir is the current directory listing, already MIME-classified, that
	// feeds the file grid. It may be nil when no directory is available.
	Dir() *Dir
	// Resolve answers the "open with" query for a bare file name plus optional
	// leading content bytes: the classified MIME type, its default application
	// and the ordered candidate applications.
	Resolve(name string, content []byte) OpenWith
	// IconBytes returns the encoded image bytes (PNG/JPEG/GIF) for an icon
	// referenced by theme name (e.g. "web-browser") or by absolute path (e.g.
	// a thumbnail), and ok=false when it cannot be resolved. The render layer
	// decodes the bytes into a go-widgets Image (falling back to a placeholder
	// swatch on ok=false), so this is the single icon-pixel seam a source must
	// satisfy — no source touches the toolkit.
	IconBytes(name string) ([]byte, bool)
	// ThumbKey returns the IconBytes key of a file item's thumbnail — an
	// absolute cache path for the native source, a virtual asset name for the
	// embedded one — or "" when the item is not thumbnailed.
	ThumbKey(it FileItem) string
}
