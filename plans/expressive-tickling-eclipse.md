# fix: user_prompt_ingest 投入不能 + doctor 誤報の修正

## Context

v0.2.4 で enqueue の goroutine レースは修正したが、**そもそも `user_prompt_ingest` が DB の CHECK 制約に含まれていない**ため enqueue が全件 constraint violation で失敗している。加えて doctor コマンドが ingest_worker の fail を集計結果に反映しておらず「All checks passed」と誤報する。

### ログで確認した3つの問題

1. **CHECK 制約違反**: `constraint failed: CHECK constraint failed: job_type IN (...)` — `user_prompt_ingest` が許可リストにない
2. **doctor 誤報**: `[fail] ingest_worker: not running` なのに `All checks passed.` と表示
3. **ingest worker 停止中**: idle timeout で落ちた後、再起動されていない（これは 1. が直れば自然に解消）

## 修正内容

### 1. マイグレーション 0005: jobs CHECK 制約に `user_prompt_ingest` を追加

**ファイル**: `internal/db/migrations/0005_user_prompt_job_type.sql`

SQLite は ALTER TABLE で CHECK 制約を変更できないため、テーブル再作成が必要:

```sql
-- jobs テーブルの CHECK 制約に user_prompt_ingest を追加
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
```

### 2. doctor: ingest_worker / embedding_worker の fail を result.OK に反映

**ファイル**: `internal/cli/doctor.go`

- L156, 168, 171, 174: ingest_worker の各 fail 分岐に `result.OK = false` を追加
- L187: embedding_worker の fail 分岐に `result.OK = false` を追加

### 3. worker 処理ループに `user_prompt_ingest` ハンドラを追加

**ファイル**: `internal/worker/daemon.go` の `processJob()` — `user_prompt_ingest` の case を追加
**ファイル**: `internal/worker/processor.go` — `JobProcessor` に `HandleUserPromptIngest` を追加

※ user_prompt_ingest は現時点では「キューに積む」だけで十分（将来の拡張ポイント）。最小実装として Ack するだけのハンドラで良い。

## 対象ファイル一覧

| ファイル | 変更内容 |
|---|---|
| `internal/db/migrations/0005_user_prompt_job_type.sql` | 新規: CHECK 制約追加マイグレーション |
| `internal/cli/doctor.go` | L156,168,171,174,187: `result.OK = false` 追加 |
| `internal/worker/daemon.go` | `processJob()` に `user_prompt_ingest` case 追加 |
| `internal/worker/processor.go` | `HandleUserPromptIngest` メソッド追加 |

## 検証

1. `make test` — 全テスト green
2. `memoria doctor` — ingest worker 停止時に `Some checks failed.` と表示されること
3. `memoria worker start` → user-prompt hook 発火 → ingest.log に constraint エラーが出ないこと
4. `sqlite3 ~/.local/share/memoria/memoria.db "SELECT sql FROM sqlite_master WHERE name='jobs'"` — CHECK 制約に `user_prompt_ingest` が含まれること
