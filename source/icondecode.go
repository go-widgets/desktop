// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package source

import (
	"bytes"
	"image/png"

	"github.com/go-gfx/gfx/codec"
)

// Icon decoding for the desktop shell's icon seam. An app icon reaches the shell
// as an Apple .icns container, a Windows .ico container, or (via peicon) an .ico
// reassembled from a PE module's RT_ICON resources — each holding several size
// representations, of which the shell needs a single one to blit.
//
// Decoding is delegated to the fleet's unified decode registry
// github.com/go-gfx/gfx/codec (CGO-free, BSD-3-Clause). codec sniffs the
// container, demuxes it with a bounds-checked TLV walk (.icns) or the reference
// github.com/sergeymakinen/go-ico codec (.ico), and decodes each representation
// with the standard-library image decoders. This REPLACES a hand-rolled .icns
// TLV unpacker and a direct go-ico call: codec subsumes both behind one entry
// point, so the shell decodes every icon container the same way and reimplements
// no decoder.
//
// codec.DecodeBest picks the representation best matching a target pixel size —
// the smallest at least as large as the target, or the largest available when
// none reaches it — which is exactly the shell's "an icon at the display size"
// need. A target of 0 selects the largest representation, reproducing the prior
// hand-rolled behaviour that always kept the largest.

// pngEncode is a seam over png.Encode so the (otherwise unreachable, since
// bytes.Buffer never fails) encode-error path can be exercised deterministically.
var pngEncode = png.Encode

// iconPNG decodes an icon container (icns, ico, or any single-image format codec
// recognises), selects the representation best matching targetPx via
// codec.DecodeBest, and returns it re-encoded as PNG bytes for the shell's
// []byte icon seam. targetPx <= 0 selects the largest representation. It returns
// the decoder's error for an unrecognised or corrupt container (which the
// callers degrade to "no icon").
func iconPNG(data []byte, targetPx int) ([]byte, error) {
	img, err := codec.DecodeBest(data, targetPx)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pngEncode(&buf, img.ToNRGBA()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
