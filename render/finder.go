// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// View modes, in the toolbar segmented control's order.
const (
	ViewList    = 0 // Liste
	ViewIcons   = 1 // Vignettes
	ViewColumns = 2 // Colonnes
	ViewGallery = 3 // Galerie
)

// Toolbar metrics.
const (
	finderToolbarH = 44
	finderSidebarW = 194
	switcherW      = 288 // four segments: Liste / Vignettes / Colonnes / Galerie
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
// Liste/Vignettes/Colonnes/Galerie view switcher and an icon-size slider, and
// the four swappable content views over a shared, sortable directory model. It
// composes only public toolkit widgets (Border, HBox, ViewSwitcher, Scale,
// Table, IconGrid, ColumnBrowser, GalleryView, Stack) plus shell-level adapters
// for the gaps the toolkit has no widget for (the sectioned sidebar) and thin
// projections of the shared model onto each toolkit view — the toolkit itself is
// never modified.
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
	root        *toolkit.Border
	overlay     *finderRoot // returned by Root(): draws root + the modal dialog
	toolbar     *toolkit.HBox
	titleLabel  *toolkit.Label
	switcher    *toolkit.ViewSwitcher
	slider      *toolkit.Scale
	sidebar     *sidebar
	content     *toolkit.Stack
	listView    *listView
	iconView    *iconView
	columnView  *columnView
	galleryView *galleryView

	// dialog is the modal move-confirmation (or error) overlay, non-nil while a
	// dialog is shown; the overlay routes input to it exclusively while it is up.
	dialog *toolkit.Dialog

	// clip is the file clipboard driving the ⌘C / ⌘X / ⌘V (Ctrl on Linux/Windows)
	// keyboard file operations; a path marked for a cut is drawn dimmed in the
	// views until it is pasted or the clipboard is cleared.
	clip shell.Clipboard

	// pending holds the paste a confirmation dialog will apply on confirm, so the
	// filesystem is mutated only when the user accepts a move / overwrite.
	pending *pendingPaste
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

	// Sidebar. A file dropped on a directory row requests a move into it.
	f.sidebar = newSidebar(f.theme, f.placeGlyphs, f.cfg.Places, f.navigatePlace)
	f.sidebar.onFileDrop = func(dest shell.Place, src string) {
		f.requestMove(src, dest.Path, dest.Label)
	}

	// Toolbar: title + view switcher + icon-size slider.
	f.titleLabel = toolkit.NewLabel("")
	f.titleLabel.VAlign = toolkit.VMiddle
	f.switcher = toolkit.NewViewSwitcher([]string{"Liste", "Vignettes", "Colonnes", "Galerie"}, ViewIcons)
	f.switcher.Current().Subscribe(func(mode int) { f.SetView(mode) })
	f.slider = toolkit.NewScale(iconSizeMin, iconSizeMax, f.iconSize.Get())
	f.slider.Value().Subscribe(func(v float64) { f.SetIconSize(int(v)) })

	f.toolbar = toolkit.NewHBox()
	f.toolbar.AddFlex(f.padded(f.titleLabel), 1)
	f.toolbar.AddFixed(f.vcenter(f.switcher, toolkit.ViewSwitcherH), switcherW)
	f.toolbar.AddFixed(f.vcenter(f.slider, 18), sliderW)

	// Content views over the shared model. A file dropped on a folder cell/row in
	// the icon or list view requests a move into that folder.
	f.listView = newListView(f.fileModel, f.theme, f.listIcon, f.open, f.sortBy, f.emptyMessage)
	f.listView.onFileDrop = f.dropIntoFolder
	f.iconView = newIconView(f.fileModel, int(f.iconSize.Get()), f.cellImage, f.open, f.emptyMessage)
	f.iconView.onFileDrop = f.dropIntoFolder
	f.columnView = newColumnView(f.cfg.Lister, f.listIcon, f.open)
	// The gallery view reuses the exact same cellImage path as Vignettes, so its
	// big preview + filmstrip show the same real raster thumbnails (a folder /
	// document glyph otherwise). A file dropped on a folder thumbnail is a move.
	f.galleryView = newGalleryView(f.fileModel, f.cellImage, f.open, f.emptyMessage)
	f.galleryView.onFileDrop = f.dropIntoFolder

	f.content = toolkit.NewStack()
	f.content.AddPage("liste", f.listView)
	f.content.AddPage("icones", f.iconView)
	f.content.AddPage("colonnes", f.columnView)
	f.content.AddPage("galerie", f.galleryView)
	f.content.Visible = pageName(f.viewMode.Get())

	// Root border.
	f.root = toolkit.NewBorder()
	f.root.North = f.toolbar
	f.root.NorthSize = finderToolbarH
	f.root.West = f.sidebar
	f.root.WestSize = finderSidebarW
	f.root.Center = f.content

	// The overlay wraps the border so a modal confirmation dialog can paint over
	// (and grab input from) the whole pane.
	f.overlay = &finderRoot{f: f}
}

