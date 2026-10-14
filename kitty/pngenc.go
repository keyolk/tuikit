package kitty

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	// Decoders for ToPNG: the PNG format (f=100) takes PNG bytes only, and
	// JPEG or GIF sent as f=100 are dropped without an error (ccproxy).
	_ "image/gif"
	_ "image/jpeg"
)

var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// IsPNG reports whether data starts with the PNG signature.
func IsPNG(data []byte) bool { return bytes.HasPrefix(data, pngMagic) }

// ToPNG returns data unchanged when it is PNG, else decodes it (PNG, JPEG,
// GIF and any decoder the app registered) and encodes it as PNG.
func ToPNG(data []byte) ([]byte, error) {
	if IsPNG(data) {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("kitty: decode image: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("kitty: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// gridMaxSide caps the long edge EncodeForGrid produces.
const gridMaxSide = 720

// EncodeForGrid shrinks img to about half the pixels of a cols×rows cell box
// (cell, or DefaultCellSize) and encodes it as PNG. The terminal stretches
// the image to the box anyway, so the picture looks the same, while a
// multi-megapixel screenshot drops from megabytes to tens of kilobytes:
// few enough chunks that a frame flush cannot land between them and tear
// the sequence (ccproxy). The long edge is capped at 720 pixels.
func EncodeForGrid(img image.Image, cols, rows int, cell CellSize) ([]byte, error) {
	cell = cell.OrDefault()
	maxW := min(max(cols*cell.W/2, 1), gridMaxSide)
	maxH := min(max(rows*cell.H/2, 1), gridMaxSide)
	var buf bytes.Buffer
	if err := png.Encode(&buf, Downscale(img, maxW, maxH)); err != nil {
		return nil, fmt.Errorf("kitty: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// Downscale returns img shrunk to fit maxW×maxH, keeping its aspect ratio,
// by averaging each destination pixel's source box. An image that already
// fits comes back as it is: scaling up only grows the payload.
func Downscale(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || maxW <= 0 || maxH <= 0 || (sw <= maxW && sh <= maxH) {
		return img
	}
	scale := math.Min(float64(maxW)/float64(sw), float64(maxH)/float64(sh))
	dw := max(1, int(math.Round(float64(sw)*scale)))
	dh := max(1, int(math.Round(float64(sh)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xr, yr := float64(sw)/float64(dw), float64(sh)/float64(dh)
	for dy := range dh {
		sy0 := b.Min.Y + int(float64(dy)*yr)
		sy1 := max(b.Min.Y+int(float64(dy+1)*yr), sy0+1)
		for dx := range dw {
			sx0 := b.Min.X + int(float64(dx)*xr)
			sx1 := max(b.Min.X+int(float64(dx+1)*xr), sx0+1)
			var rs, gs, bs, as, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bb, a := img.At(sx, sy).RGBA()
					rs, gs, bs, as = rs+uint64(r), gs+uint64(g), bs+uint64(bb), as+uint64(a)
					n++
				}
			}
			dst.Set(dx, dy, color.RGBA64{R: uint16(rs / n), G: uint16(gs / n), B: uint16(bs / n), A: uint16(as / n)})
		}
	}
	return dst
}
