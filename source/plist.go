// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
)

// This is a minimal, dependency-free reader for Apple property lists in the two
// forms an .app bundle's Contents/Info.plist ships in: the textual XML plist and
// the binary "bplist00" plist. It exists because no go-* plist library is
// available in the fleet; it is deliberately format-complete (every plist value
// kind) and self-contained, so it is reusable beyond the desktop source. It
// yields a generic Go value tree — map[string]any for a dict, []any for an
// array, string / bool / int64 / float64 / []byte for scalars — with a dict at
// the root, which is all an Info.plist ever is.

// parsePlist decodes a property list (binary bplist00 or XML) whose root is a
// dict, returning its key/value map. A non-dict root, an empty input or a
// malformed body is an error.
func parsePlist(data []byte) (map[string]any, error) {
	if bytes.HasPrefix(data, []byte("bplist00")) {
		return parseBinaryPlist(data)
	}
	return parseXMLPlist(data)
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

// ---------------------------------------------------------------------------
// XML plist
// ---------------------------------------------------------------------------

var (
	errNoPlistRoot  = errors.New("plist: no <plist> root element")
	errPlistNotDict = errors.New("plist: root value is not a dict")
	errKeyExpected  = errors.New("plist: <key> expected in dict")
	errValueMissing = errors.New("plist: value missing after <key>")
)

// parseXMLPlist decodes a textual XML property list.
func parseXMLPlist(data []byte) (map[string]any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, errNoPlistRoot
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "plist" {
			continue
		}
		vse, ok, err := xmlNextChild(dec)
		if err != nil {
			return nil, err
		}
		if !ok || vse.Name.Local != "dict" {
			return nil, errPlistNotDict
		}
		return xmlDict(dec)
	}
}

// xmlNextChild returns the next child start element of the element currently
// being parsed, or ok=false when the parent's end element is reached first.
func xmlNextChild(dec *xml.Decoder) (xml.StartElement, bool, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, true, nil
		case xml.EndElement:
			return xml.StartElement{}, false, nil
		}
	}
}

// xmlDict parses the body of a <dict> (the start element already consumed),
// returning its key/value map and consuming the closing </dict>.
func xmlDict(dec *xml.Decoder) (map[string]any, error) {
	m := map[string]any{}
	for {
		kse, ok, err := xmlNextChild(dec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return m, nil
		}
		if kse.Name.Local != "key" {
			return nil, errKeyExpected
		}
		key, err := xmlText(dec)
		if err != nil {
			return nil, err
		}
		vse, ok, err := xmlNextChild(dec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errValueMissing
		}
		v, err := xmlValue(dec, vse)
		if err != nil {
			return nil, err
		}
		m[key] = v
	}
}

// xmlValue parses the element se (its start already read) into a Go value,
// consuming through its end element.
func xmlValue(dec *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "string", "date":
		return xmlText(dec)
	case "true":
		_, err := xmlText(dec)
		return true, err
	case "false":
		_, err := xmlText(dec)
		return false, err
	case "integer":
		s, err := xmlText(dec)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return n, err
	case "real":
		s, err := xmlText(dec)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f, err
	case "data":
		s, err := xmlText(dec)
		if err != nil {
			return nil, err
		}
		return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	case "dict":
		return xmlDict(dec)
	case "array":
		return xmlArray(dec)
	default:
		// An unrecognised element kind is skipped (its end element consumed) and
		// yields a nil value, so an unknown plist type never aborts the parse.
		_, err := xmlText(dec)
		return nil, err
	}
}

// xmlArray parses the body of an <array> into a slice, consuming </array>.
func xmlArray(dec *xml.Decoder) ([]any, error) {
	var out []any
	for {
		se, ok, err := xmlNextChild(dec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		v, err := xmlValue(dec, se)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

// xmlText accumulates the character data of the current element up to its end
// element, skipping any (unexpected) nested elements.
func xmlText(dec *xml.Decoder) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			if err := dec.Skip(); err != nil {
				return "", err
			}
		case xml.EndElement:
			return b.String(), nil
		}
	}
}

// ---------------------------------------------------------------------------
// Binary plist (bplist00)
// ---------------------------------------------------------------------------

var (
	errBinShort   = errors.New("plist: binary plist too short")
	errBinTrailer = errors.New("plist: bad binary plist trailer")
	errBinTable   = errors.New("plist: offset table out of range")
	errBinRef     = errors.New("plist: object reference out of range")
	errBinOffset  = errors.New("plist: object offset out of range")
	errBinTrunc   = errors.New("plist: truncated object")
	errBinMarker  = errors.New("plist: unknown object marker")
	errBinCount   = errors.New("plist: bad element count")
	errBinReal    = errors.New("plist: bad real width")
	errBinKey     = errors.New("plist: dict key is not a string")
	errBinCycle   = errors.New("plist: object graph too deep")
)

// bplist is the decode state of a binary property list.
type bplist struct {
	data    []byte
	offsets []int
	refSize int
	num     int
}

