// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"encoding/base64"
	"math"
	"testing"
)

// realBinaryPlist is a genuine bplist00 Info.plist produced by macOS `plutil
// -convert binary1` from an XML fixture carrying every key the darwin source
// reads plus a bool and an int, so the binary decoder is validated against a
// real Apple-encoded plist rather than a hand-rolled one.
const realBinaryPlist = "YnBsaXN0MDDYAQIDBAUGBwgJCgsMDQ4PEF8QEkNGQnVuZGxlSWRlbnRpZmllclhTb21lQm9vbF8QEENGQnVuZGxlSWNvbkZpbGVfEBNDRkJ1bmRsZURpc3BsYXlOYW1lXENGQnVuZGxlTmFtZV8QGUxTQXBwbGljYXRpb25DYXRlZ29yeVR5cGVfEBZMU01pbmltdW1TeXN0ZW1WZXJzaW9uV1NvbWVJbnRfEBNjb20uZXhhbXBsZS5maXh0dXJlCVdBcHBJY29uXxAPRml4dHVyZSBEaXNwbGF5W0ZpeHR1cmUgQXBwXxAjcHVibGljLmFwcC1jYXRlZ29yeS5kZXZlbG9wZXItdG9vbHNUMTEuMBAqAAgAGQAuADcASgBgAG0AiQCiAKoAwADBAMkA2wDnAQ0BEgAAAAAAAAIBAAAAAAAAABEAAAAAAAAAAAAAAAAAAAEU"

func mustBase64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	return b
}

func TestParseRealBinaryPlist(t *testing.T) {
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
	if n, ok := m["SomeInt"].(int64); !ok || n != 42 {
		t.Errorf("SomeInt = %v, want 42", m["SomeInt"])
	}
}

func TestPlistStringMissingAndWrongType(t *testing.T) {
	m := map[string]any{"n": int64(1)}
	if got := plistString(m, "absent"); got != "" {
		t.Errorf("absent = %q, want empty", got)
	}
	if got := plistString(m, "n"); got != "" {
		t.Errorf("non-string = %q, want empty", got)
	}
}

