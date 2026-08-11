// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"strings"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// sidebar is a thin shell adapter over toolkit.SourceList (the sectioned
// "source list" the toolkit now ships). It projects the Favoris/Emplacements
// Places model into the source list's two sections — Favoris drag-reorderable,
// each row an accent-tinted place glyph beside its label — maps a row selection
// back to a shell navigation, and mirrors the active location as the selected
// pill. Every bit of rendering, the source-list selection styling and the
// favourite drag-reorder come from the toolkit widget unchanged.
//
// Beyond what the toolkit widget offers, the adapter also accepts a file dragged
// from a content view and dropped onto a real-directory row (a Favoris or
// Emplacements folder), reporting it as a move request via onFileDrop — the
// shell-side wiring for the Finder's drag-to-move. The row hit-test mirrors the
// SourceList's own top-to-bottom layout (its metrics, replicated below as the
// sb* constants), since the toolkit widget exposes no public row hit-test.
type sidebar struct {
	toolkit.Base

	list *toolkit.SourceList

	// favorites / locations mirror the two sections' Places (with their Kind and
	// Path), kept in sync with a reorder so a (section,row) index still resolves
	// to the right Place after the user drags a favourite.
	favorites []shell.Place
	locations []shell.Place

	onNavigate func(shell.Place)
	onFileDrop func(dest shell.Place, srcPath string)
}

// Sidebar metrics — identical to toolkit.SourceList's own (private) metrics, so
// the file-drop row hit-test lands on exactly the row the widget drew. sbIconPx
// also sizes the prebuilt place glyphs (see FinderPane.build).
const (
	sbHeaderH = 24
	sbRowH    = 28
	sbIconPx  = 17
	sbSectGap = 8
	sbTopPad  = 10
)

// newSidebar builds a sidebar over the given places, wiring the navigation
// callback. A nil places yields an empty (but valid) sidebar.
func newSidebar(th *toolkit.Theme, glyphs map[shell.PlaceKind]*toolkit.Image, places *shell.Places, onNavigate func(shell.Place)) *sidebar {
	s := &sidebar{onNavigate: onNavigate}
	if places != nil {
		s.favorites = append([]shell.Place(nil), places.Favorites...)
		s.locations = append([]shell.Place(nil), places.Locations...)
	}
	s.list = toolkit.NewSourceList(
		toolkit.SourceSection{Title: "Favoris", Reorderable: true, Items: placeItems(glyphs, s.favorites)},
		toolkit.SourceSection{Title: "Emplacements", Items: placeItems(glyphs, s.locations)},
	)
	s.list.OnSelect = s.selectRow
	s.list.OnReorder = s.reordered
	return s
}

// placeItems projects a Places slice into SourceList rows: each carries the
// kind's accent-tinted glyph, the label, and the directory path as the opaque
// Key (empty for a non-navigable place such as the network).
func placeItems(glyphs map[shell.PlaceKind]*toolkit.Image, places []shell.Place) []toolkit.SourceItem {
	items := make([]toolkit.SourceItem, len(places))
	for i, pl := range places {
		items[i] = toolkit.SourceItem{Icon: placeGlyph(glyphs, pl.Kind), Label: pl.Label, Key: pl.Path}
	}
	return items
}

// SetActive marks the row whose path equals p as the selected pill (or clears
// the selection when no row matches, e.g. the network empty state).
func (s *sidebar) SetActive(p string) {
	if sec, row, ok := s.rowForPath(p); ok {
		s.list.SetSelected(sec, row)
		return
	}
	s.list.SetSelected(-1, -1)
}

// rowForPath finds the (section,row) whose place path equals p (favorites first,
// then locations); an empty p matches nothing.
func (s *sidebar) rowForPath(p string) (section, row int, ok bool) {
	if p == "" {
		return 0, 0, false
	}
	for i, pl := range s.favorites {
		if pl.Path == p {
			return 0, i, true
		}
	}
	for i, pl := range s.locations {
		if pl.Path == p {
			return 1, i, true
		}
	}
	return 0, 0, false
}

// placeAt resolves a (section,row) back to its Place.
func (s *sidebar) placeAt(section, row int) (shell.Place, bool) {
	var list []shell.Place
	switch section {
	case 0:
		list = s.favorites
	case 1:
		list = s.locations
	default:
		return shell.Place{}, false
	}
	if row < 0 || row >= len(list) {
		return shell.Place{}, false
	}
	return list[row], true
}

