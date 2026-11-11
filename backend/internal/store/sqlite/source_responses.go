package sqlite

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/yexca/kano-navi/backend/internal/sourcecapture"
)

const (
	SourceResponseSuccessDays          = 30
	SourceResponseFailureDays          = 90
	SourceResponseMaxStoredBytes int64 = 512 << 20
)

type SourceResponseCapture struct {
	ID          int64
	Source      string
	Endpoint    string
	Method      string
	StatusCode  int
	ContentType string
	CapturedAt  string
	RunID       string
	WorkflowID  string
	StepID      string
	Attempt     int
	SHA256      string
	BodyBytes   int64
	StoredBytes int64
	Truncated   bool
	Body        []byte
}

// RecordSourceResponse stores the bytes delivered to the source parser. Its
// failures are intentionally best effort at the transport boundary.
func (s *Store) RecordSourceResponse(ctx context.Context, response sourcecapture.Response) error {
	if err := s.requireWorkflow(); err != nil {
		return err
	}
	if strings.TrimSpace(response.Source) == "" || strings.TrimSpace(response.Endpoint) == "" || response.Method == "" {
		return ErrWorkflowValidation
	}
	body := response.Body
	truncated := response.Truncated
	if int64(len(body)) > sourcecapture.MaxBodyBytes {
		body = body[:sourcecapture.MaxBodyBytes]
		truncated = true
	}
	digest := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(digest[:])
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(body); err != nil {
		return ErrWorkflowStorage
	}
	if err := writer.Close(); err != nil {
		return ErrWorkflowStorage
	}
	capturedAt := response.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ErrWorkflowStorage
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO source_response_captures(source,endpoint,method,status_code,content_type,captured_at,run_id,workflow_id,step_id,attempt,body_sha256,body_bytes,stored_bytes,body_gzip,truncated) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		response.Source, response.Endpoint, response.Method, response.StatusCode, response.ContentType, workflowTime(capturedAt),
		nullableCapture(response.RunID), nullableCapture(response.WorkflowID), nullableCapture(response.StepID), nullableAttempt(response.Attempt),
		bodyHash, len(body), compressed.Len(), compressed.Bytes(), truncated)
	if err != nil {
		return ErrWorkflowStorage
	}
	if err = trimSourceResponseBytes(ctx, tx); err != nil {
		return err
	}
	if tx.Commit() != nil {
		return ErrWorkflowStorage
	}
	return nil
}

func nullableCapture(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableAttempt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

// ListSourceResponseCaptures returns metadata only; response bytes stay out of
// the browser API and are available through the local maintenance command.
func (s *Store) ListSourceResponseCaptures(ctx context.Context, source string, limit int) ([]SourceResponseCapture, error) {
	if s == nil || s.db == nil {
		return nil, ErrWorkflowStorage
	}
	limit = min(max(limit, 1), 200)
	query := "SELECT id,source,endpoint,method,status_code,content_type,captured_at,COALESCE(run_id,''),COALESCE(workflow_id,''),COALESCE(step_id,''),COALESCE(attempt,0),body_sha256,body_bytes,stored_bytes,truncated FROM source_response_captures"
	args := []any{}
	if source != "" {
		query += " WHERE source=?"
		args = append(args, source)
	}
	query += " ORDER BY captured_at DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrWorkflowStorage
	}
	defer rows.Close()
	out := []SourceResponseCapture{}
	for rows.Next() {
		item, scanErr := scanSourceResponseCapture(rows)
		if scanErr != nil {
			return nil, ErrWorkflowStorage
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, ErrWorkflowStorage
	}
	return out, nil
}

type sourceResponseScanner interface{ Scan(...any) error }

func scanSourceResponseCapture(row sourceResponseScanner) (SourceResponseCapture, error) {
	var item SourceResponseCapture
	var truncated int
	err := row.Scan(&item.ID, &item.Source, &item.Endpoint, &item.Method, &item.StatusCode, &item.ContentType,
		&item.CapturedAt, &item.RunID, &item.WorkflowID, &item.StepID, &item.Attempt,
		&item.SHA256, &item.BodyBytes, &item.StoredBytes, &truncated)
	item.Truncated = truncated != 0
	return item, err
}

// ReadSourceResponseCapture returns a verified, decompressed response body.
func (s *Store) ReadSourceResponseCapture(ctx context.Context, id int64) (SourceResponseCapture, error) {
	if s == nil || s.db == nil || id < 1 {
		return SourceResponseCapture{}, ErrWorkflowValidation
	}
	var item SourceResponseCapture
	var truncated int
	var compressed []byte
	err := s.db.QueryRowContext(ctx, "SELECT id,source,endpoint,method,status_code,content_type,captured_at,COALESCE(run_id,''),COALESCE(workflow_id,''),COALESCE(step_id,''),COALESCE(attempt,0),body_sha256,body_bytes,stored_bytes,truncated,body_gzip FROM source_response_captures WHERE id=?", id).Scan(
		&item.ID, &item.Source, &item.Endpoint, &item.Method, &item.StatusCode, &item.ContentType,
		&item.CapturedAt, &item.RunID, &item.WorkflowID, &item.StepID, &item.Attempt,
		&item.SHA256, &item.BodyBytes, &item.StoredBytes, &truncated, &compressed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SourceResponseCapture{}, ErrWorkflowNotFound
		}
		return SourceResponseCapture{}, ErrWorkflowStorage
	}
	item.Truncated = truncated != 0
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return SourceResponseCapture{}, ErrWorkflowStorage
	}
	item.Body, err = io.ReadAll(io.LimitReader(reader, sourcecapture.MaxBodyBytes+1))
	closeErr := reader.Close()
	if err != nil || closeErr != nil || int64(len(item.Body)) > sourcecapture.MaxBodyBytes {
		return SourceResponseCapture{}, ErrWorkflowStorage
	}
	digest := sha256.Sum256(item.Body)
	if hex.EncodeToString(digest[:]) != item.SHA256 || int64(len(item.Body)) != item.BodyBytes {
		return SourceResponseCapture{}, ErrWorkflowStorage
	}
	return item, nil
}