func TestParseXMLPlist(t *testing.T) {
	const doc = `<?xml version="1.0"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>Hello</string>
	<key>Nested</key><string>a<b/>c</string>
	<key>Int</key><integer>7</integer>
	<key>Real</key><real>1.5</real>
	<key>Yes</key><true/>
	<key>No</key><false></false>
	<key>Blob</key><data>aGk=</data>
	<key>When</key><date>2001-01-01T00:00:00Z</date>
	<key>List</key><array><string>x</string><integer>2</integer></array>
	<key>Sub</key><dict><key>k</key><string>v</string></dict>
	<key>Weird</key><weirdtype>ignored</weirdtype>
</dict>
</plist>`
	m, err := parsePlist([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if plistString(m, "CFBundleName") != "Hello" {
		t.Errorf("CFBundleName = %q", m["CFBundleName"])
	}
	if m["Nested"] != "ac" {
		t.Errorf("Nested = %q, want ac", m["Nested"])
	}
	if m["Int"].(int64) != 7 {
		t.Errorf("Int = %v", m["Int"])
	}
	if m["Real"].(float64) != 1.5 {
		t.Errorf("Real = %v", m["Real"])
	}
	if m["Yes"].(bool) != true || m["No"].(bool) != false {
		t.Errorf("bools = %v %v", m["Yes"], m["No"])
	}
	if string(m["Blob"].([]byte)) != "hi" {
		t.Errorf("Blob = %v", m["Blob"])
	}
	if m["When"] != "2001-01-01T00:00:00Z" {
		t.Errorf("When = %v", m["When"])
	}
	arr := m["List"].([]any)
	if len(arr) != 2 || arr[0] != "x" || arr[1].(int64) != 2 {
		t.Errorf("List = %v", arr)
	}
	sub := m["Sub"].(map[string]any)
	if sub["k"] != "v" {
		t.Errorf("Sub = %v", sub)
	}
	if m["Weird"] != nil {
		t.Errorf("Weird = %v, want nil", m["Weird"])
	}
}

func TestParseXMLPlistErrors(t *testing.T) {
	cases := map[string]string{
		"no root":       `<other/>`,
		"root not dict": `<plist><string>x</string></plist>`,
		"key expected":  `<plist><dict><string>x</string></dict></plist>`,
		"value missing": `<plist><dict><key>k</key></dict></plist>`,
		"bad int":       `<plist><dict><key>k</key><integer>zz</integer></dict></plist>`,
		"bad real":      `<plist><dict><key>k</key><real>zz</real></dict></plist>`,
		"bad data":      `<plist><dict><key>k</key><data>!!!!</data></dict></plist>`,
		"malformed":     `<plist><dict><key>k</key><string>oops`,
		"trunc key":     `<plist><dict><key>oops`,
		"trunc dict":    `<plist><dict>`,
		"trunc plist":   `<plist>`,
		"xml attr err":  `<plist version=1></plist>`,
		"xml bad child": `<plist><1`,
		"trunc post-key": `<plist><dict><key>k</key>`,
		"trunc int val": `<plist><dict><key>k</key><integer>5`,
		"trunc real":    `<plist><dict><key>k</key><real>1`,
		"trunc data":    `<plist><dict><key>k</key><data>aa`,
		"trunc array":   `<plist><dict><key>k</key><array>`,
		"trunc arr elt": `<plist><dict><key>k</key><array><integer>5`,
		"trunc skip":    `<plist><dict><key>k</key><string>a<b>`,
	}
	for name, doc := range cases {
		if _, err := parsePlist([]byte(doc)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// --- white-box binary-plist branch coverage ---------------------------------

// bp builds a decode state whose objects are the given raw object blobs laid out
// contiguously, with refSize=1 (so a reference byte is simply an object index).
func bp(objs ...[]byte) *bplist {
	p := &bplist{refSize: 1, num: len(objs)}
	var data []byte
	for _, o := range objs {
		p.offsets = append(p.offsets, len(data))
		data = append(data, o...)
	}
	p.data = data
	return p
}

func obj(p *bplist, i int) (any, error) { return p.object(i, 0) }

func TestBplistObjectScalars(t *testing.T) {
	p := bp(
		[]byte{0x00},             // 0 null
		[]byte{0x08},             // 1 false
		[]byte{0x09},             // 2 true
		[]byte{0x0f},             // 3 fill
		[]byte{0x10, 0x2a},       // 4 int 42
		[]byte{0x22, 0x3f, 0x80, 0x00, 0x00},                         // 5 real32 1.0
		[]byte{0x23, 0x3f, 0xf0, 0, 0, 0, 0, 0, 0},                   // 6 real64 1.0
		[]byte{0x33, 0x40, 0x09, 0x21, 0xfb, 0x54, 0x44, 0x2d, 0x18}, // 7 date ~3.1415
		[]byte{0x42, 0x68, 0x69},                                     // 8 data "hi"
		[]byte{0x53, 0x61, 0x62, 0x63},                               // 9 ascii "abc"
		[]byte{0x61, 0x00, 0x41},                                     // 10 utf16 "A"
		[]byte{0x80, 0x07},                                           // 11 uid 7
	)
	checks := []struct {
		i    int
		want any
	}{
		{0, nil}, {1, false}, {2, true}, {3, nil}, {4, int64(42)},
		{5, float64(1)}, {6, float64(1)}, {8, []byte("hi")}, {9, "abc"},
		{10, "A"}, {11, int64(7)},
	}
	for _, c := range checks {
		got, err := obj(p, c.i)
		if err != nil {
			t.Fatalf("obj %d: %v", c.i, err)
		}
		switch w := c.want.(type) {
		case []byte:
			if string(got.([]byte)) != string(w) {
				t.Errorf("obj %d = %v, want %v", c.i, got, w)
			}
		default:
			if got != c.want {
				t.Errorf("obj %d = %v, want %v", c.i, got, c.want)
			}
		}
	}
	if v, _ := obj(p, 7); math.Abs(v.(float64)-3.14159) > 1e-3 {
		t.Errorf("date = %v", v)
	}
}

func TestBplistArrayAndDict(t *testing.T) {
	// object 0 = array of [obj1, obj2]; obj1 int 1, obj2 ascii "z".
	p := bp(
		[]byte{0xa2, 0x01, 0x02}, // 0 array -> refs 1,2
		[]byte{0x10, 0x01},       // 1 int 1
		[]byte{0x51, 0x7a},       // 2 "z"
	)
	arr, err := obj(p, 0)
	if err != nil {
		t.Fatalf("array: %v", err)
	}
	a := arr.([]any)
	if len(a) != 2 || a[0].(int64) != 1 || a[1] != "z" {
		t.Errorf("array = %v", a)
	}

	// dict {"k": 5}
	pd := bp(
		[]byte{0xd1, 0x01, 0x02}, // 0 dict: 1 key ref @1, 1 val ref @2
		[]byte{0x51, 0x6b},       // 1 key "k"
		[]byte{0x10, 0x05},       // 2 val int 5
	)
	m, err := obj(pd, 0)
	if err != nil {
		t.Fatalf("dict: %v", err)
	}
	if m.(map[string]any)["k"].(int64) != 5 {
		t.Errorf("dict = %v", m)
	}
}

func TestBplistObjectErrors(t *testing.T) {
	// depth guard.
	p := bp([]byte{0x08})
	if _, err := p.object(0, p.num+1); err != errBinCycle {
		t.Errorf("cycle: %v", err)
	}
	// ref out of range.
	if _, err := obj(p, 5); err != errBinRef {
		t.Errorf("ref: %v", err)
	}
	// offset out of range.
	bad := &bplist{data: []byte{0x08}, offsets: []int{99}, refSize: 1, num: 1}
	if _, err := obj(bad, 0); err != errBinOffset {
		t.Errorf("offset: %v", err)
	}
	// unknown low marker in 0x0 group, and an unknown high group.
	for _, b := range [][]byte{{0x01}, {0x90}} {
		if _, err := obj(bp(b), 0); err != errBinMarker {
			t.Errorf("marker %x: %v", b, err)
		}
	}
	// bad real width (2-byte real).
	if _, err := obj(bp([]byte{0x21, 0x00, 0x00}), 0); err != errBinReal {
		t.Errorf("real width: %v", err)
	}
	// truncated int (declares 2 bytes, has 1).
	if _, err := obj(bp([]byte{0x11, 0x00}), 0); err != errBinTrunc {
		t.Errorf("trunc int: %v", err)
	}
	// dict with a non-string key.
	pk := bp(
		[]byte{0xd1, 0x01, 0x02},
		[]byte{0x10, 0x01}, // key is an int, not a string
		[]byte{0x10, 0x05},
	)
	if _, err := obj(pk, 0); err != errBinKey {
		t.Errorf("bad key: %v", err)
	}
	// array whose element ref is out of range.
	if _, err := obj(bp([]byte{0xa1, 0x09}), 0); err != errBinRef {
		t.Errorf("array elem: %v", err)
	}
	// dict whose key ref is out of range, and whose value ref is out of range.
	if _, err := obj(bp([]byte{0xd1, 0x09, 0x09}), 0); err != errBinRef {
		t.Errorf("dict key ref: %v", err)
	}
	// dict key ok but value object errors (value marker unknown).
	pv := bp([]byte{0xd1, 0x01, 0x02}, []byte{0x51, 0x6b}, []byte{0x01})
	if _, err := obj(pv, 0); err != errBinMarker {
		t.Errorf("dict val err: %v", err)
	}

	// Per-type truncations: each marker declares a payload the object is too
	// short to hold, exercising every read-error guard in object().
	trunc := map[string][]byte{
		"real":       {0x23},       // real64, no payload
		"date":       {0x33},       // date, no payload
		"data":       {0x41},       // 1 data byte, none present
		"data count": {0x4f},       // data extended count, truncated
		"ascii":      {0x51},       // 1 char, none present
		"asc count":  {0x5f},       // ascii extended count, truncated
		"utf16":      {0x61},       // 1 unit, none present
		"u16 count":  {0x6f},       // utf16 extended count, truncated
		"uid":        {0x81},       // 2-byte uid, none present
		"arr count":  {0xaf},       // array extended count, truncated
		"arr refs":   {0xa1},       // 1 element, no ref byte
		"dict count": {0xdf},       // dict extended count, truncated
		"dict krefs": {0xd1},       // 1 pair, no key ref byte
		"dict vrefs": {0xd1, 0x01}, // key ref present, value ref missing
	}
	for name, o := range trunc {
		if _, err := obj(bp(o), 0); err == nil {
			t.Errorf("%s: expected truncation error", name)
		}
	}
}

func TestBplistCount(t *testing.T) {
	// inline nibble count.
	p := &bplist{data: []byte{0x5a}, num: 1}
	if c, h, err := p.count(0, 0x0a); c != 10 || h != 0 || err != nil {
		t.Errorf("inline = %d,%d,%v", c, h, err)
	}
	// extended count: marker 0x5f then int 0x10 0x20 -> count 32, hdr 2.
	pe := &bplist{data: []byte{0x5f, 0x10, 0x20}, num: 1}
	if c, h, err := pe.count(0, 0x0f); c != 32 || h != 2 || err != nil {
		t.Errorf("extended = %d,%d,%v", c, h, err)
	}
	// extended count truncated (nothing after marker).
	pt := &bplist{data: []byte{0x5f}, num: 1}
	if _, _, err := pt.count(0, 0x0f); err != errBinTrunc {
		t.Errorf("trunc count: %v", err)
	}
	// extended count with a non-int size marker.
	pb := &bplist{data: []byte{0x5f, 0x50}, num: 1}
	if _, _, err := pb.count(0, 0x0f); err != errBinCount {
		t.Errorf("bad count marker: %v", err)
	}
	// extended count whose int bytes are truncated.
	pi := &bplist{data: []byte{0x5f, 0x11, 0x00}, num: 1}
	if _, _, err := pi.count(0, 0x0f); err != errBinTrunc {
		t.Errorf("trunc count int: %v", err)
	}
}

func TestBplistRefsAndAt(t *testing.T) {
	p := &bplist{data: []byte{0, 1, 2, 3}, refSize: 1}
	if _, err := p.refs(-1, 1); err != errBinRef {
		t.Errorf("neg refs: %v", err)
	}
	if _, err := p.refs(2, 5); err != errBinRef {
		t.Errorf("oob refs: %v", err)
	}
	if r, err := p.refs(1, 2); err != nil || r[0] != 1 || r[1] != 2 {
		t.Errorf("refs = %v,%v", r, err)
	}
	if _, err := p.at(0, -1); err != errBinTrunc {
		t.Errorf("neg at: %v", err)
	}
	if _, err := p.at(2, 10); err != errBinTrunc {
		t.Errorf("oob at: %v", err)
	}
	if b, err := p.at(1, 2); err != nil || b[0] != 1 || b[1] != 2 {
		t.Errorf("at = %v,%v", b, err)
	}
}

func TestBe(t *testing.T) {
	if be([]byte{0x01, 0x02}) != 0x0102 {
		t.Errorf("be short")
	}
	// more than 8 bytes keeps the low 8.
	if be([]byte{9, 9, 1, 2, 3, 4, 5, 6, 7, 8}) != 0x0102030405060708 {
		t.Errorf("be long")
	}
}

func TestParseBinaryPlistErrors(t *testing.T) {
	// too short.
	if _, err := parseBinaryPlist([]byte("bplist00")); err != errBinShort {
		t.Errorf("short: %v", err)
	}
	// A valid-length buffer but a zero offSize/refSize/num trailer.
	buf := make([]byte, 8+32)
	copy(buf, "bplist00")
	if _, err := parseBinaryPlist(buf); err != errBinTrailer {
		t.Errorf("trailer: %v", err)
	}
	// offSize=1, refSize=1, num=1, table offset absurd.
	tr := buf[len(buf)-32:]
	tr[6], tr[7] = 1, 1
	tr[15] = 1   // num = 1
	tr[31] = 200 // table offset way past end
	if _, err := parseBinaryPlist(buf); err != errBinTable {
		t.Errorf("table: %v", err)
	}
	// root object is not a dict: a single false object.
	// layout: header(8) + object false(1) + offset table(1) + trailer(32).
	b2 := make([]byte, 0, 8+1+1+32)
	b2 = append(b2, "bplist00"...)
	b2 = append(b2, 0x08) // false at offset 8
	b2 = append(b2, 8)    // offset table: object 0 at offset 8
	t2 := make([]byte, 32)
	t2[6] = 1  // offSize
	t2[7] = 1  // refSize
	t2[15] = 1 // num = 1
	t2[23] = 0 // top = 0
	t2[31] = 9 // table offset = 9
	b2 = append(b2, t2...)
	if _, err := parseBinaryPlist(b2); err != errPlistNotDict {
		t.Errorf("not dict: %v", err)
	}
	// top ref out of range surfaces through object().
	t2[23] = 9 // top = 9, but num = 1
	copy(b2[len(b2)-32:], t2)
	if _, err := parseBinaryPlist(b2); err != errBinRef {
		t.Errorf("top ref: %v", err)
	}
}
