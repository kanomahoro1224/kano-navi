package workflow

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
	"github.com/yexca/kano-navi/backend/internal/variants"
)

func syntheticJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8((x ^ y) * 3), A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestVariantsStepMakesEveryGroupAndResumes(t *testing.T) {
	p := testutil.NewSnapshot(t)
	s, err := sqlite.OpenWorkflowCore(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	data, static := t.TempDir(), t.TempDir()
	files := map[string][]byte{
		filepath.Join(data, "lineblog", "0123456789ab.jpg"):         syntheticJPEG(t, 1000, 500),
		filepath.Join(data, "lineblog", "abcdefabcdef.jpg"):         syntheticJPEG(t, 200, 100),
		filepath.Join(data, "works", "fedcba987654.jpg"):            syntheticJPEG(t, 800, 800),
		filepath.Join(static, "assets", "history", "synthetic.jpg"): syntheticJPEG(t, 700, 400),
	}
	for path, content := range files {
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	steps := &SourceSteps{Store: s, Variants: &variants.Roots{Data: data, Static: static}}
	out, err := steps.Execute(context.Background(), StepInput{StepID: "variants"})
	if err != nil || out.Status != "succeeded" || out.Result["made"] != 3 || out.Result["kept"] != 1 || out.Result["remaining"] != 0 || out.Result["stopped"] != nil {
		t.Fatalf("first out=%#v err=%v", out, err)
	}
	// The next run loads nothing: every image is done.
	out, err = steps.Execute(context.Background(), StepInput{StepID: "variants"})
	if err != nil || out.Status != "succeeded" || out.Result["attempted"] != 0 || out.Result["skipped"] != 4 {
		t.Fatalf("second out=%#v err=%v", out, err)
	}
	// Without the roots the step is not available.
	if _, err = (&SourceSteps{Store: s}).Execute(context.Background(), StepInput{StepID: "variants"}); err == nil {
		t.Fatal("variants step ran without roots")
	}
}
