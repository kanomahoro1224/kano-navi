package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"

	_ "image/png"
)

// noisyJPEG is a synthetic photo-like image large enough to shrink.
func noisyJPEG(t *testing.T, width, height int) []byte {
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

func decodedWidth(t *testing.T, content []byte) int {
	t.Helper()
	config, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("variant does not decode: %v", err)
	}
	return config.Width
}

func TestLineblogMediaServesSmallerCopies(t *testing.T) {
	store, dir := lineblogStore(t)
	original := noisyJPEG(t, 1000, 500)
	small := noisyJPEG(t, 200, 100)
	video := append([]byte("\x00\x00\x00\x18ftypmp42"), []byte(strings.Repeat("synthetic", 20))...)
	for name, content := range map[string][]byte{"0123456789ab.jpg": original, "abcdefabcdef.jpg": small, "ba9876543210.mp4": video} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	makeCopies(t, store, original)
	makeCopies(t, store, small)
	handler := NewHandler(store)

	copy := request(t, handler, http.MethodGet, "/media/lineblog/0123456789ab.jpg?w=320")
	if copy.Code != http.StatusOK || copy.Body.Len() >= len(original) || decodedWidth(t, copy.Body.Bytes()) != 320 {
		t.Fatalf("copy status=%d length=%d", copy.Code, copy.Body.Len())
	}
	headers := copy.Header()
	if headers.Get("Content-Type") != "image/jpeg" || headers.Get("X-Content-Type-Options") != "nosniff" ||
		headers.Get("Content-Security-Policy") != "default-src 'none'; sandbox" || headers.Get("ETag") != `"`+sha256Of(original)+`-320"` ||
		headers.Get("Cache-Control") != mediaCacheControl(false) {
		t.Fatalf("copy headers=%v", headers)
	}
	cached, _ := filepath.Glob(filepath.Join(store.DataRoot(), "cache", "variants", "*", "*-320.jpg"))
	if len(cached) != 1 {
		t.Fatalf("copy not cached: %v", cached)
	}
	again := request(t, handler, http.MethodGet, "/media/lineblog/0123456789ab.jpg?w=320")
	if !bytes.Equal(again.Body.Bytes(), copy.Body.Bytes()) || again.Header().Get("ETag") != headers.Get("ETag") {
		t.Fatal("cached copy differs")
	}
	fresh := httptest.NewRecorder()
	revalidate := httptest.NewRequest(http.MethodGet, "/media/lineblog/0123456789ab.jpg?w=320", nil)
	revalidate.Header.Set("If-None-Match", headers.Get("ETag"))
	handler.ServeHTTP(fresh, revalidate)
	if fresh.Code != http.StatusNotModified {
		t.Fatalf("revalidation status=%d", fresh.Code)
	}
	if head := request(t, handler, http.MethodHead, "/media/lineblog/0123456789ab.jpg?w=320"); head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD status=%d length=%d", head.Code, head.Body.Len())
	}

	// No width, an image no wider than the width and a video get the original.
	for path, want := range map[string][]byte{
		"/media/lineblog/0123456789ab.jpg":       original,
		"/media/lineblog/abcdefabcdef.jpg?w=320": small,
		"/media/lineblog/ba9876543210.mp4?w=320": video,
	} {
		recorder := request(t, handler, http.MethodGet, path)
		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), want) {
			t.Fatalf("%s status=%d length=%d", path, recorder.Code, recorder.Body.Len())
		}
	}
	for _, path := range []string{
		"/media/lineblog/0123456789ab.jpg?w=333",
		"/media/lineblog/0123456789ab.jpg?w=",
		"/media/lineblog/0123456789ab.jpg?w=320&w=640",
		"/media/lineblog/ba9876543210.mp4?w=abc",
		"/media/lineblog/cccccccccccc.jpg?w=320",
		"/media/lineblog/../0123456789ab.jpg?w=320",
	} {
		if recorder := request(t, handler, http.MethodGet, path); recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d", path, recorder.Code)
		}
	}
}

func TestLineblogMediaRereadsAReplacedFile(t *testing.T) {
	store, dir := lineblogStore(t)
	path := filepath.Join(dir, "0123456789ab.jpg")
	original, replaced := noisyJPEG(t, 1000, 500), noisyJPEG(t, 1400, 300)
	makeCopies(t, store, original)
	makeCopies(t, store, replaced)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store)
	first := request(t, handler, http.MethodGet, "/media/lineblog/0123456789ab.jpg?w=640")
	if err := os.WriteFile(path, replaced, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second := request(t, handler, http.MethodGet, "/media/lineblog/0123456789ab.jpg?w=640")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatalf("replaced file served its old copy: %d %d", first.Code, second.Code)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(second.Body.Bytes()))
	if err != nil || config.Width != 640 || config.Height != 137 {
		t.Fatalf("copy of the replaced file %dx%d %v", config.Width, config.Height, err)
	}
}

