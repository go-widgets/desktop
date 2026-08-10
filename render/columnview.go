// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// columnView is the file manager's "Colonnes" (Miller) mode: each directory in
// the current path is a vertical column of names; selecting a folder opens the
// next column to its right, and the strip scrolls horizontally to keep the
// deepest columns in view. A selected file opens a compact info/preview column.
//
// The toolkit has no Miller/column widget (a flagged gap — see
// FINDER-TOOLKIT-GAPS.md), so columnView composes one column per directory from
// a toolkit.ListBox (whose per-row ItemRenderer draws the type icon + name + a
// disclosure chevron for folders) and lays them side by side itself.
type columnView struct {
	toolkit.Base

	theme  *toolkit.Theme
	lister func(path string) (*shell.Dir, error)
	iconFn func(shell.FileItem) *toolkit.Image
	openFn func(shell.FileItem)

	cols    []*millerCol
	colW    int
	scrollX int

	// reusable per-row icon Image, bounds mutated per row.
	rowIcon *toolkit.Image

	// preview is the compact info column shown when a non-directory is selected.
	preview *shell.FileItem
}

// millerCol is one directory column: its listing plus the ListBox that renders
// it. selected is the row index chosen in this column (drives the next column).
type millerCol struct {
	path     string
	dir      *shell.Dir
	list     *toolkit.ListBox
	selected int
}

const millerColW = 220

// newColumnView builds a Miller view rooted at rootPath, listing it into the
// first column.
func newColumnView(th *toolkit.Theme, lister func(string) (*shell.Dir, error),
	iconFn func(shell.FileItem) *toolkit.Image, openFn func(shell.FileItem)) *columnView {
	cv := &columnView{
		theme:   th,
		lister:  lister,
		iconFn:  iconFn,
		openFn:  openFn,
		colW:    millerColW,
		rowIcon: toolkit.NewImageFit(nil, 0, 0),
	}
	return cv
}

// SetRoot resets the strip to a single column listing rootPath.
func (cv *columnView) SetRoot(rootPath string) {
	cv.cols = nil
	cv.preview = nil
	cv.scrollX = 0
	if col := cv.makeColumn(rootPath); col != nil {
		cv.cols = []*millerCol{col}
	}
	cv.relayout()
}

// makeColumn lists path and builds its column (or nil when the listing fails —
// e.g. a permission error or a non-directory path).
func (cv *columnView) makeColumn(path string) *millerCol {
	dir, err := cv.lister(path)
	if err != nil || dir == nil {
		return nil
	}
	col := &millerCol{path: path, dir: dir, selected: -1}
	lb := toolkit.NewListBox(nil)
	lb.RowHeight = 26
	names := make([]string, len(dir.Items))
	for i, it := range dir.Items {
		names[i] = it.Name
	}
	lb.Items = names
	idx := len(cv.cols) // this column's position in the strip
	lb.ItemRenderer = func(p painter.Painter, th *toolkit.Theme, rc toolkit.Rect, index int, item string, selected bool, ink toolkit.RGBA) {
		cv.drawColRow(p, th, rc, dir, index, ink, selected)
	}
	lb.OnActivate = func(row int) { cv.onPick(idx, row) }
	col.list = lb
	return col
}

// onPick handles a row activation in column ci: a folder truncates the strip
// after ci and opens the folder in a fresh column to the right; a file drops any
// deeper columns and shows the preview (and opens on a re-pick).
func (cv *columnView) onPick(ci, row int) {
	if ci < 0 || ci >= len(cv.cols) {
		return
	}
	col := cv.cols[ci]
	if row < 0 || row >= len(col.dir.Items) {
		return
	}
	item := col.dir.Items[row]
	repick := col.selected == row
	col.selected = row
	col.list.Selected = row
	cv.cols = cv.cols[:ci+1]

	if item.IsDir {
		cv.preview = nil
		if next := cv.makeColumn(item.Path); next != nil {
			cv.cols = append(cv.cols, next)
		}
	} else {
		cv.preview = &item
		if repick && cv.openFn != nil {
			cv.openFn(item)
		}
	}
	cv.relayout()
}

