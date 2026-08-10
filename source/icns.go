// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"
)

// This is a minimal reader for the Apple .icns icon container, extracting the
// best (largest) PNG-encoded representation an icon carries. Modern .icns files
// store each retina/size variant as an embedded PNG (chunk types ic07/ic08/
// ic09/ic10/ic11/ic12/ic13/ic14) — and some legacy variants as raw ARGB or
// JPEG-2000, which this reader skips: the goal is a decodable icon for the
// shell's icon seam, and stdlib image/png decodes the PNG variants directly. No
// go-* icns library exists in the fleet, so this lives here and is reusable.

var (
	errNotICNS  = errors.New("icns: not an icns container")
	errICNSSize = errors.New("icns: declared size out of range")
	errICNSBad  = errors.New("icns: corrupt chunk")
	errICNSNone = errors.New("icns: no PNG representation")
)

// pngMagic is the 8-byte PNG file signature.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// icnsBestPNG parses an .icns container and returns the encoded PNG bytes of its
// largest (highest pixel-width) PNG representation. Chunks that are not PNG
// (raw ARGB, JPEG-2000, masks, the TOC) are skipped. It errors when the input
// is not an icns container, is structurally corrupt, or carries no PNG variant.
func icnsBestPNG(data []byte) ([]byte, error) {
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, errNotICNS
	}
	total := int(binary.BigEndian.Uint32(data[4:8]))
	if total < 8 || total > len(data) {
		return nil, errICNSSize
	}
	var best []byte
	bestW := -1
	for off := 8; off+8 <= total; {
		length := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if length < 8 || off+length > total {
			return nil, errICNSBad
		}
		payload := data[off+8 : off+length]
		if bytes.HasPrefix(payload, pngMagic) {
			if cfg, err := png.DecodeConfig(bytes.NewReader(payload)); err == nil && cfg.Width > bestW {
				bestW = cfg.Width
				best = payload
			}
		}
		off += length
	}
	if best == nil {
		return nil, errICNSNone
	}
	return append([]byte(nil), best...), nil
}
