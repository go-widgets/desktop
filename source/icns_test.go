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
	"testing"
)

// pngOf encodes a solid w×h RGBA PNG (deterministic test icon bytes).
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

func TestICNSBestPNG(t *testing.T) {
	small := pngOf(t, 32, 32)
	large := pngOf(t, 256, 256)
	// A real-world mix: an ARGB (raw) chunk and two PNG chunks of different
	// sizes; the largest PNG must win and the ARGB chunk must be skipped.
	data := makeICNS(
		chunk{"ic07", small},
		chunk{"ic04", append([]byte("ARGB"), 1, 2, 3, 4)},
		chunk{"ic13", large},
	)
	got, err := icnsBestPNG(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(got, large) {
		t.Fatalf("picked wrong representation (len %d, want %d)", len(got), len(large))
	}
	// The returned slice must be an independent copy, not aliasing the input.
	got[0] ^= 0xff
	if data[0] == got[0] {
		t.Errorf("returned slice aliases input")
	}
}

func TestICNSErrors(t *testing.T) {
	small := pngOf(t, 16, 16)
	valid := makeICNS(chunk{"ic07", small})

	// not an icns container.
	if _, err := icnsBestPNG([]byte("nope")); err != errNotICNS {
		t.Errorf("not icns: %v", err)
	}
	if _, err := icnsBestPNG(nil); err != errNotICNS {
		t.Errorf("nil: %v", err)
	}

	// declared total length larger than the buffer.
	tooBig := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(tooBig[4:], uint32(len(tooBig)+100))
	if _, err := icnsBestPNG(tooBig); err != errICNSSize {
		t.Errorf("oversize: %v", err)
	}
	// declared total length below the 8-byte header.
	tiny := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(tiny[4:], 4)
	if _, err := icnsBestPNG(tiny); err != errICNSSize {
		t.Errorf("undersize: %v", err)
	}

	// a chunk whose declared length is below its 8-byte header.
	corrupt := makeICNS(chunk{"ic07", small})
	binary.BigEndian.PutUint32(corrupt[12:], 4) // first chunk length = 4
	if _, err := icnsBestPNG(corrupt); err != errICNSBad {
		t.Errorf("corrupt chunk: %v", err)
	}

	// no PNG representation at all (only a raw ARGB chunk).
	noPNG := makeICNS(chunk{"ic04", []byte("ARGBxxxx")})
	if _, err := icnsBestPNG(noPNG); err != errICNSNone {
		t.Errorf("no png: %v", err)
	}

	// a chunk with the PNG signature but a corrupt body: DecodeConfig fails, so
	// it is skipped and, being the only chunk, the container yields no PNG.
	fakePNG := append(append([]byte(nil), pngMagic...), 0, 0, 0)
	if _, err := icnsBestPNG(makeICNS(chunk{"ic07", fakePNG})); err != errICNSNone {
		t.Errorf("fake png: %v", err)
	}
}
