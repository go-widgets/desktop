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
type Config struct {
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
	root     *toolkit.Border
	dock     *toolkit.HBox
	launcher *toolkit.ListBox
	grid     *virtual.VirtualGrid[shell.FileItem]
	menubar  *toolkit.MenuBar

	dockCount int
	toasts    []*toolkit.Toast

	// reusable per-cell widgets for the file grid.
	cellIcon  *toolkit.Image
	cellLabel *toolkit.Label
}

// New composes a Scene from cfg, wiring the mvvm bindings immediately.
func New(cfg Config) *Scene {
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
	s.build()
	return s
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

	s.root = toolkit.NewBorder()
	s.root.North = s.menubar
	s.root.NorthSize = toolkit.MenuBarH
	s.root.South = s.dock
	s.root.SouthSize = DefaultIconSize + 12
	s.root.West = toolkit.NewFrame(s.launcher)
	s.root.WestSize = 220
	s.root.Center = toolkit.NewFrame(s.grid)
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

// syncLauncher mirrors the results list into the launcher's item labels.
func (s *Scene) syncLauncher() {
	items := make([]string, s.results.Len())
	for i := 0; i < s.results.Len(); i++ {
		items[i] = s.results.At(i).Label()
	}
	s.launcher.Items = items
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
	src := s.cfg.Icons.Image(iconNameFor(item, s.cfg.Thumbnailer))
	s.cellIcon.Pixels, s.cellIcon.W, s.cellIcon.H, s.cellIcon.Scale =
		src.Pixels, src.W, src.H, toolkit.ScaleFit
	return s.cellIcon
}

// iconNameFor maps a file item to an icon-theme name (or a thumbnail path when
// one exists in the cache).
func iconNameFor(item shell.FileItem, tn *shell.Thumbnailer) string {
	if item.IsDir {
		return "folder"
	}
	if tn != nil {
		if p := tn.Path(item); p != "" && fileExists(p) {
			return p // absolute path -> loaded directly
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

// ShowToast adds a toast to the floating stack (used to present notifications).
func (s *Scene) ShowToast(t *toolkit.Toast) {
	if t == nil {
		return
	}
	t.Visible = true
	s.toasts = append(s.toasts, t)
}

// SetToasts replaces the floating toast stack (e.g. from a notification daemon).
func (s *Scene) SetToasts(ts []*toolkit.Toast) { s.toasts = ts }

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
	p.FillRect(toolkit.Rect{X: 0, Y: 0, W: w, H: h}, s.theme.Background)

	host := toolkit.Rect{X: 0, Y: 0, W: w, H: h}
	s.root.SetBounds(host)
	s.root.Draw(p, s.theme)

	for i, t := range s.toasts {
		t.AnchorIn(host, toolkit.TopRight, i)
		t.Draw(p, s.theme)
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, buf)
	return img, nil
}
