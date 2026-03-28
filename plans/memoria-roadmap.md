# Roadmap: memoria

## Context

memoria は Claude Code 向けのプロジェクト認識型ローカル RAG メモリシステム。
docs/specs/ に6つの詳細設計書が完成しており、実装フェーズに移行する。
SPEC.ja.md §16 の4フェーズを、ユーザー要望に基づき15マイルストーンに細分化する。

**変更点（スペックからの差分）**:
- フェーズを15マイルストーンに細分化（オリジナルは4フェーズ）
- LLM enrichment は Ruri v3（embedding model）を使用。生成的なLLM呼び出しは行わず、ヒューリスティックベースで kind/importance/scope を推定
- TDD (Red→Green→Refactor) を全マイルストーンで適用

## Meta

| 項目 | 値 |
|------|---|
| ゴール | Claude Code のセッション横断・プロジェクト横断のローカル長期記憶システムを構築 |
| 成功基準 | `memoria hook session-start` が過去の意思決定を additionalContext として注入できる |
| 制約 | ローカル完結（外部API不要）、Go + Python、SQLite、hook は block しない |
| 対象リポジトリ | /Users/youyo/src/github.com/youyo/memoria |
| 作成日 | 2026-03-27 |
| 最終更新 | 2026-03-27 16:15 |
| ステータス | 未着手 |

## Current Focus

- **マイルストーン**: M01 cli-skeleton
- **直近の完了**: スペック設計書6本 + CLAUDE.md + README
- **次のアクション**: Go module 初期化と Kong CLI skeleton 実装

## 依存関係グラフ

```
M01 → M02 → M03 → M04 ──→ M05 → M06
                     │
                     ├──→ M07 → M08 ──→ M11 → M12 → M13 → M14 → M15
                     │                    ↑
         M02 ──→ M09 ──→ M10 ────────────┘
```

**並列開発可能なペア**:
- M05-M06（hooks）と M07（worker lifecycle）は M04 完了後に並列着手可能
- M09（Python embedding）は M02 完了後いつでも着手可能（Go側と独立）

## Progress

### M01: Go module + Kong CLI 骨格 (`cli-skeleton`)
- [ ] Go module 初期化
- [ ] Kong CLI セットアップ + グローバルフラグ
- [ ] 全サブコマンドの空構造体登録
- [ ] `version` コマンド実装
- [ ] Makefile
- 📄 詳細: plans/memoria-m01-cli-skeleton.md

### M02: 設定ファイル + XDG パス (`config-system`)
- [ ] XDG パス解決
- [ ] config.toml の読み書き
- [ ] `config init/show/path` コマンド
- 📄 詳細: plans/memoria-m02-config-system.md（着手時に生成）

### M03: SQLite スキーマ + マイグレーション (`sqlite-schema`)
- [ ] SQLite ドライバ接続
- [ ] 全テーブル DDL
- [ ] マイグレーション管理
- [ ] `doctor` 基本実装
- 📄 詳細: plans/memoria-m03-sqlite-schema.md（着手時に生成）

### M04: SQLite ベースジョブキュー (`job-queue`)
- [ ] Enqueue / Dequeue / Ack / Fail
- [ ] Retry ロジック
- [ ] 排他制御
- 📄 詳細: plans/memoria-m04-job-queue.md（着手時に生成）

### M05: Stop hook + checkpoint enqueue (`stop-hook`) ✅
- [x] `memoria hook stop` 実装
- [x] Project ID 解決
- [x] checkpoint_ingest enqueue
- 📄 詳細: plans/memoria-m05-stop-hook.md

### M06: SessionEnd hook + transcript enqueue (`session-end-hook`)
- [ ] `memoria hook session-end` 実装
- [ ] session_end_ingest enqueue
- 📄 詳細: plans/memoria-m06-session-end-hook.md（着手時に生成）

### M07: Ingest worker 起動・停止・状態管理 (`ingest-worker-lifecycle`)
- [ ] `memoria daemon ingest` 内部コマンド
- [ ] `worker start/stop/status`
- [ ] File lock + heartbeat + idle timeout
- 📄 詳細: plans/memoria-m07-ingest-worker-lifecycle.md（着手時に生成）

### M08: Ingest worker ジョブ処理ループ (`ingest-worker-loop`)
- [ ] Worker main loop
- [ ] Transcript パーサー
- [ ] Chunking + 重複排除
- [ ] ヒューリスティック enrichment
- [ ] FTS5 同期書き込み
- 📄 詳細: plans/memoria-m08-ingest-worker-loop.md（着手時に生成）

### M09: Python embedding worker (`embedding-worker`)
- [ ] FastAPI + Ruri v3
- [ ] `/health` + `/embed` エンドポイント
- [ ] UDS バインド + idle timeout
- 📄 詳細: plans/memoria-m09-embedding-worker.md（着手時に生成）

