// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"

	"github.com/go-freedesktop/mime"
	"github.com/go-freedesktop/mimeapps"
)

// sniffLen is the number of leading bytes read from a file for MIME magic
// content sniffing. The Shared MIME-info magic rules never look past a few KiB.
const sniffLen = 4096

// osOpen is a seam over os.Open so the open-failure branch of head is testable
// deterministically, independent of the test process's uid.
var osOpen = os.Open

// OpenWith is the resolved "open with" answer for a path or MIME type: the
// classified MIME type, the default application (if any) and the ordered list
// of candidate applications.
type OpenWith struct {
	MimeType   string
	Default    App
	HasDefault bool
	Candidates []App
}

// Resolver answers MIME classification and application-association queries by
// combining a shared MIME-info database with a mimeapps.list resolver.
type Resolver struct {
	db   *mime.Database
	apps *mimeapps.Resolver
}

// NewResolver wires a MIME database and an application-association resolver
// into a Resolver.
func NewResolver(db *mime.Database, apps *mimeapps.Resolver) *Resolver {
	return &Resolver{db: db, apps: apps}
}

// ResolvePath classifies path by both name and content and resolves its
// associated applications. Directories (and other non-regular paths) are
// classified by name only. A stat failure (e.g. the path does not exist) or a
// content-read failure is returned as an error.
func (r *Resolver) ResolvePath(path string) (OpenWith, error) {
	info, err := os.Stat(path)
	if err != nil {
		return OpenWith{}, err
	}
	data, err := head(path, info)
	if err != nil {
		return OpenWith{}, err
	}
	return r.ResolveName(filepath.Base(path), data), nil
}

// ResolveName classifies a bare name plus optional content bytes and resolves
// the associated applications. It performs no I/O, so it is the pure seam the
// path-based helper builds on.
func (r *Resolver) ResolveName(name string, content []byte) OpenWith {
	mt := r.db.TypeByNameAndContent(name, content)
	ow := OpenWith{MimeType: mt}
	if def, err := r.apps.DefaultApp(mt); err == nil {
		ow.Default = newApp(def)
		ow.HasDefault = true
	}
	for _, e := range r.apps.Candidates(mt) {
		ow.Candidates = append(ow.Candidates, newApp(e))
	}
	return ow
}

// head returns up to sniffLen bytes of a regular file's leading content, or nil
// for a non-regular path (directory, device, ...). An open failure on a regular
// file is surfaced as an error.
func head(path string, info os.FileInfo) ([]byte, error) {
	if !info.Mode().IsRegular() {
		return nil, nil
	}
	f, err := osOpen(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, sniffLen)
	n, _ := f.Read(buf)
	return buf[:n], nil
}
