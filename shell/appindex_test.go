// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"reflect"
	"testing"

	"github.com/go-freedesktop/desktopentry"
)

func TestLaunchable(t *testing.T) {
	cases := []struct {
		name string
		e    *desktopentry.Entry
		want bool
	}{
		{"nil", nil, false},
		{"wrong type", &desktopentry.Entry{Type: "Link", Exec: "x"}, false},
		{"empty exec", &desktopentry.Entry{Type: "Application", Exec: "   "}, false},
		{"blank type ok", &desktopentry.Entry{Exec: "run"}, true},
		{"application ok", &desktopentry.Entry{Type: "Application", Exec: "run"}, true},
	}
	for _, c := range cases {
		if got := launchable(c.e); got != c.want {
			t.Errorf("%s: launchable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAppLabelAndEntry(t *testing.T) {
	e := &desktopentry.Entry{ID: "id-only", Exec: "x"}
	a := newApp(e)
	if a.Label() != "id-only" {
		t.Errorf("Label fallback = %q, want id-only", a.Label())
	}
	if a.Entry() != e {
		t.Error("Entry() did not return the source entry")
	}
	named := newApp(&desktopentry.Entry{ID: "x", Name: "Nice", Exec: "x"})
	if named.Label() != "Nice" {
		t.Errorf("Label = %q, want Nice", named.Label())
	}
	if (App{}).Entry() != nil {
		t.Error("zero App Entry() should be nil")
	}
}

func TestNewAppIndexDedupSortAndFilter(t *testing.T) {
	entries := []*desktopentry.Entry{
		{ID: "z", Name: "Zebra", Exec: "z"},
		{ID: "a", Name: "apple", Exec: "a"},
		{ID: "a", Name: "apple-dup", Exec: "a2"}, // duplicate id -> dropped
		{ID: "", Name: "beta", Exec: "b"},        // empty id -> no dedup, kept
		{ID: "", Name: "beta2", Exec: "b2"},      // empty id -> kept too
		{ID: "link", Name: "Link", Type: "Link", Exec: "l"}, // not launchable
		nil,
	}
	idx := NewAppIndex(entries)
	got := make([]string, idx.Len())
	for i := 0; i < idx.Len(); i++ {
		got[i] = idx.At(i).Label()
	}
	want := []string{"apple", "beta", "beta2", "Zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("index labels = %v, want %v", got, want)
	}
	if all := idx.All(); len(all) != idx.Len() || all[0].Label() != "apple" {
		t.Errorf("All() = %v", all)
	}
	// All returns a copy: mutating it must not affect the index.
	all := idx.All()
	all[0] = App{}
	if idx.At(0).Label() != "apple" {
		t.Error("All() did not return a copy")
	}
}

func TestMatchesDirect(t *testing.T) {
	e := &desktopentry.Entry{
		ID:          "org.x.Editor",
		Name:        "Text Editor",
		GenericName: "Editor",
		Comment:     "edits things",
		Keywords:    []string{"write", ""},
		Categories:  []string{"Utility"},
		Names:       map[string]string{"fr": "Éditeur", "": "Text Editor"},
	}
	a := newApp(e)
	for _, q := range []string{"editor", "édit", "write", "utility", "org.x", "edits"} {
		if !matches(a, q) {
			t.Errorf("matches(%q) = false, want true", q)
		}
	}
	if matches(a, "zzz-nope") {
		t.Error("unexpected match")
	}
	// entry == nil branch: fields come only from the App itself.
	nilEntry := App{Name: "Solo"}
	if !matches(nilEntry, "solo") {
		t.Error("nil-entry App should still match its own Name")
	}
	if matches(App{}, "anything") {
		t.Error("empty App matched a non-empty query")
	}
}

func TestSearch(t *testing.T) {
	idx := NewAppIndex([]*desktopentry.Entry{
		{ID: "a", Name: "Alpha", Exec: "a", Keywords: []string{"first"}},
		{ID: "b", Name: "Bravo", Exec: "b"},
	})
	if got := idx.Search("   "); len(got) != 2 {
		t.Errorf("blank query = %d apps, want 2", len(got))
	}
	got := idx.Search("first")
	if len(got) != 1 || got[0].Label() != "Alpha" {
		t.Errorf("keyword search = %v", got)
	}
	if got := idx.Search("nomatch"); got != nil {
		t.Errorf("no-match search = %v, want nil", got)
	}
}
