# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

**memoria** は Claude Code 向けのプロジェクト認識型ローカル RAG メモリシステム。コーディングセッションから意思決定・制約・失敗・TODO・知見を自動抽出し、SQLite にローカル蓄積する。

**全マイルストーン完了（M01〜M15）**。Kong CLI 骨格 + XDG パス解決 + config.toml 読み書き + config init/show/path コマンド + SQLite スキーマ + マイグレーション管理 + doctor コマンド（完全版）+ SQLite ベースジョブキュー（Enqueue/Dequeue/Ack/Fail/Purge/Stats）+ `memoria hook stop`（checkpoint_ingest enqueue + project ID 解決）+ `memoria hook session-end`（session_end_ingest enqueue + transcript_path 保存）+ ingest worker ライフサイクル管理（daemon ingest / worker start/stop/status / heartbeat / lease / flock / EnsureIngest 本実装）+ ingest worker ジョブ処理ループ（checkpoint_ingest / session_end_ingest 処理 / transcript パーサー / chunker / ヒューリスティック enrichment / chunks/sessions/turns DB 書き込み / SHA-256 重複排除 / FTS5 自動同期）+ **Python embedding worker**（FastAPI + sentence-transformers Ruri v3 / Unix Domain Socket / /embed + /health エンドポイント / idle timeout / PID・lock ファイル管理）+ **Go ↔ Python UDS 通信統合**（internal/embedding.Client / EnsureEmbedding / worker start+stop+status embedding 対応）+ **Ingest に embedding 統合**（chunk 保存後に自動 embedding / chunk_embeddings 保存 / バッチ embedding / embedding worker 未起動時フォールバック）+ **SessionStart/UserPrompt retrieval hooks**（`memoria hook session-start` / `memoria hook user-prompt` / FTS5+Vector+RRF+project boost / `config print-hook`）+ **プロジェクト識別 + similarity（M13）**（fingerprint 生成 / TTL 管理 / project_refresh / project_similarity_refresh background job / hook 統合）+ **ベクトル検索最適化 + memory reindex（M14）**（float32 バイナリ blob 形式 / vectorSearch blob 高速パス / JSON フォールバック / `memoria memory reindex` コマンド）+ **Plugin manifest + memory コマンド + CI/CD 設定（M15）**（`memory search/get/list/stats` 本実装 / doctor 完全版 / `plugin/memoria/manifest.json` / `.goreleaser.yml` / GitHub Actions CI/CD）が実装済み。

## ビルド・テスト・リント

```bash
# ビルド（バージョン情報埋め込み）
make build

# テスト
make test
# または
go test ./...

# リント
make lint
# または
go vet ./...

# クリーン
make clean

# Python embedding worker
uv run python/worker.py
```

## 設計ドキュメント

実装前に必ず参照すること：

| ファイル | 内容 |
|---|---|
| `docs/specs/SPEC.ja.md` | システム全体の設計（主要設計書、438行） |
| `docs/specs/CLI.ja.md` | CLI コマンド設計（Kong framework 使用） |
| `docs/specs/HOOKS.ja.md` | Claude Code hook 契約（入出力・タイムアウト） |
| `docs/specs/SCHEMA.ja.md` | SQLite スキーマ定義 |
| `docs/specs/WORKERS.ja.md` | ingest worker / embedding worker 設計 |
| `docs/specs/RETRIEVAL.ja.md` | 3層 retrieval 設計と scoring |

## アーキテクチャ

```
Claude Code hooks
  ↓
memoria CLI (Go / Kong)
  ↓
ingest worker (Go) ←→ embedding worker (Python / uv / UDS)
  ↓
SQLite (~/.local/share/memoria/)
```

### 主要コンポーネント

- **memoria CLI**: Kong framework による subcommand 構成。JSON 出力デフォルト、`--format text` で人間向け。
- **ingest worker**: queue job を処理し chunk 化・LLM enrichment・DB 書き込みを担当。`memoria daemon ingest` で起動。
- **embedding worker**: Python + sentence-transformers（Ruri v3）。Unix Domain Socket で ingest worker と通信。`uv run` 経由で起動。
- **SQLite**: `~/.local/share/memoria/` に配置。XDG 仕様準拠。

### Hook 統合（4ライフサイクル）

| Hook | タイムアウト | 役割 |
|---|---|---|
| `SessionStart` | 2〜5秒 | 関連メモリを `additionalContext` として注入 |
| `UserPromptSubmit` | 2〜5秒 | プロンプト関連メモリを semantic + FTS で検索 |
| `Stop` | 1〜2秒 | 重要な意思決定を checkpoint として enqueue |
| `SessionEnd` | <1秒 | トランスクリプト全体を ingestion キューに積む |

