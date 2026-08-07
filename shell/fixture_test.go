// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-freedesktop/mime"
	"github.com/go-freedesktop/mimeapps"
)

// mimeDB builds a small MIME database (globs for *.txt and *.png) via AddXML.
func mimeDB(t *testing.T) *mime.Database {
	t.Helper()
	db := mime.New()
	doc := `<mime-info xmlns="http://www.freedesktop.org/standards/shared-mime-info">
	  <mime-type type="text/plain"><glob pattern="*.txt"/></mime-type>
	  <mime-type type="image/png"><glob pattern="*.png"/></mime-type>
	</mime-info>`
	if err := db.AddXML(strings.NewReader(doc)); err != nil {
		t.Fatalf("AddXML: %v", err)
	}
	return db
}

// mkfile writes name under dir with the given content.
func mkfile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// fixtureResolver builds a Resolver over the temp MIME DB plus a mimeapps
// resolver fed from a temp config dir (mimeapps.list) and a temp applications
// dir (editor + viewer .desktop files).
func fixtureResolver(t *testing.T) *Resolver {
	t.Helper()
	cfg := t.TempDir()
	appsDir := t.TempDir()

	mkfile(t, appsDir, "editor.desktop",
		"[Desktop Entry]\nType=Application\nName=Editor\nExec=editor %f\nMimeType=text/plain;\n")
	mkfile(t, appsDir, "viewer.desktop",
		"[Desktop Entry]\nType=Application\nName=Viewer\nExec=viewer %f\nMimeType=text/plain;image/png;\n")

	mkfile(t, cfg, "mimeapps.list",
		"[Default Applications]\ntext/plain=editor.desktop\n\n"+
			"[Added Associations]\ntext/plain=viewer.desktop\nimage/png=viewer.desktop\n")

	apps := mimeapps.LoadDirs([]string{cfg}, []string{appsDir}, nil)
	return NewResolver(mimeDB(t), apps)
}
