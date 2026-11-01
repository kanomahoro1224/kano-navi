package mediavariant

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"testing"
)

func syntheticImage(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8((x ^ y) * 3), A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// gradientPNG is a half-transparent smooth gradient, whose copies compress
// better than the original.
func gradientPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / width), G: uint8(y * 255 / height), B: 0x80, A: 0x80})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestResizeEachHandsTheLastWidthOverLast(t *testing.T) {
	original := syntheticImage(t, 1000, 500)
	order := []int{}
	sizes := map[int]image.Point{}
	ResizeEach(original, Widths, func(i int, copy Copy) bool {
		order = append(order, Widths[i])
		if copy.Err == nil {
			if copy.MIME != "image/jpeg" || len(copy.Content) >= len(original) {
				t.Fatalf("%d: %s %d bytes", Widths[i], copy.MIME, len(copy.Content))
			}
			decoded, err := jpeg.Decode(bytes.NewReader(copy.Content))
			if err != nil {
				t.Fatal(err)
			}
			sizes[Widths[i]] = decoded.Bounds().Size()
		}
		return true
	})
	// The widest width the original is wider than comes first among the
	// copies, narrower ones follow, and the last width closes the call.
	if want := []int{1280, 960, 640, 320, 1920}; !slices.Equal(order, want) {
		t.Fatalf("order %v, want %v", order, want)
	}
	want := map[int]image.Point{960: {960, 480}, 640: {640, 320}, 320: {320, 160}}
	for width, size := range want {
		if sizes[width] != size {
			t.Fatalf("%d: %v", width, sizes[width])
		}
	}
	if len(sizes) != len(want) {
		t.Fatalf("copies %v", sizes)
	}
}

func TestResizeEachStopsWhenEmitRefusesAndKeepsTransparency(t *testing.T) {
	original := gradientPNG(t, 1400, 600)
	seen := []int{}
	ResizeEach(original, Widths, func(i int, copy Copy) bool {
		seen = append(seen, Widths[i])
		if Widths[i] == 640 && (copy.Err != nil || copy.MIME != "image/png") {
			t.Fatalf("640: %+v", copy.Err)
		}
		return Widths[i] != 640
	})
	// 1920 is held for last, so stopping at 640 never reaches it.
	if want := []int{1280, 960, 640}; !slices.Equal(seen, want) {
		t.Fatalf("seen %v, want %v", seen, want)
	}
	// An unsupported original hands every width over as served whole.
	count := 0
	ResizeEach([]byte("GIF89a synthetic"), Widths, func(_ int, copy Copy) bool {
		if copy.Err == nil {
			t.Fatal("copy of an unsupported original")
		}
		count++
		return true
	})
	if count != len(Widths) {
		t.Fatalf("emitted %d", count)
	}
}
