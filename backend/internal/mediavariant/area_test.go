package mediavariant

import (
	"image"
	"image/color"
	"testing"
)

func TestAreaScaleAveragesTheCoveredSourceArea(t *testing.T) {
	// One-pixel black and white stripes average to mid grey at any ratio
	// that covers whole pairs, and a uniform colour stays itself.
	stripes := image.NewGray(image.Rect(0, 0, 1000, 300))
	for y := range 300 {
		for x := range 1000 {
			if x%2 == 0 {
				stripes.SetGray(x, y, color.Gray{Y: 0xff})
			}
		}
	}
	out := areaScale(stripes, 1000, 300, 250)
	if out.Bounds().Size() != (image.Point{250, 75}) {
		t.Fatalf("size %v", out.Bounds().Size())
	}
	for _, at := range []image.Point{{0, 0}, {124, 37}, {249, 74}} {
		if got := out.RGBAAt(at.X, at.Y); got != (color.RGBA{0x80, 0x80, 0x80, 0xff}) {
			t.Fatalf("stripes at %v: %v", at, got)
		}
	}
	// A fractional ratio still weighs every source pixel by its overlap.
	uniform := image.NewNRGBA(image.Rect(0, 0, 777, 333))
	for i := 0; i < len(uniform.Pix); i += 4 {
		copy(uniform.Pix[i:], []byte{0x20, 0x90, 0xd0, 0xff})
	}
	out = areaScale(uniform, 777, 333, 320)
	for _, at := range []image.Point{{0, 0}, {161, 70}, {319, 136}} {
		if got := out.RGBAAt(at.X, at.Y); got != (color.RGBA{0x20, 0x90, 0xd0, 0xff}) {
			t.Fatalf("uniform at %v: %v", at, got)
		}
	}
}

func TestAreaScaleBlendsTransparencyPremultiplied(t *testing.T) {
	// Opaque red beside transparent pixels averages to half-transparent red,
	// not to a darkened colour.
	half := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	for y := range 100 {
		for x := range 400 {
			if x%2 == 0 {
				half.SetNRGBA(x, y, color.NRGBA{R: 0xff, A: 0xff})
			}
		}
	}
	got := areaScale(half, 400, 100, 200).RGBAAt(10, 10)
	if got != (color.RGBA{R: 0x80, A: 0x80}) {
		t.Fatalf("premultiplied %v", got)
	}
}
