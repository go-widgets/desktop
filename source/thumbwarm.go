// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import "github.com/go-widgets/desktop/shell"

// warmThumbs eagerly generates the real downscaled thumbnail for every image
// item in a freshly listed directory, so the native ThumbKey (which returns an
// existing on-disk cache path) resolves to a real preview on the first frame
// rather than the generic photo glyph. Each item is decoded + downscaled +
// cached once through the go-thumbnail cache (shell.Thumbnailer.Ensure); a
// non-image, an unreadable or an undecodable source degrades to no thumbnail
// (the grid keeps the drawn glyph) rather than failing the scan. It is one-time
// work bounded by the single directory's listing — the per-frame grid never
// touches the disk to generate.
func warmThumbs(t *shell.Thumbnailer, d *shell.Dir) {
	if t == nil || d == nil {
		return
	}
	for _, it := range d.Items {
		t.Ensure(it)
	}
}
