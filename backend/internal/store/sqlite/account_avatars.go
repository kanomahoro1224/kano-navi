package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/digest"
	xsource "github.com/yexca/kano-navi/backend/internal/source/x"
)

// Modes governing which cached candidate an account resolves to on the board.
const (
	AccountAvatarModeAuto   = "auto"
	AccountAvatarModeManual = "manual"
	AccountAvatarModeHidden = "hidden"
)

// Candidate origins in account_avatar_candidates.
const (
	AccountAvatarOriginSync     = "sync"
	AccountAvatarOriginDiscover = "discover"
	AccountAvatarOriginURL      = "url"
	AccountAvatarOriginUpload   = "upload"
)

var (
	ErrAccountAvatarInvalid  = errors.New("account avatar input is invalid")
	ErrAccountAvatarNotFound = errors.New("account avatar not found")
	ErrAccountAvatarNotReady = errors.New("account avatar candidate is not ready")
)

// AccountAvatarCandidate is one discovered, added, or uploaded avatar image for
// a social account.
type AccountAvatarCandidate struct {
	ID         string
	Source     string
	AccountKey string
	Origin     string
	SourceURL  string
	MediaID    string
	CachePath  string
	MIMEType   string
	ByteSize   int64
	SHA256     string
	Width      int
	Height     int
	Status     string
	LastError  string
	CreatedAt  string
	UpdatedAt  string
}

// PublicURL returns the public media route when this candidate is ready to serve.
func (c AccountAvatarCandidate) PublicURL() string {
	if c.Status != "ready" || !safeHash(c.ID) || !safeHash(c.SHA256) || c.CachePath == "" {
		return ""
	}
	return "/media/account-avatar/" + c.ID + "?v=" + c.SHA256
}

// AccountAvatar is the stored avatar configuration and candidate history for
// one social account.
type AccountAvatar struct {
	Source            string
	AccountKey        string
	AccountHandle     string
	Mode              string
	ActiveCandidateID string
	UpdatedAt         string
	Active            *AccountAvatarCandidate
	Candidates        []AccountAvatarCandidate
}

// NormalizeAccountAvatarKey validates a social account identifier for avatar
// storage and returns the normalized source, lowercase key, and display handle.
func NormalizeAccountAvatarKey(source, handle string) (src, key, display string, ok bool) {
	src = strings.ToLower(strings.TrimSpace(source))
	if src != "x" {
		return "", "", "", false
	}
	display = strings.TrimPrefix(strings.TrimSpace(handle), "@")
	key = strings.ToLower(display)
	if key == "" || len(key) > 64 {
		return "", "", "", false
	}
	for _, r := range key {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if !isAlphaNum && r != '_' && r != '-' && r != '.' {
			return "", "", "", false
		}
	}
	return src, key, display, true
}

// AccountAvatarCandidateID computes a deterministic 64-hex identifier for a
// candidate so public media routes remain opaque and cache-safe.
func AccountAvatarCandidateID(source, accountKey, origin, sourceURL, contentHash string) string {
	if origin == AccountAvatarOriginUpload {
		return digest.SHA256Hex("account-avatar\nupload\n" + source + "\n" + accountKey + "\n" + contentHash)
	}
	return digest.SHA256Hex("account-avatar\nurl\n" + source + "\n" + accountKey + "\n" + sourceURL)
}

func validAccountAvatarMode(mode string) (string, bool) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case AccountAvatarModeAuto, AccountAvatarModeManual, AccountAvatarModeHidden:
		return m, true
	default:
		return "", false
	}
}

