// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Config bundles the shell models and rendering resources a Scene composes.
//
// There are two ways to fill it. Supply Source (a shell.AppSource) and New
// derives Apps, Menu, Dir, the icon loader and the thumbnail-key policy from it
// — the portable path: the identical Scene is produced whether Source scans a
// real XDG filesystem (native) or serves a curated set from an embed.FS
// (browser). Or supply the individual model fields directly (Apps, Menu, Dir,
// Thumbnailer, Icons) — the explicit path used by the render unit tests. Source
// takes precedence over the individual fields when both are set.
type Config struct {
	// Source, when non-nil, provides the app index, menu, directory listing,
	// icon bytes and thumbnail keys; New fills Apps/Menu/Dir/Icons from it.
	Source shell.AppSource

	Apps        *shell.AppIndex
	Menu        *shell.MenuModel
	Dir         *shell.Dir
	Thumbnailer *shell.Thumbnailer
	Icons       *IconLoader
	Theme       *toolkit.Theme
	Width       int
	Height      int
	// DockMax caps how many app icons appear in the dock (0 -> a sane default).
	DockMax int

	// Places is the file-manager sidebar model (Favoris + Emplacements). When
	// nil the finder resolves shell.DefaultPlaces() for the running OS.
	Places *shell.Places
	// Lister is the directory-navigation backend the finder calls as the user
	// browses. When nil it defaults to render.NativeLister (real filesystem +
	// name-based classification); the browser build degrades to an empty state.
	Lister func(path string) (*shell.Dir, error)
}

// Scene is the composed desktop shell: a Border(menubar / dock / launcher /
// file-grid) plus a floating toast stack. All mutable state (the launcher
// search query and its results, the file model) flows through go-widgets/mvvm.
type Scene struct {
	cfg   Config
	theme *toolkit.Theme

	// mvvm state.
	query   *mvvm.Observable[string]
	results *mvvm.ObservableList[shell.App]

	// widgets.
	root          *toolkit.Border
	dock          *toolkit.HBox
	launcher      *toolkit.ListBox
	launcherFrame *toolkit.Frame
	finder        *FinderPane
	menubar       *toolkit.MenuBar

	dockCount int
	toasts    []*toolkit.Toast

	// onLauncherChanged / onToastsChanged are the incremental-present hooks the
	// windowed HostRoot installs (nil on the -capture / plain-Widget paths):
	// they fire when the launcher's result list or the toast stack mutates, so
	// the damage-aware root can invalidate exactly the region that changed. See
	// (*Scene).HostRoot in host.go.
	onLauncherChanged func()
	onToastsChanged   func()

	// appGlyph is the tasteful generic app tile drawn once for a dock/launcher
	// app with no resolvable icon — so the shell never shows a blank grey
	// placeholder square. (The file-manager fallback glyphs live on the finder.)
	appGlyph *toolkit.Image

	// launcherIcons is a forked icon loader (its own cache, same backend) and
	// launcherAppGlyph a dedicated fallback tile, both OWNED by the launcher's
	// per-row renderer. The launcher draws icons imperatively, mutating each
	// Image's bounds per row; the dock holds icon Images as retained scene
	// children. Sharing one Image between the two would let a launcher-row
	// bounds mutation move a dock child into the launcher region, so the
	// damage-aware scene would repaint a stale dock icon there. Distinct objects
	// keep the two consumers independent.
	launcherIcons    *IconLoader
	launcherAppGlyph *toolkit.Image

	// thumbKey maps a file item to its thumbnail's icon-loader key ("" for
	// none). It is the source's ThumbKey when a Source was supplied, otherwise
	// a Thumbnailer-backed native policy (an existing on-disk cache path).
	thumbKey func(shell.FileItem) string
}

