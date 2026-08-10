// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
	"runtime"
)

// PlaceKind classifies a sidebar entry so the render layer can pick an icon and
// so navigation can special-case the non-directory kinds (the network location
// has no path and shows an honest empty state).
type PlaceKind int

const (
	// PlaceFolder is an ordinary navigable directory (a Favoris entry).
	PlaceFolder PlaceKind = iota
	// PlaceApplications is the applications folder (a Favoris entry).
	PlaceApplications
	// PlacePictures / PlaceMovies / PlaceMusic / PlaceDownloads are the media
	// Favoris entries, split out so each gets a distinct sidebar glyph.
	PlacePictures
	PlaceMovies
	PlaceMusic
	PlaceDownloads
	PlaceDesktop
	PlaceDocuments
	// PlaceHome is the user's home directory (an Emplacements entry).
	PlaceHome
	// PlaceVolume is the startup volume / root drive (an Emplacements entry).
	PlaceVolume
	// PlaceNetwork is the network location: there is no real network browsing
	// in a pure-Go shell, so it navigates to an honest empty "Aucun partage"
	// state rather than a fake listing. Its Path is "".
	PlaceNetwork
	// PlaceTrash is the trash / recycle bin (an Emplacements entry).
	PlaceTrash
)

// Place is one sidebar row: a display label, the directory it navigates to
// (empty when the kind is not navigable, e.g. the network) and its kind.
type Place struct {
	Label string
	Path  string
	Kind  PlaceKind
}

// Navigable reports whether clicking the place opens a real directory listing.
// The network location is not navigable — it shows the empty-shares state.
func (p Place) Navigable() bool {
	return p.Kind != PlaceNetwork && p.Path != ""
}

// Places is the file manager's sidebar model: the two labelled sections a
// macOS Finder shows — Favoris (user folders) and Emplacements (home, the
// startup volume, the network and the trash).
type Places struct {
	Favorites []Place
	Locations []Place
}

// DefaultPlaces resolves the sidebar model for the running OS: real per-user
// paths on macOS (~/Desktop, ~/Documents, /Applications, ~/Pictures, ~/Movies,
// ~/Music, ~/Downloads; "Macintosh HD" -> /, ~/.Trash), the equivalent XDG
// user-dirs on Linux, and the known folders on Windows. When the home directory
// cannot be resolved (notably the js/wasm browser build, which has no real
// filesystem) it returns a minimal, non-crashing model so the sidebar still
// renders and the shell still builds.
func DefaultPlaces() *Places {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return placesFor(runtime.GOOS, home)
}

// placesFor is the OS-parameterised core of DefaultPlaces, so every per-OS
// branch is exercisable in a test on any single host. An empty home (notably the
// js/wasm browser build, which has no real filesystem) yields a minimal,
// non-crashing model so the sidebar still renders and the shell still builds.
func placesFor(goos, home string) *Places {
	if home == "" {
		return &Places{
			Locations: []Place{
				{Label: "Réseau", Kind: PlaceNetwork},
			},
		}
	}

	volPath, volLabel := volumePlace(goos)
	favorites := []Place{
		{Label: "Bureau", Path: filepath.Join(home, "Desktop"), Kind: PlaceDesktop},
		{Label: "Documents", Path: filepath.Join(home, "Documents"), Kind: PlaceDocuments},
		{Label: "Applications", Path: applicationsPath(goos), Kind: PlaceApplications},
		{Label: "Images", Path: filepath.Join(home, "Pictures"), Kind: PlacePictures},
		{Label: "Vidéos", Path: videosPath(goos, home), Kind: PlaceMovies},
		{Label: "Musique", Path: filepath.Join(home, "Music"), Kind: PlaceMusic},
		{Label: "Téléchargements", Path: filepath.Join(home, "Downloads"), Kind: PlaceDownloads},
	}
	locations := []Place{
		{Label: "Accueil", Path: home, Kind: PlaceHome},
		{Label: volLabel, Path: volPath, Kind: PlaceVolume},
		{Label: "Réseau", Kind: PlaceNetwork},
		{Label: "Corbeille", Path: trashPath(goos, home), Kind: PlaceTrash},
	}
	return &Places{Favorites: favorites, Locations: locations}
}

// applicationsPath resolves the "Applications" favourite's path for goos:
// /Applications on macOS, the freedesktop application dir on Linux, and the
// Program Files folder on Windows.
func applicationsPath(goos string) string {
	switch goos {
	case "darwin":
		return "/Applications"
	case "windows":
		if p := os.Getenv("ProgramFiles"); p != "" {
			return p
		}
		return `C:\Program Files`
	default:
		return "/usr/share/applications"
	}
}

// videosPath resolves the movies/videos favourite: macOS backs it with ~/Movies;
// other systems back it with ~/Videos.
func videosPath(goos, home string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Movies")
	}
	return filepath.Join(home, "Videos")
}

// volumePlace resolves the startup-volume location for goos: "Macintosh HD" -> /
// on macOS, the system drive on Windows, and "/" on Linux/other.
func volumePlace(goos string) (path, label string) {
	switch goos {
	case "darwin":
		return "/", "Macintosh HD"
	case "windows":
		if sd := os.Getenv("SystemDrive"); sd != "" {
			return sd + `\`, sd
		}
		return `C:\`, "C:"
	default:
		return "/", "Ordinateur"
	}
}

// trashPath resolves the trash directory for goos from home: ~/.Trash on macOS,
// the XDG trash on Linux, and the profile Recycle Bin path on Windows.
func trashPath(goos, home string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, ".Trash")
	case "windows":
		if sd := os.Getenv("SystemDrive"); sd != "" {
			return sd + `\$Recycle.Bin`
		}
		return `C:\$Recycle.Bin`
	default:
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, "Trash", "files")
		}
		return filepath.Join(home, ".local", "share", "Trash", "files")
	}
}
