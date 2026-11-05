package mediaformat

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func encoded(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSniffAndDimensionsReadOnlyTheHeader(t *testing.T) {
	for name, content := range map[string][]byte{
		"image/png":  encoded(t, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) }),
		"image/jpeg": encoded(t, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) }),
		"image/gif":  encoded(t, func(b *bytes.Buffer, i image.Image) error { return gif.Encode(b, i, nil) }),
	} {
		if got := Sniff(content); got != name {
			t.Errorf("Sniff = %q, want %q", got, name)
		}
		if w, h, ok := Dimensions(content); !ok || w != 3 || h != 2 {
			t.Errorf("%s: Dimensions = %d, %d, %v", name, w, h, ok)
		}
	}
	avif := []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00")
	if Sniff(avif) != "image/avif" {
		t.Fatal("avif not sniffed")
	}
	if _, _, ok := Dimensions(avif); ok {
		t.Fatal("AVIF has no header decoder here")
	}
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	if Sniff(webp) != "image/webp" {
		t.Fatal("webp not sniffed")
	}
}

func TestDimensionsRejectsMismatchedOrUnknownContent(t *testing.T) {
	pngBytes := encoded(t, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) })
	for name, content := range map[string][]byte{
		"empty":     nil,
		"text":      []byte("not an image"),
		"truncated": pngBytes[:12],
		// A GIF signature in front of PNG data disagrees with its header.
		"mismatched": append([]byte("GIF89a"), pngBytes[6:]...),
	} {
		if _, _, ok := Dimensions(content); ok {
			t.Errorf("%s: Dimensions accepted it", name)
		}
	}
	for mime, ext := range map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp", "image/avif": "avif", "text/html": ""} {
		if got := Extension(mime); got != ext {
			t.Errorf("Extension(%s) = %q, want %q", mime, got, ext)
		}
	}
}