// AccountAvatars returns the public avatar lookup keyed by "source:account_key".
// Hidden accounts are present with a nil pointer so callers can suppress
// default portrait fallbacks while unset accounts remain absent.
func (s *Store) AccountAvatars(ctx context.Context) (map[string]*string, error) {
	if s != nil && s.readDB != nil {
		return s.publicReader().AccountAvatars(ctx)
	}
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.source, a.account_key, a.mode,
		       COALESCE(c.id,''), COALESCE(c.status,''), COALESCE(c.cache_path,''), COALESCE(c.sha256,'')
		FROM account_avatars a
		LEFT JOIN account_avatar_candidates c
		  ON c.id = a.active_candidate_id AND c.source = a.source AND c.account_key = a.account_key
	`)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()

	out := map[string]*string{}
	for rows.Next() {
		var source, key, mode, candID, status, cachePath, hash string
		if err := rows.Scan(&source, &key, &mode, &candID, &status, &cachePath, &hash); err != nil {
			return nil, dbErr(err)
		}
		mapKey := source + ":" + key
		if mode == AccountAvatarModeHidden {
			out[mapKey] = nil
			continue
		}
		cand := AccountAvatarCandidate{ID: candID, Status: status, CachePath: cachePath, SHA256: hash}
		if url := cand.PublicURL(); url != "" {
			u := url
			out[mapKey] = &u
		}
	}
	if err := rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return out, nil
}

// ListAccountAvatars returns every stored account avatar state together with
// its candidates ordered newest first.
func (s *Store) ListAccountAvatars(ctx context.Context) ([]AccountAvatar, error) {
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT source, account_key, account_handle, mode, COALESCE(active_candidate_id,''), updated_at
		FROM account_avatars
		ORDER BY source ASC, account_key ASC
	`)
	if err != nil {
		return nil, dbErr(err)
	}
	accounts := []AccountAvatar{}
	byKey := map[string]int{}
	for rows.Next() {
		var item AccountAvatar
		if err := rows.Scan(&item.Source, &item.AccountKey, &item.AccountHandle, &item.Mode, &item.ActiveCandidateID, &item.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, dbErr(err)
		}
		item.Candidates = []AccountAvatarCandidate{}
		byKey[item.Source+":"+item.AccountKey] = len(accounts)
		accounts = append(accounts, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, dbErr(err)
	}
	_ = rows.Close()

	candRows, err := s.db.QueryContext(ctx, `
		SELECT id, source, account_key, origin, COALESCE(source_url,''), COALESCE(media_id,''),
		       COALESCE(cache_path,''), COALESCE(mime_type,''), COALESCE(byte_size,0), COALESCE(sha256,''),
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(last_error,''), created_at, updated_at
		FROM account_avatar_candidates
		ORDER BY updated_at DESC, created_at DESC, id ASC
	`)
	if err != nil {
		return nil, dbErr(err)
	}
	defer candRows.Close()
	for candRows.Next() {
		c, scanErr := scanAccountAvatarCandidate(candRows)
		if scanErr != nil {
			return nil, dbErr(scanErr)
		}
		idx, ok := byKey[c.Source+":"+c.AccountKey]
		if !ok {
			continue
		}
		accounts[idx].Candidates = append(accounts[idx].Candidates, c)
		if accounts[idx].ActiveCandidateID != "" && accounts[idx].ActiveCandidateID == c.ID && c.Status == "ready" {
			copyC := c
			accounts[idx].Active = &copyC
		}
	}
	if err := candRows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return accounts, nil
}