// parseBinaryPlist decodes a bplist00 property list whose root object is a dict.
func parseBinaryPlist(data []byte) (map[string]any, error) {
	if len(data) < 8+32 {
		return nil, errBinShort
	}
	tr := data[len(data)-32:]
	offSize := int(tr[6])
	refSize := int(tr[7])
	num := int(be(tr[8:16]))
	top := int(be(tr[16:24]))
	tableOff := int(be(tr[24:32]))
	if offSize < 1 || refSize < 1 || num < 1 {
		return nil, errBinTrailer
	}
	if tableOff < 8 || tableOff+num*offSize > len(data) {
		return nil, errBinTable
	}
	p := &bplist{data: data, refSize: refSize, num: num, offsets: make([]int, num)}
	for i := 0; i < num; i++ {
		p.offsets[i] = int(be(data[tableOff+i*offSize : tableOff+(i+1)*offSize]))
	}
	v, err := p.object(top, 0)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errPlistNotDict
	}
	return m, nil
}

// object decodes the object with reference index i. depth guards against a
// cyclic (or pathologically deep) object graph.
func (p *bplist) object(i, depth int) (any, error) {
	if depth > p.num {
		return nil, errBinCycle
	}
	if i < 0 || i >= p.num {
		return nil, errBinRef
	}
	off := p.offsets[i]
	if off < 0 || off >= len(p.data) {
		return nil, errBinOffset
	}
	marker := p.data[off]
	hi, lo := marker>>4, marker&0x0f
	switch hi {
	case 0x0:
		switch marker {
		case 0x00, 0x0f: // null / fill
			return nil, nil
		case 0x08:
			return false, nil
		case 0x09:
			return true, nil
		default:
			return nil, errBinMarker
		}
	case 0x1: // integer, 2^lo bytes
		b, err := p.at(off+1, 1<<lo)
		if err != nil {
			return nil, err
		}
		return int64(be(b)), nil
	case 0x2: // real
		n := 1 << lo
		b, err := p.at(off+1, n)
		if err != nil {
			return nil, err
		}
		switch n {
		case 4:
			return float64(math.Float32frombits(uint32(be(b)))), nil
		case 8:
			return math.Float64frombits(be(b)), nil
		default:
			return nil, errBinReal
		}
	case 0x3: // date: 8-byte big-endian float (seconds since 2001)
		b, err := p.at(off+1, 8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(be(b)), nil
	case 0x4: // data
		cnt, hdr, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		b, err := p.at(off+1+hdr, cnt)
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), b...), nil
	case 0x5: // ASCII string
		cnt, hdr, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		b, err := p.at(off+1+hdr, cnt)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case 0x6: // UTF-16BE string
		cnt, hdr, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		b, err := p.at(off+1+hdr, cnt*2)
		if err != nil {
			return nil, err
		}
		u := make([]uint16, cnt)
		for k := 0; k < cnt; k++ {
			u[k] = binary.BigEndian.Uint16(b[k*2:])
		}
		return string(utf16.Decode(u)), nil
	case 0x8: // uid: lo+1 bytes
		b, err := p.at(off+1, int(lo)+1)
		if err != nil {
			return nil, err
		}
		return int64(be(b)), nil
	case 0xa: // array
		cnt, hdr, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		refs, err := p.refs(off+1+hdr, cnt)
		if err != nil {
			return nil, err
		}
		out := make([]any, cnt)
		for k, r := range refs {
			if out[k], err = p.object(r, depth+1); err != nil {
				return nil, err
			}
		}
		return out, nil
	case 0xd: // dict
		cnt, hdr, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		keyRefs, err := p.refs(off+1+hdr, cnt)
		if err != nil {
			return nil, err
		}
		valRefs, err := p.refs(off+1+hdr+cnt*p.refSize, cnt)
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, cnt)
		for k := 0; k < cnt; k++ {
			kv, err := p.object(keyRefs[k], depth+1)
			if err != nil {
				return nil, err
			}
			ks, ok := kv.(string)
			if !ok {
				return nil, errBinKey
			}
			if m[ks], err = p.object(valRefs[k], depth+1); err != nil {
				return nil, err
			}
		}
		return m, nil
	default:
		return nil, errBinMarker
	}
}

// count reads an element/byte count from an object marker: the low nibble is the
// count itself unless it is 0xf, in which case an inline integer object follows.
// hdr is the number of bytes the extended count occupies (0 for an inline
// nibble).
func (p *bplist) count(off int, lo byte) (cnt, hdr int, err error) {
	if lo != 0x0f {
		return int(lo), 0, nil
	}
	if off+1 >= len(p.data) {
		return 0, 0, errBinTrunc
	}
	m := p.data[off+1]
	if m>>4 != 0x1 {
		return 0, 0, errBinCount
	}
	n := 1 << (m & 0x0f)
	b, err := p.at(off+2, n)
	if err != nil {
		return 0, 0, err
	}
	return int(be(b)), 1 + n, nil
}

// refs reads count object references (each refSize bytes) starting at start.
func (p *bplist) refs(start, count int) ([]int, error) {
	if count < 0 || start < 0 || start+count*p.refSize > len(p.data) {
		return nil, errBinRef
	}
	out := make([]int, count)
	for k := 0; k < count; k++ {
		out[k] = int(be(p.data[start+k*p.refSize : start+(k+1)*p.refSize]))
	}
	return out, nil
}

// at returns the n-byte slice at start, bounds-checked.
func (p *bplist) at(start, n int) ([]byte, error) {
	if n < 0 || start < 0 || start+n > len(p.data) {
		return nil, errBinTrunc
	}
	return p.data[start : start+n], nil
}

// be reads a big-endian unsigned integer from up to 8 bytes.
func be(b []byte) uint64 {
	var v uint64
	if len(b) > 8 {
		b = b[len(b)-8:]
	}
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v
}
