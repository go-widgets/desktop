// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// sidebar is the file manager's Finder-style source list: two labelled
// sections — "Favoris" (the user's folders, drag-reorderable) and
// "Emplacements" (home, the startup volume, the network and the trash) — each
// row an accent-tinted glyph beside its label. Clicking a row navigates the
// content pane; the active location is highlighted. It is a shell-level widget
// (a sectioned source list is a toolkit gap — see FINDER-TOOLKIT-GAPS.md), so
// it composes only public toolkit primitives and never touches the toolkit.
type sidebar struct {
	toolkit.Base

	theme  *toolkit.Theme
	glyphs map[shell.PlaceKind]*toolkit.Image

	favorites []shell.Place // reorderable (Favoris section)
	locations []shell.Place // fixed (Emplacements section)

	active string // path of the highlighted row ("" = none)

	onNavigate func(shell.Place)

	// Reusable per-row icon Image, its bounds mutated per row at Draw time.
	iconImg *toolkit.Image

	// laid-out row rectangles, recomputed on SetBounds/Draw, shared by
	// hit-testing so a click lands on exactly the row it was drawn over.
	rows []sbRow

	// drag-to-reorder state (Favoris only).
	pressedFav int // favorite index the pointer went down on, or -1
	dragging   bool
	dragY      int // current pointer Y while dragging
}

// sbRow is one laid-out sidebar entry: either a section header or a place row.
type sbRow struct {
	rect    toolkit.Rect
	header  string      // non-empty for a section header row
	place   shell.Place // valid when header == ""
	favIdx  int         // index in favorites, or -1 when not a Favoris row
	isPlace bool
}

// Sidebar metrics.
const (
	sbHeaderH  = 24
	sbRowH     = 28
	sbIconPx   = 17
	sbLeftPad  = 14
	sbIconGap  = 9
	sbSectGap  = 8
	sbTopPad   = 10
	sbRowInset = 6 // left/right inset of the selection highlight
)

// newSidebar builds a sidebar over the given places, wiring the navigation
// callback. A nil places yields an empty (but valid) sidebar.
func newSidebar(th *toolkit.Theme, glyphs map[shell.PlaceKind]*toolkit.Image, places *shell.Places, onNavigate func(shell.Place)) *sidebar {
	s := &sidebar{
		theme:      th,
		glyphs:     glyphs,
		onNavigate: onNavigate,
		iconImg:    toolkit.NewImageFit(nil, 0, 0),
		pressedFav: -1,
	}
	if places != nil {
		s.favorites = append([]shell.Place(nil), places.Favorites...)
		s.locations = append([]shell.Place(nil), places.Locations...)
	}
	return s
}

// SetActive marks the row whose path equals p as highlighted.
func (s *sidebar) SetActive(p string) { s.active = p }

// SetBounds records the widget bounds and recomputes the row layout.
func (s *sidebar) SetBounds(r toolkit.Rect) {
	s.Base.SetBounds(r)
	s.layout()
}

// layout recomputes every row rectangle (headers + place rows) top to bottom.
func (s *sidebar) layout() {
	b := s.Bounds()
	y := b.Y + sbTopPad
	s.rows = s.rows[:0]

	addSection := func(title string, places []shell.Place, fav bool) {
		s.rows = append(s.rows, sbRow{
			rect:   toolkit.Rect{X: b.X, Y: y, W: b.W, H: sbHeaderH},
			header: title,
			favIdx: -1, // a header is never a favorite drop slot
		})
		y += sbHeaderH
		for i, pl := range places {
			row := sbRow{
				rect:    toolkit.Rect{X: b.X, Y: y, W: b.W, H: sbRowH},
				place:   pl,
				favIdx:  -1,
				isPlace: true,
			}
			if fav {
				row.favIdx = i
			}
			s.rows = append(s.rows, row)
			y += sbRowH
		}
		y += sbSectGap
	}

	addSection("Favoris", s.favorites, true)
	addSection("Emplacements", s.locations, false)
}

// Draw paints the sidebar panel, its two sections and every row, clipped to the
// panel bounds so a short window (fewer pixels than the rows need) never bleeds
// a bottom row into the region below.
func (s *sidebar) Draw(p painter.Painter, th *toolkit.Theme) {
	b := s.Bounds()
	p.FillRect(b, th.SurfaceAlt)
	// A hairline separates the sidebar from the content on its right edge.
	p.FillRect(toolkit.Rect{X: b.X + b.W - 1, Y: b.Y, W: 1, H: b.H}, th.Border)

	clipTo(p, b, func() {
		headerInk := mutedInk(th, 0.45)
		for _, row := range s.rows {
			if row.header != "" {
				ty := row.rect.Y + (row.rect.H-toolkit.GlyphHeight())/2 + 2
				toolkit.DrawText(p, row.rect.X+sbLeftPad, ty, row.header, headerInk)
				continue
			}
			s.drawRow(p, th, row)
		}

		// While dragging a favorite, draw an insertion line at the drop target.
		if s.dragging {
			if ti, ok := s.dropTarget(s.dragY); ok {
				ly := s.favInsertY(ti)
				p.FillRect(toolkit.Rect{X: b.X + sbRowInset, Y: ly - 1, W: b.W - 2*sbRowInset, H: 2}, th.Accent)
			}
		}
	})
}