// New composes a Scene from cfg, wiring the mvvm bindings immediately. When
// cfg.Source is set, the app index, menu, directory listing, icon loader and
// thumbnail policy are all derived from it (so a browser embed.FS source and a
// native XDG source yield the same Scene); otherwise the explicit model fields
// are used as given.
func New(cfg Config) *Scene {
	// Install the proportional TrueType UI face before any widget lays out, so
	// the whole shell renders in clean anti-aliased text instead of the toolkit's
	// built-in 5x7 bitmap font. Pure app-level: it only calls the toolkit's
	// public SetFont seam (the toolkit is untouched) and runs at most once.
	UseUIFont()

	if cfg.Source != nil {
		cfg.Apps = cfg.Source.Apps()
		cfg.Menu = cfg.Source.Menu()
		cfg.Dir = cfg.Source.Dir()
		if cfg.Icons == nil {
			cfg.Icons = NewIconLoaderFunc(cfg.Source.IconBytes, DefaultIconSize)
		}
	}
	if cfg.Theme == nil {
		cfg.Theme = toolkit.DefaultDark()
	}
	if cfg.Icons == nil {
		cfg.Icons = NewIconLoader("", DefaultIconSize)
	}
	if cfg.DockMax <= 0 {
		cfg.DockMax = 12
	}
	s := &Scene{
		cfg:     cfg,
		theme:   cfg.Theme,
		query:   mvvm.NewObservable(""),
		results: mvvm.NewObservableList[shell.App](),
	}
	s.thumbKey = thumbKeyPolicy(cfg)
	s.build()
	return s
}

// thumbKeyPolicy picks the file-grid thumbnail-key resolver: the source's
// ThumbKey when a Source was supplied (portable — an embed.FS asset name in the
// browser, an on-disk cache path natively), otherwise a Thumbnailer-backed
// native policy that returns an existing on-disk cache path (and "" when the
// thumbnail has not been generated). It is nil when neither is available, in
// which case the file grid falls back to per-MIME theme icons.
func thumbKeyPolicy(cfg Config) func(shell.FileItem) string {
	if cfg.Source != nil {
		return cfg.Source.ThumbKey
	}
	if cfg.Thumbnailer != nil {
		tn := cfg.Thumbnailer
		return func(it shell.FileItem) string {
			if p := tn.Path(it); p != "" && fileExists(p) {
				return p
			}
			return ""
		}
	}
	return nil
}

// build lays out the widget tree and wires the mvvm bindings.
func (s *Scene) build() {
	// Rasterise the generic app tile once (theme-aware); the file-manager
	// fallback glyphs are owned by the finder.
	s.appGlyph = appTileIcon(DefaultIconSize, s.theme)

	// The launcher draws its icons imperatively (mutating each Image's bounds
	// per row), so it must own Image objects distinct from the dock's retained
	// scene children — a forked loader (own cache, same backend) and its own
	// fallback tile. See the field docs.
	s.launcherIcons = s.cfg.Icons.Fork()
	s.launcherAppGlyph = appTileIcon(DefaultIconSize, s.theme)

	s.menubar = toolkit.NewMenuBar()
	s.menubar.AddMenu("Applications", s.appMenu())

	s.dock = toolkit.NewHBox()
	apps := s.appsSlice()
	n := len(apps)
	if n > s.cfg.DockMax {
		n = s.cfg.DockMax
	}
	for _, a := range apps[:n] {
		s.dock.AddFixed(s.dockImage(a.Icon), DefaultIconSize)
		s.dockCount++
	}

	// Launcher: a ListBox whose items track the search results list. Each row
	// is drawn (via the ListBox's ItemRenderer seam — the toolkit is untouched)
	// as the app's real icon followed by its label, so the rail reads like an
	// application list rather than a bare column of names. The row is made tall
	// enough to seat the small icon with breathing room.
	s.launcher = toolkit.NewListBox(nil)
	s.launcher.RowHeight = launcherRowH
	s.launcher.ItemRenderer = s.drawLauncherRow
	s.results.SubscribeChanged(s.syncLauncher)
	s.query.SubscribeChanged(s.applyQuery)
	s.applyQuery() // seed results (and, via its subscription, the launcher)

	// File manager: the macOS-Finder-like browser fills the content region —
	// its Favoris/Emplacements sidebar, the Liste/Vignettes/Colonnes view
	// switcher + size slider, and the three views over the current directory.
	s.finder = NewFinderPane(FinderConfig{
		Icons:      s.cfg.Icons,
		Theme:      s.theme,
		Places:     s.cfg.Places,
		Lister:     s.cfg.Lister,
		ThumbKey:   s.thumbKey,
		InitialDir: s.cfg.Dir,
	})

	// A titled panel gives the launcher rail composed structure on the left.
	s.launcherFrame = toolkit.NewFrame(s.launcher)
	s.launcherFrame.Title = "Launcher"

	s.root = toolkit.NewBorder()
	s.root.North = s.menubar
	s.root.NorthSize = toolkit.MenuBarH
	s.root.South = s.dock
	s.root.SouthSize = DefaultIconSize + 12
	s.root.West = s.launcherFrame
	s.root.WestSize = 200
	s.root.Center = s.finder.Root()
}

