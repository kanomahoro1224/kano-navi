package mediavariant

import (
	"image"
	"image/draw"
)

// contribution is one source pixel's share of a target pixel along one axis:
// the length of the source pixel inside the target pixel's span.
type contribution struct {
	index  int
	weight float32
}

// coverage maps each of dst target pixels to the source pixels its span of
// src/dst source pixels covers, with their overlap as weights. dst is at most
// src, so every weight is at most 1.
func coverage(src, dst int) ([][]contribution, []float32) {
	scale := float64(src) / float64(dst)
	spans := make([][]contribution, dst)
	totals := make([]float32, dst)
	for d := range dst {
		start, end := float64(d)*scale, float64(d+1)*scale
		for s := int(start); s < src && float64(s) < end; s++ {
			weight := min(end, float64(s+1)) - max(start, float64(s))
			if weight <= 0 {
				continue
			}
			spans[d] = append(spans[d], contribution{s, float32(weight)})
			totals[d] += float32(weight)
		}
	}
	return spans, totals
}

// areaScale scales source down to width, with the height of an original w×h
// image at that width, making each target pixel the average of the source
// area it covers (premultiplied, so transparency blends correctly). It holds
// the target, one converted source row and one row of sums, whatever the
// source's size, and never enlarges.
func areaScale(source image.Image, w, h, width int) *image.RGBA {
	bounds := source.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	height := max(1, int(int64(h)*int64(width)/int64(w)))
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	cols, colTotals := coverage(sw, width)
	rows, rowTotals := coverage(sh, height)
	row := image.NewRGBA(image.Rect(0, 0, sw, 1))
	fetched := -1
	sums := make([]float32, width*4)
	for y := range height {
		clear(sums)
		for _, r := range rows[y] {
			// A source row on a boundary serves two target rows in turn.
			if r.index != fetched {
				draw.Draw(row, row.Bounds(), source, image.Pt(bounds.Min.X, bounds.Min.Y+r.index), draw.Src)
				fetched = r.index
			}
			for x, span := range cols {
				var red, green, blue, alpha float32
				for _, c := range span {
					p := row.Pix[c.index*4 : c.index*4+4 : c.index*4+4]
					red += c.weight * float32(p[0])
					green += c.weight * float32(p[1])
					blue += c.weight * float32(p[2])
					alpha += c.weight * float32(p[3])
				}
				s := sums[x*4 : x*4+4 : x*4+4]
				s[0] += r.weight * red
				s[1] += r.weight * green
				s[2] += r.weight * blue
				s[3] += r.weight * alpha
			}
		}
		out := target.Pix[y*target.Stride : y*target.Stride+width*4]
		for x := range width {
			norm := 1 / (colTotals[x] * rowTotals[y])
			for c := range 4 {
				out[x*4+c] = uint8(min(255, sums[x*4+c]*norm+0.5))
			}
		}
	}
	return target
}
