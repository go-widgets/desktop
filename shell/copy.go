// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Copy refusals: CopyFile returns one of these sentinel errors when it declines
// a copy for a structural reason (as opposed to an OS error it passes through).
var (
	// ErrCopyExists is returned by CopyFile when an entry of the same name
	// already exists in a DIFFERENT destination directory: the copy is refused
	// rather than silently overwriting it, so the caller can offer a
	// confirm-overwrite dialog (then call CopyFileReplace). A SAME-directory
	// copy never returns this — it auto-suffixes (" copie") instead.
	ErrCopyExists = errors.New("copy: an item with that name already exists")
	// ErrCopyNotDir is returned when destDir exists but is not a directory.
	ErrCopyNotDir = errors.New("copy: destination is not a directory")
	// ErrCopyIntoSelf is returned when the copy would place a directory inside
	// its own subtree (an infinite recursion), e.g. copying /a into /a/b.
	ErrCopyIntoSelf = errors.New("copy: cannot copy a directory into itself")
)

// Seams over the destructive/creating filesystem calls whose error branches
// cannot be provoked portably with real files (a Chmod/MkdirAll/RemoveAll
// failure on an otherwise valid path). A test swaps one for a stub that
// reports an error, exercising the surface-the-error path without a contrived
// on-disk layout — the same technique move.go uses for os.Rename.
var (
	osChmod     = os.Chmod
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
)

// CopyFile copies the file (or directory) at src into destDir and returns the
// final destination path. Directories are copied recursively; every entry's
// permission bits are preserved.
//
// Collisions are handled the way macOS's Finder does:
//
//   - a SAME-directory copy (destDir is src's own parent) is never a refusal:
//     the copy is written under a free " copie"-suffixed name (then " copie 2",
//     …), so ⌘C/⌘V in place always duplicates;
//   - a copy into a DIFFERENT directory that already holds an entry of src's
//     base name returns ErrCopyExists (nothing written), so the caller can
//     confirm an overwrite and, on confirmation, call CopyFileReplace.
//
// It also refuses, without touching the filesystem, a destDir that is not a
// directory (ErrCopyNotDir) and a copy of a directory into its own subtree
// (ErrCopyIntoSelf). Any other filesystem error (a missing destDir, a
// permission denial, an unreadable source) is returned unwrapped — CopyFile
// never panics on one.
func CopyFile(src, destDir string) (string, error) { return copyInto(src, destDir, false) }

// CopyFileReplace is CopyFile's overwrite peer: it copies src into destDir under
// src's own base name, first removing any existing entry of that name (so a
// confirmed "Remplacer" overwrites it). It is the call a caller makes after a
// CopyFile reported ErrCopyExists and the user confirmed the replacement. A
// no-op self-copy (the computed destination is src itself) is left untouched.
func CopyFileReplace(src, destDir string) (string, error) { return copyInto(src, destDir, true) }

// copyInto is the shared body of CopyFile / CopyFileReplace. replace selects the
// overwrite behaviour: false auto-suffixes a same-dir copy and refuses a
// different-dir collision; true writes under the exact base name, removing any
// existing entry first.
func copyInto(src, destDir string, replace bool) (string, error) {
	src = filepath.Clean(src)
	destDir = filepath.Clean(destDir)

	fi, err := os.Stat(destDir)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return "", ErrCopyNotDir
	}

	base := filepath.Base(src)
	var dest string
	if filepath.Dir(src) == destDir && !replace {
		dest = freeCopyName(destDir, base)
	} else {
		dest = filepath.Join(destDir, base)
		if _, err := os.Lstat(dest); err == nil && !replace {
			return "", ErrCopyExists
		}
	}

	if dest == src {
		return src, nil // copying onto itself: nothing to do
	}
	if within(dest, src) {
		return "", ErrCopyIntoSelf
	}
	if replace {
		if err := osRemoveAll(dest); err != nil {
			return "", err
		}
	}
	if err := copyTree(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// copyTree copies src to dst recursively, preserving each entry's permission
// bits. A directory is created then its children copied; a regular file's bytes
// are written. Symlinks and other special files are dereferenced by os.Stat, so
// a link to a file copies the file's bytes (the file-manager copy the shell
// offers, never a dangling link).
func copyTree(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := osMkdirAll(dst, 0o700); err != nil {
			return err
		}
		ents, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range ents {
			if err := copyTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return osChmod(dst, fi.Mode().Perm())
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, fi.Mode().Perm()); err != nil {
		return err
	}
	return osChmod(dst, fi.Mode().Perm())
}

// freeCopyName returns a path inside dir for a " copie"-suffixed duplicate of
// base that no entry yet occupies: "note.txt" → "note copie.txt", then
// "note copie 2.txt", …, and for an extensionless name "notes" → "notes copie".
// The suffix is inserted before the extension so the duplicate keeps its type.
func freeCopyName(dir, base string) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		name := stem + " copie" + ext
		if n > 1 {
			name = stem + " copie " + strconv.Itoa(n) + ext
		}
		cand := filepath.Join(dir, name)
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
}

// MoveFileReplace is MoveFile's overwrite peer: it removes any existing entry of
// src's base name in destDir, then moves src in under that name — so a confirmed
// "Remplacer"/"Déplacer" overwrites the collision. It delegates the move itself
// to MoveFile, inheriting its same-volume rename and cross-volume (EXDEV)
// copy+remove fallback and its destDir/not-a-directory checks. It refuses a
// no-op (src already directly inside destDir, ErrMoveIntoSelf) before removing
// anything, so a same-directory call can never delete the source. Other
// filesystem errors are returned unwrapped.
func MoveFileReplace(src, destDir string) (string, error) {
	src = filepath.Clean(src)
	destDir = filepath.Clean(destDir)
	if filepath.Dir(src) == destDir {
		return "", ErrMoveIntoSelf
	}
	if err := osRemoveAll(filepath.Join(destDir, filepath.Base(src))); err != nil {
		return "", err
	}
	return MoveFile(src, destDir)
}

// within reports whether path is root itself or lives inside root's subtree, so
// copyInto can refuse copying a directory into a descendant of itself.
func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
