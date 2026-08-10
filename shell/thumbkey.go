// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"path/filepath"
	"strings"

	"github.com/go-thumbnail/thumbnail"
)

// imageExts are the file extensions treated as image files for thumbnailing —
// the reliable last resort where MIME classification is weak, notably on macOS
// (no shared MIME database, so a plain .png lands as application/octet-stream)
// and in any listing whose items were never content-classified.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".bmp": true, ".tif": true, ".tiff": true, ".heic": true, ".heif": true,
	".ico": true, ".avif": true,
}

// IsImageName reports whether name has a known image file extension. It is the
// single source of truth for extension-based image detection, shared by the
// thumbnail-eligibility policy here and the render layer's picture-glyph
// fallback, so the two never drift.
func IsImageName(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

// Thumbnailable reports whether a file item should be given an image preview: a
// regular file that is an image, recognised either by its classified MIME type
// (image/*) or, when MIME classification is unavailable or weak, by a known
// image file extension — so a macOS .png classified as application/octet-stream
// is still thumbnailed. Directories and non-image files are never thumbnailed.
func Thumbnailable(it FileItem) bool {
	if it.IsDir {
		return false
	}
	return strings.HasPrefix(it.Mime, "image/") || IsImageName(it.Name)
}

// Thumbnailer derives the freedesktop Thumbnail Managing Standard cache keys
// and paths for a file grid. It delegates the URI canonicalization, MD5 hash,
// cache-path layout and the actual decode+downscale+cache to
// github.com/go-thumbnail/thumbnail (the owner of that mechanism) and adds only
// the shell's policy: which items are thumbnailed and under which stable key.
type Thumbnailer struct {
	cache *thumbnail.Cache
	// memo caches the generation outcome per source path — a produced
	// thumbnail's cache path, or "" for a non-thumbnailable / unproducible item
	// — so the per-frame file grid can call Ensure every draw without re-hitting
	// the disk once an item has been resolved.
	memo map[string]string
}

// NewThumbnailer builds a Thumbnailer for the given standard thumbnail size.
func NewThumbnailer(size thumbnail.Size) *Thumbnailer {
	return &Thumbnailer{cache: thumbnail.New(size), memo: map[string]string{}}
}

// Ensure generates (or refreshes) the item's thumbnail through the go-thumbnail
// cache — decode the source image, downscale it into the standard size bucket
// and write the freedesktop-keyed PNG under $XDG_CACHE_HOME/thumbnails — and
// returns its on-disk cache path (the IconBytes key the render layer decodes),
// or "" when the item is not thumbnailable or the thumbnail could not be
// produced (an unreadable or undecodable source). The outcome is memoized per
// source path, positive and negative, so a repeated call is a map lookup.
func (t *Thumbnailer) Ensure(it FileItem) string {
	if !Thumbnailable(it) {
		return ""
	}
	if p, ok := t.memo[it.Path]; ok {
		return p
	}
	p, err := t.cache.Get(thumbnail.FileURI(it.Path))
	if err != nil {
		p = ""
	}
	t.memo[it.Path] = p
	return p
}

// Key returns the stable cache key (the MD5 hash of the file's canonical
// file:// URI) for a thumbnailable item, or "" when the item is not
// thumbnailable. Two items with the same path always produce the same key.
func (t *Thumbnailer) Key(it FileItem) string {
	if !Thumbnailable(it) {
		return ""
	}
	return thumbnail.Hash(thumbnail.FileURI(it.Path))
}

// Path returns the on-disk cache path where the item's thumbnail lives (or
// would be generated), or "" for a non-thumbnailable item.
func (t *Thumbnailer) Path(it FileItem) string {
	if !Thumbnailable(it) {
		return ""
	}
	return t.cache.Path(it.Path)
}