// GetAccountAvatar loads one account's avatar state and candidate history.
func (s *Store) GetAccountAvatar(ctx context.Context, source, handle string) (*AccountAvatar, error) {
	src, key, _, ok := NormalizeAccountAvatarKey(source, handle)
	if !ok {
		return nil, ErrAccountAvatarInvalid
	}
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, ErrUnavailable
	}
	defer tx.Rollback()
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// GetAccountAvatarCandidate loads one candidate by ID within an account.
func (s *Store) GetAccountAvatarCandidate(ctx context.Context, source, handle, candidateID string) (*AccountAvatarCandidate, error) {
	src, key, _, ok := NormalizeAccountAvatarKey(source, handle)
	if !ok || !safeHash(candidateID) {
		return nil, ErrAccountAvatarInvalid
	}
	if s == nil || s.db == nil {
		return nil, ErrUnavailable
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, source, account_key, origin, COALESCE(source_url,''), COALESCE(media_id,''),
		       COALESCE(cache_path,''), COALESCE(mime_type,''), COALESCE(byte_size,0), COALESCE(sha256,''),
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(last_error,''), created_at, updated_at
		FROM account_avatar_candidates
		WHERE id = ? AND source = ? AND account_key = ?
	`, candidateID, src, key)
	cand, err := scanAccountAvatarCandidate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountAvatarNotFound
	}
	if err != nil {
		return nil, dbErr(err)
	}
	return &cand, nil
}

// RecordAccountAvatarURL registers a remote candidate URL for an account and,
// when activate is true and the underlying asset is already cached, selects it.
func (s *Store) RecordAccountAvatarURL(ctx context.Context, source, handle, origin, rawURL string, activate bool) (AccountAvatar, AccountAvatarCandidate, error) {
	if !s.Writable() {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrReadOnly
	}
	src, key, display, ok := NormalizeAccountAvatarKey(source, handle)
	url := normalizeSourceURL(rawURL)
	if !ok || url == "" {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrAccountAvatarInvalid
	}
	if origin != AccountAvatarOriginSync && origin != AccountAvatarOriginDiscover && origin != AccountAvatarOriginURL {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrUnavailable
	}
	defer tx.Rollback()

	candID, bumped, err := recordAccountAvatarCandidateTx(ctx, tx, src, display, origin, url, now)
	if err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, err
	}
	if activate {
		var status string
		if err = tx.QueryRowContext(ctx, `SELECT status FROM account_avatar_candidates WHERE id=?`, candID).Scan(&status); err == nil && status == "ready" {
			changed, selErr := setActiveAccountAvatarTx(ctx, tx, src, key, AccountAvatarModeManual, candID, now)
			if selErr != nil {
				return AccountAvatar{}, AccountAvatarCandidate{}, selErr
			}
			bumped = bumped || changed
		}
	}
	if bumped {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
	}
	var cand AccountAvatarCandidate
	for _, c := range item.Candidates {
		if c.ID == candID {
			cand = c
			break
		}
	}
	return *item, cand, nil
}

// SaveUploadedAccountAvatar records an operator-uploaded image stored under
// data/avatar/ as a ready candidate and locks the account to it when activate is true.
func (s *Store) SaveUploadedAccountAvatar(ctx context.Context, source, handle, cachePath, mime string, byteSize int64, hash string, width, height int, activate bool) (AccountAvatar, AccountAvatarCandidate, error) {
	if !s.Writable() {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrReadOnly
	}
	src, key, display, ok := NormalizeAccountAvatarKey(source, handle)
	cleanMime, mimeOK := allowedMediaMIME(mime)
	if !ok || !mimeOK || !safeHash(hash) || cachePath == "" || byteSize <= 0 {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	candID := AccountAvatarCandidateID(src, key, AccountAvatarOriginUpload, "", hash)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, ErrUnavailable
	}
	defer tx.Rollback()

	if err = ensureAccountAvatarRowTx(ctx, tx, src, key, display, now); err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO account_avatar_candidates (
			id, source, account_key, origin, source_url, media_id, cache_path, mime_type,
			byte_size, sha256, width, height, status, last_error, created_at, updated_at
		) VALUES (?, ?, ?, 'upload', NULL, NULL, ?, ?, ?, ?, ?, ?, 'ready', NULL, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			cache_path=excluded.cache_path,
			mime_type=excluded.mime_type,
			byte_size=excluded.byte_size,
			sha256=excluded.sha256,
			width=excluded.width,
			height=excluded.height,
			status='ready',
			last_error=NULL,
			updated_at=excluded.updated_at
	`, candID, src, key, cachePath, cleanMime, byteSize, hash, nullableInt(width), nullableInt(height), now, now); err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
	}
	if err = observeAccountAvatarCandidateTx(ctx, tx, src, key, candID); err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
	}

	bumped := false
	if activate {
		changed, selErr := setActiveAccountAvatarTx(ctx, tx, src, key, AccountAvatarModeManual, candID, now)
		if selErr != nil {
			return AccountAvatar{}, AccountAvatarCandidate{}, selErr
		}
		bumped = changed
	} else {
		changed, autoErr := promoteLatestReadyAutoTx(ctx, tx, src, key, now)
		if autoErr != nil {
			return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(autoErr)
		}
		bumped = changed
	}
	if bumped {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, AccountAvatarCandidate{}, dbErr(err)
	}
	var cand AccountAvatarCandidate
	for _, c := range item.Candidates {
		if c.ID == candID {
			cand = c
			break
		}
	}
	return *item, cand, nil
}