// drawRow paints one place row: the selection highlight (when active), the
// accent-tinted glyph and the label.
func (s *sidebar) drawRow(p painter.Painter, th *toolkit.Theme, row sbRow) {
	r := row.rect
	ink := th.OnSurface
	if row.place.Path != "" && row.place.Path == s.active {
		hl := toolkit.Rect{X: r.X + sbRowInset, Y: r.Y + 2, W: r.W - 2*sbRowInset, H: r.H - 4}
		p.FillRoundRect(hl, 6, th.Accent)
		ink = th.Background
	}

	g := placeGlyph(s.glyphs, row.place.Kind)
	iy := r.Y + (r.H-sbIconPx)/2
	// Reuse one Image object, retinted per row would require a fresh glyph; the
	// glyphs are prebuilt tinted, so we just draw the prebuilt one at this rect.
	g.SetBounds(toolkit.Rect{X: r.X + sbLeftPad, Y: iy, W: sbIconPx, H: sbIconPx})
	g.Draw(p, th)

	tx := r.X + sbLeftPad + sbIconPx + sbIconGap
	ty := r.Y + (r.H-toolkit.GlyphHeight())/2
	label := row.place.Label
	toolkit.DrawText(p, tx, ty, elide(label, 18), ink)
}

// placeRowAt returns the place row whose rectangle contains y (ignoring
// headers), and ok=false when y falls on a header or outside every row.
func (s *sidebar) placeRowAt(y int) (sbRow, bool) {
	for _, row := range s.rows {
		if row.isPlace && y >= row.rect.Y && y < row.rect.Y+row.rect.H {
			return row, true
		}
	}
	return sbRow{}, false
}

// OnEvent navigates on a click and (in the Favoris section) reorders on a
// drag-and-drop. A click records the pressed favorite so DragData can report it
// when the host begins a drag; the drag itself is driven by the toolkit's DnD
// protocol (DragData/AcceptsDrop + EventDragMove/EventDrop).
func (s *sidebar) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventClick:
		s.pressedFav = -1
		row, ok := s.placeRowAt(ev.Y)
		if !ok {
			return
		}
		if row.favIdx >= 0 {
			s.pressedFav = row.favIdx
		}
		if s.onNavigate != nil {
			s.onNavigate(row.place)
		}
	case toolkit.EventDragMove:
		if s.pressedFav >= 0 {
			s.dragging = true
			s.dragY = ev.Y
		}
	case toolkit.EventDragLeave:
		s.dragging = false
	case toolkit.EventDrop:
		if s.dragging && s.pressedFav >= 0 {
			if ti, ok := s.dropTarget(ev.Y); ok {
				s.moveFavorite(s.pressedFav, ti)
			}
		}
		s.dragging = false
		s.pressedFav = -1
	}
}

// DragData reports the payload of a favorite-row drag: "fav:<index>" when a
// press landed on a Favoris row, or "" otherwise (so a drag from a location row
// or a header carries nothing and cannot reorder). It makes the sidebar a
// toolkit DragSource.
func (s *sidebar) DragData() string {
	if s.pressedFav < 0 {
		return ""
	}
	return "fav"
}

// AcceptsDrop makes the sidebar a DropTarget for its own favorite-reorder
// payload, so the host routes the drop back here.
func (s *sidebar) AcceptsDrop(payload string) bool { return payload == "fav" }

// dropTarget maps a pointer Y to the favorite insertion index [0, len], and
// ok=false when the pointer is not over the Favoris band.
func (s *sidebar) dropTarget(y int) (int, bool) {
	var favRows []sbRow
	for _, row := range s.rows {
		if row.favIdx >= 0 {
			favRows = append(favRows, row)
		}
	}
	if len(favRows) == 0 {
		return 0, false
	}
	first := favRows[0].rect.Y
	last := favRows[len(favRows)-1].rect.Y + favRows[len(favRows)-1].rect.H
	if y < first-sbRowH || y > last+sbRowH {
		return 0, false
	}
	for _, row := range favRows {
		mid := row.rect.Y + row.rect.H/2
		if y < mid {
			return row.favIdx, true
		}
	}
	return len(favRows), true
}

// favInsertY is the Y of the insertion line for favorite index ti.
func (s *sidebar) favInsertY(ti int) int {
	var favRows []sbRow
	for _, row := range s.rows {
		if row.favIdx >= 0 {
			favRows = append(favRows, row)
		}
	}
	if len(favRows) == 0 {
		return s.Bounds().Y
	}
	if ti >= len(favRows) {
		last := favRows[len(favRows)-1]
		return last.rect.Y + last.rect.H
	}
	return favRows[ti].rect.Y
}

// moveFavorite reorders the Favoris slice, moving the entry at from to before
// index to, then re-lays out. In-session order persistence: the sidebar owns
// the reordered slice.
func (s *sidebar) moveFavorite(from, to int) {
	if from < 0 || from >= len(s.favorites) {
		return
	}
	pl := s.favorites[from]
	s.favorites = append(s.favorites[:from], s.favorites[from+1:]...)
	if to > from {
		to--
	}
	if to < 0 {
		to = 0
	}
	if to > len(s.favorites) {
		to = len(s.favorites)
	}
	s.favorites = append(s.favorites, shell.Place{})
	copy(s.favorites[to+1:], s.favorites[to:])
	s.favorites[to] = pl
	s.layout()
}

// mutedInk blends the theme's surface text toward its background by t, for the
// quiet section-header labels.
func mutedInk(th *toolkit.Theme, t float64) toolkit.RGBA {
	return toolkit.RGBA{
		R: blend(th.OnSurface.R, th.SurfaceAlt.R, t),
		G: blend(th.OnSurface.G, th.SurfaceAlt.G, t),
		B: blend(th.OnSurface.B, th.SurfaceAlt.B, t),
		A: 0xFF,
	}
}