// dropIntoFolder is the icon/list views' file-drop handler: it requests a move
// of srcPath into the folder at dstDir (labelled by its base name).
func (f *FinderPane) dropIntoFolder(dstDir, srcPath string) {
	f.requestMove(srcPath, dstDir, filepath.Base(dstDir))
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
	case ViewGallery:
		return "galerie"
	default:
		return "icones"
	}
}

// SetView switches the visible content view (and roots the Miller strip at the
// current directory the first time it is shown).
func (f *FinderPane) SetView(mode int) {
	if mode < ViewList || mode > ViewGallery {
		return
	}
	f.viewMode.Set(mode)
	f.switcher.Current().Set(mode)
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
	f.titleLabel.Text().Set("Réseau")
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
		f.titleLabel.Text().Set(titleFor(path))
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
	f.titleLabel.Text().Set(titleFor(dir.Path))
	f.sidebar.SetActive(dir.Path)
	f.iconView.clearSelection()
	f.galleryView.clearSelection()
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
// so the swapped or resized content is positioned before the next paint, and
// re-centres the modal dialog when one is up.
func (f *FinderPane) relayout() {
	if b := f.root.Bounds(); b.W > 0 && b.H > 0 {
		f.root.SetBounds(b)
	}
	f.layoutDialog()
}

// cellImage resolves a file item to its icon-view Image plus whether that Image
// is a real raster thumbnail, dimming the icon to a translucent "ghost" while
// the item is marked for a cut (⌘X) — the Finder/Explorer cue that it will move
// on the next paste.
func (f *FinderPane) cellImage(it shell.FileItem) (*toolkit.Image, bool) {
	img, raster := f.cellImageRaw(it)
	if img != nil && f.clip.IsCut(it.Path) {
		return dimImage(img), raster
	}
	return img, raster
}

// cellImageRaw resolves a file item to its icon-view Image plus whether that
// Image is a real raster thumbnail (so the cell can back a dark photo with a
// light chip): a cached thumbnail when the thumbnail policy supplies one,
// otherwise a theme icon or a tasteful drawn glyph (folder / picture / document).
func (f *FinderPane) cellImageRaw(it shell.FileItem) (*toolkit.Image, bool) {
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

// Root returns the composed pane (the border plus its modal-dialog overlay) for
// embedding in the shell.
func (f *FinderPane) Root() toolkit.Widget { return f.overlay }

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

// FocusGalleryImage selects the current directory's first image item in the
// Galerie view, so the big preview shows a real photo rather than the gallery's
// default first-item selection — which, because folders sort first, is often a
// folder glyph. It switches to the gallery view first, then selects the image (a
// no-op when the directory has no image). It is the gallery peer of
// CascadeColumns: a screenshot/testing helper that stages the signature look of
// the view without a live pointer.
func (f *FinderPane) FocusGalleryImage() {
	f.SetView(ViewGallery)
	for i := 0; i < f.fileModel.Len(); i++ {
		if f.fileModel.At(i).IsImage() {
			f.galleryView.syncItems()
			f.galleryView.SetSelected(i)
			f.relayout()
			return
		}
	}
}

// Finder dialog metrics.
const (
	finderDialogW = 420
	finderDialogH = 148
)

// finderScrim dims the pane behind the modal dialog (a ~40% black wash the pixel
// painter src-over composites).
var finderScrim = toolkit.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x66}

// finderRoot wraps the Finder's Border with a modal-dialog overlay. Normally it
// just forwards layout, paint and input to the border; while a confirmation (or
// error) dialog is up it dims the pane and grabs every event for the dialog, so
// the move confirmation is truly modal.
type finderRoot struct {
	toolkit.Base
	f *FinderPane
}

// SetBounds records the overlay bounds, forwards them to the border and
// re-centres any open dialog.
func (w *finderRoot) SetBounds(r toolkit.Rect) {
	w.Base.SetBounds(r)
	w.f.root.SetBounds(r)
	w.f.layoutDialog()
}

// Draw paints the border, then (when a dialog is up) the dim scrim and the
// dialog on top.
func (w *finderRoot) Draw(p painter.Painter, th *toolkit.Theme) {
	w.f.root.Draw(p, th)
	if w.f.dialog != nil {
		p.FillRect(w.Bounds(), finderScrim)
		w.f.dialog.Draw(p, th)
	}
}

// OnEvent routes input to the border, unless a dialog is up — then it forwards
// exclusively to the dialog (translated into the dialog's own local coordinate
// space, which toolkit.Dialog.OnEvent expects). A keyboard file-operation
// shortcut (⌘C/⌘X/⌘V and ⌘⌥V, Ctrl on Linux/Windows) is consumed here before it
// reaches the border, so it works regardless of which sub-view has the pointer.
func (w *finderRoot) OnEvent(ev toolkit.Event) {
	if w.f.dialog != nil {
		w.f.dialog.OnEvent(dialogLocalEvent(ev, w.Bounds(), w.f.dialog.Bounds()))
		return
	}
	if ev.Kind == toolkit.EventKeyDown && w.f.onShortcut(ev) {
		return
	}
	w.f.root.OnEvent(ev)
}

// dialogLocalEvent rewrites an overlay-local event into the dialog's own local
// space, mirroring the toolkit's internal parent→child event translation.
func dialogLocalEvent(ev toolkit.Event, parent, child toolkit.Rect) toolkit.Event {
	ev.X = ev.X + parent.X - child.X
	ev.Y = ev.Y + parent.Y - child.Y
	return ev
}

// requestMove pops the move-confirmation dialog for moving srcPath into destDir
// (shown to the user as destLabel). A no-op — src already inside destDir — is
// refused silently: no dialog, and the filesystem is left untouched.
func (f *FinderPane) requestMove(srcPath, destDir, destLabel string) {
	if srcPath == "" || destDir == "" {
		return
	}
	if filepath.Clean(filepath.Dir(srcPath)) == filepath.Clean(destDir) {
		return // dropping a file back into its own directory: nothing to do
	}
	body := centeredLabel("Déplacer « " + filepath.Base(srcPath) + " » vers « " + destLabel + " » ?")
	cancel := toolkit.NewButton("Annuler", f.closeDialog)
	confirm := toolkit.NewButton("Déplacer", func() { f.confirmMove(srcPath, destDir) })
	f.dialog = toolkit.NewDialog("Déplacer", body, cancel, confirm)
	f.dialog.OnClose = f.closeDialog
	f.layoutDialog()
}

// confirmMove performs the confirmed move: it closes the dialog, mutates the
// filesystem via shell.MoveFile (the ONLY point the real files change), refreshes
// the affected views, and on failure replaces the dialog with a dismissable
// error message rather than crashing.
func (f *FinderPane) confirmMove(srcPath, destDir string) {
	f.closeDialog()
	if _, err := shell.MoveFile(srcPath, destDir); err != nil {
		f.showMoveError(err)
		return
	}
	f.refreshAfterMove()
}

// showMoveError shows a dismissable dialog explaining why a move failed.
func (f *FinderPane) showMoveError(err error) {
	f.dialog = toolkit.NewDialog("Déplacement impossible",
		centeredLabel(moveErrorMessage(err)), toolkit.NewButton("OK", f.closeDialog))
	f.dialog.OnClose = f.closeDialog
	f.layoutDialog()
}

// closeDialog dismisses the modal dialog (if any) and re-lays the pane so the
// scrim clears on the next paint.
func (f *FinderPane) closeDialog() {
	f.dialog = nil
	if b := f.root.Bounds(); b.W > 0 && b.H > 0 {
		f.root.SetBounds(b)
	}
}

// layoutDialog centres the modal dialog within the overlay bounds (clamped to
// the pane when the pane is smaller than the dialog).
func (f *FinderPane) layoutDialog() {
	if f.dialog == nil {
		return
	}
	b := f.overlay.Bounds()
	if b.W <= 0 || b.H <= 0 {
		b = f.root.Bounds()
	}
	w, h := finderDialogW, finderDialogH
	if w > b.W {
		w = b.W
	}
	if h > b.H {
		h = b.H
	}
	f.dialog.SetBounds(toolkit.Rect{X: b.X + (b.W-w)/2, Y: b.Y + (b.H-h)/2, W: w, H: h})
}

// refreshAfterMove re-lists the current directory (the moved file left it, or a
// dropped file arrived in it) so every view reflects the new filesystem state.
func (f *FinderPane) refreshAfterMove() {
	if p := f.cwd.Get(); p != "" {
		f.Navigate(p)
		return
	}
	f.relayout()
}

// DemoMoveConfirm pops the move-confirmation dialog for the first file over the
// first folder in the current directory — a screenshot/testing helper so the
// confirmation overlay can be captured without a live drag. It is a no-op when
// the current directory has no file + folder pair.
func (f *FinderPane) DemoMoveConfirm() {
	var src, dstDir, dstLabel string
	for i := 0; i < f.fileModel.Len(); i++ {
		it := f.fileModel.At(i)
		switch {
		case it.IsDir && dstDir == "":
			dstDir, dstLabel = it.Path, it.Name
		case !it.IsDir && src == "":
			src = it.Path
		}
	}
	if src != "" && dstDir != "" {
		f.requestMove(src, dstDir, dstLabel)
	}
}

// DialogActive reports whether a modal dialog (move confirmation or error) is
// currently shown — for the capture CLI and tests.
func (f *FinderPane) DialogActive() bool { return f.dialog != nil }

// centeredLabel builds a horizontally + vertically centred dialog-body label.
func centeredLabel(text string) *toolkit.Label {
	l := toolkit.NewLabel(text)
	l.Align = toolkit.AlignCenter
	l.VAlign = toolkit.VMiddle
	return l
}

// moveErrorMessage maps a shell.MoveFile error to a French, user-facing sentence.
func moveErrorMessage(err error) string {
	switch {
	case errors.Is(err, shell.ErrMoveExists):
		return "Un élément du même nom existe déjà à cet endroit."
	case errors.Is(err, shell.ErrMoveIntoSelf):
		return "L'élément se trouve déjà dans ce dossier."
	case errors.Is(err, shell.ErrMoveNotDir):
		return "La destination n'est pas un dossier."
	default:
		return "Le déplacement a échoué : " + err.Error()
	}
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
