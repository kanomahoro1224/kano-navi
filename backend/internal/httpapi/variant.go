package httpapi

import (
	"context"
	"io"
	"io/fs"
	"net/http"

	"github.com/yexca/kano-navi/backend/internal/store/sqlite"
)

// fileHasher remembers files' hashes across requests and restarts
// (sqlite.Store.FileHash).
type fileHasher interface {
	FileHash(ctx context.Context, path string, info fs.FileInfo, file io.Reader) (string, error)
}

// variantReader finds the stored copies of an image known by its SHA-256
// (sqlite.Store.OpenVariant).
type variantReader interface {
	fileHasher
	OpenVariant(hash string, width int) (*sqlite.MediaStream, bool)
}

// maxVariantSourceBytes matches the cached-media bound: a larger fixed image
// has no copies and is always served whole.
const maxVariantSourceBytes = 10 * 1024 * 1024

// fixedFileHash returns the SHA-256 of the opened fixed file at path, whose
// stat is info, remembered by store or, when store is nil, by this process
// only. The caller seeks back before serving the file.
func fixedFileHash(ctx context.Context, store fileHasher, path string, info fs.FileInfo, file io.Reader) (string, error) {
	if store == nil {
		return sqlite.FileHash(ctx, "", path, info, file)
	}
	return store.FileHash(ctx, path, info, file)
}

// serveImageVariant answers a w request for the fixed image at path with its
// stored copy no wider than width. It reports false, having written nothing,
// when there is none (no copy was made, the original is kept for that width,
// the file is too large or its hash cannot be read); the caller then streams
// the original itself. Requests never decode: copies are made ahead by the
// variants command. The hash is remembered, so a copy is served without
// reading the original. immutable decides from the original's SHA-256
// whether the response may be cached as immutable.
func serveImageVariant(w http.ResponseWriter, r *http.Request, variants variantReader, path string, info fs.FileInfo, open func() (fs.File, error), width int, immutable func(hash string) bool) bool {
	if variants == nil || info.Size() > maxVariantSourceBytes {
		return false
	}
	source, err := open()
	if err != nil {
		return false
	}
	hash, err := variants.FileHash(r.Context(), path, info, source)
	source.Close()
	if err != nil {
		return false
	}
	copy, ok := variants.OpenVariant(hash, width)
	if !ok {
		return false
	}
	defer copy.File.Close()
	serveMediaStream(w, r, copy, immutable(hash))
	return true
}
