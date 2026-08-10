# Finder file browser — flagged go-widgets/toolkit gaps

_Horodate: 2026-08-10 21:04 CEST_

The macOS-Finder-like file browser (`render.FinderPane`) is built **entirely at
the app/shell level**. `github.com/go-widgets/toolkit` is **not modified** — it
is co-developed by the maintainer. Where the browser needed a widget the toolkit
does not yet provide, a **minimal shell-level widget** was built inside the
`render` package and is flagged here as a candidate future toolkit addition.

Everything else reuses public toolkit widgets unchanged:
`Border`, `HBox`, `VBox`, `Frame`, `Stack`, `ViewSwitcher`, `Scale`, `Table`,
`ListBox`, `Label`, `Image`, plus `painter.Clipper` for bounds clipping.

## Gap 1 — Sectioned source list (Finder sidebar)

**Shell widget:** `render.sidebar` (`render/sidebar.go`).

The toolkit has `ListBox` (flat, single-section, uniformly reorderable) but no
**sectioned source list**: two (or more) labelled groups — "Favoris",
"Emplacements" — where a per-row icon sits beside the label, only *one* section
is drag-reorderable, and the selected row is a rounded accent pill (the
NSOutlineView "source list" style). `ListBox` cannot express section headers or
per-section reorderability.

**Candidate toolkit addition:** a `SourceList` / `SidebarList` widget — labelled
sections, per-row leading icon, per-section `Reorderable`, source-list selection
styling. Would also serve a mail/settings sidebar.

## Gap 2 — Icon (thumbnail) grid with framed, chip-backed cells

**Shell widget:** `render.iconView` (`render/iconview.go`).

The toolkit's `virtual.VirtualGrid` reflows uniform cells but has no built-in
notion of *icon-grid cell chrome*: a thumbnail centred and fit inside a subtle
rounded frame, a **light chip behind a raster thumbnail** so a dark image stays
visible on a dark theme, a centred label elided to the cell width, and a
size-driven cell footprint. The improved cell treatment is app policy, so it was
built as a dedicated view rather than a `VirtualGrid` `Render` closure (which
draws cell *content* but owns no cell chrome, selection field, or hit-testing).

**Candidate toolkit addition:** an `IconGrid` widget (selectable, size-driven,
framed/chip cells) — or a richer `VirtualGrid` cell context exposing selection +
a themed cell frame.

## Gap 3 — Miller / column (Colonnes) view

**Shell widget:** `render.columnView` (`render/columnview.go`).

There is no column/Miller widget: N side-by-side directory columns where picking
a folder opens the next column to the right, a file opens a preview column, and
the strip scrolls horizontally to keep the deepest columns visible. It is
composed here from one `toolkit.ListBox` per directory (each with a per-row
`ItemRenderer` that draws the type icon, name and a disclosure chevron) laid out
by hand.

**Candidate toolkit addition:** a `ColumnBrowser` / `MillerView` widget over a
tree/`AppSource`, owning the column chain, horizontal scroll and preview pane.

---

Each gap is intentionally scoped **inside `render/`** so the toolkit stays
untouched; promoting any of these into the toolkit later is a clean lift (the
shell widget becomes the reference implementation).
