// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"path/filepath"
	"testing"
)

func TestDefaultPlacesResolves(t *testing.T) {
	p := DefaultPlaces()
	if p == nil {
		t.Fatal("DefaultPlaces returned nil")
	}
	// On any host with a home dir there are 7 Favoris + 4 Emplacements; the
	// browser (empty home) path is covered by TestPlacesForEmptyHome.
	if len(p.Favorites) != 0 && len(p.Favorites) != 7 {
		t.Errorf("Favorites = %d, want 0 or 7", len(p.Favorites))
	}
}

func TestDefaultPlacesNoHome(t *testing.T) {
	// Force os.UserHomeDir to fail (empty HOME), exercising the error branch that
	// degrades to the minimal browser-style sidebar.
	t.Setenv("HOME", "")
	t.Setenv("home", "") // plan9/other, harmless elsewhere
	p := DefaultPlaces()
	if len(p.Favorites) != 0 {
		t.Errorf("no-home Favorites = %d, want 0", len(p.Favorites))
	}
}

func TestPlacesForEmptyHome(t *testing.T) {
	p := placesFor("js", "")
	if len(p.Favorites) != 0 {
		t.Errorf("empty-home Favorites = %d, want 0", len(p.Favorites))
	}
	if len(p.Locations) != 1 || p.Locations[0].Kind != PlaceNetwork {
		t.Errorf("empty-home Locations = %+v, want a single network place", p.Locations)
	}
}

func TestPlacesForEachOS(t *testing.T) {
	const home = "/home/u"
	cases := []struct {
		goos     string
		apps     string
		videos   string
		volLabel string
		volPath  string
		trash    string
	}{
		{"darwin", "/Applications", filepath.Join(home, "Movies"), "Macintosh HD", "/", filepath.Join(home, ".Trash")},
		{"linux", "/usr/share/applications", filepath.Join(home, "Videos"), "Ordinateur", "/", filepath.Join(home, ".local", "share", "Trash", "files")},
	}
	for _, c := range cases {
		p := placesFor(c.goos, home)
		if len(p.Favorites) != 7 || len(p.Locations) != 4 {
			t.Fatalf("%s: sections = %d/%d, want 7/4", c.goos, len(p.Favorites), len(p.Locations))
		}
		if got := p.Favorites[2].Path; got != c.apps {
			t.Errorf("%s apps = %q, want %q", c.goos, got, c.apps)
		}
		if got := p.Favorites[4].Path; got != c.videos {
			t.Errorf("%s videos = %q, want %q", c.goos, got, c.videos)
		}
		if got := p.Locations[1]; got.Label != c.volLabel || got.Path != c.volPath {
			t.Errorf("%s volume = %+v, want %s/%s", c.goos, got, c.volLabel, c.volPath)
		}
		if got := p.Locations[3].Path; got != c.trash {
			t.Errorf("%s trash = %q, want %q", c.goos, got, c.trash)
		}
	}
}

func TestPlacesWindows(t *testing.T) {
	const home = `C:\Users\u`
	// With the env unset, Windows falls back to the hard-coded defaults.
	t.Setenv("ProgramFiles", "")
	t.Setenv("SystemDrive", "")
	p := placesFor("windows", home)
	if got := p.Favorites[2].Path; got != `C:\Program Files` {
		t.Errorf("apps fallback = %q", got)
	}
	if got := p.Locations[1]; got.Label != "C:" || got.Path != `C:\` {
		t.Errorf("volume fallback = %+v", got)
	}
	if got := p.Locations[3].Path; got != `C:\$Recycle.Bin` {
		t.Errorf("trash fallback = %q", got)
	}
	// With the env set, they win.
	t.Setenv("ProgramFiles", `D:\Apps`)
	t.Setenv("SystemDrive", "E:")
	p = placesFor("windows", home)
	if got := p.Favorites[2].Path; got != `D:\Apps` {
		t.Errorf("apps env = %q", got)
	}
	if got := p.Locations[1]; got.Label != "E:" || got.Path != `E:\` {
		t.Errorf("volume env = %+v", got)
	}
	if got := p.Locations[3].Path; got != `E:\$Recycle.Bin` {
		t.Errorf("trash env = %q", got)
	}
}

func TestTrashPathXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got := trashPath("linux", "/home/u"); got != filepath.Join("/xdg", "Trash", "files") {
		t.Errorf("xdg trash = %q", got)
	}
}

func TestPlaceNavigable(t *testing.T) {
	if (Place{Kind: PlaceNetwork, Path: ""}).Navigable() {
		t.Error("network should not be navigable")
	}
	if (Place{Kind: PlaceHome, Path: ""}).Navigable() {
		t.Error("empty path should not be navigable")
	}
	if !(Place{Kind: PlaceHome, Path: "/home"}).Navigable() {
		t.Error("home with a path should be navigable")
	}
}
