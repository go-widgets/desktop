// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// columnView is a thin shell adapter over toolkit.ColumnBrowser (the Miller /
// "Colonnes" widget the toolkit now ships). It implements the widget's
// ColumnProvider over the shell's directory lister: each directory becomes a
// column of type-icon + name rows (a folder carries a disclosure chevron and
// opens the next column; a file opens the preview pane), and the whole column
// cascade, horizontal scroll and preview come from the toolkit widget unchanged.
type columnView struct {
	toolkit.Base

	browser *toolkit.ColumnBrowser
	lister  func(path string) (*shell.Dir, error)
	iconFn  func(shell.FileItem) *toolkit.Image
	openFn  func(shell.FileItem)

	colW    int
	rootKey string

	// items maps a node key (path) back to its FileItem so Preview can render the
	// leaf's kind + size and a re-pick can open the real item.
	items map[string]shell.FileItem
}

// cbRowHeight mirrors toolkit.ColumnBrowser's per-row height, so cascadeFirst's
// synthetic row clicks land on the intended rows.
const cbRowHeight = 26

// millerColW is the default column / preview width (mirrors the widget default).
const millerColW = 220

// newColumnView builds a Miller view over lister. iconFn resolves an item's
// small leading type icon; openFn opens a re-picked leaf.
func newColumnView(lister func(string) (*shell.Dir, error),
	iconFn func(shell.FileItem) *toolkit.Image, openFn func(shell.FileItem)) *columnView {
	cv := &columnView{
		lister: lister,
		iconFn: iconFn,
		openFn: openFn,
		colW:   millerColW,
		items:  map[string]shell.FileItem{},
	}
	cv.browser = toolkit.NewColumnBrowser(cv)
	cv.browser.OnActivate = cv.activate
	return cv
}

// Children implements toolkit.ColumnProvider: it lists key via the shell lister
// and projects the directory into column nodes (folders are containers), or
// rejects the key (ok=false) when the listing fails.
func (cv *columnView) Children(key string) ([]toolkit.ColumnNode, bool) {
	dir, err := cv.lister(key)
	if err != nil || dir == nil {
		return nil, false
	}
	nodes := make([]toolkit.ColumnNode, len(dir.Items))
	for i, it := range dir.Items {
		cv.items[it.Path] = it
		nodes[i] = toolkit.ColumnNode{Name: it.Name, Key: it.Path, Icon: cv.iconFn(it), Container: it.IsDir}
	}
	return nodes, true
}

// Preview implements toolkit.ColumnProvider: the leaf's kind and human size,
// shown under its name in the preview pane.
func (cv *columnView) Preview(node toolkit.ColumnNode) []string {
	it, ok := cv.items[node.Key]
	if !ok {
		return nil
	}
	return []string{it.TypeLabel(), it.HumanSize()}
}

// activate opens the re-picked leaf behind a node (the ColumnBrowser fires
// OnActivate on a second pick of the same file).
func (cv *columnView) activate(node toolkit.ColumnNode) {
	if it, ok := cv.items[node.Key]; ok && cv.openFn != nil {
		cv.openFn(it)
	}
}

// SetRoot resets the strip to a single column listing rootPath.
func (cv *columnView) SetRoot(rootPath string) {
	cv.rootKey = rootPath
	cv.browser.SetRoot(rootPath)
}

// SetBounds records bounds and forwards them to the browser.
func (cv *columnView) SetBounds(r toolkit.Rect) {
	cv.Base.SetBounds(r)
	cv.browser.SetBounds(r)
}

// Draw paints the column strip + preview pane.
func (cv *columnView) Draw(p painter.Painter, th *toolkit.Theme) { cv.browser.Draw(p, th) }

// OnEvent forwards a click/scroll to the browser (which routes it to the column
// under the pointer).
func (cv *columnView) OnEvent(ev toolkit.Event) { cv.browser.OnEvent(ev) }

// ColumnCount is the number of open directory columns (for tests / screenshots).
func (cv *columnView) ColumnCount() int { return cv.browser.ColumnCount() }

// cascadeFirst opens the first sub-directory of each rightmost column until
// maxCols columns show (or no deeper folder exists) — the folder chain a user
// builds by clicking down a folder tree, used to populate the column-view
// screenshot. It drives the ColumnBrowser through synthetic row clicks (the
// widget exposes no programmatic pick), walking the first-folder chain via the
// lister so it knows which row to click in the newest column.
func (cv *columnView) cascadeFirst(maxCols int) {
	// Cascade under a synthetic, left-anchored layout wide and tall enough that
	// no horizontal scroll kicks in and every column's first rows are visible, so
	// a click on the deepest column's first-folder row lands predictably whatever
	// the pane's real (possibly not-yet-assigned) bounds are. The caller re-lays
	// the view to its real bounds immediately afterwards (open columns persist).
	cv.SetBounds(toolkit.Rect{W: (maxCols + 1) * cv.colW, H: 64 * cbRowHeight})
	key := cv.rootKey
	for cv.ColumnCount() > 0 && cv.ColumnCount() < maxCols {
		dir, err := cv.lister(key)
		if err != nil || dir == nil {
			break
		}
		row, child, ok := firstContainer(dir)
		if !ok {
			break
		}
		before := cv.ColumnCount()
		// Click the first-folder row in the rightmost (deepest) column, in the
		// widget-local coordinates the ColumnBrowser routes: X inside the deepest
		// column band (left-anchored, so column ci starts at ci*colW), Y at the
		// row centre (0-based from the strip top).
		ci := cv.ColumnCount() - 1
		cv.browser.OnEvent(toolkit.Event{
			Kind: toolkit.EventClick,
			X:    ci*cv.colW + cv.colW/2,
			Y:    row*cbRowHeight + cbRowHeight/2,
		})
		if cv.ColumnCount() == before {
			break // no new column opened; avoid a spin
		}
		key = child.Path
	}
}

// firstContainer returns the row index and item of the first directory entry in
// dir, or ok=false when the directory has no sub-directory.
func firstContainer(dir *shell.Dir) (int, shell.FileItem, bool) {
	for i, it := range dir.Items {
		if it.IsDir {
			return i, it, true
		}
	}
	return 0, shell.FileItem{}, false
}

// columnView implements toolkit.ColumnProvider.
var _ toolkit.ColumnProvider = (*columnView)(nil)
