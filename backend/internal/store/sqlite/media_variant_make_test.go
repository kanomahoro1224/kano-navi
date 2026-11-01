package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yexca/kano-navi/backend/internal/mediavariant"
)

// makeReady makes the copies of every ready asset, as the media step does, and
// returns how many images it stored widths for.
func makeReady(t *testing.T, s *Store) int {
	t.Helper()
	ready, err := s.ReadyMedia(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	made := 0
	for _, item := range ready {
		result, err := MakeVariants(context.Background(), s.dataRoot, item.Hash, func() ([]byte, error) { return s.LoadReadyMedia(item) })
		if err == nil && result.Copies+result.Kept > 0 {
			made++
		}
	}
	return made
}

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func readStream(t *testing.T, stream *MediaStream) []byte {
	t.Helper()
	defer stream.File.Close()
	content, err := io.ReadAll(stream.File)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestMediaStepMakesCopiesThatRequestsServeWithoutDecoding(t *testing.T) {
	s := openVariantStore(t)
	ctx := context.Background()
	original := syntheticImage(t, "image/jpeg", 1000, 500, false)
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-wide.jpg", original, "image/jpeg")
	var decodes atomic.Int32
	t.Cleanup(func() { resizeVariants = mediavariant.ResizeAll })
	resizeVariants = func(content []byte, widths []int) []mediavariant.Copy {
		decodes.Add(1)
		return mediavariant.ResizeAll(content, widths)
	}
	// Before the step, a request gets the original and decodes nothing.
	stream, err := s.OpenMediaVariant(ctx, id, "", 320)
	if err != nil || stream.ETag != hashOf(original) || !bytes.Equal(readStream(t, stream), original) || decodes.Load() != 0 {
		t.Fatalf("cold request: %v decodes=%d", err, decodes.Load())
	}
	if resized := makeReady(t, s); resized != 1 || decodes.Load() != 1 {
		t.Fatalf("step resized %d decodes=%d", resized, decodes.Load())
	}
	stream, err = s.OpenMediaVariant(ctx, id, "", 320)
	if err != nil || stream.MIMEType != "image/jpeg" || stream.ETag != hashOf(original)+"-320" || stream.Size >= int64(len(original)) {
		t.Fatalf("copy: %v %+v", err, stream)
	}
	img, _, err := image.DecodeConfig(bytes.NewReader(readStream(t, stream)))
	if err != nil || img.Width != 320 || img.Height != 160 {
		t.Fatalf("copy size %dx%d %v", img.Width, img.Height, err)
	}
	// Widths the original cannot fill are marked and served whole.
	markers, _ := filepath.Glob(filepath.Join(s.dataRoot, "cache", "variants", "*", "*.orig"))
	if len(markers) != 2 {
		t.Fatalf("markers %v", markers)
	}
	if stream, err = s.OpenMediaVariant(ctx, id, "", 1280); err != nil || !bytes.Equal(readStream(t, stream), original) {
		t.Fatalf("kept original: %v", err)
	}
	// A done image is not loaded again.
	if resized := makeReady(t, s); resized != 0 || decodes.Load() != 1 {
		t.Fatalf("second step resized %d decodes=%d", resized, decodes.Load())
	}
	if _, err = s.OpenMediaVariant(ctx, id, "", 333); err != ErrMediaNotFound {
		t.Fatalf("unlisted width accepted: %v", err)
	}
	if _, err = s.OpenMediaVariant(ctx, id, strings.Repeat("0", 64), 320); err != ErrMediaVersionNotFound {
		t.Fatalf("unknown version accepted: %v", err)
	}
	// The original stays untouched.
	whole, err := s.ReadMedia(ctx, id, "")
	if err != nil || !bytes.Equal(whole.Content, original) {
		t.Fatalf("original changed: %v", err)
	}
}

func TestMakeVariantsKeepsOriginalWhenItCannotShrink(t *testing.T) {
	s := openVariantStore(t)
	ctx := context.Background()
	cases := map[string][]byte{
		"https://pbs.twimg.com/media/synthetic-small.jpg": syntheticImage(t, "image/jpeg", 200, 100, false),
		"https://pbs.twimg.com/media/synthetic-anim.gif":  syntheticImage(t, "image/gif", 800, 400, false),
	}
	ids := map[string][]byte{}
	for source, content := range cases {
		mime := "image/jpeg"
		if filepath.Ext(source) == ".gif" {
			mime = "image/gif"
		}
		ids[readyAsset(t, s, source, content, mime)] = content
	}
	if resized := makeReady(t, s); resized != 2 {
		t.Fatalf("step: %d", resized)
	}
	markers, _ := filepath.Glob(filepath.Join(s.dataRoot, "cache", "variants", "*", "*.orig"))
	if len(markers) != 2*len(mediavariant.Widths) {
		t.Fatalf("keep-original markers %v", markers)
	}
	for id, content := range ids {
		stream, err := s.OpenMediaVariant(ctx, id, "", 320)
		if err != nil || !bytes.Equal(readStream(t, stream), content) {
			t.Fatalf("original not served: %v", err)
		}
	}
}

func TestMakeVariantsKeepsTransparency(t *testing.T) {
	s := openVariantStore(t)
	original := noisyAlphaPNG(t, 1400, 700)
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-alpha.png", original, "image/png")
	if resized := makeReady(t, s); resized != 1 {
		t.Fatalf("step: %d", resized)
	}
	stream, err := s.OpenMediaVariant(context.Background(), id, "", 320)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.File.Close()
	if stream.MIMEType != "image/png" || stream.ETag != hashOf(original)+"-320" {
		t.Fatalf("transparent copy: %+v", stream)
	}
}

func TestMediaStepLeavesAReadOnlyCacheToTheOriginal(t *testing.T) {
	s := openVariantStore(t)
	original := syntheticImage(t, "image/jpeg", 1000, 500, false)
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-ro.jpg", original, "image/jpeg")
	// A file where the variant directory should be makes every write fail.
	if err := os.WriteFile(filepath.Join(s.dataRoot, "cache", "variants"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if resized := makeReady(t, s); resized != 0 {
		t.Fatalf("unstored copies counted: %d", resized)
	}
	stream, err := s.OpenMediaVariant(context.Background(), id, "", 320)
	if err != nil || !bytes.Equal(readStream(t, stream), original) {
		t.Fatalf("original not served: %v", err)
	}
}

func TestMakeVariantsMakesOneImageOnce(t *testing.T) {
	s := openVariantStore(t)
	original := syntheticImage(t, "image/jpeg", 1000, 500, false)
	hash := hashOf(original)
	var decodes, loads atomic.Int32
	t.Cleanup(func() { resizeVariants = mediavariant.ResizeAll })
	resizeVariants = func(content []byte, widths []int) []mediavariant.Copy {
		decodes.Add(1)
		time.Sleep(50 * time.Millisecond)
		return mediavariant.ResizeAll(content, widths)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := MakeVariants(context.Background(), s.dataRoot, hash, func() ([]byte, error) {
				loads.Add(1)
				return original, nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if decodes.Load() != 1 || loads.Load() != 1 {
		t.Fatalf("decodes=%d loads=%d for one image", decodes.Load(), loads.Load())
	}
}

func TestOpenVariantSkipsUnusableCopies(t *testing.T) {
	s := openVariantStore(t)
	hash := strings.Repeat("ab", 32)
	name := filepath.Join(s.dataRoot, "cache", "variants", hash[:2], hash+"-320.jpg")
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	for label, content := range map[string][]byte{"empty": {}, "mistyped": []byte("\x89PNG\r\n\x1a\nsynthetic")} {
		if err := os.WriteFile(name, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if stream, ok := s.OpenVariant(hash, 320); ok {
			stream.File.Close()
			t.Fatalf("%s copy served", label)
		}
	}
	content := syntheticImage(t, "image/jpeg", 8, 8, false)
	if err := os.WriteFile(name, content, 0o600); err != nil {
		t.Fatal(err)
	}
	stream, ok := s.OpenVariant(hash, 320)
	if !ok || stream.MIMEType != "image/jpeg" || stream.ETag != hash+"-320" || !bytes.Equal(readStream(t, stream), content) {
		t.Fatalf("usable copy not served: %v %+v", ok, stream)
	}
	if _, ok = s.OpenVariant("not-a-hash", 320); ok {
		t.Fatal("unsafe hash opened")
	}
}

func TestMakeVariantsKeepsOnlyBytesMatchingTheHash(t *testing.T) {
	s := openVariantStore(t)
	ctx := context.Background()
	content := syntheticImage(t, "image/jpeg", 1000, 500, false)
	hash := hashOf(content)
	load := func() ([]byte, error) { return content, nil }
	if _, err := MakeVariants(ctx, s.dataRoot, "not-a-hash", load); err != ErrMediaNotFound {
		t.Fatalf("unsafe hash accepted: %v", err)
	}
	other := strings.Repeat("0", 64)
	if made, err := MakeVariants(ctx, s.dataRoot, other, load); made != (VariantsMade{}) || !errors.Is(err, ErrVariantSource) {
		t.Fatalf("mismatched bytes: %v %v", made, err)
	}
	if entries, _ := filepath.Glob(filepath.Join(s.dataRoot, "cache", "variants", "00", "*")); len(entries) != 0 {
		t.Fatalf("mismatched bytes cached: %v", entries)
	}
	if made, err := MakeVariants(ctx, s.dataRoot, hash, load); made != (VariantsMade{Copies: 3, Kept: 2}) || err != nil {
		t.Fatalf("matching bytes: %v %v", made, err)
	}
	// Done images are found without loading the original.
	if made, err := MakeVariants(ctx, s.dataRoot, hash, func() ([]byte, error) { return nil, errors.New("synthetic load failure") }); made != (VariantsMade{}) || err != nil {
		t.Fatalf("done image loaded: %v %v", made, err)
	}
}

func TestMakeVariantsBoundsOriginalsInMemory(t *testing.T) {
	s := openVariantStore(t)
	// holding counts originals from their load until their decode ends.
	var holding, peak, loads atomic.Int32
	started, proceed := make(chan struct{}, 16), make(chan struct{})
	t.Cleanup(func() { resizeVariants = mediavariant.ResizeAll })
	resizeVariants = func(content []byte, widths []int) []mediavariant.Copy {
		defer holding.Add(-1)
		return mediavariant.ResizeAll(content, widths)
	}
	const makers = 8
	var wg sync.WaitGroup
	for i := range makers {
		content := syntheticImage(t, "image/jpeg", 700+i, 300, false)
		wg.Go(func() {
			made, err := MakeVariants(context.Background(), s.dataRoot, hashOf(content), func() ([]byte, error) {
				loads.Add(1)
				now := holding.Add(1)
				for {
					old := peak.Load()
					if now <= old || peak.CompareAndSwap(old, now) {
						break
					}
				}
				started <- struct{}{}
				<-proceed
				return content, nil
			})
			if made.Copies+made.Kept == 0 || err != nil {
				t.Errorf("maker: %v %v", made, err)
			}
		})
	}
	for range cap(sourceSlots) {
		<-started
	}
	// The other makers wait for a slot without loading their originals.
	time.Sleep(50 * time.Millisecond)
	if got := loads.Load(); got != int32(cap(sourceSlots)) {
		t.Fatalf("%d originals loaded while %d slots are taken", got, cap(sourceSlots))
	}
	close(proceed)
	wg.Wait()
	if loads.Load() != makers || peak.Load() > int32(cap(sourceSlots)) {
		t.Fatalf("loads=%d peak=%d", loads.Load(), peak.Load())
	}
}

func TestMakeVariantsWaitsForASlotWithoutLoading(t *testing.T) {
	s := openVariantStore(t)
	content := syntheticImage(t, "image/jpeg", 1000, 500, false)
	for range cap(sourceSlots) {
		sourceSlots <- struct{}{}
	}
	defer func() {
		for range cap(sourceSlots) {
			<-sourceSlots
		}
	}()
	waiting, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	loaded := false
	made, err := MakeVariants(waiting, s.dataRoot, hashOf(content), func() ([]byte, error) {
		loaded = true
		return content, nil
	})
	if made != (VariantsMade{}) || loaded || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("busy slots: made=%v loaded=%v %v", made, loaded, err)
	}
}

func TestOpenMediaChecksEachFileOnce(t *testing.T) {
	s := openVariantStore(t)
	ctx := context.Background()
	original := syntheticImage(t, "image/jpeg", 40, 20, false)
	id := readyAsset(t, s, "https://pbs.twimg.com/media/synthetic-once.jpg", original, "image/jpeg")
	stream, err := s.OpenMedia(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	path := stream.File.Name()
	if !bytes.Equal(readStream(t, stream), original) {
		t.Fatal("original not served")
	}
	// The download recorded the file's hash, so it survives a restart.
	clearRememberedHashes()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash, ok := KnownFileHash(s.dataRoot, path, info); !ok || hash != hashOf(original) {
		t.Fatalf("download not remembered: %q %v", hash, ok)
	}
	// A changed file is checked again and no longer served.
	if err = os.WriteFile(path, bytes.Repeat([]byte{0xff}, len(original)), 0o600); err != nil {
		t.Fatal(err)
	}
	later := info.ModTime().Add(time.Minute)
	if err = os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err = s.OpenMedia(ctx, id, ""); err != ErrMediaNotFound {
		t.Fatalf("changed file served: %v", err)
	}
}

// clearRememberedHashes stands in for a restart of the process.
func clearRememberedHashes() {
	fileHashesMu.Lock()
	clear(fileHashes)
	fileHashesMu.Unlock()
}

func TestFileHashRecordsSurviveARestart(t *testing.T) {
	dataRoot := t.TempDir()
	path := filepath.Join(t.TempDir(), "0123456789ab.jpg")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("ab", 32)
	RememberFileHash(dataRoot, path, info, hash)
	clearRememberedHashes()
	if got, ok := KnownFileHash(dataRoot, path, info); !ok || got != hash {
		t.Fatalf("remembered hash %q %v", got, ok)
	}
	clearRememberedHashes()
	// Another size, modification time or path reads the file anew.
	if err = os.WriteFile(path, []byte("synthetic!"), 0o600); err != nil {
		t.Fatal(err)
	}
	grown, _ := os.Stat(path)
	later := info.ModTime().Add(time.Minute)
	if err = os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	touched, _ := os.Stat(path)
	for label, lookup := range map[string]func() (string, bool){
		"size": func() (string, bool) { return KnownFileHash(dataRoot, path, grown) },
		"time": func() (string, bool) { return KnownFileHash(dataRoot, path, touched) },
		"path": func() (string, bool) { return KnownFileHash(dataRoot, path+"x", info) },
	} {
		if got, ok := lookup(); ok {
			t.Fatalf("%s change kept %q", label, got)
		}
	}
	// Without a data directory the hash lives in memory only.
	RememberFileHash("", path+"y", info, hash)
	if entries, _ := filepath.Glob(filepath.Join(dataRoot, "cache", "file-hashes", "*", "*")); len(entries) != 1 {
		t.Fatalf("records %v", entries)
	}
}

// noisyAlphaPNG is a synthetic photo-like PNG with transparency, too noisy to
// compress below its smaller copies.
func noisyAlphaPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	random := rand.New(rand.NewPCG(1, 2))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(random.Uint32()), G: uint8(random.Uint32()), B: uint8(random.Uint32()), A: uint8(64 + x%128)})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
