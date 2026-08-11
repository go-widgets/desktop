// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"encoding/base64"
	"testing"
)

// realBinaryPlist is a genuine bplist00 Info.plist produced by macOS `plutil
// -convert binary1` from a SYNTHETIC XML fixture carrying every key the darwin
// source reads plus a bool and an int, so the decoder is validated against a
// real Apple-encoded binary plist rather than a hand-rolled one. It carries no
// data from any installed application. Also consumed by darwin_test.go.
const realBinaryPlist = "YnBsaXN0MDDYAQIDBAUGBwgJCgsMDQ4PEF8QEkNGQnVuZGxlSWRlbnRpZmllclhTb21lQm9vbF8QEENGQnVuZGxlSWNvbkZpbGVfEBNDRkJ1bmRsZURpc3BsYXlOYW1lXENGQnVuZGxlTmFtZV8QGUxTQXBwbGljYXRpb25DYXRlZ29yeVR5cGVfEBZMU01pbmltdW1TeXN0ZW1WZXJzaW9uV1NvbWVJbnRfEBNjb20uZXhhbXBsZS5maXh0dXJlCVdBcHBJY29uXxAPRml4dHVyZSBEaXNwbGF5W0ZpeHR1cmUgQXBwXxAjcHVibGljLmFwcC1jYXRlZ29yeS5kZXZlbG9wZXItdG9vbHNUMTEuMBAqAAgAGQAuADcASgBgAG0AiQCiAKoAwADBAMkA2wDnAQ0BEgAAAAAAAAIBAAAAAAAAABEAAAAAAAAAAAAAAAAAAAEU"

func mustBase64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	return b
}

// TestParseBinaryPlist decodes the genuine Apple-encoded binary fixture and
// checks every field the darwin source consumes, plus the bool/int scalars, so
// the reference library is exercised end-to-end on a real bplist00.
func TestParseBinaryPlist(t *testing.T) {
	m, err := parsePlist(mustBase64(t, realBinaryPlist))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]string{
		"CFBundleIdentifier":        "com.example.fixture",
		"CFBundleDisplayName":       "Fixture Display",
		"CFBundleName":              "Fixture App",
		"CFBundleIconFile":          "AppIcon",
		"LSApplicationCategoryType": "public.app-category.developer-tools",
		"LSMinimumSystemVersion":    "11.0",
	}
	for k, v := range want {
		if got := plistString(m, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if b, ok := m["SomeBool"].(bool); !ok || !b {
		t.Errorf("SomeBool = %v, want true", m["SomeBool"])
	}
	// howett decodes a non-negative integer as uint64.
	if n, ok := m["SomeInt"].(uint64); !ok || n != 42 {
		t.Errorf("SomeInt = %v (%T), want uint64 42", m["SomeInt"], m["SomeInt"])
	}
}

// TestParseXMLPlist decodes a SYNTHETIC XML Info.plist covering every plist
// value kind (dict/array/string/integer/real/bool/data/date) and checks the
// consumed string fields plus the imbricated array/dict structure.
func TestParseXMLPlist(t *testing.T) {
	const doc = `<?xml version="1.0"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Hello App</string>
	<key>CFBundleIdentifier</key><string>com.example.hello</string>
	<key>CFBundleIconFile</key><string>Hello</string>
	<key>LSApplicationCategoryType</key><string>public.app-category.utilities</string>
	<key>Count</key><integer>7</integer>
	<key>Ratio</key><real>1.5</real>
	<key>Agent</key><true/>
	<key>Hidden</key><false/>
	<key>Blob</key><data>aGk=</data>
	<key>Built</key><date>2001-01-01T00:00:00Z</date>
	<key>CFBundleDocumentTypes</key>
	<array>
		<dict>
			<key>CFBundleTypeName</key><string>Plain Text</string>
			<key>LSItemContentTypes</key>
			<array><string>public.text</string><string>public.plain-text</string></array>
		</dict>
	</array>
</dict>
</plist>`
	m, err := parsePlist([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	strs := map[string]string{
		"CFBundleName":              "Hello App",
		"CFBundleIdentifier":        "com.example.hello",
		"CFBundleIconFile":          "Hello",
		"LSApplicationCategoryType": "public.app-category.utilities",
	}
	for k, v := range strs {
		if got := plistString(m, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if n, ok := m["Count"].(uint64); !ok || n != 7 {
		t.Errorf("Count = %v (%T)", m["Count"], m["Count"])
	}
	if f, ok := m["Ratio"].(float64); !ok || f != 1.5 {
		t.Errorf("Ratio = %v (%T)", m["Ratio"], m["Ratio"])
	}
	if m["Agent"] != true || m["Hidden"] != false {
		t.Errorf("bools = %v %v", m["Agent"], m["Hidden"])
	}
	if b, ok := m["Blob"].([]byte); !ok || string(b) != "hi" {
		t.Errorf("Blob = %v", m["Blob"])
	}
	// Imbricated array-of-dict structure decodes to []any / map[string]any.
	arr, ok := m["CFBundleDocumentTypes"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("CFBundleDocumentTypes = %v", m["CFBundleDocumentTypes"])
	}
	sub, ok := arr[0].(map[string]any)
	if !ok || plistString(sub, "CFBundleTypeName") != "Plain Text" {
		t.Fatalf("doc-type dict = %v", arr[0])
	}
	types, ok := sub["LSItemContentTypes"].([]any)
	if !ok || len(types) != 2 || types[0] != "public.text" {
		t.Errorf("LSItemContentTypes = %v", sub["LSItemContentTypes"])
	}
}

// TestParsePlistErrors covers the error branch of parsePlist: a garbage body
// and a well-formed plist whose root is not a dict both fail.
func TestParsePlistErrors(t *testing.T) {
	cases := map[string][]byte{
		"garbage":       []byte("\x00\x01 not a plist"),
		"truncated bin": []byte("bplist00\x00\x00"),
		"non-dict root": []byte(`<?xml version="1.0"?>
<plist version="1.0"><array><string>x</string></array></plist>`),
	}
	for name, data := range cases {
		if _, err := parsePlist(data); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

// TestPlistString covers the present, absent and wrong-type lookups.
func TestPlistString(t *testing.T) {
	m := map[string]any{"str": "value", "num": uint64(1)}
	if got := plistString(m, "str"); got != "value" {
		t.Errorf("present = %q, want value", got)
	}
	if got := plistString(m, "absent"); got != "" {
		t.Errorf("absent = %q, want empty", got)
	}
	if got := plistString(m, "num"); got != "" {
		t.Errorf("non-string = %q, want empty", got)
	}
}
