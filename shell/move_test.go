// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// forceRename swaps the osRename seam for the test's lifetime, so the
// cross-volume copy+remove fallback can be driven without a second real volume.
func forceRename(t *testing.T, fn func(string, string) error) {
	t.Helper()
	prev := osRename
	osRename = fn
	t.Cleanup(func() { osRename = prev })
}

// exdev builds the cross-device error os.Rename reports when src and dest are on
// different volumes, so forceRename can make MoveFile take the copy fallback.
func exdev(src, dest string) error {
	return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EXDEV}
}

// requireDenyable skips a permission-denial subtest where chmod bits do not
// gate access: as root (euid 0) and on Windows (euid -1), a 0500 directory is
// still writable, so the "expect a permission error" assertions do not hold.
func requireDenyable(t *testing.T) {
	t.Helper()
	if os.Geteuid() <= 0 {
		t.Skip("permission-denial branch needs a non-root POSIX host")
	}
}

// denyDir chmods dir to r-x (0500) for the test and restores 0700 on cleanup so
// t.TempDir can remove it.
func denyDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod deny %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestMoveFileSameVolumeRename(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(srcDir, "note.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest, err := MoveFile(src, dstDir)
	if err != nil {
		t.Fatalf("MoveFile: %v", err)
	}
	if dest != filepath.Join(dstDir, "note.txt") {
		t.Errorf("dest = %q", dest)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("source should be gone after a same-volume move")
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "hello" {
		t.Errorf("moved content = %q, err=%v", b, err)
	}
}

func TestMoveFileCrossVolumeCopyRemove(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(dstDir, 0o755)
	src := filepath.Join(srcDir, "data.bin")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	forceRename(t, exdev) // pretend src and dst are on different volumes

	dest, err := MoveFile(src, dstDir)
	if err != nil {
		t.Fatalf("cross-volume MoveFile: %v", err)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("source should be removed after copy+remove")
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != "payload" {
		t.Errorf("copied content = %q, err=%v", b, err)
	}
}

func TestMoveFileNoop(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(src, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MoveFile(src, dir); !errors.Is(err, ErrMoveIntoSelf) {
		t.Errorf("no-op move err = %v, want ErrMoveIntoSelf", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("a refused no-op must leave the source untouched")
	}
}

func TestMoveFileCollision(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(dstDir, 0o755)
	src := filepath.Join(srcDir, "dup.txt")
	_ = os.WriteFile(src, []byte("new"), 0o644)
	_ = os.WriteFile(filepath.Join(dstDir, "dup.txt"), []byte("old"), 0o644)

	if _, err := MoveFile(src, dstDir); !errors.Is(err, ErrMoveExists) {
		t.Errorf("collision err = %v, want ErrMoveExists", err)
	}
	// The refusal must not overwrite the existing destination file.
	b, _ := os.ReadFile(filepath.Join(dstDir, "dup.txt"))
	if string(b) != "old" {
		t.Errorf("collision overwrote destination: %q", b)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("collision must leave the source in place")
	}
}

func TestMoveFileDestMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(src, []byte("a"), 0o644)
	if _, err := MoveFile(src, filepath.Join(dir, "no-such-dir")); err == nil {
		t.Error("moving into a missing directory should error")
	}
}

func TestMoveFileDestNotDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(src, []byte("a"), 0o644)
	notDir := filepath.Join(dir, "file")
	_ = os.WriteFile(notDir, []byte("f"), 0o644)
	if _, err := MoveFile(src, notDir); !errors.Is(err, ErrMoveNotDir) {
		t.Errorf("dest-not-dir err = %v, want ErrMoveNotDir", err)
	}
}

func TestMoveFileSameVolumePermissionDenied(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(dstDir, 0o755)
	src := filepath.Join(srcDir, "p.txt")
	_ = os.WriteFile(src, []byte("p"), 0o644)
	denyDir(t, dstDir) // read-only destination -> os.Rename fails (not EXDEV)

	if _, err := MoveFile(src, dstDir); err == nil {
		t.Error("moving into a read-only directory should error")
	} else if errors.Is(err, ErrMoveIntoSelf) || errors.Is(err, ErrMoveExists) || errors.Is(err, ErrMoveNotDir) {
		t.Errorf("permission failure misreported as a refusal: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("a failed move must leave the source in place")
	}
}

func TestMoveFileCrossVolumeReadError(t *testing.T) {
	root := t.TempDir()
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(dstDir, 0o755)
	forceRename(t, exdev)
	// src does not exist, so the copy fallback's ReadFile fails.
	missing := filepath.Join(root, "gone", "ghost.txt")
	if _, err := MoveFile(missing, dstDir); err == nil {
		t.Error("cross-volume move of a missing source should error")
	}
}

func TestMoveFileCrossVolumeWriteError(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(dstDir, 0o755)
	src := filepath.Join(srcDir, "w.txt")
	_ = os.WriteFile(src, []byte("w"), 0o644)
	forceRename(t, exdev)
	denyDir(t, dstDir) // read-only dest -> the copy's WriteFile fails

	if _, err := MoveFile(src, dstDir); err == nil {
		t.Error("cross-volume copy into a read-only dir should error")
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("a failed copy must leave the source in place")
	}
}

func TestMoveFileCrossVolumeRemoveError(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.MkdirAll(dstDir, 0o755)
	src := filepath.Join(srcDir, "r.txt")
	_ = os.WriteFile(src, []byte("r"), 0o644)
	forceRename(t, exdev)
	// srcDir is read-only: the copy reads src fine, writes dest fine, but the
	// final os.Remove(src) fails because its parent is not writable.
	denyDir(t, srcDir)

	if _, err := MoveFile(src, dstDir); err == nil {
		t.Error("cross-volume remove of a source in a read-only dir should error")
	}
	// The copy still landed at the destination.
	if _, err := os.Stat(filepath.Join(dstDir, "r.txt")); err != nil {
		t.Error("the copy should have reached the destination")
	}
}