// SaveDownloadedAccountAvatarCandidate marks a candidate ready after an
// operator-initiated download (and synchronizes its linked media_asset if any).
func (s *Store) SaveDownloadedAccountAvatarCandidate(ctx context.Context, candidateID, cachePath, mime string, byteSize int64, hash string, width, height int, etag, lastModified string, promoteIfAuto bool) (AccountAvatar, error) {
	if !s.Writable() {
		return AccountAvatar{}, ErrReadOnly
	}
	cleanMime, mimeOK := allowedMediaMIME(mime)
	if !safeHash(candidateID) || !mimeOK || !safeHash(hash) || cachePath == "" || byteSize <= 0 {
		return AccountAvatar{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, ErrUnavailable
	}
	defer tx.Rollback()

	var src, key, mediaID string
	err = tx.QueryRowContext(ctx, `SELECT source, account_key, COALESCE(media_id,'') FROM account_avatar_candidates WHERE id=?`, candidateID).Scan(&src, &key, &mediaID)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountAvatar{}, ErrAccountAvatarNotFound
	}
	if err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	bumped := false
	if mediaID != "" {
		var previous string
		if err = tx.QueryRowContext(ctx, `SELECT CASE WHEN status='ready' THEN COALESCE(sha256,'') ELSE '' END FROM media_assets WHERE id=?`, mediaID).Scan(&previous); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
		if previous != "" && previous != hash {
			if err = recordMediaVersion(ctx, tx, mediaID, now); err != nil {
				return AccountAvatar{}, dbErr(err)
			}
		}
		if _, err = tx.ExecContext(ctx, `
			UPDATE media_assets
			SET status='ready', cache_path=?, mime_type=?, byte_size=?, sha256=?, width=?, height=?,
			    etag=?, last_modified=?, last_error=NULL, retry_attempts=0, next_retry_at=NULL,
			    fetched_at=?, last_checked_at=?, updated_at=?
			WHERE id=?
		`, cachePath, cleanMime, byteSize, hash, nullableInt(width), nullableInt(height), nullable(etag), nullable(lastModified), now, now, now, mediaID); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
		excludePromotion := ""
		if !promoteIfAuto {
			excludePromotion = candidateID
		}
		bumped, err = syncAccountAvatarFromMediaAssetTx(ctx, tx, mediaID, cachePath, cleanMime, byteSize, hash, width, height, now, excludePromotion)
		bumped = bumped || previous != hash
	} else {
		bumped, err = updateReadyAccountAvatarCandidateTx(ctx, tx, src, key, candidateID, cachePath, cleanMime, byteSize, hash, width, height, now, promoteIfAuto)
	}
	if err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	if bumped {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	return *item, nil
}

// MarkAccountAvatarCandidateFailed records an operator download failure without
// disturbing an active cached avatar.
func (s *Store) MarkAccountAvatarCandidateFailed(ctx context.Context, candidateID, code string) error {
	if !s.Writable() {
		return ErrReadOnly
	}
	if !safeHash(candidateID) {
		return ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback()
	var prevStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM account_avatar_candidates WHERE id=?`, candidateID).Scan(&prevStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAccountAvatarNotFound
	}
	if err != nil {
		return dbErr(err)
	}
	nextStatus := "failed"
	if prevStatus == "ready" {
		nextStatus = "ready"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE account_avatar_candidates SET status=?, last_error=?, updated_at=? WHERE id=?`, nextStatus, nullable(code), now, candidateID); err != nil {
		return dbErr(err)
	}
	return dbErr(tx.Commit())
}

// SetAccountAvatarMode switches an account between auto, manual, and hidden.
// Switching back to auto promotes the newest ready candidate automatically.
func (s *Store) SetAccountAvatarMode(ctx context.Context, source, handle, mode string) (AccountAvatar, error) {
	if !s.Writable() {
		return AccountAvatar{}, ErrReadOnly
	}
	src, key, display, ok := NormalizeAccountAvatarKey(source, handle)
	validMode, modeOK := validAccountAvatarMode(mode)
	if !ok || !modeOK {
		return AccountAvatar{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, ErrUnavailable
	}
	defer tx.Rollback()

	if err = ensureAccountAvatarRowTx(ctx, tx, src, key, display, now); err != nil {
		return AccountAvatar{}, dbErr(err)
	}

	var prevMode, prevActive string
	if err = tx.QueryRowContext(ctx, `SELECT mode, COALESCE(active_candidate_id,'') FROM account_avatars WHERE source=? AND account_key=?`, src, key).Scan(&prevMode, &prevActive); err != nil {
		return AccountAvatar{}, dbErr(err)
	}

	nextActive := prevActive
	if validMode == AccountAvatarModeAuto {
		var latestReady sql.NullString
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM account_avatar_candidates
			WHERE source=? AND account_key=? AND status='ready'
			ORDER BY observation_order DESC, updated_at DESC, created_at DESC, id ASC LIMIT 1
		`, src, key).Scan(&latestReady)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return AccountAvatar{}, dbErr(err)
		}
		if latestReady.Valid && latestReady.String != "" {
			nextActive = latestReady.String
		} else {
			nextActive = ""
		}
	}

	if _, err = tx.ExecContext(ctx, `UPDATE account_avatars SET mode=?, active_candidate_id=?, updated_at=? WHERE source=? AND account_key=?`,
		validMode, nullable(nextActive), now, src, key); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	if prevMode != validMode || prevActive != nextActive {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	return *item, nil
}

// SelectAccountAvatarCandidate activates one ready candidate and locks the
// account in manual mode so automatic sync preserves the choice.
func (s *Store) SelectAccountAvatarCandidate(ctx context.Context, source, handle, candidateID string) (AccountAvatar, error) {
	if !s.Writable() {
		return AccountAvatar{}, ErrReadOnly
	}
	src, key, display, ok := NormalizeAccountAvatarKey(source, handle)
	if !ok || !safeHash(candidateID) {
		return AccountAvatar{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, ErrUnavailable
	}
	defer tx.Rollback()

	if err = ensureAccountAvatarRowTx(ctx, tx, src, key, display, now); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM account_avatar_candidates WHERE id=? AND source=? AND account_key=?`, candidateID, src, key).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountAvatar{}, ErrAccountAvatarNotFound
	}
	if err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	if status != "ready" {
		return AccountAvatar{}, ErrAccountAvatarNotReady
	}
	changed, err := setActiveAccountAvatarTx(ctx, tx, src, key, AccountAvatarModeManual, candidateID, now)
	if err != nil {
		return AccountAvatar{}, err
	}
	if changed {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	return *item, nil
}

// DeleteAccountAvatarCandidate removes one candidate from an account's history.
// When the deleted candidate was active, auto mode falls back to the next ready
// candidate while manual mode clears the active avatar slot.
func (s *Store) DeleteAccountAvatarCandidate(ctx context.Context, source, handle, candidateID string) (AccountAvatar, error) {
	if !s.Writable() {
		return AccountAvatar{}, ErrReadOnly
	}
	src, key, _, ok := NormalizeAccountAvatarKey(source, handle)
	if !ok || !safeHash(candidateID) {
		return AccountAvatar{}, ErrAccountAvatarInvalid
	}
	now := workflowTime(time.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AccountAvatar{}, ErrUnavailable
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM account_avatar_candidates WHERE id=? AND source=? AND account_key=?`, candidateID, src, key)
	if err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return AccountAvatar{}, ErrAccountAvatarNotFound
	}

	var mode, activeID string
	err = tx.QueryRowContext(ctx, `SELECT mode, COALESCE(active_candidate_id,'') FROM account_avatars WHERE source=? AND account_key=?`, src, key).Scan(&mode, &activeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AccountAvatar{}, dbErr(err)
	}
	bumped := false
	if err == nil && activeID == candidateID {
		nextID := ""
		if mode == AccountAvatarModeAuto {
			_ = tx.QueryRowContext(ctx, `
				SELECT id FROM account_avatar_candidates
				WHERE source=? AND account_key=? AND status='ready'
				ORDER BY observation_order DESC, updated_at DESC, created_at DESC, id ASC LIMIT 1
			`, src, key).Scan(&nextID)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE account_avatars SET active_candidate_id=?, updated_at=? WHERE source=? AND account_key=?`, nullable(nextID), now, src, key); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
		if mode != AccountAvatarModeHidden {
			bumped = true
		}
	}
	if bumped {
		if err = bumpRevision(ctx, tx, now); err != nil {
			return AccountAvatar{}, dbErr(err)
		}
	}
	item, err := loadAccountAvatarTx(ctx, tx, src, key)
	if err != nil {
		return AccountAvatar{}, err
	}
	if err = tx.Commit(); err != nil {
		return AccountAvatar{}, dbErr(err)
	}
	return *item, nil
}

func recordAccountAvatarCandidateTx(ctx context.Context, tx *sql.Tx, source, handle, origin, rawURL, now string) (string, bool, error) {
	src, key, display, ok := NormalizeAccountAvatarKey(source, handle)
	clean := normalizeSourceURL(rawURL)
	if !ok || clean == "" {
		return "", false, nil
	}
	mediaID, url, idErr := mediaIdentity(clean)
	if idErr != nil {
		return "", false, nil
	}
	if err := ensureAccountAvatarRowTx(ctx, tx, src, key, display, now); err != nil {
		return "", false, dbErr(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO media_assets (id, source, source_url, status, created_at, updated_at)
		VALUES (?, 'x', ?, 'pending', ?, ?)
	`, mediaID, url, now, now); err != nil {
		return "", false, dbErr(err)
	}

	var status, cachePath, mimeType, hash, lastError sql.NullString
	var byteSize, width, height sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT status, cache_path, mime_type, byte_size, sha256, width, height, last_error
		FROM media_assets WHERE id=?
	`, mediaID).Scan(&status, &cachePath, &mimeType, &byteSize, &hash, &width, &height, &lastError); err != nil {
		return "", false, dbErr(err)
	}

	candID := AccountAvatarCandidateID(src, key, origin, url, "")
	candStatus := "pending"
	if status.String == "ready" && cachePath.String != "" && safeHash(hash.String) {
		candStatus = "ready"
	} else if status.String == "failed" || status.String == "missing" {
		candStatus = "failed"
	}

	// Preserve existing candidate ID when the exact source URL already exists under
	// this account so re-syncs stay idempotent across sync vs discover origins.
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM account_avatar_candidates WHERE source=? AND account_key=? AND source_url=?`, src, key, url).Scan(&existingID)
	if err == nil && existingID != "" {
		candID = existingID
		if err = observeAccountAvatarCandidateTx(ctx, tx, src, key, candID); err != nil {
			return "", false, dbErr(err)
		}
		if candStatus == "ready" {
			bumped, promoErr := updateReadyAccountAvatarCandidateTx(ctx, tx, src, key, candID, cachePath.String, mimeType.String, byteSize.Int64, hash.String, int(width.Int64), int(height.Int64), now, true)
			return candID, bumped, promoErr
		}
		return candID, false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, dbErr(err)
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO account_avatar_candidates (
			id, source, account_key, origin, source_url, media_id, cache_path, mime_type,
			byte_size, sha256, width, height, status, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, candID, src, key, origin, url, mediaID, nullable(cachePath.String), nullable(mimeType.String),
		nullInt64(byteSize), nullable(hash.String), nullInt64(width), nullInt64(height), candStatus, nullable(lastError.String), now, now); err != nil {
		return "", false, dbErr(err)
	}
	if err = observeAccountAvatarCandidateTx(ctx, tx, src, key, candID); err != nil {
		return "", false, dbErr(err)
	}

	if candStatus == "ready" {
		bumped, promoErr := promoteLatestReadyAutoTx(ctx, tx, src, key, now)
		return candID, bumped, promoErr
	}
	return candID, false, nil
}

func syncAccountAvatarFromMediaAssetTx(ctx context.Context, tx *sql.Tx, mediaID, cachePath, mime string, byteSize int64, hash string, width, height int, now, excludePromotion string) (bool, error) {
	if !safeHash(mediaID) || cachePath == "" || !safeHash(hash) {
		return false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, source, account_key FROM account_avatar_candidates WHERE media_id=?`, mediaID)
	if err != nil {
		return false, dbErr(err)
	}
	type target struct{ id, src, key string }
	var targets []target
	for rows.Next() {
		var t target
		if scanErr := rows.Scan(&t.id, &t.src, &t.key); scanErr != nil {
			_ = rows.Close()
			return false, dbErr(scanErr)
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return false, dbErr(err)
	}
	_ = rows.Close()

	bumped := false
	for _, t := range targets {
		changed, promoErr := updateReadyAccountAvatarCandidateTx(ctx, tx, t.src, t.key, t.id, cachePath, mime, byteSize, hash, width, height, now, t.id != excludePromotion)
		if promoErr != nil {
			return false, dbErr(promoErr)
		}
		bumped = bumped || changed
	}
	return bumped, nil
}

func updateReadyAccountAvatarCandidateTx(ctx context.Context, tx *sql.Tx, src, key, candidateID, cachePath, mime string, byteSize int64, hash string, width, height int, now string, promote bool) (bool, error) {
	var visibleChange bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM account_avatars a JOIN account_avatar_candidates c ON c.id=a.active_candidate_id
			WHERE c.id=? AND a.mode<>'hidden' AND (c.status<>'ready' OR COALESCE(c.sha256,'')<>?))
	`, candidateID, hash).Scan(&visibleChange); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_avatar_candidates
		SET cache_path=?, mime_type=?, byte_size=?, sha256=?, width=?, height=?, status='ready', last_error=NULL, updated_at=?
		WHERE id=?
	`, cachePath, mime, byteSize, hash, nullableInt(width), nullableInt(height), now, candidateID); err != nil {
		return false, err
	}
	if !promote {
		return visibleChange, nil
	}
	changed, err := promoteLatestReadyAutoTx(ctx, tx, src, key, now)
	return visibleChange || changed, err
}

