// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
)

// icoEntry is one image to place in a built .ico: its directory byte dimensions
// (0 encodes 256) and its raw payload (a PNG or a BMP DIB).
type icoEntry struct {
	w, h byte
	data []byte
}

// buildICO assembles a valid ICONDIR + entry table + concatenated image data.
func buildICO(entries ...icoEntry) []byte {
	var dir bytes.Buffer
	binary.Write(&dir, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&dir, binary.LittleEndian, uint16(1)) // type = icon
	binary.Write(&dir, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + len(entries)*16
	var blob bytes.Buffer
	for _, e := range entries {
		dir.WriteByte(e.w)
		dir.WriteByte(e.h)
		dir.WriteByte(0) // colour count
		dir.WriteByte(0) // reserved
		binary.Write(&dir, binary.LittleEndian, uint16(1))              // planes
		binary.Write(&dir, binary.LittleEndian, uint16(32))             // bit count
		binary.Write(&dir, binary.LittleEndian, uint32(len(e.data)))    // bytes in res
		binary.Write(&dir, binary.LittleEndian, uint32(offset))         // image offset
		offset += len(e.data)
		blob.Write(e.data)
	}
	return append(dir.Bytes(), blob.Bytes()...)
}

func dibHeader(w, h, bits int) []byte {
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:4], 40)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(int32(w)))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(int32(2*h))) // doubled height
	binary.LittleEndian.PutUint16(hdr[12:14], 1)
	binary.LittleEndian.PutUint16(hdr[14:16], uint16(bits))
	return hdr
}

// dib32 builds a 32-bpp BGRA icon DIB filled with a per-pixel colour.
func dib32(w, h int) []byte {
	px := make([]byte, w*4*h)
	for i := 0; i < w*h; i++ {
		px[i*4+0] = 0x10 // B
		px[i*4+1] = 0x20 // G
		px[i*4+2] = 0x30 // R
		px[i*4+3] = 0xFF // A (opaque: premultiplied == straight, survives PNG)
	}
	return append(dibHeader(w, h, 32), px...)
}

// dib24 builds a 24-bpp BGR icon DIB, optionally with a 1-bpp AND mask.
func dib24(w, h int, withMask bool) []byte {
	xorStride := ((w*24 + 31) / 32) * 4
	px := make([]byte, xorStride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*xorStride + x*3
			px[i], px[i+1], px[i+2] = 0x11, 0x22, 0x33
		}
	}
	out := append(dibHeader(w, h, 24), px...)
	if withMask {
		andStride := ((w + 31) / 32) * 4
		mask := make([]byte, andStride*h)
		mask[0] = 0x80 // top-left (bottom-up: last row) pixel transparent
		out = append(out, mask...)
	}
	return out
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	return img
}

func TestICOBestPNGPicksLargest32(t *testing.T) {
	ico := buildICO(
		icoEntry{16, 16, dib32(16, 16)},
		icoEntry{32, 32, dib32(32, 32)},
	)
	out, err := icoBestPNG(ico)
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out)
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size = %v, want 32x32", b)
	}
	r, g, bl, a := img.At(0, 0).RGBA()
	if r>>8 != 0x30 || g>>8 != 0x20 || bl>>8 != 0x10 || a>>8 != 0xFF {
		t.Errorf("pixel = %#x %#x %#x %#x", r>>8, g>>8, bl>>8, a>>8)
	}
}

func TestICOBestPNG256Dimension(t *testing.T) {
	// A 0 dimension byte encodes 256, so this entry wins over a 32x32.
	ico := buildICO(
		icoEntry{32, 32, dib32(32, 32)},
		icoEntry{0, 0, dib32(64, 64)},
	)
	out, err := icoBestPNG(ico)
	if err != nil {
		t.Fatal(err)
	}
	if b := decodePNG(t, out).Bounds(); b.Dx() != 64 {
		t.Fatalf("size = %v, want the 256-coded (64px) entry", b)
	}
}

