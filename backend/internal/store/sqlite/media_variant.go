package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/yexca/kano-navi/backend/internal/mediaformat"
	"github.com/yexca/kano-navi/backend/internal/mediavariant"
)

// variantRoot holds resized copies under data/cache/variants. It is a derived
// cache: originals are never changed and a removed directory is rebuilt by the
// media step and the variants command.
const variantRoot = "cache/variants"

// variantKeepOriginal marks a width whose copy would not be smaller (or the
// format is served whole), so the original is served without decoding again.
const variantKeepOriginal = "orig"

// sourceSlots bounds the originals held in memory to make copies. A slot is
// taken before an original is read and kept until its copies are made, so at
// most two originals (10 MiB each) and their decodes (a 40-megapixel image
// needs about 160 MB as RGBA) exist at once, each with the copy being scaled
// from it. A maker waiting for a slot holds no original.
var sourceSlots = make(chan struct{}, 2)

// resizeVariants makes the copies of one original; tests count and pace it.
var resizeVariants = mediavariant.ResizeEach

// ErrVariantSource means the loaded original does not have the hash its
// copies are filed under, so none are made.
var ErrVariantSource = errors.New("variant source does not match its hash")

// errVariantNotStored means a width could not be written; the image is made
// again next time.
var errVariantNotStored = errors.New("variant not stored")

