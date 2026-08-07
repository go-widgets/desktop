// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"testing"

	"github.com/go-freedesktop/desktopentry"
	"github.com/go-freedesktop/menu"
)

func app(id, name string) *desktopentry.Entry {
	return &desktopentry.Entry{ID: id, Name: name, Exec: id}
}

func TestNewMenuModelNilAndEmptyRoot(t *testing.T) {
	if m := NewMenuModel(nil); m.Len() != 0 {
		t.Errorf("nil tree = %d categories, want 0", m.Len())
	}
	if m := NewMenuModel(&menu.Tree{}); m.Len() != 0 {
		t.Errorf("nil root = %d categories, want 0", m.Len())
	}
}

func TestNewMenuModelFlatten(t *testing.T) {
	tree := &menu.Tree{Root: &menu.Menu{
		Name: "Applications",
		// Root itself has no apps -> not a category, but is descended into.
		Submenus: []*menu.Menu{
			{
				Name:          "Internet",
				DirectoryName: "Internet & Network",
				Icon:          "applications-internet",
				Apps:          []*desktopentry.Entry{app("browser", "Browser")},
			},
			{
				// no DirectoryName -> label falls back to Name
				Name: "Utilities",
				Apps: []*desktopentry.Entry{
					app("term", "Terminal"),
					{ID: "broken", Type: "Link"}, // non-launchable, filtered out
				},
				Submenus: []*menu.Menu{
					{Name: "System", DirectoryName: "System Tools",
						Apps: []*desktopentry.Entry{app("monitor", "Monitor")}},
				},
			},
			{
				// a submenu whose only apps are non-launchable -> emitted as
				// zero categories (apps slice ends up empty).
				Name: "Empty",
				Apps: []*desktopentry.Entry{{ID: "x", Type: "Link"}},
			},
			{
				// purely structural: no apps, but nested apps deeper down.
				Name: "Structural",
				Submenus: []*menu.Menu{
					{Name: "Deep", DirectoryName: "Deep", Apps: []*desktopentry.Entry{app("d", "Deep App")}},
				},
			},
		},
	}}
	m := NewMenuModel(tree)
	got := make([]string, m.Len())
	for i, c := range m.Categories {
		got[i] = c.Name
	}
	want := []string{"Internet & Network", "Utilities", "System Tools", "Deep"}
	if len(got) != len(want) {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("categories = %v, want %v", got, want)
		}
	}
	if m.Categories[0].Icon != "applications-internet" {
		t.Errorf("icon = %q", m.Categories[0].Icon)
	}
	if len(m.Categories[1].Apps) != 1 || m.Categories[1].Apps[0].Label() != "Terminal" {
		t.Errorf("Utilities apps = %v", m.Categories[1].Apps)
	}
}
