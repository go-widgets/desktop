// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// The resource tree is laid out at fixed section offsets so the fixtures read
// clearly. Every directory is 16-byte header + 8-byte entries; leaves are
// 16-byte IMAGE_RESOURCE_DATA_ENTRY records.
const (
	offRoot          = 0x00
	offIconType      = 0x20
	offIconName      = 0x38
	offIconData      = 0x50
	offGroupType     = 0x60
	offGroupName     = 0x78
	offGroupData     = 0x90
	offImages        = 0xA0
	rsrcVA           = 0x1000
	langID           = 0x0409
	memberID         = 1 // the RT_ICON id the group references
)

func putDir(b []byte, off int, entries [][2]uint32) {
	binary.LittleEndian.PutUint16(b[off+12:], 0)                 // named count
	binary.LittleEndian.PutUint16(b[off+14:], uint16(len(entries))) // id count
	for i, e := range entries {
		o := off + 16 + i*8
		binary.LittleEndian.PutUint32(b[o:], e[0])   // id
		binary.LittleEndian.PutUint32(b[o+4:], e[1]) // offset (high bit = subdir)
	}
}

func putDataEntry(b []byte, off int, rva, size uint32) {
	binary.LittleEndian.PutUint32(b[off:], rva)
	binary.LittleEndian.PutUint32(b[off+4:], size)
}

// makeRsrc builds a .rsrc section carrying one RT_GROUP_ICON referencing one
// RT_ICON (an 8x8 32-bpp DIB). memberOverride lets a test point the group at a
// non-existent RT_ICON id to exercise the missing-member branch.
func makeRsrc(memberOverride uint32) []byte {
	icon := dib32(8, 8)
	groupImageOff := offImages + len(icon)

	nID := uint32(memberID)
	if memberOverride != 0 {
		nID = memberOverride
	}
	// GRPICONDIR + one GRPICONDIRENTRY.
	grp := make([]byte, 6+14)
	binary.LittleEndian.PutUint16(grp[2:], 1) // type icon
	binary.LittleEndian.PutUint16(grp[4:], 1) // count
	grp[6], grp[7] = 8, 8                      // width, height
	binary.LittleEndian.PutUint16(grp[10:], 1)             // planes
	binary.LittleEndian.PutUint16(grp[12:], 32)            // bit count
	binary.LittleEndian.PutUint32(grp[14:], uint32(len(icon)))
	binary.LittleEndian.PutUint16(grp[18:], uint16(nID))

	total := groupImageOff + len(grp)
	b := make([]byte, total)
	// Root: RT_ICON (3) and RT_GROUP_ICON (14), both subdirectories.
	putDir(b, offRoot, [][2]uint32{
		{rtIcon, offIconType | 0x80000000},
		{rtGroupIcon, offGroupType | 0x80000000},
	})
	putDir(b, offIconType, [][2]uint32{{memberID, offIconName | 0x80000000}})
	putDir(b, offIconName, [][2]uint32{{langID, offIconData}})
	putDataEntry(b, offIconData, rsrcVA+offImages, uint32(len(icon)))
	putDir(b, offGroupType, [][2]uint32{{1, offGroupName | 0x80000000}})
	putDir(b, offGroupName, [][2]uint32{{langID, offGroupData}})
	putDataEntry(b, offGroupData, rsrcVA+uint32(groupImageOff), uint32(len(grp)))
	copy(b[offImages:], icon)
	copy(b[groupImageOff:], grp)
	return b
}

// buildPE wraps a .rsrc section in a minimal but debug/pe-parseable PE32 image.
func buildPE(t *testing.T, rsrc []byte, sectionName string) []byte {
	t.Helper()
	const (
		peOff    = 0x40
		optSize  = 224
		rawStart = 0x200
	)
	buf := make([]byte, rawStart+len(rsrc))
	copy(buf, "MZ")
	binary.LittleEndian.PutUint32(buf[0x3C:], peOff)
	copy(buf[peOff:], "PE\x00\x00")

	coff := peOff + 4
	binary.LittleEndian.PutUint16(buf[coff:], 0x8664)          // Machine amd64
	binary.LittleEndian.PutUint16(buf[coff+2:], 1)             // NumberOfSections
	binary.LittleEndian.PutUint16(buf[coff+16:], optSize)      // SizeOfOptionalHeader
	binary.LittleEndian.PutUint16(buf[coff+18:], 0x0102)       // Characteristics

	opt := coff + 20
	binary.LittleEndian.PutUint16(buf[opt:], 0x10b)            // PE32 magic
	binary.LittleEndian.PutUint32(buf[opt+92:], 16)           // NumberOfRvaAndSizes
	// Resource data directory (index 2) — informational; the code uses the
	// section directly.
	binary.LittleEndian.PutUint32(buf[opt+96+2*8:], rsrcVA)
	binary.LittleEndian.PutUint32(buf[opt+96+2*8+4:], uint32(len(rsrc)))

	sec := opt + optSize
	copy(buf[sec:], sectionName)
	binary.LittleEndian.PutUint32(buf[sec+8:], uint32(len(rsrc)))  // VirtualSize
	binary.LittleEndian.PutUint32(buf[sec+12:], rsrcVA)           // VirtualAddress
	binary.LittleEndian.PutUint32(buf[sec+16:], uint32(len(rsrc))) // SizeOfRawData
	binary.LittleEndian.PutUint32(buf[sec+20:], rawStart)         // PointerToRawData
	binary.LittleEndian.PutUint32(buf[sec+36:], 0x40000040)       // Characteristics

	copy(buf[rawStart:], rsrc)
	return buf
}

