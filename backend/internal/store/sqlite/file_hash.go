package sqlite

import (
	"context"
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
)

// fileHashRoot holds the remembered SHA-256 of served files under
// data/cache/file-hashes: cached media, whose bytes are checked against their
// row once, and the LINE BLOG archive and history images, which have no rows.
// Like the variant cache it is derived: a missing or stale record only means
// the file is hashed again.
const fileHashRoot = "cache/file-hashes"

// maxFileHashRecord bounds a record: "<size> <modification unix ns> <hash>\n".
const maxFileHashRecord = 128

// rememberedHash is the SHA-256 of a file as it was when last read.
type rememberedHash struct {
	size, modTime int64
	hash          string
}

// fileHashes keeps the records read or written by this process, by absolute
// path, so a request needs no record read either.
var (
	fileHashesMu sync.Mutex
	fileHashes   = map[string]rememberedHash{}
)

// hashSlots bounds the files hashed at once. Hashing streams a file through a
// small buffer, so it holds no file in memory; the slots bound its disk reads.
var hashSlots = make(chan struct{}, 2)

// fileHashName files the record of path under the SHA-256 of the path, so
// any path maps to a fixed, contained name.
func fileHashName(path string) string {
	sum := sha256.Sum256([]byte(path))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(key[:2], key)
}

func fileHashPrefix(info fs.FileInfo) string {
	return strconv.FormatInt(info.Size(), 10) + " " + strconv.FormatInt(info.ModTime().UnixNano(), 10) + " "
}

// KnownFileHash returns the remembered SHA-256 of the file at path, an
// absolute path whose stat is info, while the file's size and modification
// time still match. dataRoot holds the persisted records; "" uses only this
// process's memory.
func KnownFileHash(dataRoot, path string, info fs.FileInfo) (string, bool) {
	fileHashesMu.Lock()
	known, ok := fileHashes[path]
	fileHashesMu.Unlock()
	if ok && known.size == info.Size() && known.modTime == info.ModTime().UnixNano() {
		return known.hash, true
	}
	if dataRoot == "" {
		return "", false
	}
	root, err := os.OpenRoot(filepath.Join(dataRoot, fileHashRoot))
	if err != nil {
		return "", false
	}
	defer root.Close()
	file, err := root.Open(fileHashName(path))
	if err != nil {
		return "", false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxFileHashRecord+1))
	if err != nil || len(content) > maxFileHashRecord {
		return "", false
	}
	hash, found := strings.CutPrefix(string(content), fileHashPrefix(info))
	hash = strings.TrimSuffix(hash, "\n")
	if !found || !safeHash(hash) {
		return "", false
	}
	rememberInMemory(path, info, hash)
	return hash, true
}

// RememberFileHash records hash, read from the bytes of the file at path,
// under the file's size and modification time, in memory and, when dataRoot
// is set, as a record there.
func RememberFileHash(dataRoot, path string, info fs.FileInfo, hash string) {
	if !safeHash(hash) {
		return
	}
	rememberInMemory(path, info, hash)
	if dataRoot != "" {
		writeCacheFile(dataRoot, fileHashRoot, ".hash-", fileHashName(path), []byte(fileHashPrefix(info)+hash+"\n"))
	}
}

func rememberInMemory(path string, info fs.FileInfo, hash string) {
	fileHashesMu.Lock()
	fileHashes[path] = rememberedHash{size: info.Size(), modTime: info.ModTime().UnixNano(), hash: hash}
	fileHashesMu.Unlock()
}

// FileHash returns the SHA-256 of the opened file at path, whose stat is
// info. A file not remembered is read from file to its end, at most
// cap(hashSlots) files at a time, and remembered (see RememberFileHash); the
// caller seeks back before serving it. An error means the file could not be
// read or ctx ended while waiting.
func FileHash(ctx context.Context, dataRoot, path string, info fs.FileInfo, file io.Reader) (string, error) {
	if hash, ok := KnownFileHash(dataRoot, path, info); ok {
		return hash, nil
	}
	hash, err := HashFile(ctx, file, info.Size())
	if err != nil {
		return "", err
	}
	RememberFileHash(dataRoot, path, info, hash)
	return hash, nil
}

// HashFile streams file, which must hold exactly size bytes, through SHA-256
// in one of the hash slots.
func HashFile(ctx context.Context, file io.Reader, size int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case hashSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-hashSlots }()
	digest := sha256.New()
	read, err := io.Copy(digest, io.LimitReader(file, size+1))
	if err != nil || read != size {
		return "", errors.New("file unreadable")
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// KnownFileHash is the package function for this store's data directory.
func (s *Store) KnownFileHash(path string, info fs.FileInfo) (string, bool) {
	return KnownFileHash(s.DataRoot(), path, info)
}

// FileHash is the package function for this store's data directory.
func (s *Store) FileHash(ctx context.Context, path string, info fs.FileInfo, file io.Reader) (string, error) {
	return FileHash(ctx, s.DataRoot(), path, info, file)
}
