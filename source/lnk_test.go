// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// lnkBuilder assembles a .lnk byte stream field by field so each parse branch
// has an exact fixture. The header is always the valid 76-byte ShellLinkHeader;
// flags, the IDList, the LinkInfo and the StringData run are appended by the
// With* methods in on-disk order.
type lnkBuilder struct {
	flags     uint32
	iconIndex int32
	idList    []byte // full LinkTargetIDList block (2-byte size + data), or nil
	linkInfo  []byte
	strings   []byte
	unicode   bool
}

func (b *lnkBuilder) bytes() []byte {
	hdr := make([]byte, lnkHeaderSize)
	binary.LittleEndian.PutUint32(hdr[0:4], lnkHeaderSize)
	copy(hdr[4:20], lnkCLSID[:])
	binary.LittleEndian.PutUint32(hdr[20:24], b.flags)
	binary.LittleEndian.PutUint32(hdr[56:60], uint32(b.iconIndex))
	out := append([]byte(nil), hdr...)
	out = append(out, b.idList...)
	out = append(out, b.linkInfo...)
	out = append(out, b.strings...)
	return out
}

func (b *lnkBuilder) addString(flag uint32, s string) {
	b.flags |= flag
	n := len([]rune(s))
	buf := make([]byte, 2)
	if b.unicode {
		u := utf16.Encode([]rune(s))
		binary.LittleEndian.PutUint16(buf, uint16(len(u)))
		for _, c := range u {
			var two [2]byte
			binary.LittleEndian.PutUint16(two[:], c)
			buf = append(buf, two[:]...)
		}
	} else {
		binary.LittleEndian.PutUint16(buf, uint16(n))
		buf = append(buf, []byte(s)...)
	}
	b.strings = append(b.strings, buf...)
}

// ansiLinkInfo builds a header-size-0x1C LinkInfo with a local base path and a
// common path suffix at the given offsets. Passing base/suffix offsets directly
// lets tests exercise the out-of-range and zero-offset guards.
func ansiLinkInfo(path, suffix string) []byte {
	const hdr = 0x1C
	pathBytes := append([]byte(path), 0)
	sufBytes := append([]byte(suffix), 0)
	base := hdr
	sufOff := base + len(pathBytes)
	total := sufOff + len(sufBytes)
	b := make([]byte, total)
	binary.LittleEndian.PutUint32(b[0:4], uint32(total))
	binary.LittleEndian.PutUint32(b[4:8], hdr)
	binary.LittleEndian.PutUint32(b[8:12], linkInfoHasVolumeID)
	binary.LittleEndian.PutUint32(b[16:20], uint32(base))
	binary.LittleEndian.PutUint32(b[24:28], uint32(sufOff))
	copy(b[base:], pathBytes)
	copy(b[sufOff:], sufBytes)
	return b
}

// unicodeLinkInfo builds a header-size-0x24 LinkInfo whose optional Unicode
// local-base-path offset points at a UTF-16LE path.
func unicodeLinkInfo(path string) []byte {
	const hdr = 0x24
	u := utf16.Encode([]rune(path))
	pathBytes := make([]byte, 0, len(u)*2+2)
	for _, c := range u {
		var two [2]byte
		binary.LittleEndian.PutUint16(two[:], c)
		pathBytes = append(pathBytes, two[:]...)
	}
	pathBytes = append(pathBytes, 0, 0)
	uOff := hdr
	total := uOff + len(pathBytes)
	b := make([]byte, total)
	binary.LittleEndian.PutUint32(b[0:4], uint32(total))
	binary.LittleEndian.PutUint32(b[4:8], hdr)
	binary.LittleEndian.PutUint32(b[8:12], linkInfoHasVolumeID)
	binary.LittleEndian.PutUint32(b[28:32], uint32(uOff))
	copy(b[uOff:], pathBytes)
	return b
}

