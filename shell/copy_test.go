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

// forceChmod / forceMkdirAll / forceRemoveAll swap a creating/destructive
// filesystem seam for the test's lifetime, so its error-return path can be
// exercised deterministically without a contrived on-disk layout (the peer of
// move_test.go's forceRename).
func forceChmod(t *testing.T, fn func(string, os.FileMode) error) {
	t.Helper()
	prev := osChmod
	osChmod = fn
	t.Cleanup(func() { osChmod = prev })
}

func forceMkdirAll(t *testing.T, fn func(string, os.FileMode) error) {
	t.Helper()
	prev := osMkdirAll
	osMkdirAll = fn
	t.Cleanup(func() { osMkdirAll = prev })
}

func forceRemoveAll(t *testing.T, fn func(string) error) {
	t.Helper()
	prev := osRemoveAll
	osRemoveAll = fn
	t.Cleanup(func() { osRemoveAll = prev })
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// --- CopyFile: the happy paths ------------------------------------------------

func TestCopyFileToOtherDir(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "src"), filepath.Join(root, "dst")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "note.txt")
	writeFile(t, src, "hello")

	dest, err := CopyFile(src, dstDir)
	if err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	if dest != filepath.Join(dstDir, "note.txt") {
		t.Fatalf("dest = %q", dest)
	}
	if got := readFile(t, dest); got != "hello" {
		t.Fatalf("copied body = %q", got)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source vanished: %v", err)
	}
}

func TestCopyFileSameDirSuffixesTwice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	writeFile(t, src, "x")

	first, err := CopyFile(src, dir)
	if err != nil {
		t.Fatalf("first copy: %v", err)
	}
	if got := filepath.Base(first); got != "photo copie.png" {
		t.Fatalf("first suffix = %q", got)
	}
	second, err := CopyFile(src, dir)
	if err != nil {
		t.Fatalf("second copy: %v", err)
	}
	if got := filepath.Base(second); got != "photo copie 2.png" {
		t.Fatalf("second suffix = %q", got)
	}
}

func TestCopyFileSameDirExtensionless(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "notes")
	writeFile(t, src, "x")
	dest, err := CopyFile(src, dir)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := filepath.Base(dest); got != "notes copie" {
		t.Fatalf("suffix = %q", got)
	}
}

func TestCopyFileRecursiveDirectoryPreservesMode(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tree")
	mustMkdir(t, filepath.Join(src, "sub"))
	writeFile(t, filepath.Join(src, "a.txt"), "A")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "B")
	if err := os.Chmod(filepath.Join(src, "a.txt"), 0o640); err != nil {
		t.Fatal(err)
	}
	dstDir := filepath.Join(root, "dst")
	mustMkdir(t, dstDir)

	dest, err := CopyFile(src, dstDir)
	if err != nil {
		t.Fatalf("CopyFile dir: %v", err)
	}
	if got := readFile(t, filepath.Join(dest, "sub", "b.txt")); got != "B" {
		t.Fatalf("nested copy = %q", got)
	}
	fi, err := os.Stat(filepath.Join(dest, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode not preserved: %v", fi.Mode().Perm())
	}
}

// --- CopyFile: refusals -------------------------------------------------------

func TestCopyFileMissingDestDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "f")
	writeFile(t, src, "x")
	if _, err := CopyFile(src, filepath.Join(dir, "nope")); err == nil {
		t.Fatal("want error for missing destDir")
	}
}

func TestCopyFileMissingSourceSurfacesError(t *testing.T) {
	root := t.TempDir()
	dstDir := filepath.Join(root, "d")
	mustMkdir(t, dstDir)
	if _, err := CopyFile(filepath.Join(root, "ghost.txt"), dstDir); err == nil {
		t.Fatal("want a stat error for a missing source")
	}
}

func TestCopyFileDestNotDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "f")
	writeFile(t, src, "x")
	notDir := filepath.Join(dir, "file")
	writeFile(t, notDir, "y")
	if _, err := CopyFile(src, notDir); !errors.Is(err, ErrCopyNotDir) {
		t.Fatalf("want ErrCopyNotDir, got %v", err)
	}
}

func TestCopyFileCollisionRefused(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "dup.txt")
	writeFile(t, src, "new")
	writeFile(t, filepath.Join(dstDir, "dup.txt"), "old")

	if _, err := CopyFile(src, dstDir); !errors.Is(err, ErrCopyExists) {
		t.Fatalf("want ErrCopyExists, got %v", err)
	}
	if got := readFile(t, filepath.Join(dstDir, "dup.txt")); got != "old" {
		t.Fatalf("collision overwrote: %q", got)
	}
}

func TestCopyFileIntoOwnSubtree(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a")
	sub := filepath.Join(src, "sub")
	mustMkdir(t, sub)
	if _, err := CopyFile(src, sub); !errors.Is(err, ErrCopyIntoSelf) {
		t.Fatalf("want ErrCopyIntoSelf, got %v", err)
	}
}

func TestCopyFileUnreadableSourceSurfacesError(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	dstDir := filepath.Join(root, "d")
	mustMkdir(t, dstDir)
	src := filepath.Join(root, "secret.txt")
	writeFile(t, src, "x")
	if err := os.Chmod(src, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o600) })
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want a read error on an unreadable source")
	}
}

func TestCopyFileWriteIntoReadonlyDirSurfacesError(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "f.txt")
	writeFile(t, src, "x")
	denyDir(t, dstDir)
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want a write error into a read-only destination")
	}
}