**hook は絶対に block しない**。retrieval 失敗時は空の context を返す。

### Retrieval 優先順位

`same project > similar project > global`

- **UserPromptSubmit**: semantic relevance + FTS (Reciprocal Rank Fusion) + project boost
- **SessionStart**: project boost + importance + recency + weak semantic

### chunk の種類（kind）

`decision` / `constraint` / `todo` / `failure` / `fact` / `preference` / `pattern`

### chunk の scope

- `project`: 同一プロジェクトのみ
- `similarity_shareable`: 類似プロジェクトと共有
- `global`: 全プロジェクト共有

## 技術スタック

- **Go**: CLI、ingest worker（配布性重視）
- **Python + uv**: embedding worker（ML エコシステム活用）
- **SQLite**: ローカル完結、外部 API 不要
- **Kong**: Go CLI framework
- **sentence-transformers / Ruri v3**: テキスト embedding モデル
- **github.com/BurntSushi/toml**: TOML 設定ファイル読み書き

## パス設計（XDG 準拠）

| 用途 | パス |
|---|---|
| 設定 | `~/.config/memoria/` |
| データ（SQLite） | `~/.local/share/memoria/` |
| 実行時ファイル（PID, UDS） | `~/.local/state/memoria/run/` |
| ログ | `~/.local/state/memoria/logs/` |

## Claude Code plugin

```
repo-root/
├── .claude-plugin/
│   ├── marketplace.json   # マーケットプレイスカタログ
│   └── plugin.json        # プラグインメタデータ
├── hooks/
│   └── hooks.json         # フック定義（4ライフサイクル）
└── skills/
    └── memoria/
        └── SKILL.md       # エージェントスキル
```

インストール: `/plugin` → `youyo/memoria` でマーケットプレイスから追加

## M14 からのハンドオフ（実装済み ベクトル検索最適化 + memory reindex）

- `internal/retrieval/vector.go`: `Float32SliceToBytes(vec)` / `BytesToFloat32Slice(b)` — float32 ↔ little-endian バイナリ変換
- `internal/retrieval/retrieval.go`: `vectorSearch()` — embedding_blob が存在する場合はバイナリ高速パス（JSON フォールバック付き）
  - LIMIT 200 → 500 に拡張
  - blob デコードは JSON パース（55,729 ns）より約 83 倍高速（669 ns @ 768 dim、Apple M4 計測）
- `internal/ingest/embedder.go`: `EmbedChunks()` — 新規 embedding 保存時に JSON + blob の両形式で INSERT
- `internal/cli/memory.go`: `MemoryReindexCmd.Run()` — `memoria memory reindex [--batch-size N] [--dry-run]`
  - `reindexChunkEmbeddings(ctx, db, batchSize, dryRun)` — chunk_embeddings の JSON → blob 変換
  - `reindexProjectEmbeddings(ctx, db, batchSize, dryRun)` — project_embeddings の JSON → blob 変換
  - バッチ処理（デフォルト 100 件）/ dry-run モード / 冪等（既 blob 行はスキップ）
- `internal/db/migrations/0002_embedding_blob.sql`: `chunk_embeddings.embedding_blob BLOB` / `project_embeddings.embedding_blob BLOB` カラム追加
- SQLite スキーマバージョン: 2

### M14 アーキテクチャ決定

| # | 決定 | 理由 |
|---|------|------|
| 1 | sqlite-vec は採用しない（M14） | `modernc.org/sqlite`（Pure Go）は C 拡張不可。mattn/go-sqlite3（CGO）移行は配布性低下のため M14 では見送り |
| 2 | float32 バイナリ blob で最適化 | JSON パースを排除しデコードを 83 倍高速化。embedding_json は後方互換として維持 |
| 3 | vectorSearch LIMIT を 500 に拡張 | blob 化によりパフォーマンス余裕ができたため検索候補数を増加 |

## M13 からのハンドオフ（実装済み プロジェクト識別 + similarity）

- `internal/fingerprint/fingerprint.go`: `Generate(rootPath)` — フィンガープリント生成
  - `DetectPrimaryLanguage(root)` — ファイル拡張子から言語検出（Go/Python/TypeScript 等）
  - `DetectFrameworks(root)` — 設定ファイルの存在からフレームワーク/ツール検出
  - `DetectProjectKind(root)` — プロジェクト種別推定（cli/web/library/infra/unknown）
  - `GenerateFingerprintText(info)` — embedding 対象の自然言語テキスト生成
  - `GenerateFingerprintJSON(info)` — 構造化 JSON 文字列生成
