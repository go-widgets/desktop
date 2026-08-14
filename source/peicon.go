// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
)

// This extracts an application icon from a Windows PE image (.exe / .dll) in
// pure Go: a shortcut whose IconLocation points at an executable carries no
// standalone .ico, so the icon must be reassembled from the module's resource
// section. It parses the PE with the stdlib debug/pe, walks the .rsrc resource
// tree to the RT_GROUP_ICON directory (the icon's ICONDIR), gathers the matching
// RT_ICON images, and stitches them back into a single in-memory .ico — which
// iconPNG then decodes to a PNG. It is GOOS-agnostic (debug/pe reads bytes,
// no syscalls) so it compiles and is tested on every platform.

const (
	rtIcon      = 3  // RT_ICON resource type
	rtGroupIcon = 14 // RT_GROUP_ICON resource type
)

var (
	errPEParse    = errors.New("peicon: not a PE image")
	errPENoRsrc   = errors.New("peicon: no .rsrc section")
	errPENoGroup  = errors.New("peicon: no RT_GROUP_ICON resource")
	errPERsrcBad  = errors.New("peicon: malformed resource tree")
	errPEIconMiss = errors.New("peicon: RT_ICON member missing")
)

// peIconPNG extracts the module's group icon, decodes the representation best
// matching targetPx, and returns it as PNG bytes (targetPx <= 0 = largest).
func peIconPNG(data []byte, targetPx int) ([]byte, error) {
	ico, err := peExtractICO(data)
	if err != nil {
		return nil, err
	}
	return iconPNG(ico, targetPx)
}

// rsrc is the parsed .rsrc section: its raw bytes plus its virtual address, so
// an IMAGE_RESOURCE_DATA_ENTRY's RVA can be mapped back into the section slice.
type rsrc struct {
	data []byte
	va   uint32
}

// rsrcEntry is one IMAGE_RESOURCE_DIRECTORY_ENTRY: an integer id (or a name,
// whose high bit is set — irrelevant to numeric icon ids), and either a
// subdirectory offset or a leaf data-entry offset within the section.
type rsrcEntry struct {
	id    uint32
	isDir bool
	off   int
}

// peExtractICO parses a PE image, reads its RT_GROUP_ICON directory and the
// RT_ICON members it references, and assembles a standard .ico byte stream.
func peExtractICO(data []byte) ([]byte, error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, errPEParse
	}
	sec := f.Section(".rsrc")
	if sec == nil {
		return nil, errPENoRsrc
	}
	raw, err := sec.Data()
	if err != nil {
		return nil, errPENoRsrc
	}
	r := &rsrc{data: raw, va: sec.VirtualAddress}

	groupDir, err := r.findType(rtGroupIcon)
	if err != nil {
		return nil, err
	}
	grp, err := r.firstLeafData(groupDir)
	if err != nil {
		return nil, err
	}
	return r.assembleICO(grp)
}

// entries reads the directory at dirOff and returns its entries. The 16-byte
// IMAGE_RESOURCE_DIRECTORY header is followed by NumberOfNamedEntries +
// NumberOfIdEntries 8-byte entries.
func (r *rsrc) entries(dirOff int) ([]rsrcEntry, error) {
	if dirOff < 0 || dirOff+16 > len(r.data) {
		return nil, errPERsrcBad
	}
	named := int(binary.LittleEndian.Uint16(r.data[dirOff+12 : dirOff+14]))
	ids := int(binary.LittleEndian.Uint16(r.data[dirOff+14 : dirOff+16]))
	n := named + ids
	base := dirOff + 16
	if n <= 0 || base+n*8 > len(r.data) {
		return nil, errPERsrcBad
	}
	out := make([]rsrcEntry, 0, n)
	for i := 0; i < n; i++ {
		o := base + i*8
		id := binary.LittleEndian.Uint32(r.data[o : o+4])
		od := binary.LittleEndian.Uint32(r.data[o+4 : o+8])
		out = append(out, rsrcEntry{id: id, isDir: od&0x80000000 != 0, off: int(od & 0x7fffffff)})
	}
	return out, nil
}

