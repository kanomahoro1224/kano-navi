package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrMediaNotFound        = errors.New("media not found")
	ErrMediaNotReady        = errors.New("media not ready")
	ErrMediaVersionNotFound = errors.New("media version not found")
)

// Source media downloads are capped at 10 MiB by the media downloader.
// Public reads enforce the same bound before allocating a byte
// slice, so a corrupt or operator-edited cache row cannot trigger an
// unbounded read.
const maxCachedMediaBytes = 10 * 1024 * 1024

// Profile candidates and selected files share the 15 MiB image bound.
const maxProfileMediaBytes = 15 * 1024 * 1024

type MediaFile struct {
	Content      []byte
	MIMEType     string
	SHA256       string
	Size         int64
	LastModified time.Time
}

// MediaStream is an opened, checked file to serve: a ready asset, an archive
// or history original, or a copy. The caller closes File. ETag is unquoted.
type MediaStream struct {
	File         *os.File
	MIMEType     string
	ETag         string
	Size         int64
	LastModified time.Time
}

type ProfileMediaFile struct {
	Content  []byte
	MIMEType string
	SHA256   string
	Size     int64
}

func allowedMediaMIME(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
	switch normalized {
	case "image/avif", "image/gif", "image/jpeg", "image/png", "image/webp":
		return normalized, true
	default:
		return "", false
	}
}

// mediaRecord locates a ready asset's file, or an earlier version a
// revalidation replaced, before its bytes are read.
type mediaRecord struct {
	cachePath, mime, hash string
	fetchedAt             sql.NullString
}

// OpenMedia opens a ready asset's file, or the earlier version version names,
// for streaming.
func (s *Store) OpenMedia(ctx context.Context, id, version string) (*MediaStream, error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().OpenMedia(ctx, id, version)
	}
	record, err := s.lookupMedia(ctx, id, version)
	if err != nil {
		return nil, err
	}
	return s.openMediaRecord(ctx, record)
}

// ReadMedia reads a ready asset's file whole, checked like OpenMedia.
func (s *Store) ReadMedia(ctx context.Context, id, version string) (*MediaFile, error) {
	stream, err := s.OpenMedia(ctx, id, version)
	if err != nil {
		return nil, err
	}
	defer stream.File.Close()
	content, err := io.ReadAll(io.LimitReader(stream.File, stream.Size+1))
	if err != nil || int64(len(content)) != stream.Size {
		return nil, ErrMediaNotFound
	}
	return &MediaFile{Content: content, MIMEType: stream.MIMEType, SHA256: stream.ETag, Size: stream.Size, LastModified: stream.LastModified}, nil
}