- `internal/project/similarity.go`: `SimilarityManager` — project_similarity/project_embeddings CRUD
  - `NewSimilarityManager(db)` / `GetSimilarProjects(ctx, projectID)` / `UpsertSimilarity(ctx, ...)`
  - `UpsertProjectEmbedding(ctx, ...)` / `GetProjectEmbedding(ctx, ...)` / `GetAllProjectEmbeddings(ctx)`
  - `IsFingerprintFresh(ctx, projectID, ttl)` / `IsSimilarityFresh(ctx, projectID, ttl)`
  - `UpdateFingerprintDB(ctx, projectID, fpJSON, fpText, lang, kind)` — projects テーブル更新
  - TTL 定数: `FingerprintTTL = 24h` / `SimilarityTTL = 7d`
- `internal/project/refresh.go`: hook 用 TTL チェック + 非同期キュー投入
  - `EnsureFreshFingerprint(ctx, db, q, projectID, projectRoot)` — フィンガープリント TTL チェック
  - `EnsureFreshSimilarity(ctx, db, q, projectID)` — similarity TTL チェック
  - `GetSimilarProjectsForHook(ctx, db, q, projectID)` — hook 用 similar projects 取得（TTL チェック付き）
  - `RefreshEnqueuer` インターフェース（queue.Queue が実装）
- `internal/worker/fingerprint_handler.go`: background job ハンドラ
  - `ProjectRefreshHandler.Handle(ctx, job)` — fingerprint 生成 + DB 更新 + embedding 保存
  - `ProjectSimilarityRefreshHandler.Handle(ctx, job)` — コサイン類似度計算 + project_similarity 保存
  - `ProjectRefreshPayload` / `ProjectSimilarityRefreshPayload` — ジョブペイロード型
- `internal/worker/processor.go`: `JobProcessor` インターフェースに `HandleProjectRefresh` / `HandleProjectSimilarityRefresh` を追加
- `internal/worker/daemon.go`: `processJob()` に `project_refresh` / `project_similarity_refresh` case を追加
- `internal/cli/hook.go`: SessionStart/UserPrompt hook で `EnsureFreshFingerprint` + `GetSimilarProjectsForHook` を呼び出す

## M12 からのハンドオフ（実装済み SessionStart/UserPrompt retrieval hooks）

- `internal/retrieval/retrieval.go`: `Retriever` — retrieval エンジン本体
  - `New(db, embedder)` — embedder が nil の場合は FTS only (degraded mode)
  - `SessionStart(ctx, projectID, similarProjects, maxResults)` — project boost + importance + recency
  - `UserPrompt(ctx, projectID, similarProjects, prompt, maxResults)` — FTS + vector + RRF + project boost
  - `FTSSearch(ctx, query, limit)` — FTS5 全文検索
  - `FormatContext(results)` — additionalContext 用テキスト整形
- `internal/retrieval/vector.go`: `CosineSimilarity(a, b)` — JSON blob からの cosine similarity 計算
- `internal/retrieval/rrf.go`: `MergeRRF(lists, k)` — Reciprocal Rank Fusion（k=60）
- `internal/retrieval/boost.go`: `ApplyProjectBoost(results, projectID, similarProjects)` — same project +2.0 / similar project +1.0
- `internal/cli/hook.go`: `HookSessionStartCmd.RunWithReader()` / `HookUserPromptCmd.RunWithReader()` — 本実装済み
  - `HookOutput` / `HookSpecificOutput` — JSON 出力型（公開型）
  - `writeHookOutput(w, eventName, additionalContext)` — hook 共通出力ヘルパー
  - embedding worker 未起動時は FTS only で degraded 動作
  - `MEMORIA_EMBEDDING_SOCK` 環境変数で UDS パスをオーバーライド可能
- `internal/cli/config.go`: `ConfigPrintHookCmd.Run()` — Claude Code の settings.json 向け hooks 設定断片を JSON 出力

## M11 からのハンドオフ（実装済み Ingest に embedding 統合）