// Observation order changes only when a URL is reported or an image is uploaded,
// never when a download or retry finishes. A per-account counter avoids clock ties.
func observeAccountAvatarCandidateTx(ctx context.Context, tx *sql.Tx, src, key, candidateID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE account_avatar_candidates
		SET observation_order=(SELECT COALESCE(MAX(observation_order),0)+1 FROM account_avatar_candidates WHERE source=? AND account_key=?)
		WHERE id=?`, src, key, candidateID)
	return err
}

func failAccountAvatarFromMediaAssetTx(ctx context.Context, tx *sql.Tx, mediaID, lastError, now string) error {
	if !safeHash(mediaID) {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE account_avatar_candidates
		SET status=CASE WHEN status='ready' THEN 'ready' ELSE 'failed' END,
		    last_error=?, updated_at=?
		WHERE media_id=?
	`, nullable(lastError), now, mediaID)
	return dbErr(err)
}

func ensureAccountAvatarRowTx(ctx context.Context, tx *sql.Tx, src, key, display, now string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_avatars (source, account_key, account_handle, mode, updated_at)
		VALUES (?, ?, ?, 'auto', ?)
		ON CONFLICT(source, account_key) DO UPDATE SET
			account_handle=CASE WHEN excluded.account_handle<>'' THEN excluded.account_handle ELSE account_avatars.account_handle END
	`, src, key, display, now); err != nil {
		return err
	}
	return nil
}

func promoteLatestReadyAutoTx(ctx context.Context, tx *sql.Tx, src, key, now string) (bool, error) {
	var mode, activeID string
	err := tx.QueryRowContext(ctx, `SELECT mode, COALESCE(active_candidate_id,'') FROM account_avatars WHERE source=? AND account_key=?`, src, key).Scan(&mode, &activeID)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return false, err
	}
	if mode != AccountAvatarModeAuto {
		return false, nil
	}
	var candID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM account_avatar_candidates
		WHERE source=? AND account_key=? AND status='ready'
		ORDER BY observation_order DESC, updated_at DESC, created_at DESC, id ASC LIMIT 1`, src, key).Scan(&candID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || activeID == candID {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE account_avatars SET active_candidate_id=?, updated_at=? WHERE source=? AND account_key=?`, candID, now, src, key); err != nil {
		return false, err
	}
	return true, nil
}

func setActiveAccountAvatarTx(ctx context.Context, tx *sql.Tx, src, key, mode, candID, now string) (bool, error) {
	var prevMode, prevActive string
	if err := tx.QueryRowContext(ctx, `SELECT mode, COALESCE(active_candidate_id,'') FROM account_avatars WHERE source=? AND account_key=?`, src, key).Scan(&prevMode, &prevActive); err != nil {
		return false, dbErr(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE account_avatars SET mode=?, active_candidate_id=?, updated_at=? WHERE source=? AND account_key=?`,
		mode, nullable(candID), now, src, key); err != nil {
		return false, dbErr(err)
	}
	return prevMode != mode || prevActive != candID, nil
}