func TestParseLinkANSI(t *testing.T) {
	b := &lnkBuilder{iconIndex: 3}
	b.flags |= flagHasLinkInfo
	b.linkInfo = ansiLinkInfo(`C:\Program Files\App\app.exe`, "")
	b.addString(flagHasName, "My App")
	b.addString(flagHasRelativePath, `..\App\app.exe`)
	b.addString(flagHasWorkingDir, `C:\Program Files\App`)
	b.addString(flagHasArguments, "--flag")
	b.addString(flagHasIconLocation, `C:\icons\app.ico`)

	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `C:\Program Files\App\app.exe` {
		t.Errorf("target = %q", sl.target)
	}
	if sl.description != "My App" || sl.relativePath != `..\App\app.exe` ||
		sl.workingDir != `C:\Program Files\App` || sl.arguments != "--flag" ||
		sl.iconLocation != `C:\icons\app.ico` || sl.iconIndex != 3 {
		t.Errorf("fields = %+v", sl)
	}
}

func TestParseLinkUnicode(t *testing.T) {
	b := &lnkBuilder{unicode: true}
	b.flags |= flagIsUnicode | flagHasLinkInfo
	b.linkInfo = unicodeLinkInfo(`C:\Приложения\app.exe`)
	b.addString(flagHasName, "Юникод")
	b.addString(flagHasIconLocation, `%SystemRoot%\app.dll`)

	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `C:\Приложения\app.exe` {
		t.Errorf("unicode target = %q", sl.target)
	}
	if sl.description != "Юникод" || sl.iconLocation != `%SystemRoot%\app.dll` {
		t.Errorf("unicode fields = %+v", sl)
	}
}

func TestParseLinkWithIDList(t *testing.T) {
	// A HasLinkTargetIDList link: a 2-byte size + that many item-id bytes,
	// skipped before the LinkInfo. RELATIVE_PATH is the only target here (no
	// LinkInfo) — the target-fallback branch.
	b := &lnkBuilder{}
	b.flags |= flagHasLinkTargetIDList
	id := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	size := make([]byte, 2)
	binary.LittleEndian.PutUint16(size, uint16(len(id)))
	b.idList = append(append([]byte(nil), size...), id...)
	b.addString(flagHasRelativePath, `..\rel\app.exe`)

	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `..\rel\app.exe` {
		t.Errorf("fallback target = %q", sl.target)
	}
}

func TestParseLinkNetworkOnlyFallsBackToRelative(t *testing.T) {
	// LinkInfo present but with no VolumeIDAndLocalBasePath flag -> empty target
	// from LinkInfo, so RELATIVE_PATH is used.
	li := make([]byte, 0x1C)
	binary.LittleEndian.PutUint32(li[0:4], 0x1C)
	binary.LittleEndian.PutUint32(li[4:8], 0x1C)
	binary.LittleEndian.PutUint32(li[8:12], linkInfoHasNetworkRel) // no VolumeID
	b := &lnkBuilder{}
	b.flags |= flagHasLinkInfo
	b.linkInfo = li
	b.addString(flagHasRelativePath, `..\net\app.exe`)

	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `..\net\app.exe` {
		t.Errorf("target = %q", sl.target)
	}
}

