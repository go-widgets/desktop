// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	mkfile(t, dir, "b.txt", "b")
	mkfile(t, dir, "A.txt", "a")
	// Hidden entries (a dotfile and a dot-directory) must be filtered out.
	mkfile(t, dir, ".hidden", "secret")
	if err := os.Mkdir(filepath.Join(dir, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "zoo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "Alpha"), 0o755); err != nil {
		t.Fatal(err)
	}

	d, err := ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(d.Items))
	for i, it := range d.Items {
		got[i] = it.Name
		if it.Name == ".hidden" || it.Name == ".config" {
			t.Fatalf("hidden entry %q was not filtered out: %v", it.Name, got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("listing = %v, want 4 visible entries (hidden filtered)", got)
	}
	// Directories first (case-insensitive), then files (case-insensitive).
	want := []string{"Alpha", "zoo", "A.txt", "b.txt"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if d.Path != dir {
		t.Errorf("Path = %q, want %q", d.Path, dir)
	}

	if _, err := ListDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("ListDir(missing) expected error")
	}
}

func TestClassify(t *testing.T) {
	r := fixtureResolver(t)
	dir := t.TempDir()
	mkfile(t, dir, "note.txt", "hello")
	mkfile(t, dir, "pic.png", "\x89PNG fake")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Append an item pointing at a nonexistent path: its resolution errors and
	// is skipped, leaving Mime empty (exercises the error branch).
	d.Items = append(d.Items, FileItem{Name: "ghost", Path: filepath.Join(dir, "ghost")})

	d.Classify(r)
	byName := map[string]string{}
	for _, it := range d.Items {
		byName[it.Name] = it.Mime
	}
	if byName["sub"] != MimeDirectory {
		t.Errorf("sub mime = %q, want %q", byName["sub"], MimeDirectory)
	}
	if byName["note.txt"] != "text/plain" {
		t.Errorf("note.txt mime = %q, want text/plain", byName["note.txt"])
	}
	if byName["pic.png"] != "image/png" {
		t.Errorf("pic.png mime = %q, want image/png", byName["pic.png"])
	}
	if byName["ghost"] != "" {
		t.Errorf("ghost mime = %q, want empty", byName["ghost"])
	}
}
