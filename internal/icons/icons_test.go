package icons

import (
	"bytes"
	"image/png"
	"testing"
)

func TestPNG(t *testing.T) {
	for _, level := range []int{0, 20, 50, 100} {
		lv := level
		raw := PNG(&lv, false, 20)
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%d%%: %v", level, err)
		}
		b := img.Bounds()
		if b.Dx() != 64 || b.Dy() != 64 {
			t.Fatalf("%d%%: size %dx%d", level, b.Dx(), b.Dy())
		}
		opaque := 0
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
					opaque++
				}
			}
		}
		if opaque == 0 {
			t.Fatalf("%d%%: fully transparent icon", level)
		}
	}
	if len(PNG(nil, false, 20)) == 0 {
		t.Fatal("nil level should still render")
	}
}
