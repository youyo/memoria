# memoria SQLite スキーマ詳細設計

## 全体方針

SQLite は memoria の唯一の永続ストアであり、以下を格納する。

- プロジェクトメタ情報
- 類似プロジェクト情報
- セッション / transcript 由来の memory chunks
- job queue
- worker lease / probe 状態
- embedding / FTS インデックス

## projects

各プロジェクトの fingerprint と更新時刻を保持する。

主なカラム:

- `project_id`
- `project_root`
- `repo_name`
- `primary_language`
- `project_summary`
- `fingerprint_json`
- `fingerprint_text`
- `fingerprint_updated_at`
- `similarity_updated_at`
- `last_seen_at`

## project_aliases

パス変更や symlink を吸収するための別名テーブル。

## project_similarity

プロジェクト間の類似度キャッシュ。

- `project_id`
- `similar_project_id`
- `similarity`
- `computed_at`

## sessions

Claude Code セッション単位のメタ情報。

- `session_id`
- `project_id`
- `cwd`
- `transcript_path`
- `started_at`
- `ended_at`

## turns

正規化された transcript ターン。

- `turn_id`
- `session_id`
- `role`
- `content`
- `created_at`

## chunks

memory 本体。

- `chunk_id`
- `session_id`
- `project_id`
- `turn_start_id`
- `turn_end_id`
- `content`
- `summary`
- `kind`
- `importance`
- `scope`
- `project_transferability`
- `keywords_json`
- `applies_to_json`
- `content_hash`
- `created_at`

### kind

- `decision`
- `constraint`
- `todo`
- `failure`
- `fact`
- `preference`
- `pattern`

### scope

- `project`
- `similarity_shareable`
- `global`

## chunks_fts

全文検索用の FTS5 テーブル。`content`, `summary`, `keywords` を対象にする。

## chunk_embeddings

chunk ごとの embedding を保持する。MVP では JSON 文字列保存、将来的に sqlite-vec へ移行可能なように抽象化する。

## project_embeddings

project fingerprint text / summary の embedding を保持する。

## jobs

ローカル queue。

job type:

- `checkpoint_ingest`
- `session_end_ingest`
- `project_refresh`
- `project_similarity_refresh`

status:

- `queued`
- `running`
- `done`
- `failed`

## worker_leases

共有 worker の heartbeat 管理。

- `worker_name`
- `worker_id`
- `pid`
- `started_at`
- `last_heartbeat_at`
- `last_progress_at`
- `current_job_id`

## worker_probes

heartbeat 遅延時の stronger check 用。

- `probe_id`
- `worker_name`
- `target_worker_id`
- `requested_at`
- `responded_at`
- `requested_by_pid`

## TTL

- project fingerprint TTL: 24h
- project similarity TTL: 7d

TTL 切れ時は同期更新ではなく background job を enqueue する。
