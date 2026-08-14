// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

// This is a minimal, pure-Go reader for the Windows Shell Link (.lnk) binary
// format — [MS-SHLLINK] — extracting exactly what the desktop shell needs from
// a Start Menu shortcut: the launch target path, the command-line arguments,
// the icon location (+ index) and the embedded description. A .lnk is a
// ShellLinkHeader followed, per the header's LinkFlags, by an optional
// LinkTargetIDList, an optional LinkInfo (which carries the resolved local base
// path) and a run of StringData fields (NAME / RELATIVE_PATH / WORKING_DIR /
// ARGUMENTS / ICON_LOCATION). No go-* .lnk library exists in the fleet, so this
// lives here; it is deliberately GOOS-agnostic (no syscalls, no build tag) so it
// compiles and is unit-tested on every platform, with the windows-only FS scan
// (windows.go) layered on top.

var (
	errLNKShort    = errors.New("lnk: too short for header")
	errLNKMagic    = errors.New("lnk: bad ShellLinkHeader size")
	errLNKCLSID    = errors.New("lnk: bad LinkCLSID")
	errLNKIDList   = errors.New("lnk: truncated LinkTargetIDList")
	errLNKInfo     = errors.New("lnk: truncated LinkInfo")
	errLNKString   = errors.New("lnk: truncated StringData")
	errLNKNoTarget = errors.New("lnk: no target path")
)

// shellLink is the distilled content of one .lnk shortcut.
type shellLink struct {
	target       string // resolved local base path (+ common path suffix)
	relativePath string // RELATIVE_PATH StringData (e.g. ..\..\App\app.exe)
	workingDir   string // WORKING_DIR StringData
	arguments    string // COMMAND_LINE_ARGUMENTS StringData
	description  string // NAME_STRING StringData
	iconLocation string // ICON_LOCATION StringData (may hold env vars)
	iconIndex    int32  // IconIndex from the header
}

// Shell Link header constants and the fixed CLSID {00021401-0000-0000-C000-
// 000000000046} in its little-endian on-disk byte order.
const (
	lnkHeaderSize = 0x0000004C

	flagHasLinkTargetIDList = 1 << 0
	flagHasLinkInfo         = 1 << 1
	flagHasName             = 1 << 2
	flagHasRelativePath     = 1 << 3
	flagHasWorkingDir       = 1 << 4
	flagHasArguments        = 1 << 5
	flagHasIconLocation     = 1 << 6
	flagIsUnicode           = 1 << 7

	linkInfoHasVolumeID   = 1 << 0
	linkInfoHasNetworkRel = 1 << 1
)

var lnkCLSID = [16]byte{
	0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}

// parseLink decodes a .lnk shortcut. It validates the ShellLinkHeader (size and
// CLSID), skips the LinkTargetIDList, reads the LinkInfo local base path when
// present, then walks the StringData fields the LinkFlags advertise. A shortcut
// that carries no LinkInfo target falls back to its RELATIVE_PATH; one with
// neither is a parse error (errLNKNoTarget).
func parseLink(data []byte) (*shellLink, error) {
	if len(data) < lnkHeaderSize {
		return nil, errLNKShort
	}
	if binary.LittleEndian.Uint32(data[0:4]) != lnkHeaderSize {
		return nil, errLNKMagic
	}
	var clsid [16]byte
	copy(clsid[:], data[4:20])
	if clsid != lnkCLSID {
		return nil, errLNKCLSID
	}
	flags := binary.LittleEndian.Uint32(data[20:24])
	sl := &shellLink{iconIndex: int32(binary.LittleEndian.Uint32(data[56:60]))}

	off := lnkHeaderSize
	if flags&flagHasLinkTargetIDList != 0 {
		next, err := skipIDList(data, off)
		if err != nil {
			return nil, err
		}
		off = next
	}
	if flags&flagHasLinkInfo != 0 {
		target, next, err := readLinkInfo(data, off)
		if err != nil {
			return nil, err
		}
		sl.target = target
		off = next
	}

	unicode := flags&flagIsUnicode != 0
	// StringData fields appear in this fixed order, each gated by its flag.
	for _, sd := range []struct {
		flag uint32
		dst  *string
	}{
		{flagHasName, &sl.description},
		{flagHasRelativePath, &sl.relativePath},
		{flagHasWorkingDir, &sl.workingDir},
		{flagHasArguments, &sl.arguments},
		{flagHasIconLocation, &sl.iconLocation},
	} {
		if flags&sd.flag == 0 {
			continue
		}
		s, next, err := readStringData(data, off, unicode)
		if err != nil {
			return nil, err
		}
		*sd.dst = s
		off = next
	}

	if sl.target == "" {
		if sl.relativePath == "" {
			return nil, errLNKNoTarget
		}
		sl.target = sl.relativePath
	}
	return sl, nil
}

