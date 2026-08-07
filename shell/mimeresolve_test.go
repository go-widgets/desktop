// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveNameDefaultAndCandidates(t *testing.T) {
	r := fixtureResolver(t)

	ow := r.ResolveName("note.txt", []byte("hello world"))
	if ow.MimeType != "text/plain" {
		t.Fatalf("mime = %q, want text/plain", ow.MimeType)
	}
	if !ow.HasDefault || ow.Default.Label() != "Editor" {
		t.Fatalf("default = %+v, HasDefault=%v", ow.Default, ow.HasDefault)
	}
	labels := make([]string, len(ow.Candidates))
	for i, a := range ow.Candidates {
		labels[i] = a.Label()
	}
	if len(labels) != 2 || labels[0] != "Editor" || labels[1] != "Viewer" {
		t.Fatalf("candidates = %v, want [Editor Viewer]", labels)
	}
}

func TestResolveNameNoDefault(t *testing.T) {
	r := fixtureResolver(t)
	// An unknown extension with nil content classifies as octet-stream, which
	// nothing is associated with: no default, no candidates.
	ow := r.ResolveName("thing.unknownext", nil)
	if ow.HasDefault {
		t.Errorf("unexpected default %+v", ow.Default)
	}
	if len(ow.Candidates) != 0 {
		t.Errorf("candidates = %v, want none", ow.Candidates)
	}
}

func TestResolvePath(t *testing.T) {
	r := fixtureResolver(t)
	dir := t.TempDir()
	txt := mkfile(t, dir, "note.txt", "hello world")
	mkfile(t, dir, "empty.txt", "")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Regular file with content.
	ow, err := r.ResolvePath(txt)
	if err != nil || ow.MimeType != "text/plain" {
		t.Fatalf("ResolvePath(txt) = %q, %v", ow.MimeType, err)
	}
	// Empty file: non-nil zero-length content -> zero-size sentinel type.
	if ow, err := r.ResolvePath(filepath.Join(dir, "empty.txt")); err != nil || ow.MimeType == "" {
		t.Fatalf("ResolvePath(empty) = %q, %v", ow.MimeType, err)
	}
	// Directory: classified by name only (non-regular -> nil content).
	if _, err := r.ResolvePath(sub); err != nil {
		t.Fatalf("ResolvePath(dir) err = %v", err)
	}
	// Missing path: stat failure surfaces as an error.
	if _, err := r.ResolvePath(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("ResolvePath(missing) expected error")
	}
}

func TestResolvePathOpenError(t *testing.T) {
	r := fixtureResolver(t)
	dir := t.TempDir()
	reg := mkfile(t, dir, "reg.txt", "data")

	boom := errors.New("boom")
	orig := osOpen
	osOpen = func(string) (*os.File, error) { return nil, boom }
	defer func() { osOpen = orig }()

	if _, err := r.ResolvePath(reg); !errors.Is(err, boom) {
		t.Fatalf("open error not surfaced: %v", err)
	}
}
