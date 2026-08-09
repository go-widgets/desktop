//go:build ignore

// genicons draws the curated demo icon + thumbnail PNGs baked into the
// embeddedSource's embed.FS. Each icon is a rounded-ish tile: a solid hue with
// a lighter inner panel and a small accent bar, distinct per app so the browser
// desktop reads as a real, populated shell rather than uniform swatches. Run:
//
//	go run genicons.go <outdir>
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	out := "."
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	// name -> base hue (RGB)
	icons := map[string][3]uint8{
		"accessories-text-editor":  {0x3b, 0x82, 0xf6}, // blue
		"web-browser":              {0x22, 0xc5, 0x5e}, // green
		"system-file-manager":      {0xf5, 0x9e, 0x0b}, // amber
		"utilities-terminal":       {0x33, 0x37, 0x3d}, // near-black
		"accessories-calculator":   {0x8b, 0x5c, 0xf6}, // violet
		"image-viewer":             {0xec, 0x48, 0x99}, // pink
		"maps":                     {0x14, 0xb8, 0xa6}, // teal
		"media-player":             {0xef, 0x44, 0x44}, // red
		"folder":                   {0xf5, 0xb3, 0x41}, // folder gold
		"text-x-generic":           {0x94, 0xa3, 0xb8}, // slate
		"image-x-generic":          {0x60, 0xa5, 0xfa}, // light blue
		"application-x-executable": {0x6b, 0x72, 0x80}, // grey
		"application-pdf":          {0xdc, 0x26, 0x26}, // pdf red
	}
	for name, hue := range icons {
		writePNG(filepath.Join(out, name+".png"), iconTile(48, hue))
	}
	// Two "photo" thumbnails: a gradient wash so an image cell shows a real
	// preview instead of a flat icon.
	writePNG(filepath.Join(out, "thumb-photo1.png"), gradient(96, 72, [3]uint8{0x2d, 0x6c, 0xdf}, [3]uint8{0x8b, 0xd3, 0xff}))
	writePNG(filepath.Join(out, "thumb-photo2.png"), gradient(96, 72, [3]uint8{0x1f, 0x9d, 0x55}, [3]uint8{0xd7, 0xf7, 0xa8}))
}

func iconTile(size int, hue [3]uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	base := color.RGBA{hue[0], hue[1], hue[2], 0xff}
	inner := color.RGBA{clamp(int(hue[0]) + 60), clamp(int(hue[1]) + 60), clamp(int(hue[2]) + 60), 0xff}
	accent := color.RGBA{clamp(int(hue[0]) - 40), clamp(int(hue[1]) - 40), clamp(int(hue[2]) - 40), 0xff}
	m := size / 8
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := base
			// rounded-corner transparency
			if corner(x, y, size, m) {
				img.Set(x, y, color.RGBA{})
				continue
			}
			if x >= m && x < size-m && y >= m && y < size-m {
				c = inner
			}
			if y >= size-2*m && y < size-m && x >= m && x < size-m {
				c = accent
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func corner(x, y, size, m int) bool {
	cx, cy, r := 0, 0, m
	switch {
	case x < m && y < m:
		cx, cy = m, m
	case x >= size-m && y < m:
		cx, cy = size-m, m
	case x < m && y >= size-m:
		cx, cy = m, size-m
	case x >= size-m && y >= size-m:
		cx, cy = size-m, size-m
	default:
		return false
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy > r*r
}

func gradient(w, h int, a, b [3]uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		r := uint8(float64(a[0])*(1-t) + float64(b[0])*t)
		g := uint8(float64(a[1])*(1-t) + float64(b[1])*t)
		bl := uint8(float64(a[2])*(1-t) + float64(b[2])*t)
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{r, g, bl, 0xff})
		}
	}
	return img
}

func clamp(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
