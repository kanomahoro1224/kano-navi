package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"
)

func syntheticImage(t *testing.T, mime string, width, height int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			a := uint8(0xff)
			if alpha && x < width/2 {
				a = 0x40
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8((x ^ y) * 3), A: a})
		}
	}
	var out bytes.Buffer
	var err error
	switch mime {
	case "image/jpeg":
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 100})
	case "image/png":
		err = png.Encode(&out, img)
	case "image/gif":
		err = gif.Encode(&out, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func readyAsset(t *testing.T, s *Store, sourceURL string, content []byte, mime string) string {
	t.Helper()
	id, _, _ := mediaIdentity(sourceURL)
	if _, err := s.db.Exec(`INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending','','')`, id, sourceURL); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMediaDownload(context.Background(), PendingMedia{ID: id, Source: "x", SourceURL: sourceURL}, content, mime, "", "", "ready", ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func openVariantStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStandalone(filepath.Join(t.TempDir(), "database", "snapshot.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveMediaDownloadProbesDimensions(t *testing.T) {
	s := openVariantStore(t)
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-dims.png", syntheticImage(t, "image/png", 30, 20, false), "image/png")
	var width, height sql.NullInt64
	if err := s.db.QueryRow(`SELECT width,height FROM media_assets WHERE id=?`, id).Scan(&width, &height); err != nil || width.Int64 != 30 || height.Int64 != 20 {
		t.Fatalf("dimensions not probed: %v %v %v", width, height, err)
	}
	// An unreadable header is stored as 0×0 and reads as unknown.
	bad := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-bad.png", []byte("\x89PNG\r\n\x1a\nsynthetic"), "image/png")
	if err := s.db.QueryRow(`SELECT width,height FROM media_assets WHERE id=?`, bad).Scan(&width, &height); err != nil || width.Int64 != 0 || height.Int64 != 0 {
		t.Fatalf("unreadable header stored %v %v %v", width, height, err)
	}
	reference, err := s.resolveMedia(context.Background(), strPtr("https://pbs.twimg.com/media/synthetic-bad.png"))
	if err != nil || reference.Width != nil || reference.Height != nil {
		t.Fatalf("0×0 exposed as a dimension: %#v %v", reference, err)
	}
}

func TestMeasureMediaDimensionsBackfillsOlderFiles(t *testing.T) {
	s := openVariantStore(t)
	ctx := context.Background()
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-old.jpg", syntheticImage(t, "image/jpeg", 40, 10, false), "image/jpeg")
	if _, err := s.db.Exec(`UPDATE media_assets SET width=NULL,height=NULL WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	before, _ := s.GetDashboardRevision(ctx)
	measured, err := s.MeasureMediaDimensions(ctx, 10)
	if err != nil || measured != 1 {
		t.Fatalf("measured %d %v", measured, err)
	}
	var width, height int64
	if err = s.db.QueryRow(`SELECT width,height FROM media_assets WHERE id=?`, id).Scan(&width, &height); err != nil || width != 40 || height != 10 {
		t.Fatalf("backfill wrote %d×%d %v", width, height, err)
	}
	if after, _ := s.GetDashboardRevision(ctx); after == before {
		t.Fatal("backfill did not advance the revision")
	}
	if measured, err = s.MeasureMediaDimensions(ctx, 10); err != nil || measured != 0 {
		t.Fatalf("second backfill measured %d %v", measured, err)
	}
}