func loadAccountAvatarTx(ctx context.Context, tx *sql.Tx, src, key string) (*AccountAvatar, error) {
	var item AccountAvatar
	var activeID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT source, account_key, account_handle, mode, active_candidate_id, updated_at
		FROM account_avatars WHERE source=? AND account_key=?
	`, src, key).Scan(&item.Source, &item.AccountKey, &item.AccountHandle, &item.Mode, &activeID, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountAvatarNotFound
	}
	if err != nil {
		return nil, dbErr(err)
	}
	item.ActiveCandidateID = activeID.String
	item.Candidates = []AccountAvatarCandidate{}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, source, account_key, origin, COALESCE(source_url,''), COALESCE(media_id,''),
		       COALESCE(cache_path,''), COALESCE(mime_type,''), COALESCE(byte_size,0), COALESCE(sha256,''),
		       COALESCE(width,0), COALESCE(height,0), status, COALESCE(last_error,''), created_at, updated_at
		FROM account_avatar_candidates
		WHERE source=? AND account_key=?
		ORDER BY updated_at DESC, created_at DESC, id ASC
	`, src, key)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		c, scanErr := scanAccountAvatarCandidate(rows)
		if scanErr != nil {
			return nil, dbErr(scanErr)
		}
		item.Candidates = append(item.Candidates, c)
		if item.ActiveCandidateID != "" && item.ActiveCandidateID == c.ID && c.Status == "ready" {
			copyC := c
			item.Active = &copyC
		}
	}
	if err = rows.Err(); err != nil {
		return nil, dbErr(err)
	}
	return &item, nil
}