func TestPEIconPNGEndToEnd(t *testing.T) {
	pe := buildPE(t, makeRsrc(0), ".rsrc")
	out, err := peIconPNG(pe, 0)
	if err != nil {
		t.Fatalf("peIconPNG: %v", err)
	}
	if b := decodePNG(t, out).Bounds(); b.Dx() != 8 || b.Dy() != 8 {
		t.Fatalf("icon size = %v, want 8x8", b)
	}
}

func TestPEIconPNGErrors(t *testing.T) {
	t.Run("not a PE", func(t *testing.T) {
		if _, err := peIconPNG([]byte("not a pe at all"), 0); err != errPEParse {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no rsrc section", func(t *testing.T) {
		pe := buildPE(t, makeRsrc(0), ".text")
		if _, err := peIconPNG(pe, 0); err != errPENoRsrc {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing RT_ICON member", func(t *testing.T) {
		pe := buildPE(t, makeRsrc(99), ".rsrc") // group points at id 99
		if _, err := peIconPNG(pe, 0); err != errPEIconMiss {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no group resource", func(t *testing.T) {
		// A .rsrc with only an RT_ICON type -> peExtractICO's findType fails.
		blob := make([]byte, 24)
		putDir(blob, 0, [][2]uint32{{rtIcon, 0x20 | 0x80000000}})
		pe := buildPE(t, blob, ".rsrc")
		if _, err := peIconPNG(pe, 0); err != errPENoGroup {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("group subdir unreadable", func(t *testing.T) {
		// RT_GROUP_ICON present but its subdir offset is past the section, so
		// peExtractICO's firstLeafData(groupDir) fails.
		blob := make([]byte, 24)
		putDir(blob, 0, [][2]uint32{{rtGroupIcon, 0x1000 | 0x80000000}})
		pe := buildPE(t, blob, ".rsrc")
		if _, err := peIconPNG(pe, 0); err != errPERsrcBad {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("section data unreadable", func(t *testing.T) {
		// Inflate SizeOfRawData beyond the file so debug/pe's Section.Data fails.
		pe := buildPE(t, makeRsrc(0), ".rsrc")
		const sec = 0x40 + 4 + 20 + 224
		binary.LittleEndian.PutUint32(pe[sec+16:], 0xFFFFFF) // SizeOfRawData
		if _, err := peIconPNG(pe, 0); err != errPENoRsrc {
			t.Fatalf("err = %v", err)
		}
	})
}

// The rsrc walker branches are exercised directly with hand-built slices — far
// smaller than a full PE per case.

func TestRsrcEntriesErrors(t *testing.T) {
	r := &rsrc{data: make([]byte, 8)} // < 16-byte header
	if _, err := r.entries(0); err != errPERsrcBad {
		t.Fatalf("short header err = %v", err)
	}
	if _, err := r.entries(-4); err != errPERsrcBad {
		t.Fatalf("negative off err = %v", err)
	}
	b := make([]byte, 16) // header claims entries but none follow
	binary.LittleEndian.PutUint16(b[14:], 3)
	r2 := &rsrc{data: b}
	if _, err := r2.entries(0); err != errPERsrcBad {
		t.Fatalf("entry overrun err = %v", err)
	}
	r3 := &rsrc{data: make([]byte, 16)} // n == 0
	if _, err := r3.entries(0); err != errPERsrcBad {
		t.Fatalf("empty dir err = %v", err)
	}
}

func TestRsrcFindTypeAbsent(t *testing.T) {
	// A valid root dir with only RT_ICON -> no RT_GROUP_ICON.
	b := make([]byte, 24)
	putDir(b, 0, [][2]uint32{{rtIcon, 0x20 | 0x80000000}})
	if _, err := (&rsrc{data: b}).findType(rtGroupIcon); err != errPENoGroup {
		t.Fatalf("findType err = %v", err)
	}
	// A valid root dir with only RT_GROUP_ICON -> no RT_ICON.
	b2 := make([]byte, 24)
	putDir(b2, 0, [][2]uint32{{rtGroupIcon, 0x20 | 0x80000000}})
	if _, err := (&rsrc{data: b2}).findIconType(); err != errPEIconMiss {
		t.Fatalf("findIconType err = %v", err)
	}
}

func TestRsrcFindTypePropagatesEntriesError(t *testing.T) {
	r := &rsrc{data: make([]byte, 4)} // root dir unreadable
	if _, err := r.findType(rtGroupIcon); err != errPERsrcBad {
		t.Fatalf("findType err = %v", err)
	}
	if _, err := r.findIconType(); err != errPERsrcBad {
		t.Fatalf("findIconType err = %v", err)
	}
	if _, err := r.iconData(1); err != errPERsrcBad {
		t.Fatalf("iconData err = %v", err)
	}
}

func TestRsrcLeafDataErrors(t *testing.T) {
	r := &rsrc{data: make([]byte, 8), va: rsrcVA} // < 16 bytes for the entry
	if _, err := r.leafData(0); err != errPERsrcBad {
		t.Fatalf("short leaf err = %v", err)
	}
	// RVA below the section base, or size 0, or size beyond the section.
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b[0:], rsrcVA-100) // start < 0
	binary.LittleEndian.PutUint32(b[4:], 4)
	if _, err := (&rsrc{data: b, va: rsrcVA}).leafData(0); err != errPERsrcBad {
		t.Fatalf("rva-below err = %v", err)
	}
	binary.LittleEndian.PutUint32(b[0:], rsrcVA) // start 0, size 0
	binary.LittleEndian.PutUint32(b[4:], 0)
	if _, err := (&rsrc{data: b, va: rsrcVA}).leafData(0); err != errPERsrcBad {
		t.Fatalf("zero-size err = %v", err)
	}
	binary.LittleEndian.PutUint32(b[4:], 0xFFFF) // size beyond section
	if _, err := (&rsrc{data: b, va: rsrcVA}).leafData(0); err != errPERsrcBad {
		t.Fatalf("oversize err = %v", err)
	}
}

func TestRsrcFirstLeafDataPropagates(t *testing.T) {
	// A subdir whose sole entry is another (unreadable) subdir -> recursion then
	// the entries error surfaces.
	b := make([]byte, 24)
	putDir(b, 0, [][2]uint32{{1, 0x1000 | 0x80000000}}) // points beyond data
	r := &rsrc{data: b}
	if _, err := r.firstLeafData(0); err != errPERsrcBad {
		t.Fatalf("err = %v", err)
	}
}

func TestRsrcAssembleICOErrors(t *testing.T) {
	r := &rsrc{}
	if _, err := r.assembleICO([]byte{1, 2}); err != errPERsrcBad {
		t.Fatalf("short grp err = %v", err)
	}
	zero := make([]byte, 6) // count 0
	if _, err := r.assembleICO(zero); err != errPERsrcBad {
		t.Fatalf("zero-count err = %v", err)
	}
	over := make([]byte, 6)
	binary.LittleEndian.PutUint16(over[4:], 5) // count 5 but no entries
	if _, err := r.assembleICO(over); err != errPERsrcBad {
		t.Fatalf("overrun err = %v", err)
	}
}

func TestRsrcIconDataMissingMember(t *testing.T) {
	// Regions: root[0:24] iconType[24:48] nameDir[48:72] dataEntry[72:88].
	b := make([]byte, 88)
	putDir(b, 0, [][2]uint32{{rtIcon, 24 | 0x80000000}})
	putDir(b, 24, [][2]uint32{{1, 48 | 0x80000000}})
	putDir(b, 48, [][2]uint32{{langID, 72}})
	putDataEntry(b, 72, rsrcVA, 8)
	r := &rsrc{data: b, va: rsrcVA}
	if _, err := r.iconData(42); err != errPEIconMiss {
		t.Fatalf("missing member err = %v", err)
	}
	if _, err := r.iconData(1); err != nil { // id 1 present -> success
		t.Fatalf("iconData(1) = %v", err)
	}
}

func TestRsrcIconDataTypeDirUnreadable(t *testing.T) {
	// Root's RT_ICON entry points past the section, so findIconType succeeds but
	// the follow-up entries() of that subdir fails.
	b := make([]byte, 24)
	putDir(b, 0, [][2]uint32{{rtIcon, 0x1000 | 0x80000000}})
	if _, err := (&rsrc{data: b}).iconData(1); err != errPERsrcBad {
		t.Fatalf("err = %v", err)
	}
}

// ensure the assembled ICO round-trips through the ICO decoder for a group with
// a genuine member, independent of the PE wrapper.
func TestRsrcAssembleICORoundTrip(t *testing.T) {
	r := &rsrc{data: makeRsrc(0), va: rsrcVA}
	grp, err := r.firstLeafData(mustFindGroup(t, r))
	if err != nil {
		t.Fatal(err)
	}
	ico, err := r.assembleICO(grp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iconPNG(ico, 0); err != nil {
		t.Fatalf("assembled ICO undecodable: %v", err)
	}
	if !bytes.HasPrefix(ico, []byte{0, 0, 1, 0}) {
		t.Error("assembled bytes are not an ICONDIR")
	}
}

func mustFindGroup(t *testing.T, r *rsrc) int {
	t.Helper()
	off, err := r.findType(rtGroupIcon)
	if err != nil {
		t.Fatal(err)
	}
	return off
}