func TestICOBestPNG24WithMask(t *testing.T) {
	out, err := icoBestPNG(buildICO(icoEntry{4, 4, dib24(4, 4, true)}))
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out)
	// The colour is BGR 0x11,0x22,0x33 -> RGB 0x33,0x22,0x11.
	r, g, bl, _ := img.At(1, 1).RGBA()
	if r>>8 != 0x33 || g>>8 != 0x22 || bl>>8 != 0x11 {
		t.Errorf("pixel = %#x %#x %#x", r>>8, g>>8, bl>>8)
	}
	// The masked pixel (bottom-up first byte MSB) is the top-left of row h-1.
	if _, _, _, a := img.At(0, 3).RGBA(); a>>8 != 0 {
		t.Errorf("masked pixel alpha = %#x, want 0", a>>8)
	}
}

func TestICOBestPNG24NoMask(t *testing.T) {
	out, err := icoBestPNG(buildICO(icoEntry{4, 4, dib24(4, 4, false)}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := decodePNG(t, out).At(0, 0).RGBA(); a>>8 != 0xFF {
		t.Errorf("no-mask alpha = %#x, want opaque", a>>8)
	}
}

func TestICOBestPNGVerbatimPNG(t *testing.T) {
	raw := pngOf(t, 48, 48)
	out, err := icoBestPNG(buildICO(icoEntry{48, 48, raw}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, raw) {
		t.Error("PNG-backed entry not returned verbatim")
	}
}

func TestICOBestPNGErrors(t *testing.T) {
	t.Run("short", func(t *testing.T) {
		if _, err := icoBestPNG([]byte{0, 0, 1}); err != errICOShort {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad type", func(t *testing.T) {
		if _, err := icoBestPNG([]byte{0, 0, 2, 0, 1, 0}); err != errICOType {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := icoBestPNG([]byte{0, 0, 1, 0, 0, 0}); err != errICOEmpty {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("entry table out of range", func(t *testing.T) {
		// count=1 but no room for the 16-byte entry.
		if _, err := icoBestPNG([]byte{0, 0, 1, 0, 1, 0}); err != errICOEntry {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("image data out of range", func(t *testing.T) {
		ico := buildICO(icoEntry{4, 4, dib32(4, 4)})
		binary.LittleEndian.PutUint32(ico[6+12:6+16], 0xFFFF) // bogus offset
		if _, err := icoBestPNG(ico); err != errICOData {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("dib propagated", func(t *testing.T) {
		ico := buildICO(icoEntry{4, 4, []byte{1, 2, 3}}) // too short for a DIB
		if _, err := icoBestPNG(ico); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestDecodeDIBErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		if _, err := decodeDIB(make([]byte, 39)); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad geometry", func(t *testing.T) {
		hdr := dibHeader(0, 4, 32) // width 0
		if _, err := decodeDIB(hdr); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("compressed", func(t *testing.T) {
		hdr := dibHeader(4, 4, 32)
		binary.LittleEndian.PutUint32(hdr[16:20], 1) // BI_RLE, unsupported
		if _, err := decodeDIB(append(hdr, make([]byte, 4*4*4)...)); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unsupported depth", func(t *testing.T) {
		hdr := dibHeader(4, 4, 8) // 8-bpp palettised, unsupported
		if _, err := decodeDIB(append(hdr, make([]byte, 64)...)); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("truncated pixels", func(t *testing.T) {
		hdr := dibHeader(4, 4, 32) // no pixel bytes follow
		if _, err := decodeDIB(hdr); err != errICODIB {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestICOBestPNGEncodeError(t *testing.T) {
	// Force the (normally unreachable) png.Encode error branch through the seam.
	orig := pngEncode
	pngEncode = func(w io.Writer, m image.Image) error { return errFakeEncode }
	defer func() { pngEncode = orig }()
	if _, err := icoBestPNG(buildICO(icoEntry{4, 4, dib32(4, 4)})); err != errFakeEncode {
		t.Fatalf("err = %v, want errFakeEncode", err)
	}
}

var errFakeEncode = errorString("forced encode failure")

type errorString string

func (e errorString) Error() string { return string(e) }

// keep color imported for the DIB helpers above.
var _ = color.RGBA{}