// selectRow is the SourceList OnSelect handler: it navigates the finder to the
// picked place.
func (s *sidebar) selectRow(section, row int) {
	if pl, ok := s.placeAt(section, row); ok && s.onNavigate != nil {
		s.onNavigate(pl)
	}
}

// reordered is the SourceList OnReorder handler: it mirrors the favourite drag
// onto the adapter's own Places slice so later (section,row) look-ups stay
// correct. Only the Favoris section (0) is reorderable.
func (s *sidebar) reordered(section, from, to int) {
	if section == 0 {
		s.favorites = movePlace(s.favorites, from, to)
	}
}

// movePlace relocates list[from] to final index to, matching SourceList.moveItem
// (whose OnReorder reports the already-adjusted final index).
func movePlace(list []shell.Place, from, to int) []shell.Place {
	if from < 0 || from >= len(list) {
		return list
	}
	pl := list[from]
	list = append(list[:from], list[from+1:]...)
	if to < 0 {
		to = 0
	}
	if to > len(list) {
		to = len(list)
	}
	list = append(list, shell.Place{})
	copy(list[to+1:], list[to:])
	list[to] = pl
	return list
}

// SetBounds records the widget bounds and forwards them to the source list.
func (s *sidebar) SetBounds(r toolkit.Rect) {
	s.Base.SetBounds(r)
	s.list.SetBounds(r)
}

// Draw paints the source list (panel, sections, rows, reorder insertion line).
func (s *sidebar) Draw(p painter.Painter, th *toolkit.Theme) { s.list.Draw(p, th) }

// OnEvent forwards to the source list, except an external file drop (an
// EventDrop carrying a file-path payload, not the source list's own reorder
// token) which the adapter maps to a directory row and reports as a move.
func (s *sidebar) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventDrop && isFilePayload(ev.Code) {
		s.handleFileDrop(ev)
		return
	}
	s.list.OnEvent(ev)
}

// handleFileDrop resolves the row under the drop point and, when it is a real
// (navigable) directory, reports a move of the dropped file into it.
func (s *sidebar) handleFileDrop(ev toolkit.Event) {
	sec, row, ok := s.rowAt(ev.Y)
	if !ok {
		return
	}
	pl, ok := s.placeAt(sec, row)
	if !ok || !pl.Navigable() {
		return
	}
	if s.onFileDrop != nil {
		s.onFileDrop(pl, ev.Code)
	}
}

// rowAt maps a pointer Y to the (section,row) it falls on, mirroring the
// SourceList's own top-to-bottom layout (both sections carry a title, so each
// opens with a header band). ok=false when Y lands on a header, a gap or outside
// every row.
func (s *sidebar) rowAt(y int) (section, row int, ok bool) {
	cur := s.Bounds().Y + sbTopPad
	for sec, items := range [][]shell.Place{s.favorites, s.locations} {
		cur += sbHeaderH
		for i := range items {
			if y >= cur && y < cur+sbRowH {
				return sec, i, true
			}
			cur += sbRowH
		}
		cur += sbSectGap
	}
	return 0, 0, false
}

// DragData delegates to the source list (its favourite-reorder payload).
func (s *sidebar) DragData() string { return s.list.DragData() }

// AcceptsDrop accepts the source list's own reorder payload and, additionally,
// any non-empty file-path payload so the host routes a file drag onto the
// sidebar here (where handleFileDrop decides whether it landed on a directory).
func (s *sidebar) AcceptsDrop(payload string) bool {
	return s.list.AcceptsDrop(payload) || isFilePayload(payload)
}

// isFilePayload reports whether a drag payload is an external file path rather
// than a SourceList reorder token — the discriminator the sidebar and content
// views use to tell a move-drop from an internal reorder.
func isFilePayload(payload string) bool {
	return payload != "" && !strings.HasPrefix(payload, toolkit.SourceRowDragPrefix)
}

// sidebar is a DragSource + DropTarget (reorder + file-drop) like the widget it
// wraps.
var (
	_ toolkit.DragSource = (*sidebar)(nil)
	_ toolkit.DropTarget = (*sidebar)(nil)
)