// skipIDList advances past the LinkTargetIDList: a 2-byte IDListSize followed by
// that many bytes of item-id data. Its content (a shell-namespace PIDL) is not
// needed — LinkInfo carries the resolvable path — so it is only skipped.
func skipIDList(data []byte, off int) (int, error) {
	if off+2 > len(data) {
		return 0, errLNKIDList
	}
	size := int(binary.LittleEndian.Uint16(data[off : off+2]))
	end := off + 2 + size
	if end > len(data) {
		return 0, errLNKIDList
	}
	return end, nil
}

// readLinkInfo reads the LinkInfo structure and returns its local base path (the
// resolved target, e.g. C:\Program Files\App\app.exe), preferring the Unicode
// path when the optional-unicode header extension is present. A LinkInfo with
// only a network relative link (no VolumeIDAndLocalBasePath) yields "".
func readLinkInfo(data []byte, off int) (string, int, error) {
	if off+4 > len(data) {
		return "", 0, errLNKInfo
	}
	size := int(binary.LittleEndian.Uint32(data[off : off+4]))
	end := off + size
	if size < 0x1C || end > len(data) {
		return "", 0, errLNKInfo
	}
	block := data[off:end]
	headerSize := int(binary.LittleEndian.Uint32(block[4:8]))
	infoFlags := binary.LittleEndian.Uint32(block[8:12])

	var target string
	if infoFlags&linkInfoHasVolumeID != 0 {
		// Prefer the optional Unicode local base path (headerSize >= 0x24).
		if headerSize >= 0x24 && len(block) >= 32 {
			if u := int(binary.LittleEndian.Uint32(block[28:32])); u != 0 {
				target = utf16zAt(block, u)
			}
		}
		if target == "" {
			base := int(binary.LittleEndian.Uint32(block[16:20]))
			suffix := int(binary.LittleEndian.Uint32(block[24:28]))
			target = ansizAt(block, base) + ansizAt(block, suffix)
		}
	}
	return target, end, nil
}

// readStringData reads one StringData field: a 2-byte character count followed
// by that many characters — 2 bytes each (UTF-16LE) when the link is Unicode,
// 1 byte each (code page) otherwise. It returns the decoded string and the
// offset just past it.
func readStringData(data []byte, off int, unicode bool) (string, int, error) {
	if off+2 > len(data) {
		return "", 0, errLNKString
	}
	count := int(binary.LittleEndian.Uint16(data[off : off+2]))
	off += 2
	if unicode {
		n := count * 2
		if off+n > len(data) {
			return "", 0, errLNKString
		}
		u := make([]uint16, count)
		for i := 0; i < count; i++ {
			u[i] = binary.LittleEndian.Uint16(data[off+i*2 : off+i*2+2])
		}
		return string(utf16.Decode(u)), off + n, nil
	}
	if off+count > len(data) {
		return "", 0, errLNKString
	}
	return string(data[off : off+count]), off + count, nil
}

// ansizAt reads a NUL-terminated single-byte string at rel within block; an
// out-of-range offset yields "".
func ansizAt(block []byte, rel int) string {
	if rel <= 0 || rel >= len(block) {
		return ""
	}
	end := rel
	for end < len(block) && block[end] != 0 {
		end++
	}
	return string(block[rel:end])
}

// utf16zAt reads a NUL-terminated UTF-16LE string at rel within block; an
// out-of-range or odd-length run yields "".
func utf16zAt(block []byte, rel int) string {
	if rel <= 0 || rel >= len(block) {
		return ""
	}
	var u []uint16
	for i := rel; i+1 < len(block); i += 2 {
		c := binary.LittleEndian.Uint16(block[i : i+2])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
