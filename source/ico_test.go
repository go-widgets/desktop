// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"encoding/binary"
	"image"
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
		dir.WriteByte(0)                                             // colour count
		dir.WriteByte(0)                                             // reserved
		binary.Write(&dir, binary.LittleEndian, uint16(1))           // planes
		binary.Write(&dir, binary.LittleEndian, uint16(32))          // bit count
		binary.Write(&dir, binary.LittleEndian, uint32(len(e.data))) // bytes in res
		binary.Write(&dir, binary.LittleEndian, uint32(offset))      // image offset
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

// andMask builds the 1-bpp AND transparency mask that every well-formed BMP
// icon image carries after its colour data. A real .ico (and the RT_ICON
// resources peicon assembles) always ships this mask; a mask bit set to 1 marks
// the pixel transparent. It is stored bottom-up, MSB-first, rows padded to 4
// bytes. transparentTopLeft marks the image-top-left pixel transparent.
func andMask(w, h int, transparentTopLeft bool) []byte {
	stride := ((w + 31) / 32) * 4
	mask := make([]byte, stride*h)
	if transparentTopLeft {
		mask[(h-1)*stride] = 0x80 // bottom-up: last row's first byte, MSB
	}
	return mask
}

// dib32 builds a 32-bpp BGRA icon DIB (real-icon form: colour data + all-zero
// AND mask) filled with a per-pixel colour.
func dib32(w, h int) []byte {
	px := make([]byte, w*4*h)
	for i := 0; i < w*h; i++ {
		px[i*4+0] = 0x10 // B
		px[i*4+1] = 0x20 // G
		px[i*4+2] = 0x30 // R
		px[i*4+3] = 0xFF // A (opaque)
	}
	return append(append(dibHeader(w, h, 32), px...), andMask(w, h, false)...)
}

// dib24 builds a 24-bpp BGR icon DIB with its 1-bpp AND mask; transparentTopLeft
// marks the top-left pixel transparent through the mask (24-bpp has no alpha).
func dib24(w, h int, transparentTopLeft bool) []byte {
	xorStride := ((w*24 + 31) / 32) * 4
	px := make([]byte, xorStride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*xorStride + x*3
			px[i], px[i+1], px[i+2] = 0x11, 0x22, 0x33
		}
	}
	out := append(dibHeader(w, h, 24), px...)
	return append(out, andMask(w, h, transparentTopLeft)...)
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
		t.Fatalf("size = %v, want 32x32 (largest)", b)
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
	// The masked (top-left) pixel decodes transparent.
	if _, _, _, a := img.At(0, 0).RGBA(); a>>8 != 0 {
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

func TestICOBestPNGFromPNGEntry(t *testing.T) {
	// A PNG-backed entry decodes to the same image (re-encoded, not verbatim).
	raw := pngOf(t, 48, 48)
	out, err := icoBestPNG(buildICO(icoEntry{48, 48, raw}))
	if err != nil {
		t.Fatal(err)
	}
	got, want := decodePNG(t, out), decodePNG(t, raw)
	if gb, wb := got.Bounds(), want.Bounds(); gb != wb {
		t.Fatalf("bounds = %v, want %v", gb, wb)
	}
	gr, gg, gbl, ga := got.At(10, 20).RGBA()
	wr, wg, wbl, wa := want.At(10, 20).RGBA()
	if gr != wr || gg != wg || gbl != wbl || ga != wa {
		t.Errorf("pixel mismatch: %v vs %v", got.At(10, 20), want.At(10, 20))
	}
}

func TestICOBestPNGDecodeError(t *testing.T) {
	// Not an icon container: the reference decoder reports a format error.
	if _, err := icoBestPNG([]byte("not an icon at all")); err == nil {
		t.Fatal("want a decode error for non-ICO input")
	}
	// Empty input also fails to decode.
	if _, err := icoBestPNG(nil); err == nil {
		t.Fatal("want a decode error for empty input")
	}
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
