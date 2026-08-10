// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/toolkit"
)

// View modes, in the toolbar segmented control's order.
const (
	ViewList    = 0 // Liste
	ViewIcons   = 1 // Vignettes
	ViewColumns = 2 // Colonnes
)

// Toolbar metrics.
const (
	finderToolbarH = 44
	finderSidebarW = 194
	switcherW      = 216
	sliderW        = 120
)

// Icon-size slider range (icon square side in pixels).
const (
	iconSizeMin = 32.0
	iconSizeMax = 128.0
)

// FinderConfig bundles what a FinderPane needs from the shell/render layer.
type FinderConfig struct {
	Icons      *IconLoader
	Theme      *toolkit.Theme
	Places     *shell.Places
	Lister     func(path string) (*shell.Dir, error)
	ThumbKey   func(shell.FileItem) string
	InitialDir *shell.Dir
}

// FinderPane is the macOS-Finder-like file browser that fills the desktop
// shell's content region: a Favoris/Emplacements sidebar, a toolbar with a
// Liste/Vignettes/Colonnes view switcher and an icon-size slider, and the three
// swappable content views over a shared, sortable directory model. It composes
// only public toolkit widgets (Border, HBox, ViewSwitcher, Scale, Table,
// ListBox, Stack) plus three shell-level views for the gaps the toolkit has no
// widget for (the sectioned sidebar, the improved icon grid, the Miller
// columns) — the toolkit itself is never modified.
type FinderPane struct {
	cfg   FinderConfig
	theme *toolkit.Theme

	// state (mvvm).
	cwd       *mvvm.Observable[string]
	viewMode  *mvvm.Observable[int]
	iconSize  *mvvm.Observable[float64]
	fileModel *mvvm.ObservableList[shell.FileItem]

	sortCol  int
	sortAsc  bool
	emptyMsg string
	thumbKey func(shell.FileItem) string

	// fallback glyphs.
	folderGlyph  *toolkit.Image
	fileGlyph    *toolkit.Image
	pictureGlyph *toolkit.Image
	placeGlyphs  map[shell.PlaceKind]*toolkit.Image

	// widgets.
	root       *toolkit.Border
	toolbar    *toolkit.HBox
	titleLabel *toolkit.Label
	switcher   *toolkit.ViewSwitcher
	slider     *toolkit.Scale
	sidebar    *sidebar
	content    *toolkit.Stack
	listView   *listView
	iconView   *iconView
	columnView *columnView
}

// NewFinderPane builds the file browser. Places defaults to shell.DefaultPlaces
// and Lister to a native ListDir+ClassifyLite lister when left nil, so a caller
// that only supplies an InitialDir still gets a fully navigable pane on a real
// filesystem (and a graceful, empty one in the browser).
func NewFinderPane(cfg FinderConfig) *FinderPane {
	if cfg.Theme == nil {
		cfg.Theme = toolkit.DefaultDark()
	}
	if cfg.Icons == nil {
		cfg.Icons = NewIconLoader("", DefaultIconSize)
	}
	if cfg.Places == nil {
		cfg.Places = shell.DefaultPlaces()
	}
	if cfg.Lister == nil {
		cfg.Lister = NativeLister
	}
	f := &FinderPane{
		cfg:       cfg,
		theme:     cfg.Theme,
		cwd:       mvvm.NewObservable(""),
		viewMode:  mvvm.NewObservable(ViewIcons),
		iconSize:  mvvm.NewObservable(64.0),
		fileModel: mvvm.NewObservableList[shell.FileItem](),
		sortCol:   0,
		sortAsc:   true,
		thumbKey:  cfg.ThumbKey,
	}
	f.build()
	if cfg.InitialDir != nil {
		f.setDir(cfg.InitialDir)
	}
	return f
}

// NativeLister lists path from the real filesystem and classifies it by name
// (the portable, content-sniff-free classifier). It is the default navigation
// backend; in the browser (no filesystem) os.ReadDir fails and the finder shows
// its empty state.
func NativeLister(path string) (*shell.Dir, error) {
	d, err := shell.ListDir(path)
	if err != nil {
		return nil, err
	}
	d.ClassifyLite()
	return d, nil
}