func TestStaticFilesIgnoreWidths(t *testing.T) {
	store, _ := lineblogStore(t)
	root := t.TempDir()
	original := noisyJPEG(t, 1000, 500)
	for name, content := range map[string][]byte{
		"index.html":             []byte("<!doctype html><title>synthetic</title>"),
		"assets/kano-banner.jpg": original,
	} {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	makeCopies(t, store, original)
	handler := NewStandaloneHandler(store, nil, Standalone{Static: NewStaticSite(root, "")})
	// Bundled files have no copies: a w request gets the original, which
	// revalidates.
	recorder := request(t, handler, http.MethodGet, "/assets/kano-banner.jpg?w=640")
	if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), original) || recorder.Header().Get("Cache-Control") != mediaCacheControl(false) {
		t.Fatalf("status=%d length=%d cache=%q", recorder.Code, recorder.Body.Len(), recorder.Header().Get("Cache-Control"))
	}
}

func sha256Of(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestLineblogImagesAreImmutableOnlyUnderTheirOwnHash(t *testing.T) {
	store, dir := lineblogStore(t)
	image := noisyJPEG(t, 1000, 500)
	makeCopies(t, store, image)
	named := sha256Of(image)[:12] + ".jpg"
	video := append([]byte("\x00\x00\x00\x18ftypmp42"), []byte(strings.Repeat("synthetic", 20))...)
	videoName := sha256Of(video)[:12] + ".mp4"
	for name, content := range map[string][]byte{named: image, "0123456789ab.jpg": image, videoName: video} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewHandler(store)
	for path, want := range map[string]bool{
		"/media/lineblog/" + named:            true,
		"/media/lineblog/" + named + "?w=320": true,
		// Bytes that do not match the name, and videos, which are named
		// after their watch URL, revalidate.
		"/media/lineblog/0123456789ab.jpg":        false,
		"/media/lineblog/0123456789ab.jpg?w=320":  false,
		"/media/lineblog/" + videoName:            false,
		"/media/lineblog/" + videoName + "?w=320": false,
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			recorder := request(t, handler, method, path)
			if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != mediaCacheControl(want) {
				t.Fatalf("%s %s status=%d cache=%q", method, path, recorder.Code, recorder.Header().Get("Cache-Control"))
			}
		}
	}
	// A file replaced under its old name is hashed anew and loses immutable.
	path := filepath.Join(dir, named)
	if err := os.WriteFile(path, noisyJPEG(t, 1200, 400), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "?w=320"} {
		recorder := request(t, handler, http.MethodGet, "/media/lineblog/"+named+suffix)
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != mediaCacheControl(false) {
			t.Fatalf("replaced %s cache=%q", suffix, recorder.Header().Get("Cache-Control"))
		}
	}
}

// makeCopies stores every width of content in store's variant cache, as the
// variants command does.
func makeCopies(t *testing.T, store *sqlite.Store, content []byte) {
	t.Helper()
	if _, err := sqlite.MakeVariants(context.Background(), store.DataRoot(), sha256Of(content), func() ([]byte, error) { return content, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestLineblogCopyIsServedWithoutReadingTheOriginal(t *testing.T) {
	store, dir := lineblogStore(t)
	original := noisyJPEG(t, 1000, 500)
	name := sha256Of(original)[:12] + ".jpg"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store)
	// Without a stored copy the original is streamed; nothing is decoded.
	cold := request(t, handler, http.MethodGet, "/media/lineblog/"+name+"?w=320")
	if cold.Code != http.StatusOK || !bytes.Equal(cold.Body.Bytes(), original) {
		t.Fatalf("cold status=%d length=%d", cold.Code, cold.Body.Len())
	}
	if entries, _ := filepath.Glob(filepath.Join(store.DataRoot(), "cache", "variants", "*", "*")); len(entries) != 0 {
		t.Fatalf("a request made copies: %v", entries)
	}
	// The hash read for it is kept under the data directory for restarts.
	if records, _ := filepath.Glob(filepath.Join(store.DataRoot(), "cache", "file-hashes", "*", "*")); len(records) != 1 {
		t.Fatalf("hash records %v", records)
	}
	makeCopies(t, store, original)
	warm := request(t, handler, http.MethodGet, "/media/lineblog/"+name+"?w=320")
	if warm.Code != http.StatusOK || decodedWidth(t, warm.Body.Bytes()) != 320 || warm.Header().Get("Cache-Control") != mediaCacheControl(true) {
		t.Fatalf("warm status=%d cache=%q", warm.Code, warm.Header().Get("Cache-Control"))
	}
	// Other bytes of the same size and modification time are not read
	// again: the remembered hash still finds the stored copy.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, bytes.Repeat([]byte{0xff}, len(original)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	again := request(t, handler, http.MethodGet, "/media/lineblog/"+name+"?w=320")
	if !bytes.Equal(again.Body.Bytes(), warm.Body.Bytes()) || again.Header().Get("ETag") != warm.Header().Get("ETag") {
		t.Fatalf("remembered copy status=%d length=%d", again.Code, again.Body.Len())
	}
}