- `internal/ingest/embedder.go`: `EmbedClient` インターフェース / `Embedder` インターフェース / `ChunkEmbedder` — chunk_embeddings テーブルへのバッチ embedding 保存
  - `NewChunkEmbedder(client EmbedClient) *ChunkEmbedder`
  - `EmbedChunks(ctx, db, chunkIDs, modelName)` — 既存 embedding スキップ / バッチ一括呼び出し / INSERT OR IGNORE（冪等）
- `internal/worker/checkpoint.go`: `CheckpointHandlerWithEmbedder` — embedding 付き Handler
  - `NewCheckpointHandlerWithEmbedder(db, embedder, model, logf)` — embedding 付きコンストラクタ
  - embedding エラーは非致命的（warn ログのみ、ingest は成功扱い）
- `internal/worker/session_end.go`: `SessionEndHandlerWithEmbedder` — embedding 付き Handler
  - `NewSessionEndHandlerWithEmbedder(db, embedder, model, logf)` — embedding 付きコンストラクタ
- `internal/worker/processor.go`: `NewDefaultJobProcessorWithEmbedding(db, cfg, logf)` — embedding 付き DefaultJobProcessor
- `internal/worker/daemon.go`: `NewIngestDaemonWithEmbedding(db, q, runDir, logDir, idleTimeout, cfg)` — embedding 付き IngestDaemon

## M10 からのハンドオフ（実装済み Go ↔ Python UDS 通信統合）

- `internal/embedding/client.go`: `Client` — UDS HTTP クライアント。`New(socketPath)` / `NewWithHTTPClient(baseURL, httpClient)`（テスト用 DI）/ `Health(ctx)` / `Embed(ctx, texts)`
- `internal/worker/ensure_embedding.go`: `EnsureEmbedding(ctx, cfg)` — health check → spawn → health ポーリング。`spawnEmbeddingWorkerFn` 関数変数でテスト差し替え可能。`buildEmbeddingWorkerArgs(cfg, sockPath, workerScript)` で引数構築を分離
- `internal/cli/worker.go`: `WorkerStatusOutput.Embedding EmbeddingWorkerStatus` フィールド追加 / `checkEmbeddingStatus(ctx, socketPath, runDir)` / `stopWorkerByPID(ctx, pidPath)` 共通ヘルパー / `WorkerStartCmd.Run()` で `EnsureEmbedding` 呼び出し / `WorkerStopCmd.runWithSQL()` で embedding 停止

## M09 からのハンドオフ（実装済み Python embedding worker）

- `python/worker.py`: エントリポイント（argparse: `--uds`, `--model`, `--preload`, `--timeout`, `--pid-file`, `--lock-file`）
- `python/app/main.py`: `create_app(model_name, preload, idle_timeout)` — FastAPI アプリファクトリ、lifespan で preload + IdleTimer 起動
- `python/app/model.py`: `ModelManager` — SentenceTransformer ラッパー、`preload()` / `embed()` / `embed_async()` / `status()`
- `python/app/schemas.py`: `EmbedRequest` / `EmbedResponse` / `HealthResponse` — Pydantic v2 スキーマ（texts: 1〜64件バリデーション）
- `python/app/lifecycle.py`: `IdleTimer` / `PidFileManager` / `LockManager` — idle timeout・PID ファイル・flock ロック
- `python/pyproject.toml`: fastapi, uvicorn, sentence-transformers, httpx, pytest 等の依存定義
- エンドポイント: `POST /embed`（EmbedRequest → EmbedResponse）/ `GET /health`（HealthResponse）
- Unix Domain Socket: uvicorn `--uds` で起動（ingest worker と UDS 通信）
- テスト: `python/tests/` 50 テスト全 green（asgi_lifespan + httpx AsyncClient + MagicMock）

## M08 からのハンドオフ（実装済み ingest worker ジョブ処理ループ）

- `internal/ingest/transcript.go`: `ParseTranscript(path)` — Claude Code JSONL transcript パーサー（best-effort、無効行スキップ）
- `internal/ingest/chunker.go`: `Chunk(input)` — user+assistant ペア単位チャンク化、`SplitLongContent(content)` — MaxChunkBytes (16KiB) 超で分割
- `internal/ingest/enricher.go`: `Enrich(content)` — ヒューリスティック enrichment（kind/importance/scope/keywords/summary）
- `internal/ingest/store.go`: `UpsertSession()` / `InsertTurn()` / `CountTurnsBySession()` / `InsertChunk()` / `ContentHash()` — DB 書き込み + SHA-256 重複排除
- `internal/worker/checkpoint.go`: `CheckpointHandler.Handle(ctx, job)` — checkpoint_ingest 処理
- `internal/worker/session_end.go`: `SessionEndHandler.Handle(ctx, job)` — session_end_ingest 処理（冪等性保証）
- `internal/worker/processor.go`: `JobProcessor` インターフェース + `DefaultJobProcessor`
- `internal/worker/lease.go`: `UpdateLeaseJobID()` / `UpdateLeaseProgress()` を追加
- `internal/worker/daemon.go`: `processJob()` スタブ → 本実装（JobProcessor ディスパッチ、lease 更新）