// dockImage resolves a dock app's icon to real pixels, falling back to the
// tasteful generic app tile (never a blank grey square) when the icon does not
// resolve.
func (s *Scene) dockImage(name string) *toolkit.Image {
	if img, ok := s.cfg.Icons.TryImage(name); ok {
		return img
	}
	return s.appGlyph
}

// Launcher row metrics: a small logical app icon seated in a slightly taller
// row so the icon has breathing room above and below the label baseline.
const (
	launcherRowH   = 24 // pixels per launcher row (default ListBox row is 18)
	launcherIconPx = 18 // logical app-icon size drawn at the row's left
	launcherPad    = 6  // left inset and icon-to-label gap
)

// drawLauncherRow is the launcher ListBox's per-row content renderer: it paints
// the app's real icon at the row's left, vertically centred, then the label to
// its right on the same baseline the default text renderer uses. The ListBox
// still owns the row background (selection highlight), scrolling and selection —
// this only fills the content, so the rail reads as "icon + name" like a real
// application list. ink is the ListBox's resolved text colour (theme.OnSurface,
// or theme.Background on the selected row), so the label tracks selection.
func (s *Scene) drawLauncherRow(p painter.Painter, th *toolkit.Theme, rc toolkit.Rect, index int, item string, _ bool, ink toolkit.RGBA) {
	img := s.launcherIcon(index)
	iy := rc.Y + (rc.H-launcherIconPx)/2
	img.SetBounds(toolkit.Rect{X: rc.X + launcherPad, Y: iy, W: launcherIconPx, H: launcherIconPx})
	img.Draw(p, th)

	tx := rc.X + launcherPad + launcherIconPx + launcherPad
	ty := rc.Y + (rc.H-toolkit.GlyphHeight())/2
	toolkit.DrawText(p, tx, ty, item, ink)
}

// launcherIcon resolves the launcher row at index to its app's real icon,
// falling back to the tasteful drawn app tile (never a blank square) when the
// app has no resolvable icon — the launcher peer of dockImage. The row index
// tracks the results list, which syncLauncher mirrors into the ListBox items,
// so results.At(index) is the app whose label occupies that row.
func (s *Scene) launcherIcon(index int) *toolkit.Image {
	if index < 0 || index >= s.results.Len() {
		return s.launcherAppGlyph
	}
	if img, ok := s.launcherIcons.TryImage(s.results.At(index).Icon); ok {
		return img
	}
	return s.launcherAppGlyph
}

// regions returns the shell root's non-nil border regions in the border's own
// draw/hit order (North, South, West, East, Center) — exactly what
// (*toolkit.Border).Draw paints (this shell sets no splitters, so there are no
// splitter handles to reproduce). The incremental HostRoot exposes these as its
// scene children so a per-region invalidation repaints only that region.
func (s *Scene) regions() []toolkit.Widget {
	out := make([]toolkit.Widget, 0, 5)
	for _, w := range []toolkit.Widget{s.root.North, s.root.South, s.root.West, s.root.East, s.root.Center} {
		if w != nil {
			out = append(out, w)
		}
	}
	return out
}

