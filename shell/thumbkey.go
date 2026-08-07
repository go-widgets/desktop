// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"strings"

	"github.com/go-thumbnail/thumbnail"
)

// Thumbnailable reports whether a file item should be given an image preview:
// a regular file whose classified MIME type is an image/* type. Directories,
// unclassified items and non-image files are not thumbnailed.
func Thumbnailable(it FileItem) bool {
	return !it.IsDir && strings.HasPrefix(it.Mime, "image/")
}

// Thumbnailer derives the freedesktop Thumbnail Managing Standard cache keys
// and paths for a file grid. It delegates the URI canonicalization, MD5 hash
// and cache-path layout to github.com/go-thumbnail/thumbnail (the owner of that
// mechanism) and adds only the shell's policy: which items are thumbnailed and
// under which stable key.
type Thumbnailer struct {
	cache *thumbnail.Cache
}

// NewThumbnailer builds a Thumbnailer for the given standard thumbnail size.
func NewThumbnailer(size thumbnail.Size) *Thumbnailer {
	return &Thumbnailer{cache: thumbnail.New(size)}
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