// PruneSourceResponseCaptures applies retention independent of run history
// and enforces the compressed-body storage budget.
func (s *Store) PruneSourceResponseCaptures(ctx context.Context, now time.Time) (int, error) {
	if err := s.requireWorkflow(); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, ErrWorkflowStorage
	}
	defer tx.Rollback()
	failedBefore := workflowTime(now.AddDate(0, 0, -SourceResponseFailureDays))
	succeededBefore := workflowTime(now.AddDate(0, 0, -SourceResponseSuccessDays))
	result, err := tx.ExecContext(ctx, "DELETE FROM source_response_captures WHERE ((status_code<200 OR status_code>=300) AND julianday(captured_at)<julianday(?)) OR (status_code>=200 AND status_code<300 AND julianday(captured_at)<julianday(?))", failedBefore, succeededBefore)
	if err != nil {
		return 0, ErrWorkflowStorage
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, ErrWorkflowStorage
	}
	trimmed, err := trimSourceResponseBytesCount(ctx, tx)
	if err != nil {
		return 0, err
	}
	deleted += int64(trimmed)
	if tx.Commit() != nil {
		return 0, ErrWorkflowStorage
	}
	return int(deleted), nil
}

func trimSourceResponseBytes(ctx context.Context, tx *sql.Tx) error {
	_, err := trimSourceResponseBytesCount(ctx, tx)
	return err
}

func trimSourceResponseBytesCount(ctx context.Context, tx *sql.Tx) (int, error) {
	var total int64
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(stored_bytes),0) FROM source_response_captures").Scan(&total); err != nil {
		return 0, ErrWorkflowStorage
	}
	if total <= SourceResponseMaxStoredBytes {
		return 0, nil
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,stored_bytes FROM source_response_captures ORDER BY captured_at ASC,id ASC")
	if err != nil {
		return 0, ErrWorkflowStorage
	}
	ids := []int64{}
	for total > SourceResponseMaxStoredBytes && rows.Next() {
		var id, size int64
		if err = rows.Scan(&id, &size); err != nil {
			rows.Close()
			return 0, ErrWorkflowStorage
		}
		ids = append(ids, id)
		total -= size
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, ErrWorkflowStorage
	}
	if rows.Close() != nil {
		return 0, ErrWorkflowStorage
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, "DELETE FROM source_response_captures WHERE id=?", id); err != nil {
			return 0, ErrWorkflowStorage
		}
	}
	return len(ids), nil
}