// findType returns the offset of the level-1 subdirectory for a numeric
// resource type (e.g. RT_GROUP_ICON), or an error when absent.
func (r *rsrc) findType(typeID uint32) (int, error) {
	es, err := r.entries(0)
	if err != nil {
		return 0, err
	}
	for _, e := range es {
		if e.isDir && e.id == typeID {
			return e.off, nil
		}
	}
	return 0, errPENoGroup
}

// firstLeafData descends the first entry at each level until it reaches a leaf
// IMAGE_RESOURCE_DATA_ENTRY, returning the referenced bytes.
func (r *rsrc) firstLeafData(dirOff int) ([]byte, error) {
	es, err := r.entries(dirOff)
	if err != nil {
		return nil, err
	}
	e := es[0]
	if e.isDir {
		return r.firstLeafData(e.off)
	}
	return r.leafData(e.off)
}

// iconData returns the RT_ICON image with the given numeric id (the nID a group
// entry references).
func (r *rsrc) iconData(iconID uint32) ([]byte, error) {
	typeDir, err := r.findIconType()
	if err != nil {
		return nil, err
	}
	es, err := r.entries(typeDir)
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.id == iconID && e.isDir {
			return r.firstLeafData(e.off)
		}
	}
	return nil, errPEIconMiss
}

// findIconType returns the level-1 subdirectory offset for RT_ICON.
func (r *rsrc) findIconType() (int, error) {
	es, err := r.entries(0)
	if err != nil {
		return 0, err
	}
	for _, e := range es {
		if e.isDir && e.id == rtIcon {
			return e.off, nil
		}
	}
	return 0, errPEIconMiss
}

// leafData resolves an IMAGE_RESOURCE_DATA_ENTRY: OffsetToData (an RVA), Size.
// The RVA is mapped back into the .rsrc slice via the section virtual address.
func (r *rsrc) leafData(off int) ([]byte, error) {
	if off < 0 || off+16 > len(r.data) {
		return nil, errPERsrcBad
	}
	rva := binary.LittleEndian.Uint32(r.data[off : off+4])
	size := binary.LittleEndian.Uint32(r.data[off+4 : off+8])
	start := int(rva) - int(r.va)
	if start < 0 || size == 0 || start+int(size) > len(r.data) {
		return nil, errPERsrcBad
	}
	return r.data[start : start+int(size)], nil
}

// assembleICO turns a GRPICONDIR (the RT_GROUP_ICON payload) plus its RT_ICON
// members into a standalone .ico byte stream: an ICONDIR, one 16-byte
// ICONDIRENTRY per member (the group's 12 descriptor bytes + the real byte size
// and a computed file offset), then the concatenated image data.
func (r *rsrc) assembleICO(grp []byte) ([]byte, error) {
	if len(grp) < 6 {
		return nil, errPERsrcBad
	}
	count := int(binary.LittleEndian.Uint16(grp[4:6]))
	if count == 0 || 6+count*14 > len(grp) {
		return nil, errPERsrcBad
	}
	var dir bytes.Buffer
	var images bytes.Buffer
	dir.Write(grp[0:4]) // idReserved (0) + idType (1)
	binary.Write(&dir, binary.LittleEndian, uint16(count))

	offset := 6 + count*16 // image data begins after the entry table
	for i := 0; i < count; i++ {
		ge := grp[6+i*14 : 6+i*14+14]
		nID := uint32(binary.LittleEndian.Uint16(ge[12:14]))
		img, err := r.iconData(nID)
		if err != nil {
			return nil, err
		}
		dir.Write(ge[0:8]) // bWidth,bHeight,bColorCount,bReserved,wPlanes,wBitCount
		binary.Write(&dir, binary.LittleEndian, uint32(len(img)))
		binary.Write(&dir, binary.LittleEndian, uint32(offset))
		images.Write(img)
		offset += len(img)
	}
	return append(dir.Bytes(), images.Bytes()...), nil
}
