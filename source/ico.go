// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"image/png"

	ico "github.com/sergeymakinen/go-ico"
)

// Windows .ico decoding for the desktop shell's icon seam. An .ico is a
// container (an ICONDIR of ICONDIRENTRY records) whose entries hold either a
// PNG (Vista+ large icons) or a bottom-up BMP DIB (a BITMAPINFOHEADER, a colour
// XOR bitmap and a 1-bpp AND transparency mask). The shell only needs the
// single best (largest) representation, decoded to something it can blit.
//
// Decoding is delegated to the reference pure-Go ICO codec
// github.com/sergeymakinen/go-ico (CGO-free, BSD-3-Clause), whose Decode returns
// the container's largest icon as an image.Image — decoding PNG entries and BMP
// DIB entries (32-bpp BGRA and 24-bpp BGR + AND mask) alike. This replaces a
// hand-rolled ICONDIR walker + DIB decoder: a reference library is
// battle-tested and maintained, and a plain Go-module dependency is not
// vendoring. The peicon PE-resource extractor reuses this same seam to decode
// the .ico it assembles from a module's RT_ICON resources.

// pngEncode is a seam over png.Encode so the (otherwise unreachable, since
// bytes.Buffer never fails) encode-error path can be exercised deterministically.
var pngEncode = png.Encode

// icoBestPNG decodes the largest image in an .ico container and returns it as
// encoded PNG bytes. It returns an error when the input is not a decodable icon
// (the reference decoder reports a FormatError / UnsupportedError / io error),
// which the caller degrades to "no icon".
func icoBestPNG(data []byte) ([]byte, error) {
	img, err := ico.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pngEncode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