type accountAvatarRowScanner interface {
	Scan(dest ...any) error
}

func scanAccountAvatarCandidate(row accountAvatarRowScanner) (AccountAvatarCandidate, error) {
	var c AccountAvatarCandidate
	err := row.Scan(
		&c.ID, &c.Source, &c.AccountKey, &c.Origin, &c.SourceURL, &c.MediaID,
		&c.CachePath, &c.MIMEType, &c.ByteSize, &c.SHA256, &c.Width, &c.Height,
		&c.Status, &c.LastError, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func recordResultAccountAvatarsTx(ctx context.Context, tx *sql.Tx, account string, result xsource.Result, now string) (bool, error) {
	collected := map[string]string{}
	add := func(handle, avatar string) {
		_, key, _, ok := NormalizeAccountAvatarKey("x", handle)
		if !ok {
			return
		}
		if _, exists := collected[key]; exists {
			return
		}
		if clean := xsource.NormalizeAvatarURL(avatar); clean != "" {
			collected[key] = clean
		}
	}
	for handle, avatar := range result.AccountAvatars {
		add(handle, avatar)
	}
	for _, p := range result.Posts {
		handle := p.AuthorScreenName
		if handle == "" {
			handle = p.AccountHandle
		}
		if handle == "" {
			handle = account
		}
		add(handle, p.AuthorAvatarURL)
		if p.Quote != nil {
			add(p.Quote.AuthorHandle, p.Quote.AuthorAvatarURL)
		}
	}
	for _, r := range result.Reposts {
		if r.Status != nil {
			handle := r.Status.AuthorHandle
			if handle == "" {
				handle = r.Author
			}
			add(handle, r.Status.AuthorAvatarURL)
		}
	}
	if len(collected) == 0 {
		return false, nil
	}
	keys := make([]string, 0, len(collected))
	for k := range collected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bumped := false
	for _, k := range keys {
		_, changed, err := recordAccountAvatarCandidateTx(ctx, tx, "x", k, AccountAvatarOriginSync, collected[k], now)
		if err != nil {
			return false, err
		}
		bumped = bumped || changed
	}
	return bumped, nil
}

func nullableInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullInt64(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}
