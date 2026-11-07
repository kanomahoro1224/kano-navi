package variants

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/mediavariant"
	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
	"github.com/yexca/kano-navi/backend/internal/testutil"
)

func syntheticPhoto(t *testing.T, width, height int) []byte {
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

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func writeFiles(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// widths reads a status's per-width counts as copies/kept/missing triples.
func widths(status GroupStatus) [][3]int {
	out := [][3]int{}
	for _, width := range status.Widths {
		out = append(out, [3]int{width.Copies, width.Kept, width.Missing})
	}
	return out
}

func TestMakeFixedGroupsIntoTheOutputRootAndResume(t *testing.T) {
	data, out := t.TempDir(), t.TempDir()
	wide, small := syntheticPhoto(t, 1000, 500), syntheticPhoto(t, 200, 100)
	writeFiles(t, data, map[string][]byte{
		"lineblog/0123456789ab.jpg":  wide,
		"lineblog/abcdefabcdef.jpg":  small,
		"lineblog/ba9876543210.mp4":  []byte("\x00\x00\x00\x18ftypmp42synthetic"),
		"lineblog/.0123456789ab.jpg": wide,
		"lineblog/notes.txt":         []byte("synthetic"),
		// The same bytes as an archive image share its copies.
		"journey/fedcba987654.jpg": wide,
		"journey/readme.txt":       []byte("synthetic"),
		"journey/synthetic.jpg":    wide,
	})
	roots := Roots{Data: data, Out: out}
	ctx := context.Background()
	// Before any copy, the status reads no original: nothing is hashed yet.
	statuses, err := Status(ctx, nil, roots, Lineblog, Journey)
	if err != nil || len(statuses) != 2 || statuses[0].Originals != 2 || statuses[0].Unhashed != 2 || statuses[0].Ready != 0 || statuses[1].Originals != 1 {
		t.Fatalf("cold status %+v %v", statuses, err)
	}
	result, err := Make(ctx, nil, roots, Lineblog, Bounds{})
	if err != nil || result != (Result{Attempted: 2, Made: 1, Kept: 1}) {
		t.Fatalf("lineblog %+v %v", result, err)
	}
	if result, err = Make(ctx, nil, roots, Journey, Bounds{}); err != nil || result != (Result{Skipped: 1}) {
		t.Fatalf("journey %+v %v", result, err)
	}
	hash := hashOf(wide)
	for _, name := range []string{"-320.jpg", "-640.jpg", "-960.jpg", "-1280.orig", "-1920.orig"} {
		if _, err = os.Stat(filepath.Join(out, "cache", "variants", hash[:2], hash+name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	// Copies go to the output root only; the hashes read are remembered
	// under the data directory.
	if entries, _ := os.ReadDir(filepath.Join(out, "cache")); len(entries) != 1 || entries[0].Name() != "variants" {
		t.Fatalf("output cache holds %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Join(data, "cache")); len(entries) != 1 || entries[0].Name() != "file-hashes" {
		t.Fatalf("data cache holds %v", entries)
	}
	// A second call resumes: every image is done.
	if result, err = Make(ctx, nil, roots, Lineblog, Bounds{}); err != nil || result != (Result{Skipped: 2}) {
		t.Fatalf("second run %+v %v", result, err)
	}
	statuses, err = Status(ctx, nil, roots, Lineblog, Journey)
	if err != nil {
		t.Fatal(err)
	}
	if got := widths(statuses[0]); statuses[0].Unhashed != 0 || statuses[0].Ready != 2 || got[0] != [3]int{1, 1, 0} || got[3] != [3]int{0, 2, 0} {
		t.Fatalf("lineblog status %+v", statuses[0])
	}
	if got := widths(statuses[1]); statuses[1].Ready != 1 || got[2] != [3]int{1, 0, 0} {
		t.Fatalf("journey status %+v", statuses[1])
	}
}

func TestMakeWorksCoversOnlyFromTheirArchiveNames(t *testing.T) {
	data := t.TempDir()
	cover := syntheticPhoto(t, 1000, 1000)
	writeFiles(t, data, map[string][]byte{
		"works/0123456789ab.jpg":  cover,
		"works/abcdefabcdef.png":  syntheticPhoto(t, 200, 200),
		"works/ba9876543210.mp4":  []byte("\x00\x00\x00\x18ftypmp42synthetic"),
		"works/.0123456789ab.jpg": cover,
		"works/ABCDEFABCDEF.jpg":  cover,
		"works/notes.txt":         []byte("synthetic"),
		// A LINE BLOG image belongs to its own group.
		"lineblog/fedcbafedcba.jpg": syntheticPhoto(t, 900, 300),
	})
	roots := Roots{Data: data}
	ctx := context.Background()
	statuses, err := Status(ctx, nil, roots, Works)
	if err != nil || len(statuses) != 1 || statuses[0].Group != Works || statuses[0].Originals != 2 || statuses[0].Unhashed != 2 {
		t.Fatalf("cold status %+v %v", statuses, err)
	}
	result, err := Make(ctx, nil, roots, Works, Bounds{})
	if err != nil || result != (Result{Attempted: 2, Made: 1, Kept: 1}) {
		t.Fatalf("works %+v %v", result, err)
	}
	if !sqlite.VariantsDone(data, hashOf(cover)) {
		t.Fatal("cover copies not in the data cache")
	}
	if result, err = Make(ctx, nil, roots, Works, Bounds{}); err != nil || result != (Result{Skipped: 2}) {
		t.Fatalf("second run %+v %v", result, err)
	}
	if statuses, err = Status(ctx, nil, roots, Works); err != nil || statuses[0].Ready != 2 || statuses[0].Unhashed != 0 {
		t.Fatalf("status %+v %v", statuses, err)
	}
	// The pre-generated groups include the covers.
	if !slices.Contains(Groups, Works) {
		t.Fatalf("groups %v", Groups)
	}
}

func TestMakeStopsAtItsBoundsAndCountsTheRest(t *testing.T) {
	data := t.TempDir()
	files := map[string][]byte{}
	for i, name := range []string{"000000000001.jpg", "000000000002.jpg", "000000000003.jpg"} {
		files["lineblog/"+name] = syntheticPhoto(t, 700+i, 300)
	}
	writeFiles(t, data, files)
	roots := Roots{Data: data}
	ctx := context.Background()
	result, err := Make(ctx, nil, roots, Lineblog, Bounds{Files: 2})
	if err != nil || result.Attempted != 2 || result.Made != 2 || result.Remaining != 1 || result.Stopped != StoppedFileCap {
		t.Fatalf("file cap %+v %v", result, err)
	}
	// A clock past the budget starts nothing.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ticks := 0
	clock := func() time.Time {
		ticks++
		return start.Add(time.Duration(ticks) * time.Hour)
	}
	result, err = Make(ctx, nil, roots, Lineblog, Bounds{Budget: time.Minute, Now: clock})
	if err != nil || result.Attempted != 0 || result.Remaining != 3 || result.Stopped != StoppedTimeBudget {
		t.Fatalf("budget %+v %v", result, err)
	}
	// The next call finishes the image the cap left.
	result, err = Make(ctx, nil, roots, Lineblog, Bounds{})
	if err != nil || result != (Result{Attempted: 1, Made: 1, Skipped: 2}) {
		t.Fatalf("resume %+v %v", result, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = Make(cancelled, nil, Roots{Data: t.TempDir()}, Lineblog, Bounds{}); err == nil {
		t.Fatal("cancelled call reported success")
	}
}

func TestMakeMediaUsesTheReadyRowsAndReportsUnreadableFiles(t *testing.T) {
	path := testutil.NewSnapshot(t)
	store, err := sqlite.OpenWorkflowCore(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	contents := [][]byte{syntheticPhoto(t, 900, 300), syntheticPhoto(t, 120, 60)}
	for i, content := range contents {
		sourceURL := "https://pbs.twimg.com/media/synthetic-" + string(rune('a'+i)) + ".jpg"
		id := hashOf([]byte(sourceURL))
		testutil.Exec(t, path, `INSERT INTO media_assets(id,source,source_url,status,created_at,updated_at) VALUES(?,'x',?,'pending','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`, id, sourceURL)
		if err = store.SaveMediaDownload(ctx, sqlite.PendingMedia{ID: id, Source: "x", SourceURL: sourceURL}, content, "image/jpeg", "", "", "ready", ""); err != nil {
			t.Fatal(err)
		}
	}
	// One file replaced on disk no longer matches its row.
	broken := filepath.Join(store.DataRoot(), "x", "sha256", hashOf(contents[1])[:2], hashOf(contents[1])+".jpg")
	if err = os.WriteFile(broken, []byte(strings.Repeat("x", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Make(ctx, store, Roots{}, Media, Bounds{})
	if err != nil || result != (Result{Attempted: 2, Made: 1, Failed: 1}) {
		t.Fatalf("media %+v %v", result, err)
	}
	if !sqlite.VariantsDone(store.DataRoot(), hashOf(contents[0])) {
		t.Fatal("copies not in the store's cache")
	}
	statuses, err := Status(ctx, store, Roots{}, Media)
	if err != nil || statuses[0].Originals != 2 || statuses[0].Unhashed != 0 || widths(statuses[0])[0] != [3]int{1, 0, 1} {
		t.Fatalf("media status %+v %v", statuses, err)
	}
	if len(statuses[0].Widths) != len(mediavariant.Widths) {
		t.Fatalf("widths %+v", statuses[0].Widths)
	}
}
