# Finder file browser — captures

_Horodate: 2026-08-11 09:18 CEST_ — rendered via `cmd/desktop -capture`. The
three views are now composed from the **toolkit** Finder widgets
(`toolkit.SourceList` / `IconGrid` / `ColumnBrowser`, toolkit v0.136.0) through
thin shell adapters; the appearance is preserved from the previous
shell-reference widgets (verified by before/after capture — the only pixel
difference is the two sidebar section-header labels, which the toolkit's
SourceList renders in its own muted tint).

The desktop shell is the outer chrome (menubar, Applications launcher rail on
the far left, dock along the bottom); the **file-manager pane** fills the
content region: a Favoris/Emplacements sidebar, a Liste/Vignettes/Colonnes view
switcher + icon-size slider in the toolbar, and the active view.

| Capture | What it shows |
| --- | --- |
| `2026-08-11-finder-liste.png` | **Liste** — sortable `toolkit.Table`: Nom / Taille / Type / Date de modification, sort arrow on Nom, per-row type icons. |
| `2026-08-11-finder-vignettes.png` | **Vignettes** (`toolkit.IconGrid`) at the default 64px icon size — icons centred in framed cells, names elided to the cell width. |
| `2026-08-11-finder-colonnes.png` | **Colonnes** (`toolkit.ColumnBrowser`) — Miller cascade, folder rows with a disclosure chevron, a leaf preview pane. |
| `2026-08-11-finder-move-confirm.png` | **Drag-to-move confirmation** — a file dropped on a folder pops the modal « Déplacer «…» vers «…» ? » dialog (Annuler / Déplacer) over a dimmed pane. The real filesystem is mutated only after **Déplacer**. |

Reproduce, e.g.:

```
go run ./cmd/desktop -embedded -view liste        -w 960 -h 600 -capture liste.png
go run ./cmd/desktop -embedded -view vignettes    -w 960 -h 600 -capture vignettes.png
go run ./cmd/desktop -dir "$HOME" -view colonnes   -w 960 -h 600 -capture colonnes.png
go run ./cmd/desktop -embedded -view vignettes -confirm-move -w 960 -h 600 -capture move.png
```
