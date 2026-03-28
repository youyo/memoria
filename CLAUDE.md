# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

**memoria** は Claude Code 向けのプロジェクト認識型ローカル RAG メモリシステム。コーディングセッションから意思決定・制約・失敗・TODO・知見を自動抽出し、SQLite にローカル蓄積する。

現在は **M07 ingest-worker-lifecycle 完了**。Kong CLI 骨格 + XDG パス解決 + config.toml 読み書き + config init/show/path コマンド + SQLite スキーマ + マイグレーション管理 + doctor コマンド + SQLite ベースジョブキュー（Enqueue/Dequeue/Ack/Fail/Purge/Stats）+ `memoria hook stop`（checkpoint_ingest enqueue + project ID 解決）+ `memoria hook session-end`（session_end_ingest enqueue + transcript_path 保存）+ ingest worker ライフサイクル管理（daemon ingest / worker start/stop/status / heartbeat / lease / flock / EnsureIngest 本実装）が実装済み。

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

# Python embedding worker（予定）
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
plugin/memoria/
├── manifest.json   # hook コマンドと skill パスを定義
└── README.md
```

インストール: `cp -r plugin/memoria ~/.claude/plugins/`

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