func TestCopyFileUnreadableSubdirSurfacesError(t *testing.T) {
	requireDenyable(t)
	root := t.TempDir()
	src := filepath.Join(root, "tree")
	inner := filepath.Join(src, "inner")
	mustMkdir(t, inner)
	dstDir := filepath.Join(root, "d")
	mustMkdir(t, dstDir)
	if err := os.Chmod(inner, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(inner, 0o700) })
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want a ReadDir error on an unreadable subdirectory")
	}
}

// --- CopyFile: seam-driven error branches ------------------------------------

func TestCopyFileMkdirAllErrorSurfaces(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "tree")
	mustMkdir(t, src)
	dstDir := filepath.Join(root, "d")
	mustMkdir(t, dstDir)
	forceMkdirAll(t, func(string, os.FileMode) error { return errors.New("mkdir boom") })
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want the MkdirAll error surfaced")
	}
}

func TestCopyFileChmodErrorFile(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "f.txt")
	writeFile(t, src, "x")
	forceChmod(t, func(string, os.FileMode) error { return errors.New("chmod boom") })
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want the file Chmod error surfaced")
	}
}

func TestCopyFileChmodErrorDir(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "emptytree")
	mustMkdir(t, src)
	dstDir := filepath.Join(root, "d")
	mustMkdir(t, dstDir)
	forceChmod(t, func(string, os.FileMode) error { return errors.New("chmod boom") })
	if _, err := CopyFile(src, dstDir); err == nil {
		t.Fatal("want the directory Chmod error surfaced")
	}
}

// --- CopyFileReplace ----------------------------------------------------------

func TestCopyFileReplaceOverwrites(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "dup.txt")
	writeFile(t, src, "new")
	writeFile(t, filepath.Join(dstDir, "dup.txt"), "old")

	dest, err := CopyFileReplace(src, dstDir)
	if err != nil {
		t.Fatalf("CopyFileReplace: %v", err)
	}
	if got := readFile(t, dest); got != "new" {
		t.Fatalf("overwrite body = %q", got)
	}
}

func TestCopyFileReplaceNoExistingIsPlainCopy(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "fresh.txt")
	writeFile(t, src, "z")
	if _, err := CopyFileReplace(src, dstDir); err != nil {
		t.Fatalf("CopyFileReplace fresh: %v", err)
	}
}

func TestCopyFileReplaceOntoItselfIsNoop(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "self.txt")
	writeFile(t, src, "keep")
	dest, err := CopyFileReplace(src, dir)
	if err != nil {
		t.Fatalf("self replace: %v", err)
	}
	if dest != src {
		t.Fatalf("self replace dest = %q", dest)
	}
	if got := readFile(t, src); got != "keep" {
		t.Fatalf("self replace mangled the file: %q", got)
	}
}

func TestCopyFileReplaceRemoveAllErrorSurfaces(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "dup.txt")
	writeFile(t, src, "new")
	writeFile(t, filepath.Join(dstDir, "dup.txt"), "old")
	forceRemoveAll(t, func(string) error { return errors.New("rm boom") })
	if _, err := CopyFileReplace(src, dstDir); err == nil {
		t.Fatal("want the RemoveAll error surfaced")
	}
}

// --- within (directly, for the non-relatable-paths branch) -------------------

func TestWithin(t *testing.T) {
	if !within("/a/b", "/a") {
		t.Fatal("/a/b should be within /a")
	}
	if !within("/a", "/a") {
		t.Fatal("/a should be within itself")
	}
	if within("/a", "/a/b") {
		t.Fatal("/a is not within /a/b")
	}
	if within("/other", "/a") {
		t.Fatal("/other is not within /a")
	}
	// A relative path against an absolute root cannot be made relative on some
	// platforms; filepath.Rel then errors and within reports false.
	if within("relative", "/absolute") {
		t.Fatal("non-relatable pair should be reported not-within")
	}
}

// --- MoveFileReplace ----------------------------------------------------------

func TestMoveFileReplaceOverwrites(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "dup.txt")
	writeFile(t, src, "new")
	writeFile(t, filepath.Join(dstDir, "dup.txt"), "old")

	dest, err := MoveFileReplace(src, dstDir)
	if err != nil {
		t.Fatalf("MoveFileReplace: %v", err)
	}
	if got := readFile(t, dest); got != "new" {
		t.Fatalf("overwrite body = %q", got)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be gone after a move, stat err = %v", err)
	}
}

func TestMoveFileReplaceIntoSelfRefused(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "f.txt")
	writeFile(t, src, "x")
	if _, err := MoveFileReplace(src, dir); !errors.Is(err, ErrMoveIntoSelf) {
		t.Fatalf("want ErrMoveIntoSelf, got %v", err)
	}
	if got := readFile(t, src); got != "x" {
		t.Fatalf("same-dir move mangled the source: %q", got)
	}
}

func TestMoveFileReplaceRemoveAllErrorSurfaces(t *testing.T) {
	root := t.TempDir()
	srcDir, dstDir := filepath.Join(root, "s"), filepath.Join(root, "d")
	mustMkdir(t, srcDir)
	mustMkdir(t, dstDir)
	src := filepath.Join(srcDir, "f.txt")
	writeFile(t, src, "x")
	forceRemoveAll(t, func(string) error { return errors.New("rm boom") })
	if _, err := MoveFileReplace(src, dstDir); err == nil {
		t.Fatal("want the RemoveAll error surfaced")
	}
}
