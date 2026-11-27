// Package qrsvg draws a QR code as one SVG path, which the console shows on
// white in either theme: the Bilibili QR sign-in and the admin one-time code
// enrollment.
package qrsvg

import (
	"strconv"
	"strings"

	"rsc.io/qr"
)

// Picture is a QR code as one SVG path over a square of Size modules, the
// quiet zone included.
type Picture struct {
	Size int    `json:"size"`
	Path string `json:"path"`
}

// quiet is the quiet zone around a code, in modules.
const quiet = 4

// Encode draws text at error correction level M.
func Encode(text string) (*Picture, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, err
	}
	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; {
			if !code.Black(x, y) {
				x++
				continue
			}
			start := x
			for x < code.Size && code.Black(x, y) {
				x++
			}
			run := strconv.Itoa(x - start)
			path.WriteString("M" + strconv.Itoa(start+quiet) + " " + strconv.Itoa(y+quiet) + "h" + run + "v1h-" + run + "z")
		}
	}
	return &Picture{Size: code.Size + 2*quiet, Path: path.String()}, nil
}
