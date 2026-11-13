// Package icons renders the circular tray icon (a port of halo/icons.py).
package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

var (
	red   = color.NRGBA{232, 17, 35, 255}
	amber = color.NRGBA{255, 185, 0, 255}
	green = color.NRGBA{16, 196, 80, 255}
	gray  = color.NRGBA{160, 160, 160, 255}
)

func ringColour(level *int, charging bool, low int) color.NRGBA {
	if level == nil {
		return gray
	}
	if charging || *level > low+20 {
		return green
	}
	if *level > low {
		return amber
	}
	return red
}

// PNG draws a 64x64 ring filled clockwise to the battery level.
// Supersampled 4x on the fly, like the Python original.
func PNG(level *int, charging bool, low int) []byte {
	const size = 64
	const super = 4
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	fill := ringColour(level, charging, low)
	frac := 0.0
	if level != nil {
		frac = float64(*level) / 100
		if frac > 1 {
			frac = 1
		}
		// an empty ring would make the tray icon vanish at 0%
		if frac < 0.03 {
			frac = 0.03
		}
	}
	const cx, cy = size / 2.0, size / 2.0
	const outer, inner = 30.0, 23.0

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sy := 0; sy < super; sy++ {
				for sx := 0; sx < super; sx++ {
					px := float64(x) + (float64(sx)+0.5)/super
					py := float64(y) + (float64(sy)+0.5)/super
					dx, dy := px-cx, py-cy
					dist := math.Hypot(dx, dy)
					if dist < inner || dist > outer {
						continue
					}
					angle := math.Atan2(dy, dx) + math.Pi/2 // 0 at 12 o'clock
					if angle < 0 {
						angle += 2 * math.Pi
					}
					if angle > frac*2*math.Pi && frac < 1 {
						continue
					}
					r += uint32(fill.R)
					g += uint32(fill.G)
					b += uint32(fill.B)
					a++
				}
			}
			if a == 0 {
				continue
			}
			n := uint32(super * super)
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(b / a),
				A: uint8(255 * a / n),
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
