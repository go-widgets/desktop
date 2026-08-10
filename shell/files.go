// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MimeDirectory is the MIME type the shell assigns to directory entries.
const MimeDirectory = "inode/directory"

// FileItem is one entry of a listed directory.
type FileItem struct {
	Name    string
	Path    string
	IsDir   bool
	Mime    string
	Size    int64     // byte size of a regular file (0 for directories)
	ModTime time.Time // last-modification time (zero when unavailable)
}

// HumanSize renders the item's byte size for the list view: an em dash for a
// directory (Finder shows "--" there), otherwise a compact human-readable size.
func (it FileItem) HumanSize() string {
	if it.IsDir {
		return "--"
	}
	return HumanBytes(it.Size)
}

// HumanBytes formats a byte count as a compact human-readable string using
// French unit abbreviations (o, Ko, Mo, Go, To) and decimal (1000) steps, the
// convention macOS/Finder uses.
func HumanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d o", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := []string{"Ko", "Mo", "Go", "To", "Po"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}

// TypeLabel is the human-readable kind shown in the list view's Type column:
// "Dossier" for a directory, otherwise a short label derived from the MIME type
// (when classified) or the file-name extension.
func (it FileItem) TypeLabel() string {
	if it.IsDir {
		return "Dossier"
	}
	if it.Mime != "" && it.Mime != MimeDirectory {
		if s := mimeShortLabel(it.Mime); s != "" {
			return s
		}
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(it.Name)), ".")
	if ext == "" {
		return "Document"
	}
	return "Document " + strings.ToUpper(ext)
}

// mimeShortLabel maps a MIME type to a short French kind label, or "" when it
// has no special-cased label (the caller then falls back to the extension).
func mimeShortLabel(m string) string {
	switch {
	case strings.HasPrefix(m, "image/"):
		return "Image " + strings.ToUpper(strings.TrimPrefix(m, "image/"))
	case strings.HasPrefix(m, "video/"):
		return "Vidéo " + strings.ToUpper(strings.TrimPrefix(m, "video/"))
	case strings.HasPrefix(m, "audio/"):
		return "Audio " + strings.ToUpper(strings.TrimPrefix(m, "audio/"))
	case strings.HasPrefix(m, "text/"):
		return "Texte"
	case m == "application/pdf":
		return "Document PDF"
	}
	return ""
}

// ModTimeString formats the item's modification time for the list view's Date
// column ("--" when the time is unavailable).
func (it FileItem) ModTimeString() string {
	if it.ModTime.IsZero() {
		return "--"
	}
	return it.ModTime.Format("02/01/2006 15:04")
}

// Dir is a listed directory: its path and its items, directories first then
// files, each group ordered case-insensitively by name.
type Dir struct {
	Path  string
	Items []FileItem
}

// ListDir lists path into a Dir. Hidden entries (dotfiles: names beginning with
// ".", e.g. .ssh, .cache, .Trash) are skipped, so the file grid shows a clean
// user-facing directory rather than a wall of dotfile clutter. It does not
// classify MIME types (call Classify for that); a read failure (missing /
// unreadable directory) is returned as an error.
func ListDir(path string) (*Dir, error) {
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	d := &Dir{Path: path}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue // hidden / dotfile
		}
		it := FileItem{
			Name:  e.Name(),
			Path:  filepath.Join(path, e.Name()),
			IsDir: e.IsDir(),
		}
		// Size + modification time feed the list view's Taille/Date columns. A
		// stat failure (a race with a deleted entry, a permission quirk) leaves
		// the zero values, which HumanSize/ModTimeString render as "--".
		if info, err := e.Info(); err == nil {
			it.ModTime = info.ModTime()
			if !it.IsDir {
				it.Size = info.Size()
			}
		}
		d.Items = append(d.Items, it)
	}
	sort.SliceStable(d.Items, func(i, j int) bool {
		a, b := d.Items[i], d.Items[j]
		if a.IsDir != b.IsDir {
			return a.IsDir // directories first
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return d, nil
}

// ClassifyLite fills each item's Mime from its name/extension only, without the
// heavy content-sniffing Resolver — the portable classifier the file manager
// uses while navigating (a Resolver needs an XDG MIME database that does not
// exist on macOS/Windows or in the browser). Directories get MimeDirectory;
// regular files get an extension-derived MIME (or "" when the extension is
// unknown, which the Type column renders from the extension itself).
func (d *Dir) ClassifyLite() {
	for i := range d.Items {
		if d.Items[i].IsDir {
			d.Items[i].Mime = MimeDirectory
			continue
		}
		d.Items[i].Mime = MimeByExt(d.Items[i].Name)
	}
}

// MimeByExt maps a file name's extension to a MIME type by a small built-in
// table, or "" when the extension is not recognised. It covers the common
// image/video/audio/text/document kinds the file manager needs to pick an icon
// and a Type label; it never touches the filesystem.
func MimeByExt(name string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "bmp", "tiff", "tif", "heic":
		return "image/" + strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	case "mp4", "m4v", "mov", "avi", "mkv", "webm":
		return "video/" + strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	case "mp3", "wav", "flac", "aac", "ogg", "m4a":
		return "audio/" + strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	case "txt", "md", "log", "csv":
		return "text/plain"
	case "pdf":
		return "application/pdf"
	}
	return ""
}

// IsImage reports whether the item is a raster image the file manager can
// thumbnail (by MIME class when classified, else by name extension).
func (it FileItem) IsImage() bool {
	if strings.HasPrefix(it.Mime, "image/") {
		// SVG is not decoded by the raster thumbnailer path.
		return it.Mime != "image/svg+xml"
	}
	return IsImageName(it.Name)
}

// Classify fills each item's Mime using r: directories get MimeDirectory,
// regular files are classified by name+content. A file that cannot be resolved
// (e.g. it became unreadable) is left with an empty Mime rather than aborting
// the whole listing.
func (d *Dir) Classify(r *Resolver) {
	for i := range d.Items {
		if d.Items[i].IsDir {
			d.Items[i].Mime = MimeDirectory
			continue
		}
		if ow, err := r.ResolvePath(d.Items[i].Path); err == nil {
			d.Items[i].Mime = ow.MimeType
		}
	}
}