// build lays out the widget tree and wires the interactions.
func (f *FinderPane) build() {
	f.folderGlyph = folderIcon(DefaultIconSize)
	f.fileGlyph = fileIcon(DefaultIconSize)
	f.pictureGlyph = pictureIcon(DefaultIconSize)
	f.placeGlyphs = buildPlaceGlyphs(sbIconPx, placeGlyphInk(f.theme))

	// Sidebar.
	f.sidebar = newSidebar(f.theme, f.placeGlyphs, f.cfg.Places, f.navigatePlace)

	// Toolbar: title + view switcher + icon-size slider.
	f.titleLabel = toolkit.NewLabel("")
	f.titleLabel.VAlign = toolkit.VMiddle
	f.switcher = toolkit.NewViewSwitcher([]string{"Liste", "Vignettes", "Colonnes"}, ViewIcons)
	f.switcher.OnChange = f.SetView
	f.slider = toolkit.NewScale(iconSizeMin, iconSizeMax, f.iconSize.Get())
	f.slider.OnChange = func(v float64) { f.SetIconSize(int(v)) }

	f.toolbar = toolkit.NewHBox()
	f.toolbar.AddFlex(f.padded(f.titleLabel), 1)
	f.toolbar.AddFixed(f.vcenter(f.switcher, toolkit.ViewSwitcherH), switcherW)
	f.toolbar.AddFixed(f.vcenter(f.slider, 18), sliderW)

	// Content views over the shared model.
	f.listView = newListView(f.fileModel, f.theme, f.listIcon, f.open, f.sortBy, f.emptyMessage)
	f.iconView = newIconView(f.fileModel, int(f.iconSize.Get()), f.cellImage, f.open, f.emptyMessage)
	f.columnView = newColumnView(f.theme, f.cfg.Lister, f.listIcon, f.open)

	f.content = toolkit.NewStack()
	f.content.AddPage("liste", f.listView)
	f.content.AddPage("icones", f.iconView)
	f.content.AddPage("colonnes", f.columnView)
	f.content.Visible = pageName(f.viewMode.Get())

	// Root border.
	f.root = toolkit.NewBorder()
	f.root.North = f.toolbar
	f.root.NorthSize = finderToolbarH
	f.root.West = f.sidebar
	f.root.WestSize = finderSidebarW
	f.root.Center = f.content
}

// padded wraps w in a small left inset so the toolbar title does not hug the
// window edge.
func (f *FinderPane) padded(w toolkit.Widget) toolkit.Widget {
	return &insetWidget{w: w, left: 14}
}

// vcenter wraps w so it is vertically centred within the toolbar at a fixed
// content height (a switcher/slider is shorter than the toolbar).
func (f *FinderPane) vcenter(w toolkit.Widget, h int) toolkit.Widget {
	return &vcenterWidget{w: w, h: h}
}

// pageName maps a view-mode constant to its Stack page name.
func pageName(mode int) string {
	switch mode {
	case ViewList:
		return "liste"
	case ViewColumns:
		return "colonnes"
	default:
		return "icones"
	}
}

// SetView switches the visible content view (and roots the Miller strip at the
// current directory the first time it is shown).
func (f *FinderPane) SetView(mode int) {
	if mode < ViewList || mode > ViewColumns {
		return
	}
	f.viewMode.Set(mode)
	f.switcher.Current = mode
	f.content.Visible = pageName(mode)
	if mode == ViewColumns && f.columnView.ColumnCount() == 0 {
		if p := f.cwd.Get(); p != "" {
			f.columnView.SetRoot(p)
		}
	}
	f.relayout()
}

// SetIconSize sets the Vignettes icon size (clamped to the slider range) and
// keeps the slider thumb in step.
func (f *FinderPane) SetIconSize(px int) {
	if px < int(iconSizeMin) {
		px = int(iconSizeMin)
	}
	if px > int(iconSizeMax) {
		px = int(iconSizeMax)
	}
	f.iconSize.Set(float64(px))
	f.iconView.SetIconSize(px)
	f.slider.SetValue(float64(px))
	f.relayout()
}

// navigatePlace opens a sidebar place: the network location shows the honest
// empty "Aucun partage" state (there is no pure-Go network browsing), every
// other navigable place lists its directory.
func (f *FinderPane) navigatePlace(pl shell.Place) {
	if pl.Kind == shell.PlaceNetwork {
		f.showNetwork()
		return
	}
	if pl.Navigable() {
		f.Navigate(pl.Path)
	}
}

// GoToPlace navigates the finder to the sidebar place of the given kind (the
// first match in Favoris then Emplacements), a no-op when no such place exists.
// It is the programmatic peer of clicking that sidebar row (used by the capture
// CLI and tests).
func (f *FinderPane) GoToPlace(kind shell.PlaceKind) {
	all := append(append([]shell.Place(nil), f.cfg.Places.Favorites...), f.cfg.Places.Locations...)
	for _, pl := range all {
		if pl.Kind == kind {
			f.navigatePlace(pl)
			return
		}
	}
}

// showNetwork puts the pane into the network location's empty state.
func (f *FinderPane) showNetwork() {
	f.fileModel.Clear()
	f.emptyMsg = "Aucun partage"
	f.cwd.Set("")
	f.titleLabel.Text = "Réseau"
	f.sidebar.SetActive("")
	f.listView.Refresh()
	f.columnView.SetRoot("")
	f.relayout()
}

// Navigate lists path and shows it in every view.
func (f *FinderPane) Navigate(path string) {
	dir, err := f.cfg.Lister(path)
	if err != nil || dir == nil {
		f.fileModel.Clear()
		f.emptyMsg = "Dossier indisponible"
		f.cwd.Set(path)
		f.titleLabel.Text = titleFor(path)
		f.sidebar.SetActive(path)
		f.listView.Refresh()
		f.relayout()
		return
	}
	f.setDir(dir)
	f.columnView.SetRoot(path)
	f.relayout()
}

