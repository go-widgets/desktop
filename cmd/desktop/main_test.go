// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeApp drops a minimal .desktop file into an applications dir.
func writeApp(t *testing.T, appsDir, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(appsDir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCapture(t *testing.T) {
	sb := t.TempDir()
	xdg := filepath.Join(sb, "xdg")
	appsDir := filepath.Join(xdg, "applications")
	files := filepath.Join(sb, "files")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(files, 0o755); err != nil {
		t.Fatal(err)
	}
	writeApp(t, appsDir, "org.x.Editor.desktop",
		"[Desktop Entry]\nType=Application\nName=Editor\nExec=false %f\nIcon=text-editor\n")
	writeApp(t, appsDir, "org.x.Browser.desktop",
		"[Desktop Entry]\nType=Application\nName=Browser\nExec=false %u\nIcon=web-browser\n")
	if err := os.WriteFile(filepath.Join(files, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_DATA_HOME", xdg)
	t.Setenv("XDG_DATA_DIRS", xdg)

	out := filepath.Join(sb, "shell.png")
	var errb bytes.Buffer
	code := run([]string{"-dir", files, "-w", "400", "-h", "300", "-capture", out}, &errb)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr=%s", code, errb.String())
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("capture PNG missing/empty: %v", err)
	}
}

func TestRunLaunchUnknownID(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"-launch", "no.such.App.definitely"}, &errb); code != 1 {
		t.Errorf("launch unknown exit = %d, want 1", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"-nope"}, &errb); code != 2 {
		t.Errorf("bad flag exit = %d, want 2", code)
	}
}

func TestRunNoAction(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"-dir", t.TempDir()}, &errb); code != 0 {
		t.Errorf("no-action exit = %d, want 0", code)
	}
}

func TestSplitNotify(t *testing.T) {
	if s, b := splitNotify("Summary|Body text"); s != "Summary" || b != "Body text" {
		t.Errorf("split = %q,%q", s, b)
	}
	if s, b := splitNotify("bare"); s != "bare" || b != "" {
		t.Errorf("split bare = %q,%q", s, b)
	}
}

func TestRunNotifyOffLinuxErrors(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("notify is supported on linux")
	}
	var errb bytes.Buffer
	if code := run([]string{"-notify", "Hi|there", "-dir", t.TempDir()}, &errb); code != 1 {
		t.Errorf("notify off-linux exit = %d, want 1", code)
	}
}
