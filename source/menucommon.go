// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"sort"

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/menu"
	"github.com/go-freedesktop/mime"
)

// mimeLoad is a seam over mime.Load so the "no shared MIME database" fallback is
// testable deterministically, independent of the host's data dirs. Both native
// scanners (darwin, windows) share it.
var mimeLoad = mime.Load

// This file holds the small helpers the native scanners share when they project
// a filesystem of apps onto the shell model — the macOS (.app bundle) and
// Windows (.lnk shortcut) sources build the identical category-grouped menu.Tree
// and both pick a first-non-empty string, so the logic lives here once,
// untagged, rather than duplicated per GOOS.

// menuTree builds a resolved menu.Tree from the per-category app groups, ordered
// by category name for a deterministic menu, so shell.NewMenuModel flattens it
// exactly as the native menu.Load path does.
func menuTree(byCat map[string][]*desktopentry.Entry) *menu.Tree {
	cats := make([]string, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	root := &menu.Menu{Name: "Applications"}
	for _, c := range cats {
		root.Submenus = append(root.Submenus, &menu.Menu{
			Name:          c,
			DirectoryName: c,
			Apps:          byCat[c],
		})
	}
	return &menu.Tree{Root: root}
}

// firstNonEmpty returns the first non-empty argument, or "".
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
