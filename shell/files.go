// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MimeDirectory is the MIME type the shell assigns to directory entries.
const MimeDirectory = "inode/directory"

// FileItem is one entry of a listed directory.
type FileItem struct {
	Name  string
	Path  string
	IsDir bool
	Mime  string
}

// Dir is a listed directory: its path and its items, directories first then
// files, each group ordered case-insensitively by name.
type Dir struct {
	Path  string
	Items []FileItem
}

// ListDir lists path into a Dir. It does not classify MIME types (call
// Classify for that); a read failure (missing / unreadable directory) is
// returned as an error.
func ListDir(path string) (*Dir, error) {
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	d := &Dir{Path: path}
	for _, e := range ents {
		d.Items = append(d.Items, FileItem{
			Name:  e.Name(),
			Path:  filepath.Join(path, e.Name()),
			IsDir: e.IsDir(),
		})
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
