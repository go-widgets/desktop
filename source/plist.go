// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import "howett.net/plist"

// Property-list decoding for an .app bundle's Contents/Info.plist. Info.plist
// ships in two forms — the textual XML plist and the binary "bplist00" plist —
// and the darwin source reads a handful of string keys out of it
// (CFBundleName / CFBundleDisplayName / CFBundleIdentifier / CFBundleIconFile /
// LSApplicationCategoryType).
//
// Decoding is delegated to the reference pure-Go property-list library
// howett.net/plist (CGO-free), which auto-detects and decodes the binary, XML,
// OpenStep and GNUStep encodings and covers every plist value kind. This
// replaces a hand-rolled bplist+XML reader: a reference library is
// battle-tested and maintained, and a plain Go-module dependency is not
// vendoring.

// parsePlist decodes a property list (binary bplist00 or XML) whose root is a
// dict, returning its key/value map. Nested dicts decode to map[string]any and
// arrays to []any, with string / bool / int64 / uint64 / float64 / []byte /
// time.Time scalars. A non-dict root or a malformed body is an error; an empty
// input decodes to an empty dict, so a bundle with a blank Info.plist degrades
// gracefully (every lookup yields "") rather than aborting the scan.
func parsePlist(data []byte) (map[string]any, error) {
	var m map[string]any
	if _, err := plist.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// plistString returns the string value stored under key, or "" when the key is
// absent or its value is not a string.
func plistString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