// anchorToasts lays each toast into its stacked top-right slot within b, so its
// Bounds() reflects where Draw will paint it (AnchorIn sets the toast's bounds).
// Both the full-surface composite and the incremental scene read these bounds.
func (s *Scene) anchorToasts(b toolkit.Rect) {
	for i, t := range s.toasts {
		t.AnchorIn(b, toolkit.TopRight, i)
	}
}

// drawComposited paints the shell root then overlays the toast stack within b —
// the single full-surface composite shared by the plain-Widget adapter and the
// HostRoot full-repaint fallback, so the two paths cannot drift.
func (s *Scene) drawComposited(p painter.Painter, th *toolkit.Theme, b toolkit.Rect) {
	s.root.SetBounds(b)
	s.root.Draw(p, th)
	s.anchorToasts(b)
	for _, t := range s.toasts {
		t.Draw(p, th)
	}
}

// appsSlice returns the indexed apps (empty when no index was supplied).
func (s *Scene) appsSlice() []shell.App {
	if s.cfg.Apps == nil {
		return nil
	}
	return s.cfg.Apps.All()
}

// applyQuery recomputes the search results into the results list.
func (s *Scene) applyQuery() {
	var out []shell.App
	if s.cfg.Apps != nil {
		out = s.cfg.Apps.Search(s.query.Get())
	}
	s.results.Clear()
	s.results.Append(out...)
}

// syncLauncher mirrors the results list into the launcher's item labels, then
// (on the incremental path) invalidates the launcher region so only the west
// strip repaints. The hook is nil during build's initial seed and on the
// -capture / plain-Widget paths.
func (s *Scene) syncLauncher() {
	items := make([]string, s.results.Len())
	for i := 0; i < s.results.Len(); i++ {
		items[i] = s.results.At(i).Label()
	}
	s.launcher.Items = items
	if s.onLauncherChanged != nil {
		s.onLauncherChanged()
	}
}

// appMenu builds the top-level application menu: one submenu per category.
func (s *Scene) appMenu() *toolkit.Menu {
	var items []toolkit.MenuItem
	if s.cfg.Menu != nil {
		for _, cat := range s.cfg.Menu.Categories {
			var appItems []toolkit.MenuItem
			for _, a := range cat.Apps {
				appItems = append(appItems, toolkit.MenuItem{Label: a.Label()})
			}
			items = append(items, toolkit.MenuItem{
				Label:   cat.Name,
				Submenu: toolkit.NewMenu(appItems),
			})
		}
	}
	return toolkit.NewMenu(items)
}

// SetQuery updates the launcher search query, driving the results + launcher.
func (s *Scene) SetQuery(q string) { s.query.Set(q) }

// ShowToast adds a toast to the floating stack (used to present notifications),
// then fires the toast-changed hook so the incremental path repaints the toast
// overlay. A nil toast is ignored (and never fires the hook).
func (s *Scene) ShowToast(t *toolkit.Toast) {
	if t == nil {
		return
	}
	t.Visible().Set(true)
	s.toasts = append(s.toasts, t)
	s.toastsChanged()
}

// SetToasts replaces the floating toast stack (e.g. from a notification daemon)
// and fires the toast-changed hook.
func (s *Scene) SetToasts(ts []*toolkit.Toast) {
	s.toasts = ts
	s.toastsChanged()
}

// toastsChanged notifies the incremental path (if installed) that the toast
// stack mutated — appeared, ticked, reflowed or dismissed. The hook re-anchors
// the stack and damages it; it is nil on the -capture / plain-Widget paths.
func (s *Scene) toastsChanged() {
	if s.onToastsChanged != nil {
		s.onToastsChanged()
	}
}

// DockCount is the number of icons in the dock.
func (s *Scene) DockCount() int { return s.dockCount }