// setDir installs a listing into the shared model, sorted by the current sort,
// and updates the title + sidebar highlight.
func (f *FinderPane) setDir(dir *shell.Dir) {
	items := append([]shell.FileItem(nil), dir.Items...)
	sortItems(items, f.sortCol, f.sortAsc)
	f.fileModel.Clear()
	f.fileModel.Append(items...)
	f.emptyMsg = "Dossier vide"
	f.cwd.Set(dir.Path)
	f.titleLabel.Text = titleFor(dir.Path)
	f.sidebar.SetActive(dir.Path)
	f.iconView.sel = -1
	f.listView.Refresh()
}

// open activates an item: a directory navigates into it; a file is a no-op
// (the shell has no in-pane document opener).
func (f *FinderPane) open(it shell.FileItem) {
	if it.IsDir {
		f.Navigate(it.Path)
	}
}

// sortBy re-sorts the shared model by a list column and refreshes the views.
func (f *FinderPane) sortBy(col int, asc bool) {
	f.sortCol = col
	f.sortAsc = asc
	items := f.fileModel.Slice()
	sortItems(items, col, asc)
	f.fileModel.Clear()
	f.fileModel.Append(items...)
	f.listView.SetSort(col, asc)
	f.listView.Refresh()
	f.relayout()
}

// emptyMessage is the empty-state text for the current context.
func (f *FinderPane) emptyMessage() string { return f.emptyMsg }

// relayout re-lays out the whole pane (after a navigation / view / size change)
// so the swapped or resized content is positioned before the next paint.
func (f *FinderPane) relayout() {
	if b := f.root.Bounds(); b.W > 0 && b.H > 0 {
		f.root.SetBounds(b)
	}
}

// cellImage resolves a file item to its icon-view Image plus whether that Image
// is a real raster thumbnail (so the cell can back a dark photo with a light
// chip): a cached thumbnail when the thumbnail policy supplies one, otherwise a
// theme icon or a tasteful drawn glyph (folder / picture / document).
func (f *FinderPane) cellImage(it shell.FileItem) (*toolkit.Image, bool) {
	if !it.IsDir && f.thumbKey != nil {
		if k := f.thumbKey(it); k != "" {
			if img, ok := f.cfg.Icons.TryImage(k); ok {
				return img, true
			}
		}
	}
	if it.IsDir {
		if img, ok := f.cfg.Icons.TryImage("folder"); ok {
			return img, false
		}
		return f.folderGlyph, false
	}
	name := "text-x-generic"
	if it.Mime != "" {
		name = strings.ReplaceAll(it.Mime, "/", "-")
	}
	if img, ok := f.cfg.Icons.TryImage(name); ok {
		return img, false
	}
	if it.IsImage() {
		return f.pictureGlyph, false
	}
	return f.fileGlyph, false
}

// listIcon is the small leading type icon for the list and column views.
func (f *FinderPane) listIcon(it shell.FileItem) *toolkit.Image {
	img, _ := f.cellImage(it)
	return img
}

// Root returns the composed Border for embedding in the shell.
func (f *FinderPane) Root() toolkit.Widget { return f.root }

// FileCount is the number of items in the current directory model.
func (f *FinderPane) FileCount() int { return f.fileModel.Len() }

// ViewMode is the active view-mode constant.
func (f *FinderPane) ViewMode() int { return f.viewMode.Get() }

// IconSize is the current Vignettes icon size in pixels.
func (f *FinderPane) IconSize() int { return int(f.iconSize.Get()) }

// CurrentDir is the path currently shown ("" in the network empty state).
func (f *FinderPane) CurrentDir() string { return f.cwd.Get() }

// CascadeColumns opens the first sub-directory of each rightmost Miller column
// until n columns show — the folder chain a user would click, used to populate a
// column-view screenshot with the signature cascade. It switches to the column
// view first so the strip is rooted.
func (f *FinderPane) CascadeColumns(n int) {
	f.SetView(ViewColumns)
	f.columnView.cascadeFirst(n)
	f.relayout()
}

// titleFor is the toolbar title for a path: its base name, or the path itself
// for a root ("/" -> "/").
func titleFor(path string) string {
	if path == "" {
		return "Fichiers"
	}
	base := filepath.Base(path)
	if base == "" || base == "." {
		return path
	}
	return base
}

// sortItems sorts items in place by a list column, keeping directories grouped
// first (Finder shows folders above files). col: 0 Nom, 1 Taille, 2 Type, 3
// Date. asc reverses the secondary comparison.
func sortItems(items []shell.FileItem, col int, asc bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.IsDir != b.IsDir {
			return a.IsDir // directories first, regardless of column/direction
		}
		var less bool
		switch col {
		case 1:
			if a.Size != b.Size {
				less = a.Size < b.Size
			} else {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
		case 2:
			ta, tb := a.TypeLabel(), b.TypeLabel()
			if ta != tb {
				less = ta < tb
			} else {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
		case 3:
			if !a.ModTime.Equal(b.ModTime) {
				less = a.ModTime.Before(b.ModTime)
			} else {
				less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
		default:
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if !asc {
			return !less
		}
		return less
	})
}
