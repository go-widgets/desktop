// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package shell holds the pure composition/model logic of the desktop shell:
// the launchable-app index, MIME "open with" resolution, the categorized
// application-menu model, the directory listing model and the thumbnail
// cache-key derivation. None of it touches a rendering surface, so every
// branch is unit-coverable against temp-dir fixtures; the render package turns
// these models into go-widgets widgets.
package shell

import (
	"sort"
	"strings"

	"github.com/go-freedesktop/desktopentry"
)

// App is one launchable application, distilled from a desktop entry into the
// fields a dock / launcher needs. The originating entry is retained so the
// launcher can expand its Exec line at click time.
type App struct {
	ID          string
	Name        string
	GenericName string
	Comment     string
	Icon        string
	Exec        string
	Categories  []string
	Keywords    []string
	Terminal    bool

	entry *desktopentry.Entry
}

// Entry returns the desktop entry this App was built from (nil for an App that
// was not derived from one). The launcher passes it to desktopentry.ExpandExec.
func (a App) Entry() *desktopentry.Entry { return a.entry }

// Label is the user-visible name, falling back to the desktop-file id when the
// entry carries no Name.
func (a App) Label() string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}

// newApp projects a desktop entry onto an App. It never returns the entry's
// own slices by reference beyond read use.
func newApp(e *desktopentry.Entry) App {
	return App{
		ID:          e.ID,
		Name:        e.Name,
		GenericName: e.GenericName,
		Comment:     e.Comment,
		Icon:        e.Icon,
		Exec:        e.Exec,
		Categories:  e.Categories,
		Keywords:    e.Keywords,
		Terminal:    e.Terminal,
		entry:       e,
	}
}

// launchable reports whether an entry can appear in the index: it must be an
// Application (an empty Type is treated as Application, per lenient real-world
// files) and carry a non-blank Exec.
func launchable(e *desktopentry.Entry) bool {
	if e == nil {
		return false
	}
	if e.Type != "" && e.Type != "Application" {
		return false
	}
	return strings.TrimSpace(e.Exec) != ""
}

// AppIndex is a sorted, de-duplicated, searchable set of launchable apps.
type AppIndex struct {
	apps []App
}

// NewAppIndex builds an index from scanned desktop entries. Non-launchable
// entries (wrong Type, no Exec) are dropped; the first entry seen for a given
// non-empty id wins (desktopentry.Scan already applies XDG directory
// precedence, so "first" means "highest priority"). Apps are ordered
// case-insensitively by their display label.
func NewAppIndex(entries []*desktopentry.Entry) *AppIndex {
	seen := make(map[string]bool)
	apps := make([]App, 0, len(entries))
	for _, e := range entries {
		if !launchable(e) {
			continue
		}
		if e.ID != "" {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
		}
		apps = append(apps, newApp(e))
	}
	sort.SliceStable(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Label()) < strings.ToLower(apps[j].Label())
	})
	return &AppIndex{apps: apps}
}

// Len is the number of indexed apps.
func (x *AppIndex) Len() int { return len(x.apps) }

// At returns the app at index i.
func (x *AppIndex) At(i int) App { return x.apps[i] }

// All returns a copy of the indexed apps in order.
func (x *AppIndex) All() []App { return append([]App(nil), x.apps...) }

// matches reports whether app a satisfies the lowercased query, testing the
// display name, generic name, comment, id, every localized name, every keyword
// and every category as case-insensitive substrings.
func matches(a App, q string) bool {
	fields := []string{a.Name, a.GenericName, a.Comment, a.ID}
	if a.entry != nil {
		for _, v := range a.entry.Names {
			fields = append(fields, v)
		}
	}
	fields = append(fields, a.Keywords...)
	fields = append(fields, a.Categories...)
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// Search returns the apps matching query in index order. A blank (or
// whitespace-only) query returns every app.
func (x *AppIndex) Search(query string) []App {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return x.All()
	}
	var out []App
	for _, a := range x.apps {
		if matches(a, q) {
			out = append(out, a)
		}
	}
	return out
}
