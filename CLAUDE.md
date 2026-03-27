# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

**memoria** は Claude Code 向けのプロジェクト認識型ローカル RAG メモリシステム。コーディングセッションから意思決定・制約・失敗・TODO・知見を自動抽出し、SQLite にローカル蓄積する。

現在は **M01 CLI skeleton 完了**。Kong CLI 骨格が実装済み。

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

## Worker 起動方針

- `memoria worker start` で ingest + embedding 両方起動
- 各 hook / retrieval 時に `ensureWorker()` で自己防衛的に再起動（SessionStart だけが起動ポイントではない）
- ingest worker idle timeout: 60秒、embedding worker idle timeout: 600秒