func TestParseLinkSuffixAndOutOfRangeOffsets(t *testing.T) {
	// Local base path + a common path suffix, exercising ansizAt on a real
	// suffix and target concatenation.
	b := &lnkBuilder{}
	b.flags |= flagHasLinkInfo
	b.linkInfo = ansiLinkInfo(`C:\dir\`, `app.exe`)
	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `C:\dir\app.exe` {
		t.Errorf("target+suffix = %q", sl.target)
	}

	// Zero base offset -> ansizAt(rel<=0) returns "" ; huge suffix offset ->
	// ansizAt(rel>=len) returns "" ; empty target then falls back to relative.
	li := ansiLinkInfo(`C:\x\y.exe`, "")
	binary.LittleEndian.PutUint32(li[16:20], 0)    // base offset 0
	binary.LittleEndian.PutUint32(li[24:28], 9999) // suffix offset out of range
	b2 := &lnkBuilder{}
	b2.flags |= flagHasLinkInfo
	b2.linkInfo = li
	b2.addString(flagHasRelativePath, `..\z.exe`)
	sl2, err := parseLink(b2.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl2.target != `..\z.exe` {
		t.Errorf("empty-then-relative target = %q", sl2.target)
	}
}

func TestParseLinkUnicodeOffsetGuards(t *testing.T) {
	// headerSize 0x24 but the Unicode offset is 0 -> ansi fallback path.
	li := unicodeLinkInfo(`C:\u.exe`)
	// Repoint: zero the unicode offset, add an ansi base path after the header.
	ansiPath := append([]byte(`C:\ansi.exe`), 0)
	full := append(li, ansiPath...)
	binary.LittleEndian.PutUint32(full[0:4], uint32(len(full)))
	binary.LittleEndian.PutUint32(full[28:32], 0)               // unicode off = 0
	binary.LittleEndian.PutUint32(full[16:20], uint32(len(li))) // ansi base
	b := &lnkBuilder{}
	b.flags |= flagHasLinkInfo
	b.linkInfo = full
	sl, err := parseLink(b.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl.target != `C:\ansi.exe` {
		t.Errorf("ansi-fallback target = %q", sl.target)
	}

	// Unicode offset out of range -> utf16zAt(rel>=len) returns "" ; relative.
	li2 := unicodeLinkInfo(`C:\u.exe`)
	binary.LittleEndian.PutUint32(li2[28:32], 9999)
	b2 := &lnkBuilder{}
	b2.flags |= flagHasLinkInfo
	b2.linkInfo = li2
	b2.addString(flagHasRelativePath, `..\r.exe`)
	sl2, err := parseLink(b2.bytes())
	if err != nil {
		t.Fatalf("parseLink: %v", err)
	}
	if sl2.target != `..\r.exe` {
		t.Errorf("unicode-oob target = %q", sl2.target)
	}
}

func TestParseLinkErrors(t *testing.T) {
	good := (&lnkBuilder{}).bytes()

	t.Run("short", func(t *testing.T) {
		if _, err := parseLink(good[:10]); err != errLNKShort {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad header size", func(t *testing.T) {
		bad := append([]byte(nil), good...)
		binary.LittleEndian.PutUint32(bad[0:4], 0x99)
		if _, err := parseLink(bad); err != errLNKMagic {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad clsid", func(t *testing.T) {
		bad := append([]byte(nil), good...)
		bad[4] = 0xFF
		if _, err := parseLink(bad); err != errLNKCLSID {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no target", func(t *testing.T) {
		if _, err := parseLink(good); err != errLNKNoTarget {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("idlist size truncated", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasLinkTargetIDList // no idList bytes appended
		if _, err := parseLink(b.bytes()); err != errLNKIDList {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("idlist body truncated", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasLinkTargetIDList
		size := make([]byte, 2)
		binary.LittleEndian.PutUint16(size, 100) // claims 100 bytes, supplies 0
		b.idList = size
		if _, err := parseLink(b.bytes()); err != errLNKIDList {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("linkinfo size truncated", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasLinkInfo
		b.linkInfo = []byte{0x00, 0x00} // < 4 bytes for the size field
		if _, err := parseLink(b.bytes()); err != errLNKInfo {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("linkinfo size too small", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasLinkInfo
		li := make([]byte, 8)
		binary.LittleEndian.PutUint32(li[0:4], 4) // size 4 < 0x1C
		b.linkInfo = li
		if _, err := parseLink(b.bytes()); err != errLNKInfo {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("linkinfo size beyond buffer", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasLinkInfo
		li := make([]byte, 0x1C)
		binary.LittleEndian.PutUint32(li[0:4], 0x1000) // size beyond the buffer
		binary.LittleEndian.PutUint32(li[4:8], 0x1C)
		b.linkInfo = li
		if _, err := parseLink(b.bytes()); err != errLNKInfo {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stringdata count truncated", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasName // sets the flag but appends no bytes
		if _, err := parseLink(b.bytes()); err != errLNKString {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stringdata ansi body truncated", func(t *testing.T) {
		b := &lnkBuilder{}
		b.flags |= flagHasName
		cnt := make([]byte, 2)
		binary.LittleEndian.PutUint16(cnt, 50) // claims 50 chars, supplies none
		b.strings = cnt
		if _, err := parseLink(b.bytes()); err != errLNKString {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stringdata unicode body truncated", func(t *testing.T) {
		b := &lnkBuilder{unicode: true}
		b.flags |= flagIsUnicode | flagHasName
		cnt := make([]byte, 2)
		binary.LittleEndian.PutUint16(cnt, 50) // claims 50 wchars, supplies none
		b.strings = cnt
		if _, err := parseLink(b.bytes()); err != errLNKString {
			t.Fatalf("err = %v", err)
		}
	})
}