### M10: Go ↔ Python UDS 通信統合 (`embedding-integration`)
- [ ] Go 側 embedding client
- [ ] worker start/stop 拡張
- [ ] ensureWorker()
- 📄 詳細: plans/memoria-m10-embedding-integration.md（着手時に生成）

### M11: Ingest に embedding 統合 (`ingest-with-embedding`) ✅
- [x] Ingest loop に embedding 呼び出し追加
- [x] chunk_embeddings 保存
- [x] バッチ embedding
- 📄 詳細: plans/memoria-m11-ingest-with-embedding.md

### M12: SessionStart + UserPrompt retrieval (`retrieval-hooks`) ✅
- [x] `memoria hook session-start` 実装
- [x] `memoria hook user-prompt` 実装
- [x] FTS + Vector + RRF 統合
- [x] Project boost
- [x] `config print-hook`
- 📄 詳細: plans/memoria-m12-retrieval-hooks.md

### M13: プロジェクト識別 + similarity (`project-fingerprint`) ✅
- [x] Fingerprint 生成
- [x] Similarity 計算 + キャッシュ
- [x] TTL 管理 + background job
- 📄 詳細: plans/memoria-m13-project-fingerprint.md

### M14: sqlite-vec 導入 (`sqlite-vec-upgrade`)
- [ ] sqlite-vec 拡張ロード
- [ ] JSON blob → sqlite-vec マイグレーション
- [ ] KNN 検索切り替え
- [ ] `memory reindex`
- 📄 詳細: plans/memoria-m14-sqlite-vec-upgrade.md（着手時に生成）

### M15: Plugin + Release engineering (`release-packaging`)
- [ ] Plugin manifest + Skill 最終版
- [ ] `doctor` 完全版
- [ ] `memory search/get/list/stats`
- [ ] GoReleaser + Homebrew tap
- [ ] CI/CD (GitHub Actions)
- 📄 詳細: plans/memoria-m15-release-packaging.md（着手時に生成）

## Blockers

なし

## Architecture Decisions

| # | 決定 | 理由 | 日付 |
|---|------|------|------|
| 1 | LLM enrichment はヒューリスティックベース | Ruri v3 は embedding 専用モデル。kind/importance/scope の判定はルールベースで実装し、外部 LLM API への依存を排除 | 2026-03-27 |
| 2 | Embedding は JSON blob で先行実装、sqlite-vec は M14 で後付け | 段階的アプローチにより、embedding なしでも基本機能が動作する設計を維持 | 2026-03-27 |
| 3 | M09（Python embedding）は Go 側と並列開発可能 | UDS インターフェースで疎結合。M02 完了後いつでも着手可能 | 2026-03-27 |

## フェーズ対応表

| オリジナルフェーズ | マイルストーン | 概要 |
|---|---|---|
| Phase 1: CLI + Schema + Worker + Hooks + Ingest | M01-M08 | 基盤構築 |
| Phase 2: Retrieval + Embedding + Chunk保存 | M09-M12 | 検索・embedding 統合 |
| Phase 3: Fingerprint + sqlite-vec | M13-M14 | 高度な検索最適化 |
| Phase 4: Plugin + Release | M15 | 配布・リリース |

## サマリーテーブル

| # | スラッグ | 依存 | 推定ファイル数 | 概要 |
|---|---------|------|-------------|------|
| M01 | cli-skeleton | — | 8-10 | Go module + Kong CLI 骨格 |
| M02 | config-system | M01 | 6-8 | 設定ファイル + XDG パス |
| M03 | sqlite-schema | M02 | 8-12 | SQLite スキーマ + マイグレーション |
| M04 | job-queue | M03 | 4-6 | SQLite ベースジョブキュー |
| M05 | stop-hook | M04 | 6-8 | Stop hook + checkpoint enqueue |
| M06 | session-end-hook | M05 | 3-5 | SessionEnd hook + transcript enqueue |
| M07 | ingest-worker-lifecycle | M04 | 8-12 | Ingest worker 起動・停止 |
| M08 | ingest-worker-loop | M07 | 10-14 | Ingest ジョブ処理ループ |
| M09 | embedding-worker | M02 | 4-6 | Python embedding worker (Ruri v3) |
| M10 | embedding-integration | M07,M09 | 6-10 | Go ↔ Python UDS 通信 |
| M11 | ingest-with-embedding | M08,M10 | 4-6 | Ingest に embedding 統合 |
| M12 | retrieval-hooks | M11 | 12-16 | SessionStart + UserPrompt retrieval |
| M13 | project-fingerprint | M12 | 8-10 | プロジェクト識別 + similarity |
| M14 | sqlite-vec-upgrade | M13 | 6-10 | sqlite-vec 導入 |
| M15 | release-packaging | M14 | 12-18 | Plugin + Release engineering |

**合計推定ファイル数**: 106-160

## Changelog

| 日時 | 種別 | 内容 |
|------|------|------|
| 2026-03-27 16:15 | 作成 | ロードマップ初版作成。SPEC.ja.md の4フェーズを15マイルストーンに細分化 |
