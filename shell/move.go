// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Move refusals: MoveFile returns one of these sentinel errors when it declines
// a move for a structural reason (as opposed to an OS error it passes through).
var (
	// ErrMoveIntoSelf is returned when src already lives directly inside
	// destDir, so the move would be a no-op.
	ErrMoveIntoSelf = errors.New("move: source is already in the destination")
	// ErrMoveExists is returned when an entry of the same name already exists in
	// destDir: the move is refused rather than silently overwriting it.
	ErrMoveExists = errors.New("move: an item with that name already exists")
	// ErrMoveNotDir is returned when destDir exists but is not a directory.
	ErrMoveNotDir = errors.New("move: destination is not a directory")
)

// osRename is a seam over os.Rename so the cross-volume copy+remove fallback is
// exercisable in a unit test without a second real volume: a test can force
// osRename to report a cross-device (EXDEV) error and drive the fallback path.
var osRename = os.Rename

// MoveFile moves the file (or directory) at src into destDir, preserving src's
// base name, and returns the final destination path.
//
// On the same volume it is a single os.Rename; when the rename reports a
// cross-device error (EXDEV) it falls back to copying src's bytes into destDir
// and removing the original. It refuses, without touching the filesystem:
//
//   - a no-op — src is already directly inside destDir (ErrMoveIntoSelf);
//   - a name collision — destDir already holds an entry of src's base name
//     (ErrMoveExists), so an existing file is never silently overwritten;
//   - a destDir that exists but is not a directory (ErrMoveNotDir).
//
// Any other filesystem error (a missing destDir, a permission denial on the
// rename/copy/remove, a source that cannot be read) is returned unwrapped so the
// caller can surface it to the user — MoveFile never panics on one.
func MoveFile(src, destDir string) (string, error) {
	src = filepath.Clean(src)
	destDir = filepath.Clean(destDir)

	fi, err := os.Stat(destDir)
	if err != nil {
		return "", err
	}
	if !fi.IsDir() {
		return "", ErrMoveNotDir
	}
	if filepath.Dir(src) == destDir {
		return "", ErrMoveIntoSelf
	}

	dest := filepath.Join(destDir, filepath.Base(src))
	if _, err := os.Lstat(dest); err == nil {
		return "", ErrMoveExists
	}

	if err := osRename(src, dest); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return "", err
		}
		data, rerr := os.ReadFile(src)
		if rerr != nil {
			return "", rerr
		}
		if werr := os.WriteFile(dest, data, 0o644); werr != nil {
			return "", werr
		}
		if rmerr := os.Remove(src); rmerr != nil {
			return "", rmerr
		}
	}
	return dest, nil
}