// AppCount is the number of launchable apps currently listed in the launcher.
func (s *Scene) AppCount() int { return len(s.launcher.Items) }

// MenuCategoryCount is the number of application-menu categories.
func (s *Scene) MenuCategoryCount() int {
	if s.cfg.Menu == nil {
		return 0
	}
	return s.cfg.Menu.Len()
}

// FileCount is the number of items in the file grid model.
func (s *Scene) FileCount() int { return s.finder.FileCount() }

// Finder returns the composed file-manager pane, so a caller (the -capture CLI,
// tests) can select a view mode, icon size or directory before rendering.
func (s *Scene) Finder() *FinderPane { return s.finder }

// ToastCount is the number of visible toasts.
func (s *Scene) ToastCount() int { return len(s.toasts) }

// Root returns the composed root widget.
func (s *Scene) Root() toolkit.Widget { return s.root }

// Theme returns the theme the scene paints with, so a windowing backend paints
// its background and widgets with the same palette the -capture path uses.
func (s *Scene) Theme() *toolkit.Theme { return s.theme }

// Widget returns the whole shell — the Border root plus its floating toast
// overlay — as a single toolkit.Widget. A windowing backend such as
// github.com/go-widgets/window drives exactly one root widget through its Run
// loop, so wrapping the overlay here lets the live window show the same
// composition (dock, launcher, menu, file grid AND notification toasts) that
// Render paints to a PNG. It is backend-agnostic: it only touches toolkit
// primitives and reads the scene's live toast slice at Draw time, so a toast
// pushed by the notifications daemon appears on the next repaint.
func (s *Scene) Widget() toolkit.Widget { return &sceneWidget{sc: s} }

// sceneWidget adapts a Scene to one toolkit.Widget: it lays the shell root into
// its bounds, draws it, then anchors and draws the toast stack on top, and
// forwards input to the root. It mirrors Scene.Render's root-then-toasts
// compositing, but as a live widget a backend can Run rather than a one-shot
// image.
type sceneWidget struct {
	toolkit.Base
	sc *Scene
}

// SetBounds records the widget bounds and propagates them to the shell root so
// the root lays out before the first event is routed.
func (w *sceneWidget) SetBounds(r toolkit.Rect) {
	w.Base.SetBounds(r)
	w.sc.root.SetBounds(r)
}

// Draw paints the shell root then overlays the floating toast stack, anchored
// top-right within the widget's bounds exactly as Scene.Render does.
func (w *sceneWidget) Draw(p painter.Painter, th *toolkit.Theme) {
	w.sc.drawComposited(p, th, w.Bounds())
}

// OnEvent forwards input to the shell root (the toast overlay is passive).
func (w *sceneWidget) OnEvent(ev toolkit.Event) { w.sc.routeInput(ev) }

// routeInput dispatches one event into the shell. A keyboard event goes
// straight to the Finder pane (the shell's keyboard-focus target for file
// operations — ⌘C/⌘X/⌘V), because the Border routes only by pointer position
// and a key event carries none; every other event follows the normal
// pointer-hit routing through the root.
func (s *Scene) routeInput(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventKeyDown, toolkit.EventKeyUp, toolkit.EventChar:
		s.finder.Root().OnEvent(ev)
	default:
		s.root.OnEvent(ev)
	}
}

// Render paints the whole shell (root widget then the toast stack) into a fresh
// image of the configured size.
func (s *Scene) Render() (*image.RGBA, error) {
	w, h := s.cfg.Width, s.cfg.Height
	if w <= 0 {
		w = 960
	}
	if h <= 0 {
		h = 600
	}
	buf := make([]byte, 4*w*h)
	p := painter.NewPixelPainter(buf, w, h)
	host := toolkit.Rect{X: 0, Y: 0, W: w, H: h}
	p.FillRect(host, s.theme.Background)
	s.drawComposited(p, s.theme, host)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, buf)
	return img, nil
}
