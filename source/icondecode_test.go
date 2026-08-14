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
	"io"
	"testing"

	ico "github.com/sergeymakinen/go-ico"
)

// This file exercises the codec-backed icon-decode seam (icondecode.go) and
// carries the shared, SYNTHETIC icon fixtures the darwin/windows/peicon tests
// reuse. Every fixture is fabricated in the test — no real application icon is
// ever read from disk. Parity is proven against the code the migration REPLACED,
// reproduced verbatim below (icnsBestPNGReplaced / the go-ico Decode call).

// ---- synthetic fixture builders ---------------------------------------------

// pngOf encodes a solid w×h RGBA PNG (deterministic per-pixel test icon bytes).
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 0x80, 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png: %v", err)
	}
	return buf.Bytes()
}

// chunk is one 4-byte-type icns chunk carrying an arbitrary payload.
type chunk struct {
	typ     string
	payload []byte
}

// makeICNS assembles a valid icns container (magic + total length + chunks).
func makeICNS(chunks ...chunk) []byte {
	body := []byte{}
	for _, c := range chunks {
		hdr := make([]byte, 8)
		copy(hdr, c.typ)
		binary.BigEndian.PutUint32(hdr[4:], uint32(8+len(c.payload)))
		body = append(body, hdr...)
		body = append(body, c.payload...)
	}
	out := make([]byte, 8+len(body))
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(len(out)))
	copy(out[8:], body)
	return out
}

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

// dibHeader builds a 40-byte BITMAPINFOHEADER whose height is doubled (the icon
// XOR+AND convention debug/pe RT_ICON and .ico DIB entries follow).
func dibHeader(w, h, bits int) []byte {
	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:4], 40)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(int32(w)))
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(int32(2*h))) // doubled height
	binary.LittleEndian.PutUint16(hdr[12:14], 1)
	binary.LittleEndian.PutUint16(hdr[14:16], uint16(bits))
	return hdr
}

// andMask builds the 1-bpp AND transparency mask a well-formed BMP icon image
// carries after its colour data (bottom-up, MSB-first, rows padded to 4 bytes).
func andMask(w, h int) []byte {
	stride := ((w + 31) / 32) * 4
	return make([]byte, stride*h)
}

// dib32 builds a 32-bpp BGRA icon DIB (colour data + all-zero AND mask) filled
// with a per-pixel opaque colour.
func dib32(w, h int) []byte {
	px := make([]byte, w*4*h)
	for i := 0; i < w*h; i++ {
		px[i*4+0] = 0x10 // B
		px[i*4+1] = 0x20 // G
		px[i*4+2] = 0x30 // R
		px[i*4+3] = 0xFF // A (opaque)
	}
	return append(append(dibHeader(w, h, 32), px...), andMask(w, h)...)
}

// decodePNG decodes b as PNG or fails the test.
func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	return img
}

// assertSamePixels sweeps EVERY pixel (not a sample) and asserts the two images
// have identical bounds and identical straight-alpha colour at each coordinate.
func assertSamePixels(t *testing.T, got, want image.Image) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatalf("bounds = %v, want %v", got.Bounds(), want.Bounds())
	}
	b := want.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			gr, gg, gb, ga := got.At(x, y).RGBA()
			wr, wg, wb, wa := want.At(x, y).RGBA()
			if gr != wr || gg != wg || gb != wb || ga != wa {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got.At(x, y), want.At(x, y))
			}
		}
	}
}

// ---- the replaced .icns unpacker, reproduced verbatim for parity -----------

