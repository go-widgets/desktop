// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

// This is a minimal, pure-Go decoder for the Windows .ico container, returning
// the encoded PNG bytes of the icon's largest representation for the shell's
// icon seam. An .ico is an ICONDIR (a directory of ICONDIRENTRY records) whose
// entries each hold either a PNG (Vista+ large icons — returned verbatim) or a
// bottom-up BMP DIB (a BITMAPINFOHEADER with no file header, whose declared
// height is doubled to cover the colour XOR bitmap plus a 1-bpp AND transparency
// mask). It decodes 32-bpp BGRA and 24-bpp BGR + AND-mask DIBs — the two forms
// Windows shortcut icons actually use — re-encoding them to PNG; other depths
// and malformed data are reported as errors. It is GOOS-agnostic (no build tag)
// so it compiles and is tested on every platform.

var (
	errICOShort = errors.New("ico: too short for ICONDIR")
	errICOType  = errors.New("ico: not an icon directory")
	errICOEmpty = errors.New("ico: no entries")
	errICOEntry = errors.New("ico: entry out of range")
	errICOData  = errors.New("ico: image data out of range")
	errICODIB   = errors.New("ico: unsupported or malformed DIB")
)

// pngEncode is a seam over png.Encode so the (otherwise unreachable, since
// bytes.Buffer never fails) encode-error path can be exercised deterministically.
var pngEncode = png.Encode

// icoBestPNG decodes the largest image in an .ico container and returns it as
// encoded PNG bytes. PNG-backed entries are returned unchanged; BMP-backed
// entries are decoded and re-encoded.
func icoBestPNG(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, errICOShort
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, errICOType
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 {
		return nil, errICOEmpty
	}
	if 6+count*16 > len(data) {
		return nil, errICOEntry
	}
	// Pick the entry with the greatest pixel area (a 0 dimension means 256).
	best, bestArea := -1, -1
	for i := 0; i < count; i++ {
		e := data[6+i*16:]
		w, h := dim(e[0]), dim(e[1])
		if w*h > bestArea {
			bestArea, best = w*h, i
		}
	}
	e := data[6+best*16:]
	size := int(binary.LittleEndian.Uint32(e[8:12]))
	off := int(binary.LittleEndian.Uint32(e[12:16]))
	if size <= 0 || off < 0 || off+size > len(data) {
		return nil, errICOData
	}
	payload := data[off : off+size]
	if bytes.HasPrefix(payload, pngMagic) {
		return append([]byte(nil), payload...), nil
	}
	img, err := decodeDIB(payload)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pngEncode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// dim maps an ICONDIRENTRY byte dimension to pixels: 0 encodes 256.
func dim(b byte) int {
	if b == 0 {
		return 256
	}
	return int(b)
}

// decodeDIB decodes an icon BMP DIB (BITMAPINFOHEADER + XOR pixels + optional
// 1-bpp AND mask) into an RGBA image. It supports 32-bpp BGRA (alpha from the
// pixel) and 24-bpp BGR (alpha from the AND mask); the stored height is halved
// to the real image height. Rows are stored bottom-up.
func decodeDIB(p []byte) (*image.RGBA, error) {
	if len(p) < 40 {
		return nil, errICODIB
	}
	headerSize := int(binary.LittleEndian.Uint32(p[0:4]))
	w := int(int32(binary.LittleEndian.Uint32(p[4:8])))
	h2 := int(int32(binary.LittleEndian.Uint32(p[8:12])))
	bits := int(binary.LittleEndian.Uint16(p[14:16]))
	compression := binary.LittleEndian.Uint32(p[16:20])
	h := h2 / 2
	if headerSize < 40 || headerSize > len(p) || w <= 0 || h <= 0 || compression != 0 {
		return nil, errICODIB
	}
	if bits != 32 && bits != 24 {
		return nil, errICODIB
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	xorStride := ((w*bits + 31) / 32) * 4 // rows padded to 4 bytes
	xor := p[headerSize:]
	if len(xor) < xorStride*h {
		return nil, errICODIB
	}
	// The AND mask (1 bpp) follows the colour data; absent for pure-alpha 32-bpp.
	andStride := ((w + 31) / 32) * 4
	andMask := xor[xorStride*h:]
	haveMask := len(andMask) >= andStride*h

	for y := 0; y < h; y++ {
		srcRow := (h - 1 - y) * xorStride // bottom-up
		for x := 0; x < w; x++ {
			var r, g, b, a uint8
			switch bits {
			case 32:
				i := srcRow + x*4
				b, g, r, a = xor[i], xor[i+1], xor[i+2], xor[i+3]
			case 24:
				i := srcRow + x*3
				b, g, r = xor[i], xor[i+1], xor[i+2]
				a = 0xff
				if haveMask && andBit(andMask, andStride, h, x, y) {
					a = 0
				}
			}
			img.SetRGBA(x, y, color.RGBA{r, g, b, a})
		}
	}
	return img, nil
}

// andBit reports whether the 1-bpp AND-mask bit for pixel (x,y) is set (1 =
// transparent). The mask is bottom-up like the colour data, MSB-first per byte.
func andBit(mask []byte, stride, h, x, y int) bool {
	row := (h - 1 - y) * stride
	return mask[row+x/8]&(0x80>>(uint(x)%8)) != 0
}
