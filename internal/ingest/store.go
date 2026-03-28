package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// SessionRecord は sessions テーブルへの書き込みデータ。
type SessionRecord struct {
	SessionID      string
	ProjectID      string
	Cwd            string
	TranscriptPath string
	StartedAt      time.Time
	EndedAt        *time.Time
}

// TurnRecord は turns テーブルへの書き込みデータ。
type TurnRecord struct {
	TurnID    string
	SessionID string
	Role      string
	Content   string
	CreatedAt time.Time
}

// ChunkRecord は chunks テーブルへの書き込みデータ。
type ChunkRecord struct {
	ChunkID                string
	SessionID              string
	ProjectID              string
	TurnStartID            string
	TurnEndID              string
	Content                string
	Summary                string
	Kind                   string
	Importance             float64
	Scope                  string
	ProjectTransferability float64
	KeywordsJSON           string
	AppliesToJSON          string
	ContentHash            string
	CreatedAt              time.Time
}

// UpsertSession は sessions テーブルに UPSERT する。
func UpsertSession(ctx context.Context, db *sql.DB, r SessionRecord) error {
	var startedAt string
	if r.StartedAt.IsZero() {
		startedAt = time.Now().UTC().Format(time.RFC3339)
	} else {
		startedAt = r.StartedAt.UTC().Format(time.RFC3339)
	}

	var endedAt *string
	if r.EndedAt != nil {
		s := r.EndedAt.UTC().Format(time.RFC3339)
		endedAt = &s
	}

	var transcriptPath *string
	if r.TranscriptPath != "" {
		transcriptPath = &r.TranscriptPath
	}

	const query = `
INSERT INTO sessions (session_id, project_id, cwd, transcript_path, started_at, ended_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
    project_id = excluded.project_id,
    cwd = excluded.cwd,
    transcript_path = COALESCE(excluded.transcript_path, transcript_path),
    ended_at = COALESCE(excluded.ended_at, ended_at)`

	_, err := db.ExecContext(ctx, query,
		r.SessionID,
		nullableString(r.ProjectID),
		r.Cwd,
		transcriptPath,
		startedAt,
		endedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
}

// InsertTurn は turns テーブルにターンを挿入する。
func InsertTurn(ctx context.Context, db *sql.DB, r TurnRecord) error {
	var createdAt string
	if r.CreatedAt.IsZero() {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	} else {
		createdAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}

	const query = `
INSERT INTO turns (turn_id, session_id, role, content, created_at)
VALUES (?, ?, ?, ?, ?)`

	_, err := db.ExecContext(ctx, query,
		r.TurnID,
		r.SessionID,
		r.Role,
		r.Content,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert turn: %w", err)
	}
	return nil
}

// CountTurnsBySession は指定 session_id の turns 数を返す。
func CountTurnsBySession(ctx context.Context, db *sql.DB, sessionID string) (int, error) {
	const query = `SELECT COUNT(*) FROM turns WHERE session_id = ?`
	var count int
	if err := db.QueryRowContext(ctx, query, sessionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count turns by session: %w", err)
	}
	return count, nil
}

// InsertChunk は chunks テーブルに chunk を挿入する。
// content_hash が重複する場合は DO NOTHING（冪等）。
func InsertChunk(ctx context.Context, db *sql.DB, r ChunkRecord) error {
	var createdAt string
	if r.CreatedAt.IsZero() {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	} else {
		createdAt = r.CreatedAt.UTC().Format(time.RFC3339)
	}

	if r.Scope == "" {
		r.Scope = "project"
	}
	if r.Kind == "" {
		r.Kind = "fact"
	}
	if r.ProjectTransferability == 0 {
		r.ProjectTransferability = 0.5
	}

	kwJSON := r.KeywordsJSON
	if kwJSON == "" {
		kwJSON = "[]"
	}

	appliesToJSON := r.AppliesToJSON
	if appliesToJSON == "" {
		appliesToJSON = "[]"
	}

	const query = `
INSERT INTO chunks (
    chunk_id, session_id, project_id,
    turn_start_id, turn_end_id,
    content, summary, kind, importance, scope,
    project_transferability, keywords_json, applies_to_json,
    content_hash, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(content_hash) DO NOTHING`

	_, err := db.ExecContext(ctx, query,
		r.ChunkID,
		nullableString(r.SessionID),
		nullableString(r.ProjectID),
		nullableString(r.TurnStartID),
		nullableString(r.TurnEndID),
		r.Content,
		r.Summary,
		r.Kind,
		r.Importance,
		r.Scope,
		r.ProjectTransferability,
		kwJSON,
		appliesToJSON,
		r.ContentHash,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert chunk: %w", err)
	}
	return nil
}

// ContentHash はコンテンツの SHA-256 ハッシュを hex 文字列で返す。
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// nullableString は空文字列を nil に変換する（SQL NULL 対応）。
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