// icnsBestPNGReplaced is the hand-rolled .icns unpacker this migration DELETED
// (source/icns.go), copied here byte-for-byte so parity is proven against the
// code that was replaced, not against a variant of the new code. It walks the
// TLV chunks and returns the RAW bytes of the largest-width PNG representation,
// skipping non-PNG chunks.
func icnsBestPNGReplaced(data []byte) ([]byte, error) {
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(data) < 8 || string(data[:4]) != "icns" {
		return nil, errors.New("not icns")
	}
	total := int(binary.BigEndian.Uint32(data[4:8]))
	if total < 8 || total > len(data) {
		return nil, errors.New("size")
	}
	var best []byte
	bestW := -1
	for off := 8; off+8 <= total; {
		length := int(binary.BigEndian.Uint32(data[off+4 : off+8]))
		if length < 8 || off+length > total {
			return nil, errors.New("bad")
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
		return nil, errors.New("none")
	}
	return best, nil
}

// ---- parity + behaviour tests ----------------------------------------------

// The codec seam, at target 0 (= largest), must choose the SAME .icns
// representation the deleted hand-rolled unpacker chose — proven pixel-for-pixel
// against that unpacker reproduced verbatim. The raw ARGB chunk must be skipped.
func TestIconPNGICNSParityWithReplacedUnpacker(t *testing.T) {
	data := makeICNS(
		chunk{"ic07", pngOf(t, 64, 64)},
		chunk{"ic04", append([]byte("ARGB"), 1, 2, 3, 4)},
		chunk{"ic13", pngOf(t, 256, 256)},
	)
	rawRef, err := icnsBestPNGReplaced(data)
	if err != nil {
		t.Fatalf("replaced unpacker: %v", err)
	}
	want := decodePNG(t, rawRef) // the exact representation the old code returned
	if b := want.Bounds(); b.Dx() != 256 {
		t.Fatalf("replaced unpacker picked %dpx, want the 256px rep", b.Dx())
	}
	gotBytes, err := iconPNG(data, 0)
	if err != nil {
		t.Fatalf("iconPNG: %v", err)
	}
	assertSamePixels(t, decodePNG(t, gotBytes), want)
}

// The codec seam, at target 0 (= largest), must choose the SAME .ico image the
// replaced direct go-ico Decode chose — proven pixel-for-pixel against that
// reference call reproduced verbatim.
func TestIconPNGICOParityWithReplacedDecoder(t *testing.T) {
	data := buildICO(
		icoEntry{16, 16, dib32(16, 16)},
		icoEntry{32, 32, dib32(32, 32)},
	)
	want, err := ico.Decode(bytes.NewReader(data)) // the replaced reference call
	if err != nil {
		t.Fatalf("replaced decoder: %v", err)
	}
	if b := want.Bounds(); b.Dx() != 32 {
		t.Fatalf("replaced decoder picked %dpx, want the 32px rep", b.Dx())
	}
	gotBytes, err := iconPNG(data, 0)
	if err != nil {
		t.Fatalf("iconPNG: %v", err)
	}
	assertSamePixels(t, decodePNG(t, gotBytes), want)
}

// DecodeBest via the IconSize seam now selects the representation best matching
// the requested pixel size — the smallest at least that large, else the largest.
// Target 0 keeps the largest, reproducing the prior behaviour.
func TestIconPNGSizeTargeting(t *testing.T) {
	icns := makeICNS(
		chunk{"ic07", pngOf(t, 32, 32)},
		chunk{"ic12", pngOf(t, 128, 128)},
		chunk{"ic13", pngOf(t, 512, 512)},
	)
	for _, c := range []struct{ target, wantW int }{
		{0, 512},    // largest (parity default)
		{32, 32},    // exact
		{64, 128},   // smallest >= 64
		{128, 128},  // exact
		{129, 512},  // next up
		{4096, 512}, // none reaches -> largest
	} {
		b, err := iconPNG(icns, c.target)
		if err != nil {
			t.Fatalf("icns target %d: %v", c.target, err)
		}
		if w := decodePNG(t, b).Bounds().Dx(); w != c.wantW {
			t.Errorf("icns target %d -> width %d, want %d", c.target, w, c.wantW)
		}
	}

	icoData := buildICO(
		icoEntry{16, 16, pngOf(t, 16, 16)},
		icoEntry{64, 64, pngOf(t, 64, 64)},
		icoEntry{0, 0, pngOf(t, 256, 256)}, // 0 encodes 256
	)
	for _, c := range []struct{ target, wantW int }{
		{0, 256},    // largest
		{16, 16},    // exact
		{32, 64},    // smallest >= 32
		{64, 64},    // exact
		{65, 256},   // next up
		{9999, 256}, // none reaches -> largest
	} {
		b, err := iconPNG(icoData, c.target)
		if err != nil {
			t.Fatalf("ico target %d: %v", c.target, err)
		}
		if w := decodePNG(t, b).Bounds().Dx(); w != c.wantW {
			t.Errorf("ico target %d -> width %d, want %d", c.target, w, c.wantW)
		}
	}
}

// An unrecognised or corrupt container surfaces the decoder's error (which the
// callers degrade to "no icon").
func TestIconPNGDecodeError(t *testing.T) {
	if _, err := iconPNG([]byte("not an icon at all"), 0); err == nil {
		t.Fatal("want a decode error for unrecognised input")
	}
	if _, err := iconPNG(nil, 48); err == nil {
		t.Fatal("want a decode error for empty input")
	}
	// An icns with no PNG representation (only a raw ARGB chunk) fails to decode.
	if _, err := iconPNG(makeICNS(chunk{"ic04", []byte("ARGBxxxx")}), 0); err == nil {
		t.Fatal("want a decode error for a PNG-less icns")
	}
}

// The normally-unreachable png.Encode failure branch is forced through the seam.
func TestIconPNGEncodeError(t *testing.T) {
	orig := pngEncode
	pngEncode = func(w io.Writer, m image.Image) error { return errFakeEncode }
	defer func() { pngEncode = orig }()
	if _, err := iconPNG(makeICNS(chunk{"ic07", pngOf(t, 16, 16)}), 0); err != errFakeEncode {
		t.Fatalf("err = %v, want errFakeEncode", err)
	}
}

var errFakeEncode = errorString("forced encode failure")

type errorString string

func (e errorString) Error() string { return string(e) }
