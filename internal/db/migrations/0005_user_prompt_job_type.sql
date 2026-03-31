-- jobs テーブルの CHECK 制約に user_prompt_ingest を追加
-- SQLite は ALTER TABLE で CHECK 制約を変更できないためテーブル再作成
CREATE TABLE jobs_new (
    job_id        TEXT PRIMARY KEY,
    job_type      TEXT NOT NULL CHECK (job_type IN ('checkpoint_ingest','session_end_ingest','project_refresh','project_similarity_refresh','user_prompt_ingest')),
    payload_json  TEXT NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed')),
    retry_count   INTEGER NOT NULL DEFAULT 0,
    max_retries   INTEGER NOT NULL DEFAULT 3,
    run_after     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    started_at    TEXT,
    finished_at   TEXT,
    error_message TEXT,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
INSERT INTO jobs_new SELECT * FROM jobs;
DROP TABLE jobs;
ALTER TABLE jobs_new RENAME TO jobs;
CREATE INDEX IF NOT EXISTS idx_jobs_status_run_after ON jobs(status, run_after);