// OpenMediaVariant opens the stored copy of a ready asset no wider than
// width, or the original when there is none. Requests never decode: copies
// are made ahead (MakeVariants).
func (s *Store) OpenMediaVariant(ctx context.Context, id, version string, width int) (*MediaStream, error) {
	if !mediavariant.ValidWidth(width) {
		return nil, ErrMediaNotFound
	}
	record, err := s.lookupMedia(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if stream, ok := s.OpenVariant(record.hash, width); ok {
		stream.LastModified = record.lastModified(stream.LastModified)
		return stream, nil
	}
	return s.openMediaRecord(ctx, record)
}

// OpenVariant opens the stored copy no wider than width of the image whose
// SHA-256 is hash. It reports false when there is none, including a width
// whose original is kept: the caller serves the original. The copy's ETag
// names the original's hash and the width, so serving it hashes nothing.
func (s *Store) OpenVariant(hash string, width int) (*MediaStream, bool) {
	if s == nil || !mediavariant.ValidWidth(width) || !safeHash(hash) {
		return nil, false
	}
	root, err := os.OpenRoot(filepath.Join(s.dataRoot, variantRoot))
	if err != nil {
		return nil, false
	}
	defer root.Close()
	name := variantName(hash, width)
	for _, mime := range []string{"image/jpeg", "image/png"} {
		file, openErr := root.Open(name + "." + mediaformat.Extension(mime))
		if openErr != nil {
			continue
		}
		if stream := checkVariantFile(file, mime); stream != nil {
			stream.ETag = hash + "-" + strconv.Itoa(width)
			return stream, true
		}
		file.Close()
	}
	return nil, false
}

// checkVariantFile accepts an opened copy that is a complete regular file
// within the media bound whose first bytes hold mime, positioned at its
// start; otherwise it returns nil and the caller closes file.
func checkVariantFile(file *os.File, mime string) *MediaStream {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxCachedMediaBytes {
		return nil
	}
	header := make([]byte, min(info.Size(), 512))
	if _, err = io.ReadFull(file, header); err != nil || mediaformat.Sniff(header) != mime {
		return nil
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	return &MediaStream{File: file, MIMEType: mime, Size: info.Size(), LastModified: info.ModTime()}
}

// VariantsDone reports whether every width of hash has a copy or a
// keep-original marker under dataRoot. MakeVariants writes the widest width
// last and stops at the first failed write, so the widest entry stands for
// all of them. It only checks whether a file exists.
func VariantsDone(dataRoot, hash string) bool {
	if !safeHash(hash) {
		return false
	}
	root, err := os.OpenRoot(filepath.Join(dataRoot, variantRoot))
	if err != nil {
		return false
	}
	defer root.Close()
	return hasVariantEntry(root, variantName(hash, mediavariant.Widths[len(mediavariant.Widths)-1]))
}

func hasVariantEntry(root *os.Root, name string) bool {
	for _, extension := range []string{variantKeepOriginal, "jpg", "png"} {
		if info, err := root.Stat(name + "." + extension); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// VariantsMade counts the widths one MakeVariants call stored: Copies smaller
// copies and Kept keep-original markers. Both are zero for an image that was
// already done.
type VariantsMade struct{ Copies, Kept int }

// MakeVariants stores every missing width of the image whose SHA-256 is hash
// in the variant cache under dataRoot: a smaller copy, or a keep-original
// marker when none can be made. load reads the original. It is called at most
// once, while holding one of the source slots, and copies are kept only when
// its bytes match hash. An image already done loads nothing.
func MakeVariants(ctx context.Context, dataRoot, hash string, load func() ([]byte, error)) (VariantsMade, error) {
	made := VariantsMade{}
	if !safeHash(hash) || dataRoot == "" {
		return made, ErrMediaNotFound
	}
	if VariantsDone(dataRoot, hash) {
		return made, nil
	}
	// Concurrent makers of one image wait for the first and then find its
	// copies, instead of each reading and decoding the original.
	release, err := claimVariant(ctx, filepath.Join(dataRoot, variantRoot, hash))
	if err != nil {
		return made, err
	}
	defer release()
	if VariantsDone(dataRoot, hash) {
		return made, nil
	}
	freeSlot, err := takeSourceSlot(ctx)
	if err != nil {
		return made, err
	}
	defer freeSlot()
	content, err := load()
	if err != nil {
		return made, err
	}
	if sum := sha256.Sum256(content); hex.EncodeToString(sum[:]) != hash {
		return made, ErrVariantSource
	}
	// Each width is written as soon as it is encoded, so at most one copy
	// waits in memory besides the widest, which is written last: a failed
	// write stops the image before its widest entry marks it done.
	missing := missingWidths(dataRoot, hash)
	stored := true
	resizeVariants(content, missing, func(i int, copy mediavariant.Copy) bool {
		name := variantName(hash, missing[i])
		if copy.Err != nil {
			stored = writeCacheFile(dataRoot, variantRoot, ".variant-", name+"."+variantKeepOriginal, nil)
			if stored {
				made.Kept++
			}
		} else {
			stored = writeCacheFile(dataRoot, variantRoot, ".variant-", name+"."+mediaformat.Extension(copy.MIME), copy.Content)
			if stored {
				made.Copies++
			}
		}
		return stored
	})
	if !stored {
		return made, errVariantNotStored
	}
	return made, nil
}

// VariantEntry is one stored width of an image: a copy, or a keep-original
// marker (Kept).
type VariantEntry struct {
	Kept bool
}

// ListVariants lists the variant cache under dataRoot by original hash and
// width. It reads directory names only, never a file or its metadata, and
// skips temporary and unrecognized names. A missing cache lists nothing.
func ListVariants(dataRoot string) (map[string]map[int]VariantEntry, error) {
	out := map[string]map[int]VariantEntry{}
	base := filepath.Join(dataRoot, variantRoot)
	shards, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(base, shard.Name()))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			hash, width, kept, ok := parseVariantName(entry.Name())
			if !ok || hash[:2] != shard.Name() || !entry.Type().IsRegular() {
				continue
			}
			if out[hash] == nil {
				out[hash] = map[int]VariantEntry{}
			}
			if kept {
				out[hash][width] = VariantEntry{Kept: true}
			} else if _, marked := out[hash][width]; !marked {
				out[hash][width] = VariantEntry{}
			}
		}
	}
	return out, nil
}

// parseVariantName reads "<hash>-<width>.<jpg|png|orig>".
func parseVariantName(name string) (hash string, width int, kept, ok bool) {
	stem, extension, found := strings.Cut(name, ".")
	if !found || (extension != "jpg" && extension != "png" && extension != variantKeepOriginal) {
		return "", 0, false, false
	}
	hash, rawWidth, found := strings.Cut(stem, "-")
	if !found || !safeHash(hash) {
		return "", 0, false, false
	}
	width, err := strconv.Atoi(rawWidth)
	if err != nil || !mediavariant.ValidWidth(width) {
		return "", 0, false, false
	}
	return hash, width, extension == variantKeepOriginal, true
}

// missingWidths lists the widths of hash with neither a copy nor a marker.
func missingWidths(dataRoot, hash string) []int {
	root, err := os.OpenRoot(filepath.Join(dataRoot, variantRoot))
	if err != nil {
		return append([]int{}, mediavariant.Widths...)
	}
	defer root.Close()
	missing := []int{}
	for _, width := range mediavariant.Widths {
		if !hasVariantEntry(root, variantName(hash, width)) {
			missing = append(missing, width)
		}
	}
	return missing
}

// takeSourceSlot waits for a source slot until ctx ends. The returned func
// frees it and must be called.
func takeSourceSlot(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case sourceSlots <- struct{}{}:
		return func() { <-sourceSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// variantMakers holds the images whose copies are being made, keyed by their
// cache path, each with a channel closed when its maker is done.
var (
	variantMakersMu sync.Mutex
	variantMakers   = map[string]chan struct{}{}
)

// claimVariant waits until no other maker holds key and claims it. The
// returned release must be called; an error means ctx ended first.
func claimVariant(ctx context.Context, key string) (func(), error) {
	for {
		variantMakersMu.Lock()
		busy, ok := variantMakers[key]
		if !ok {
			done := make(chan struct{})
			variantMakers[key] = done
			variantMakersMu.Unlock()
			return func() {
				variantMakersMu.Lock()
				delete(variantMakers, key)
				variantMakersMu.Unlock()
				close(done)
			}, nil
		}
		variantMakersMu.Unlock()
		select {
		case <-busy:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ReadyMedia is the current file of a ready asset, for the variant maker.
type ReadyMedia struct {
	Hash      string
	cachePath string
}

// ReadyMedia lists the current files of ready X, YouTube and Bilibili assets,
// newest fetch first.
func (s *Store) ReadyMedia(ctx context.Context) ([]ReadyMedia, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cache_path,sha256 FROM media_assets WHERE status='ready' AND source IN('x','youtube','bilibili') AND cache_path IS NOT NULL AND sha256 IS NOT NULL ORDER BY fetched_at DESC,id`)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	out := []ReadyMedia{}
	for rows.Next() {
		var item ReadyMedia
		if err = rows.Scan(&item.cachePath, &item.Hash); err != nil {
			return nil, dbErr(err)
		}
		if safeHash(item.Hash) {
			out = append(out, item)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return out, nil
}

// LoadReadyMedia reads item's contained regular file whole, within the media
// bound. MakeVariants checks the bytes against item.Hash.
func (s *Store) LoadReadyMedia(item ReadyMedia) ([]byte, error) {
	path, err := s.resolveMediaPath(item.cachePath)
	if err != nil {
		return nil, err
	}
	content, _, err := readContainedFile(path, maxCachedMediaBytes)
	return content, err
}

func variantName(hash string, width int) string {
	return filepath.Join(hash[:2], hash+"-"+strconv.Itoa(width))
}

// writeCacheFile stores name under the cache directory root of dataRoot
// through a temporary file named with prefix, renamed into place. It reports
// whether the entry is in place; a read-only data directory or any other
// failure only means the entry is made again next time.
func writeCacheFile(dataRoot, root, prefix, name string, content []byte) bool {
	base, err := os.OpenRoot(dataRoot)
	if err != nil {
		return false
	}
	defer base.Close()
	dir := filepath.Join(filepath.FromSlash(root), filepath.Dir(name))
	if base.MkdirAll(dir, 0755) != nil {
		return false
	}
	suffix := make([]byte, 16)
	if _, err = rand.Read(suffix); err != nil {
		return false
	}
	temp := filepath.Join(dir, prefix+hex.EncodeToString(suffix))
	file, err := base.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false
	}
	defer base.Remove(temp)
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return false
	}
	target := filepath.Join(filepath.FromSlash(root), name)
	err = base.Rename(temp, target)
	return err == nil || errors.Is(err, os.ErrExist)
}
