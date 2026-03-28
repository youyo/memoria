-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Projects
CREATE TABLE IF NOT EXISTS projects (
    project_id              TEXT PRIMARY KEY,
    project_root            TEXT NOT NULL UNIQUE,
    repo_name               TEXT,
    primary_language        TEXT,
    project_summary         TEXT,
    fingerprint_json        TEXT,
    fingerprint_text        TEXT,
    fingerprint_updated_at  TEXT,
    similarity_updated_at   TEXT,
    last_seen_at            TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_projects_last_seen ON projects(last_seen_at);

-- Project aliases (パス変更・symlink 吸収)
CREATE TABLE IF NOT EXISTS project_aliases (
    alias_id    TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    alias_root  TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Project similarity cache
CREATE TABLE IF NOT EXISTS project_similarity (
    project_id          TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    similar_project_id  TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    similarity          REAL NOT NULL CHECK (similarity >= 0.0 AND similarity <= 1.0),
    computed_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (project_id, similar_project_id)
);

-- Sessions
CREATE TABLE IF NOT EXISTS sessions (
    session_id       TEXT PRIMARY KEY,
    project_id       TEXT REFERENCES projects(project_id),
    cwd              TEXT NOT NULL,
    transcript_path  TEXT,
    started_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    ended_at         TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id);

-- Turns (transcript の正規化レコード)
CREATE TABLE IF NOT EXISTS turns (
    turn_id     TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(session_id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(session_id);

-- Chunks (memory 本体)
CREATE TABLE IF NOT EXISTS chunks (
    chunk_id                TEXT PRIMARY KEY,
    session_id              TEXT REFERENCES sessions(session_id),
    project_id              TEXT REFERENCES projects(project_id),
    turn_start_id           TEXT REFERENCES turns(turn_id),
    turn_end_id             TEXT REFERENCES turns(turn_id),
    content                 TEXT NOT NULL,
    summary                 TEXT,
    kind                    TEXT NOT NULL CHECK (kind IN ('decision','constraint','todo','failure','fact','preference','pattern')),
    importance              REAL NOT NULL DEFAULT 0.5 CHECK (importance >= 0.0 AND importance <= 1.0),
    scope                   TEXT NOT NULL DEFAULT 'project' CHECK (scope IN ('project','similarity_shareable','global')),
    project_transferability REAL DEFAULT 0.5,
    keywords_json           TEXT,
    applies_to_json         TEXT,
    content_hash            TEXT NOT NULL,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_content_hash ON chunks(content_hash);
CREATE INDEX IF NOT EXISTS idx_chunks_project ON chunks(project_id);
CREATE INDEX IF NOT EXISTS idx_chunks_kind ON chunks(kind);
CREATE INDEX IF NOT EXISTS idx_chunks_importance ON chunks(importance DESC);
CREATE INDEX IF NOT EXISTS idx_chunks_created ON chunks(created_at DESC);

-- FTS5 仮想テーブル（content / summary / keywords の全文検索）
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content,
    summary,
    keywords,
    content='chunks',
    content_rowid='rowid'
);

-- chunks への trigger で FTS を自動同期
-- TODO(M06): keywords_json は JSON 文字列のまま投入している。
--            M06 で LLM enrichment 実装時にキーワードを展開して投入するよう改善する。
CREATE TRIGGER IF NOT EXISTS chunks_fts_insert AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, content, summary, keywords)
    VALUES (new.rowid, new.content, COALESCE(new.summary, ''), COALESCE(new.keywords_json, ''));
END;

CREATE TRIGGER IF NOT EXISTS chunks_fts_delete AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content, summary, keywords)
    VALUES ('delete', old.rowid, old.content, COALESCE(old.summary, ''), COALESCE(old.keywords_json, ''));
END;

CREATE TRIGGER IF NOT EXISTS chunks_fts_update AFTER UPDATE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, content, summary, keywords)
    VALUES ('delete', old.rowid, old.content, COALESCE(old.summary, ''), COALESCE(old.keywords_json, ''));
    INSERT INTO chunks_fts(rowid, content, summary, keywords)
    VALUES (new.rowid, new.content, COALESCE(new.summary, ''), COALESCE(new.keywords_json, ''));
END;

-- Chunk embeddings (MVP: JSON blob、M14 で sqlite-vec へ移行)
CREATE TABLE IF NOT EXISTS chunk_embeddings (
    chunk_id        TEXT PRIMARY KEY REFERENCES chunks(chunk_id) ON DELETE CASCADE,
    model           TEXT NOT NULL,
    embedding_json  TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Project embeddings (fingerprint text / summary の embedding)
CREATE TABLE IF NOT EXISTS project_embeddings (
    project_id      TEXT PRIMARY KEY REFERENCES projects(project_id) ON DELETE CASCADE,
    model           TEXT NOT NULL,
    embedding_json  TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Jobs (ローカルキュー)
CREATE TABLE IF NOT EXISTS jobs (
    job_id        TEXT PRIMARY KEY,
    job_type      TEXT NOT NULL CHECK (job_type IN ('checkpoint_ingest','session_end_ingest','project_refresh','project_similarity_refresh')),
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

CREATE INDEX IF NOT EXISTS idx_jobs_status_run_after ON jobs(status, run_after);

-- Worker leases (heartbeat 管理)
CREATE TABLE IF NOT EXISTS worker_leases (
    worker_name         TEXT PRIMARY KEY,
    worker_id           TEXT NOT NULL,
    pid                 INTEGER NOT NULL,
    started_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_heartbeat_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    last_progress_at    TEXT,
    current_job_id      TEXT REFERENCES jobs(job_id)
);

-- Worker probes (liveness 確認)
CREATE TABLE IF NOT EXISTS worker_probes (
    probe_id           TEXT PRIMARY KEY,
    worker_name        TEXT NOT NULL,
    target_worker_id   TEXT NOT NULL,
    requested_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    responded_at       TEXT,
    requested_by_pid   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_worker_probes_worker ON worker_probes(worker_name, target_worker_id);
