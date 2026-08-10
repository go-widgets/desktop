# Finder file browser — captures

_Horodate: 2026-08-10 21:04 CEST_ — rendered on macOS via
`cmd/desktop -capture` against a real filesystem (`$HOME`, an image folder).

The desktop shell is the outer chrome (menubar, Applications launcher rail on
the far left, dock along the bottom); the **file-manager pane** fills the
content region: a Favoris/Emplacements sidebar, a Liste/Vignettes/Colonnes view
switcher + icon-size slider in the toolbar, and the active view.

| Capture | What it shows |
| --- | --- |
| `2026-08-10-finder-liste.png` | **Liste** — sortable `toolkit.Table`: Nom / Taille / Type / Date de modification, sort arrow on Nom, per-row type icons, alternating row tints. |
| `2026-08-10-finder-vignettes.png` | **Vignettes** at the default (64px) icon size — folder icons centred in framed cells, names elided to the cell width. |
| `2026-08-10-finder-vignettes-thumbnails.png` | **Vignettes** over an image folder — **real thumbnails**, each centred on a light chip with a hairline frame so even dark images (a terminal screenshot, a dark stock photo) stay visible on the dark theme. |
| `2026-08-10-finder-vignettes-large-icons.png` | The **size slider** pushed to 112px — same folder, larger cells; the slider thumb sits near the right of its track. |
| `2026-08-10-finder-colonnes.png` | **Colonnes** (Miller) — four columns deep (`aot2 › ruby › bench › modules`), folder rows with a disclosure chevron, file rows with a document icon, the picked row highlighted in each column. |
| `2026-08-10-finder-reseau-empty.png` | The **Réseau** location's honest empty state — a centred "Aucun partage", never a fake listing (there is no pure-Go network browsing). |

Reproduce, e.g.:

```
go run ./cmd/desktop -dir "$HOME" -view liste     -w 1280 -h 820 -capture liste.png
go run ./cmd/desktop -dir /path/pics -view vignettes -icon-size 96 -w 1280 -h 820 -capture thumbs.png
go run ./cmd/desktop -dir "$HOME" -view colonnes  -w 1280 -h 820 -capture colonnes.png
go run ./cmd/desktop -dir "$HOME" -place reseau   -w 1280 -h 820 -capture reseau.png
```
