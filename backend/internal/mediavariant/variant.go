// Package mediavariant makes smaller copies of cached images for the board.
// It never changes the original file; callers store the result in a derived
// cache and fall back to the original whenever a copy cannot be made.
package mediavariant

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"slices"

	"github.com/yexca/kano-navi/backend/internal/mediaformat"
)

// Widths are the only sizes the server makes. A fixed list keeps the cache
// bounded and lets browsers pick from srcset without inventing sizes.
var Widths = []int{320, 640, 960, 1280, 1920}

// ErrUnsupported means the original should be served instead: an animated or
// undecodable format, an image no wider than the request, or one too large
// to decode safely.
var ErrUnsupported = errors.New("variant unsupported")

const jpegQuality = 82

// ValidWidth reports whether width is one of Widths.
func ValidWidth(width int) bool { return slices.Contains(Widths, width) }

// Copy is one width's result: the copy and its type, or Err when the
// original is served for that width.
type Copy struct {
	Content []byte
	MIME    string
	Err     error
}

// ResizeEach scales content down to each of widths, given in ascending
// order, keeping its aspect ratio, and hands every width's result to emit as
// soon as it is encoded; emit returning false stops it. The original is
// decoded at most once and averaged down once, to the widest width it is
// wider than (see areaScale); every narrower width is averaged down from that
// copy, so a large original is scanned once instead of once per width. The
// last width is handed over after every other, so a caller storing the
// results in order can let it stand for all of them. Opaque images become
// JPEG and images with transparency become PNG. A copy is kept only when it
// is smaller than the original.
func ResizeEach(content []byte, widths []int, emit func(i int, copy Copy) bool) {
	last := len(widths) - 1
	if last < 0 {
		return
	}
	unsupported := func() {
		for i := range widths {
			if !emit(i, Copy{Err: ErrUnsupported}) {
				return
			}
		}
	}
	mime := mediaformat.Sniff(content)
	// GIF may be animated and AVIF has no decoder; both are served whole.
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		unsupported()
		return
	}
	w, h, ok := mediaformat.Dimensions(content)
	base := -1
	for i, width := range widths {
		if ValidWidth(width) && w > width {
			base = i
		}
	}
	if !ok || int64(w)*int64(h) > mediaformat.MaxPixels || base < 0 {
		unsupported()
		return
	}
	source, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		unsupported()
		return
	}
	// The decoded original is not used after this, so it can be collected
	// while the narrower widths are made.
	canvas := areaScale(source, w, h, widths[base])
	held := Copy{Err: ErrUnsupported}
	for i := last; i >= 0; i-- {
		result := Copy{Err: ErrUnsupported}
		switch {
		case i == base:
			result = encode(canvas, len(content))
		case i < base && ValidWidth(widths[i]):
			result = encode(areaScale(canvas, w, h, widths[i]), len(content))
		}
		if i == last {
			held = result
			continue
		}
		if !emit(i, result) {
			return
		}
	}
	emit(last, held)
}

// encode encodes target, failing when the result is not smaller than
// original bytes.
func encode(target *image.RGBA, original int) Copy {
	var out bytes.Buffer
	outMIME := "image/jpeg"
	var err error
	if opaque(target) {
		err = jpeg.Encode(&out, target, &jpeg.Options{Quality: jpegQuality})
	} else {
		outMIME = "image/png"
		err = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&out, target)
	}
	if err != nil || out.Len() == 0 || out.Len() >= original {
		return Copy{Err: ErrUnsupported}
	}
	return Copy{Content: out.Bytes(), MIME: outMIME}
}

func opaque(img *image.RGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0xff {
			return false
		}
	}
	return true
}
