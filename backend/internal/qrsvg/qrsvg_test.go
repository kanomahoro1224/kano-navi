package qrsvg

import (
	"strings"
	"testing"
)

func TestEncodeDrawsTheCodeInsideItsQuietZone(t *testing.T) {
	picture, err := Encode("otpauth://totp/Example:admin?secret=JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	// Version 1 is 21 modules; the quiet zone adds four on each side.
	if picture.Size < 29 || !strings.HasPrefix(picture.Path, "M") {
		t.Fatalf("picture=%d %q", picture.Size, picture.Path[:min(len(picture.Path), 40)])
	}
	if strings.Contains(picture.Path, "M0 ") || strings.Contains(picture.Path, "M3 ") {
		t.Fatalf("a module is drawn in the quiet zone: %q", picture.Path)
	}
}