## M07 からのハンドオフ（実装済み ingest worker ライフサイクル）

- `internal/worker/process.go`: `AcquireLock(path)` / `WritePID(path, pid)` / `ReadPID(path)` / `RemovePID(path)` / `TouchFile(path)` / `FileExists(path)` / `RemoveFile(path)` — syscall.Flock ベースのファイルロック
- `internal/worker/lease.go`: `UpsertLease()` / `DeleteLease()` / `GetLease()` / `CheckLiveness()` / `UpdateHeartbeat()` / `InsertProbe()` / `CheckProbeResponded()` / `RespondToProbes()` / `DeletePendingProbes()` — worker_leases + worker_probes CRUD
- `internal/worker/heartbeat.go`: `RunHeartbeat(ctx, db, workerName, workerID, interval, logf)` — 1 秒間隔で heartbeat goroutine
- `internal/worker/daemon.go`: `IngestDaemon` + `Run(ctx)` — メインループ（flock → PID 書き込み → lease upsert → heartbeat goroutine → watchdog goroutine → Dequeue ループ → idle timeout / stop ファイル停止）
- `internal/worker/ensure.go`: `EnsureIngest(ctx)` 本実装（alive→リターン / suspect→probe / stale/not_running→spawn）
- `internal/cli/daemon.go`: `DaemonCmd` + `DaemonIngestCmd` — `memoria daemon ingest`（hidden コマンド）
- `internal/cli/worker.go`: `WorkerStartCmd` / `WorkerStopCmd` / `WorkerStatusCmd` 本実装
- `testutil.OpenTestDBFull(t)` が `*db.DB` を返すように拡張

## M04 からのハンドオフ（実装済み queue パッケージ）

- `internal/queue` パッケージ: `queue.New(db.SQL())` で Queue を生成
- `queue.Enqueue(ctx, jobType, payloadJSON)` / `queue.EnqueueAt(ctx, jobType, payloadJSON, runAfter)`
- `queue.Dequeue(ctx, workerID)` / `queue.DequeueWithOptions(ctx, workerID, DequeueOptions{StaleTimeout: ...})`
- `queue.Ack(ctx, jobID)` / `queue.Fail(ctx, jobID, errMsg)` / `queue.Purge(ctx, duration)` / `queue.Stats(ctx)`
- BEGIN IMMEDIATE による排他制御（withImmediateTx ヘルパー）
- Backoff: 5s → 30s → 300s（SPEC §7.3 準拠）
- M05（Stop hook）では `q.Enqueue(ctx, queue.JobTypeCheckpointIngest, payload)` で即利用可能

## M03 からのハンドオフ（実装済み DI パターン）

- `Globals.ConfigPath string` (`--config` フラグ / `MEMORIA_CONFIG` 環境変数)
- `*config.Config` は `kong.Bind(cfg)` で全コマンドの `Run()` に注入
- `config.Load()` はファイル不在時に `DefaultConfig()` を返す（エラーなし）
- `config.Save()` は一時ファイル + `os.Rename()` でアトミック書き込み
- `internal/config` パッケージ: `paths.go`（XDG パス）と `config.go`（構造体・Load/Save）
- `*db.DB` は `kong.Bind(database)` で全コマンドの `Run()` に注入可能
- `internal/db` パッケージ: `db.go`（Open/Close/Ping）と `migrate.go`（embed + 版数管理）
- `internal/db/migrations/0001_initial.sql`: 全テーブル DDL（13テーブル + schema_migrations）
- `modernc.org/sqlite v1.46.2`: CGo フリー SQLite ドライバ（FTS5 組み込み済み）
- GOPATH=/Users/youyo で `go get` するとローカルキャッシュから取得可能（TLS 証明書検証問題の回避策）

## Worker 起動方針

- `memoria worker start` で ingest + embedding 両方起動
- 各 hook / retrieval 時に `ensureWorker()` で自己防衛的に再起動（SessionStart だけが起動ポイントではない）
- ingest worker idle timeout: 60秒、embedding worker idle timeout: 600秒
