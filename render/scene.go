// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"image"
	"strings"

	"github.com/go-widgets/desktop/shell"
	"github.com/go-widgets/mvvm"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/toolkit/virtual"
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
}

// Scene is the composed desktop shell: a Border(menubar / dock / launcher /
// file-grid) plus a floating toast stack. All mutable state (the launcher
// search query and its results, the file model) flows through go-widgets/mvvm.
type Scene struct {
	cfg   Config
	theme *toolkit.Theme

	// mvvm state.
	query     *mvvm.Observable[string]
	results   *mvvm.ObservableList[shell.App]
	fileModel *mvvm.ObservableList[shell.FileItem]

	// widgets.
	root          *toolkit.Border
	dock          *toolkit.HBox
	launcher      *toolkit.ListBox
	launcherFrame *toolkit.Frame
	grid          *virtual.VirtualGrid[shell.FileItem]
	gridFrame     *toolkit.Frame
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

	// reusable per-cell widgets for the file grid.
	cellIcon  *toolkit.Image
	cellLabel *toolkit.Label

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
		cfg:       cfg,
		theme:     cfg.Theme,
		query:     mvvm.NewObservable(""),
		results:   mvvm.NewObservableList[shell.App](),
		fileModel: mvvm.NewObservableList[shell.FileItem](),
		cellLabel: toolkit.NewLabel(""),
	}
	s.cellIcon = toolkit.NewImageFit(nil, 0, 0)
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
	s.menubar = toolkit.NewMenuBar()
	s.menubar.AddMenu("Applications", s.appMenu())

	s.dock = toolkit.NewHBox()
	apps := s.appsSlice()
	n := len(apps)
	if n > s.cfg.DockMax {
		n = s.cfg.DockMax
	}
	for _, a := range apps[:n] {
		s.dock.AddFixed(s.cfg.Icons.Image(a.Icon), DefaultIconSize)
		s.dockCount++
	}

	// Launcher: a ListBox whose items track the search results list.
	s.launcher = toolkit.NewListBox(nil)
	s.results.SubscribeChanged(s.syncLauncher)
	s.query.SubscribeChanged(s.applyQuery)
	s.applyQuery() // seed results (and, via its subscription, the launcher)

	// File grid: a virtualized gengrid over the directory's items.
	if s.cfg.Dir != nil {
		s.fileModel.Append(s.cfg.Dir.Items...)
	}
	s.grid = virtual.NewVirtualGrid(
		s.fileModel,
		toolkit.Size{W: 120, H: 92},
		s.drawCell,
	)

	s.launcherFrame = toolkit.NewFrame(s.launcher)
	s.gridFrame = toolkit.NewFrame(s.grid)

	s.root = toolkit.NewBorder()
	s.root.North = s.menubar
	s.root.NorthSize = toolkit.MenuBarH
	s.root.South = s.dock
	s.root.SouthSize = DefaultIconSize + 12
	s.root.West = s.launcherFrame
	s.root.WestSize = 220
	s.root.Center = s.gridFrame
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

// drawCell renders one file-grid cell: an icon (or thumbnail) above the name.
func (s *Scene) drawCell(p painter.Painter, th *toolkit.Theme, r toolkit.Rect, _ int, item shell.FileItem) {
	iconH := r.H - toolkit.MenuBarH
	if iconH < 8 {
		iconH = r.H
	}
	img := s.cellImage(item)
	img.SetBounds(toolkit.Rect{X: r.X + (r.W-iconH)/2, Y: r.Y + 4, W: iconH, H: iconH})
	img.Draw(p, th)

	s.cellLabel.Text = elide(item.Name, 16)
	s.cellLabel.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + r.H - toolkit.MenuBarH, W: r.W, H: toolkit.MenuBarH})
	s.cellLabel.Draw(p, th)
}

// cellImage picks the icon/thumbnail Image for a file item, reusing the shared
// cell Image widget to avoid per-frame allocation.
func (s *Scene) cellImage(item shell.FileItem) *toolkit.Image {
	src := s.cfg.Icons.Image(s.iconNameFor(item))
	s.cellIcon.Pixels, s.cellIcon.W, s.cellIcon.H, s.cellIcon.Scale =
		src.Pixels, src.W, src.H, toolkit.ScaleFit
	return s.cellIcon
}

// iconNameFor maps a file item to an icon-loader key: "folder" for a directory,
// the thumbnail key when the thumbnail policy supplies one (an on-disk cache
// path natively, a virtual asset name in the browser), otherwise a per-MIME
// theme-icon name.
func (s *Scene) iconNameFor(item shell.FileItem) string {
	if item.IsDir {
		return "folder"
	}
	if s.thumbKey != nil {
		if k := s.thumbKey(item); k != "" {
			return k
		}
	}
	if item.Mime == "" {
		return "text-x-generic"
	}
	return strings.ReplaceAll(item.Mime, "/", "-")
}

// elide truncates s to max runes with an ellipsis.
func elide(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
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
	t.Visible = true
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
func (s *Scene) FileCount() int { return s.fileModel.Len() }

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
func (w *sceneWidget) OnEvent(ev toolkit.Event) { w.sc.root.OnEvent(ev) }

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
