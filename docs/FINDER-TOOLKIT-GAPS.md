# Finder file browser — toolkit gaps (now CLOSED)

_Horodate: 2026-08-11 09:18 CEST_ (originally flagged 2026-08-10 21:04 CEST)

The macOS-Finder-like file browser (`render.FinderPane`) originally built three
shell-level widgets for gaps `github.com/go-widgets/toolkit` did not yet cover.
As of **toolkit v0.136.0** all three gaps are **closed**: the toolkit ships the
widgets, and the shell now **consumes them** through thin adapters. The shell no
longer owns any reference-widget rendering — each adapter only maps the shell's
data / navigation / drag-to-move callbacks onto the toolkit widget.

Everything reuses public toolkit widgets unchanged:
`Border`, `HBox`, `VBox`, `Frame`, `Stack`, `ViewSwitcher`, `Scale`, `Table`,
`Label`, `Image`, `Dialog`, `Button`, and now `SourceList`, `IconGrid`,
`ColumnBrowser`.

## Gap 1 — Sectioned source list (Finder sidebar) → **`toolkit.SourceList`**

**Shell adapter:** `render.sidebar` (`render/sidebar.go`).

The toolkit now ships `SourceList` — labelled sections, a per-row leading icon,
per-section `Reorderable`, and source-list (rounded accent pill) selection. The
shell adapter projects the Favoris/Emplacements `Places` model into its two
sections (Favoris reorderable), maps `OnSelect` → navigation and `OnReorder` →
the mirrored favourites order, and drives the active-location pill via
`SetSelected`. The only shell-side addition is a file-drop row hit-test (the
widget exposes no public one) so a file dragged onto a directory row requests a
move.

## Gap 2 — Icon (thumbnail) grid with framed, chip-backed cells → **`toolkit.IconGrid`**

**Shell adapter:** `render.iconView` (`render/iconview.go`).

`IconGrid` owns the icon-grid cell chrome the shell used to draw by hand: a
thumbnail centred in a subtle rounded frame, a **light chip behind a raster
thumbnail** so a dark image stays visible on a dark theme, a centred elided
label, a size-driven cell footprint, selection + hit-testing, and a `DragSource`
carrying the selected cell's key. The adapter reprojects the directory model into
`IconCell`s (resolving each icon + whether it is a raster thumbnail) and maps
activation → open, plus the shell-side folder-cell drop → move.

## Gap 3 — Miller / column (Colonnes) view → **`toolkit.ColumnBrowser`**

**Shell adapter:** `render.columnView` (`render/columnview.go`).

`ColumnBrowser` owns the whole column chain, horizontal scroll and preview pane,
driven by a caller-supplied `ColumnProvider`. The adapter implements that
provider over the shell's directory lister (folders are containers; a leaf's
kind + size feed the preview) and maps a re-picked leaf → open.

---

Promoting the three widgets into the toolkit was the clean lift the original
flag anticipated: the shell widgets became the reference implementations, and the
shell now depends on the toolkit versions instead of duplicating them.