func (s *Store) lookupMedia(ctx context.Context, id, version string) (*mediaRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	if !safeHash(id) {
		return nil, ErrMediaNotFound
	}
	var cachePath, mimeType, contentHash, status, fetchedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT cache_path,mime_type,sha256,status,fetched_at FROM media_assets WHERE id = ?`, id).Scan(&cachePath, &mimeType, &contentHash, &status, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaNotReady
	}
	if err != nil {
		return nil, dbErr(err)
	}
	if status.String != "ready" || !contentHash.Valid || !safeHash(contentHash.String) || !cachePath.Valid {
		return nil, ErrMediaNotReady
	}
	if version != "" {
		if !safeHash(version) {
			return nil, ErrMediaVersionNotFound
		}
		// An earlier file a revalidation replaced stays servable by its hash,
		// so an operator can keep showing it.
		if version != contentHash.String {
			err = s.db.QueryRowContext(ctx, `SELECT cache_path,mime_type,sha256,fetched_at FROM media_versions WHERE media_id=? AND sha256=?`, id, version).Scan(&cachePath, &mimeType, &contentHash, &fetchedAt)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrMediaVersionNotFound
			}
			if err != nil {
				return nil, dbErr(err)
			}
		}
	}
	mime, ok := allowedMediaMIME(mimeType.String)
	if !ok {
		return nil, ErrMediaNotFound
	}
	return &mediaRecord{cachePath: cachePath.String, mime: mime, hash: contentHash.String, fetchedAt: fetchedAt}, nil
}

// openMediaRecord opens the contained regular file of record, within the
// media bound, whose bytes have record's hash. The hash is read once per file
// and remembered by path, size and modification time (FileHash), so a
// request streams the file without hashing it again; a changed file is
// checked anew.
func (s *Store) openMediaRecord(ctx context.Context, record *mediaRecord) (*MediaStream, error) {
	path, err := s.resolveMediaPath(record.cachePath)
	if err != nil {
		return nil, ErrMediaNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrMediaNotFound
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxCachedMediaBytes {
		file.Close()
		return nil, ErrMediaNotFound
	}
	hash, err := s.FileHash(ctx, path, info, file)
	if err != nil || hash != record.hash {
		file.Close()
		return nil, ErrMediaNotFound
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, ErrMediaNotFound
	}
	return &MediaStream{File: file, MIMEType: record.mime, ETag: record.hash, Size: info.Size(), LastModified: record.lastModified(info.ModTime())}, nil
}

// rememberMediaFile records hash as the checked hash of the file at the
// cache path relative, which was just written or compared with hash.
func (s *Store) rememberMediaFile(relative, hash string) {
	path, err := s.resolveMediaPath(relative)
	if err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil {
		RememberFileHash(s.dataRoot, path, info, hash)
	}
}

func (r *mediaRecord) lastModified(fallback time.Time) time.Time {
	if r.fetchedAt.Valid {
		if parsed, err := time.Parse(time.RFC3339Nano, r.fetchedAt.String); err == nil {
			return parsed
		}
	}
	return fallback
}

// OpenProfileMedia opens selected profile media after checking its containment,
// image signature and stored hash. The returned file is streamed by the HTTP
// layer, so concurrent public reads do not allocate one full image each.
func (s *Store) OpenProfileMedia(ctx context.Context, slot, version string) (*MediaStream, error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().OpenProfileMedia(ctx, slot, version)
	}
	record, err := s.lookupProfileMedia(ctx, slot, version)
	if err != nil {
		return nil, err
	}
	path, err := s.resolveProfileMediaPath(record.cachePath)
	if err != nil {
		return nil, ErrMediaNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrMediaNotFound
	}
	closeOnError := func() (*MediaStream, error) {
		_ = file.Close()
		return nil, ErrMediaNotFound
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxProfileMediaBytes {
		return closeOnError()
	}
	header := make([]byte, 12)
	n, readErr := file.Read(header)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return closeOnError()
	}
	if mime := sniffImageMIME(header[:n]); mime == "" || mime != record.mime {
		return closeOnError()
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return closeOnError()
	}
	hash, err := s.FileHash(ctx, path, info, file)
	if err != nil || hash != record.hash {
		return closeOnError()
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return closeOnError()
	}
	return &MediaStream{File: file, MIMEType: record.mime, ETag: record.hash, Size: info.Size(), LastModified: info.ModTime()}, nil
}

// ReadProfileMedia retains the checked whole-file API used by operator-side
// previews. Public HTTP reads use OpenProfileMedia instead.
func (s *Store) ReadProfileMedia(ctx context.Context, slot, version string) (*ProfileMediaFile, error) {
	stream, err := s.OpenProfileMedia(ctx, slot, version)
	if err != nil {
		return nil, err
	}
	defer stream.File.Close()
	content, err := io.ReadAll(io.LimitReader(stream.File, stream.Size+1))
	if err != nil || int64(len(content)) != stream.Size {
		return nil, ErrMediaNotFound
	}
	return &ProfileMediaFile{Content: content, MIMEType: stream.MIMEType, SHA256: stream.ETag, Size: stream.Size}, nil
}

func (s *Store) lookupProfileMedia(ctx context.Context, slot, version string) (*mediaRecord, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	slot = strings.ToLower(strings.TrimSpace(slot))
	if slot != "avatar" && slot != "banner" {
		return nil, ErrMediaNotFound
	}
	var cachePath, activeHash, activeMIME, status sql.NullString
	var active int64
	err := s.db.QueryRowContext(ctx, `SELECT active_cache_path,active_sha256,active_mime_type,status,is_active FROM profile_media WHERE slot = ? AND is_active = 1 LIMIT 1`, slot).Scan(&cachePath, &activeHash, &activeMIME, &status, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMediaNotReady
	}
	if err != nil {
		return nil, dbErr(err)
	}
	if active == 0 || !cachePath.Valid || !activeHash.Valid || !safeHash(activeHash.String) {
		return nil, ErrMediaNotReady
	}
	if version != "" {
		if !safeHash(version) || version != activeHash.String {
			return nil, ErrMediaVersionNotFound
		}
	}
	mime, ok := allowedMediaMIME(activeMIME.String)
	if !ok || !activeMIME.Valid {
		return nil, ErrMediaNotFound
	}
	return &mediaRecord{cachePath: cachePath.String, mime: mime, hash: activeHash.String}, nil
}

func (s *Store) resolveMediaPath(relative string) (string, error) {
	normalized := strings.ReplaceAll(relative, `\`, "/")
	if normalized == "" || strings.ContainsRune(normalized, 0) || filepath.IsAbs(normalized) {
		return "", ErrMediaNotFound
	}
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		// Empty components are harmless and are normalized by Join; parent
		// components are never accepted from database-controlled paths.
		if part == ".." {
			return "", ErrMediaNotFound
		}
	}
	root := filepath.Clean(s.dataRoot)
	if strings.HasPrefix(normalized, "sha256/") {
		root = filepath.Join(root, "cache", "media")
	} else {
		if len(parts) == 0 || (!cachedMediaSource(parts[0]) && parts[0] != "avatar") {
			return "", ErrMediaNotFound
		}
	}
	candidate := filepath.Join(root, filepath.FromSlash(normalized))
	if !pathInside(candidate, root) {
		return "", ErrMediaNotFound
	}
	return containedRegularPath(candidate, root)
}

func (s *Store) resolveProfileMediaPath(relative string) (string, error) {
	normalized := strings.ReplaceAll(relative, `\`, "/")
	if normalized == "" || strings.ContainsRune(normalized, 0) || filepath.IsAbs(normalized) {
		return "", ErrMediaNotFound
	}
	parts := strings.Split(normalized, "/")
	if len(parts) < 2 || parts[0] != "avatar" {
		return "", ErrMediaNotFound
	}
	for _, part := range parts {
		if part == ".." || part == "" {
			return "", ErrMediaNotFound
		}
	}
	root := filepath.Join(s.dataRoot, "avatar")
	candidate := filepath.Join(s.dataRoot, filepath.FromSlash(normalized))
	if !pathInside(candidate, root) {
		return "", ErrMediaNotFound
	}
	// The selected-media namespace itself must be a real directory. A
	// symlinked avatar root would make an otherwise contained path escape
	// the configured runtime namespace. The data root is compared resolved,
	// since its own path may pass through a symlink (macOS /var, for one).
	resolvedData, err := filepath.EvalSymlinks(s.dataRoot)
	if err != nil {
		return "", ErrMediaNotFound
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || resolvedRoot != filepath.Join(resolvedData, "avatar") {
		return "", ErrMediaNotFound
	}
	return containedRegularPath(candidate, root)
}

func pathInside(candidate, root string) bool {
	candidate, _ = filepath.Abs(candidate)
	root, _ = filepath.Abs(root)
	return candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator))
}

func containedRegularPath(candidate, root string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil || !pathInside(resolved, resolvedRoot) {
		return "", ErrMediaNotFound
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrMediaNotFound
	}
	return resolved, nil
}

func readContainedFile(path string, maxBytes int64) ([]byte, time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || maxBytes > 0 && info.Size() > maxBytes {
		return nil, time.Time{}, errors.New("invalid media file")
	}
	if info.Size() > int64(^uint(0)>>1) {
		return nil, time.Time{}, errors.New("media file too large")
	}
	content, err := io.ReadAll(file)
	if err != nil || int64(len(content)) != info.Size() {
		return nil, time.Time{}, errors.New("media file could not be read")
	}
	return content, info.ModTime(), nil
}

func sniffImageMIME(content []byte) string {
	if len(content) >= 3 && content[0] == 0xff && content[1] == 0xd8 && content[2] == 0xff {
		return "image/jpeg"
	}
	if len(content) >= 8 && string(content[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(content) >= 6 && (string(content[:6]) == "GIF87a" || string(content[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(content) >= 12 && string(content[:4]) == "RIFF" && string(content[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(content) >= 12 && string(content[4:8]) == "ftyp" && (string(content[8:12]) == "avif" || string(content[8:12]) == "avis") {
		return "image/avif"
	}
	return ""
}

// cachedMediaSource reports whether source is a platform namespace the media
// step downloads into: data/<source>/sha256/.
func cachedMediaSource(source string) bool {
	switch source {
	case "x", "youtube", "bilibili", "niconico":
		return true
	}
	return false
}