// drawColRow renders one column row: the small type icon, the name, and a
// disclosure chevron on the right for a folder.
func (cv *columnView) drawColRow(p painter.Painter, th *toolkit.Theme, rc toolkit.Rect, dir *shell.Dir, index int, ink toolkit.RGBA, selected bool) {
	if index < 0 || index >= len(dir.Items) {
		return
	}
	it := dir.Items[index]
	const pad, iconPx = 8, 16
	if img := cv.iconFn(it); img != nil {
		iy := rc.Y + (rc.H-iconPx)/2
		img.SetBounds(toolkit.Rect{X: rc.X + pad, Y: iy, W: iconPx, H: iconPx})
		img.Draw(p, th)
	}
	tx := rc.X + pad + iconPx + pad
	ty := rc.Y + (rc.H-toolkit.GlyphHeight())/2
	toolkit.DrawText(p, tx, ty, elide(it.Name, (cv.colW-40)/7), ink)
	if it.IsDir {
		// Disclosure chevron ">" at the right edge.
		chevX := rc.X + rc.W - pad - 6
		cy := rc.Y + rc.H/2
		for d := 0; d < 5; d++ {
			p.FillRect(toolkit.Rect{X: chevX + d, Y: cy - (4 - d), W: 1, H: 1}, ink)
			p.FillRect(toolkit.Rect{X: chevX + d, Y: cy + (4 - d), W: 1, H: 1}, ink)
		}
	}
}

// SetBounds records bounds and lays out the columns.
func (cv *columnView) SetBounds(r toolkit.Rect) {
	cv.Base.SetBounds(r)
	cv.relayout()
}

// visibleWidth is the total pixel width of all columns plus the preview.
func (cv *columnView) contentWidth() int {
	w := len(cv.cols) * cv.colW
	if cv.preview != nil {
		w += cv.colW
	}
	return w
}

// relayout positions each column left to right, auto-scrolling so the deepest
// columns stay in view.
func (cv *columnView) relayout() {
	b := cv.Bounds()
	if b.W <= 0 {
		return
	}
	// Anchor right: keep the newest columns visible.
	cv.scrollX = cv.contentWidth() - b.W
	if cv.scrollX < 0 {
		cv.scrollX = 0
	}
	x := b.X - cv.scrollX
	for _, col := range cv.cols {
		col.list.SetBounds(toolkit.Rect{X: x, Y: b.Y, W: cv.colW, H: b.H})
		x += cv.colW
	}
}

// Draw paints the columns, their separators and the preview pane, clipped to
// the view bounds so the horizontally-scrolled strip stays within its region.
func (cv *columnView) Draw(p painter.Painter, th *toolkit.Theme) {
	b := cv.Bounds()
	p.FillRect(b, th.Surface)
	clipTo(p, b, func() {
		x := b.X - cv.scrollX
		for _, col := range cv.cols {
			col.list.Draw(p, th)
			// Column separator hairline.
			p.FillRect(toolkit.Rect{X: x + cv.colW - 1, Y: b.Y, W: 1, H: b.H}, th.Border)
			x += cv.colW
		}
		if cv.preview != nil {
			cv.drawPreview(p, th, toolkit.Rect{X: x, Y: b.Y, W: cv.colW, H: b.H})
		}
	})
}

// drawPreview paints the compact file info column (icon + name + type + size).
func (cv *columnView) drawPreview(p painter.Painter, th *toolkit.Theme, r toolkit.Rect) {
	p.FillRect(r, th.SurfaceAlt)
	it := *cv.preview
	const big = 96
	if img := cv.iconFn(it); img != nil {
		img.SetBounds(toolkit.Rect{X: r.X + (r.W-big)/2, Y: r.Y + 30, W: big, H: big})
		img.Draw(p, th)
	}
	cx := func(s string) int { return r.X + (r.W-toolkit.TextWidth(s))/2 }
	name := elide(it.Name, (cv.colW-20)/7)
	toolkit.DrawText(p, cx(name), r.Y+30+big+16, name, th.OnSurface)
	kind := it.TypeLabel()
	toolkit.DrawText(p, cx(kind), r.Y+30+big+40, kind, mutedInk(th, 0.35))
	size := it.HumanSize()
	toolkit.DrawText(p, cx(size), r.Y+30+big+60, size, mutedInk(th, 0.35))
}

// OnEvent routes a click/scroll to the column under the pointer.
func (cv *columnView) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventClick, toolkit.EventScroll:
		if col := cv.columnAt(ev.X); col != nil {
			col.list.OnEvent(ev)
		}
	}
}

// columnAt returns the column whose horizontal band contains x, or nil.
func (cv *columnView) columnAt(x int) *millerCol {
	for _, col := range cv.cols {
		cb := col.list.Bounds()
		if x >= cb.X && x < cb.X+cb.W {
			return col
		}
	}
	return nil
}

// ColumnCount is the number of open directory columns (for tests).
func (cv *columnView) ColumnCount() int { return len(cv.cols) }
