# memoria 超詳細設計書

## 1. 概要

memoria は、Claude Code 向けのローカル長期記憶システムである。

主目的は、コーディングセッション中の重要な意思決定、制約、失敗、TODO、再利用可能な知見をローカルに蓄積し、将来の Claude Code セッションで自動的に再利用できるようにすることにある。

特徴は次の通り。

- Claude Code plugin と hook に統合される
- Go CLI を中心とした実装
- SQLite ベースのローカル完結構成
- Python embedding worker を補助的に利用
- same project / similar project / global の 3 層 retrieval
- Stop と SessionEnd を組み合わせた堅牢な保存戦略

## 2. 前提と設計思想

### 2.1 Claude Code 単体の限界

Claude Code にはプロジェクト横断・セッション横断のネイティブ長期記憶はない。したがって、memoria は Claude の外部に long-term memory を構築する。

### 2.2 なぜ外部 RAG か

memoria は、保存された知識を検索し、それを Claude の prompt 文脈に注入する。よって設計カテゴリとしては local / personal / incremental RAG に属する。

### 2.3 Go CLI + Python worker

アプリケーション本体は Go が担当し、モデル実行だけ Python に逃がす。

- Go: CLI, hooks, queue, DB, ingest orchestration, retrieval
- Python: embedding のみ

この分離により、配布性と ML エコシステムの両方を取る。

## 3. システム全体構成

```text
Claude Code
  -> plugin (hooks + skill)
    -> memoria CLI (Go / Kong)
      -> ingest worker (Go)
      -> embedding worker (Python / uv / UDS)
      -> SQLite
```

### 3.1 Claude Code plugin

plugin は次を配布する。

- hooks
- skill
- marketplace での導入導線

plugin 自体は実処理を持たず、`memoria hook ...` を呼ぶだけとする。

### 3.2 memoria CLI

CLI は `memoria` 単一バイナリで提供する。Kong により subcommand を構成する。

### 3.3 ingest worker

共有 Go デーモン。queue から job を取り出し、chunk 化、LLM enrichment、embedding 呼び出し、DB 書き込みを行う。

### 3.4 embedding worker

共有 Python デーモン。`uv run` により起動し、UDS 経由で `POST /embed` を受ける。

## 4. パス設計

Linux 風固定とする。XDG 可変対応は行わない。

```text
~/.config/memoria/
~/.local/share/memoria/
~/.local/state/memoria/
```

### 4.1 config

- `~/.config/memoria/config.toml`

### 4.2 data

- `~/.local/share/memoria/memoria.db`
- `~/.local/share/memoria/python/`
- `~/.local/share/memoria/cache/`

### 4.3 state

- `~/.local/state/memoria/run/`
- `~/.local/state/memoria/logs/`
- `~/.local/state/memoria/run/embedding.sock`

## 5. CLI 設計

トップレベル構成:

```text
memoria hook ...
memoria worker ...
memoria memory ...
memoria config ...
memoria completion ...
memoria plugin ...
memoria doctor
memoria version
```

### 5.1 hook

- `session-start`
- `user-prompt`
- `stop`
- `session-end`

### 5.2 worker

- `start`
- `stop`
- `restart`
- `status`

### 5.3 memory

- `search`
- `get`
- `list`
- `stats`
- `reindex`

### 5.4 config

- `init`
- `show`
- `path`
- `print-hook`

## 6. Hook 詳細

### 6.1 SessionStart

- embedding worker を ensure
- current project を解決
- same project / similar project / global から候補を取得
- `additionalContext` を返す

### 6.2 UserPromptSubmit

- prompt を embedding
- FTS + vector + boost で検索
- `additionalContext` を返す

### 6.3 Stop

- `last_assistant_message` を受ける
- checkpoint 保存対象なら `checkpoint_ingest` を enqueue
- ingest worker を ensure

### 6.4 SessionEnd

- transcript path を受ける
- `session_end_ingest` を enqueue
- ingest worker を ensure

### 6.5 SessionEnd は必須保証ではない

通常経路では呼ばれても、クラッシュや強制終了まで保証されない前提で設計する。このため Stop を最初から採用する。

## 7. Queue と ingest フロー

### 7.1 job types

- `checkpoint_ingest`
- `session_end_ingest`
- `project_refresh`
- `project_similarity_refresh`

### 7.2 典型フロー

```text
Stop hook
  -> checkpoint_ingest enqueue
  -> ingest worker ensure

SessionEnd hook
  -> session_end_ingest enqueue
  -> ingest worker ensure

ingest worker
  -> dequeue
  -> parse / normalize
  -> chunk split
  -> LLM enrichment
  -> embedding
  -> DB write
```

### 7.3 retry

- max 3 回
- backoff: 5s -> 30s -> 300s

## 8. Worker 設計

### 8.1 ingest worker

- shared daemon
- SQLite `worker_leases` heartbeat
- idle timeout 60 秒
- stop file 監視

### 8.2 embedding worker

- shared daemon
- uv 前提
- UDS デフォルト
- idle timeout 600 秒
- `/health` と `/embed`

### 8.3 起動競合

- file lock で防止
- 起動後は SQLite lease / UDS health で確認

### 8.4 liveness

- alive: heartbeat 3 秒以内
- suspect: 3〜10 秒
- stale: 10 秒超
- suspect 時は SQLite probe

## 9. データモデル

詳細は `SCHEMA.ja.md` に分割しているが、中心概念だけ再掲する。

### 9.1 projects

project fingerprint と TTL 管理を担う。

### 9.2 project_similarity

類似プロジェクトのキャッシュ。SessionStart / UserPromptSubmit で boost に利用する。

### 9.3 sessions / turns / chunks

- session 単位メタデータ
- transcript 由来の turns
- 再利用単位としての chunks

### 9.4 jobs

ローカル queue。

### 9.5 worker_leases / worker_probes

共有 worker 管理。

## 10. プロジェクト識別と fingerprint

### 10.1 project_id

repo root 優先で決める。

1. git root
2. fallback で cwd
3. root path hash -> `project_id`

### 10.2 fingerprint

材料:

- primary language
- framework
- project kind
- package manager / build tool
- 重要ファイルの存在
- project summary

### 10.3 TTL

- fingerprint TTL: 24h
- similarity TTL: 7d

TTL 切れ時は background job を enqueue して更新する。

## 11. Retrieval 設計

### 11.1 hierarchy

```text
same project > similar project > global
```

### 11.2 SessionStart

query がないため、project 主導で引く。importance / recency を重視する。

### 11.3 UserPromptSubmit

prompt embedding と FTS を併用し、RRF で統合する。さらに project / similar project / global boost を加える。

## 12. LLM enrichment

### 12.1 目的

会話をそのまま保存するのではなく、再利用に向いた構造化記憶へ変換する。

### 12.2 出力項目

- `summary`
- `kind`
- `importance`
- `scope`
- `project_transferability`
- `keywords`
- `applies_to`

### 12.3 kind

- decision
- constraint
- todo
- failure
- fact
- preference
- pattern

### 12.4 scope

- project
- similarity_shareable
- global

## 13. Chunking

### 13.1 基本方針

- 基本は user + assistant のペア
- 長すぎる assistant は分割余地を残す
- tool log はノイズなら落とす

### 13.2 dedupe

- `content_hash` ベースで重複排除
- 将来的に semantic dedupe を追加可能

## 14. Python embedding worker

### 14.1 実装方針

- FastAPI
- sentence-transformers
- `uv run`
- UDS 起動

### 14.2 API

- `GET /health`
- `POST /embed`

### 14.3 モデル

初期は Ruri v3 系。embedding 以外の責務は持たせない。

## 15. 導入と配布

### 15.1 README

- 英語版
- 日本語版
- 相互リンク
- plugin / marketplace 導入手順付き

### 15.2 skill

英語で提供する。

### 15.3 Claude Code plugin

- local install
- marketplace install を見据えた構造

### 15.4 GoReleaser

- Homebrew tap 配布
- マルチアーキ build

## 16. 実装順序

### フェーズ 1

- Kong CLI skeleton
- SQLite schema
- worker start/stop/status
- stop/session-end hook
- ingest worker loop

### フェーズ 2

- session-start/user-prompt retrieval
- embedding worker 接続
- chunk 保存

### フェーズ 3

- LLM enrichment を本物に置換
- project refresh / similarity refresh 実装
- sqlite-vec 導入

### フェーズ 4

- plugin polishing
- README / skill / release 整備

## 17. 非目標

MVP では次を行わない。

- クラウド同期
- チーム共有 DB
- GUI
- ブロッキング hook 制御
- Python 側への queue / DB 主導権移譲

## 18. まとめ

memoria は、Claude Code の短期文脈を補うためのローカル長期記憶基盤である。設計の要点は次の通り。

- Go CLI を中心にする
- Python は embedding のみ
- SessionEnd 非保証を前提に Stop を併用する
- same project / similar project / global の retrieval hierarchy を採用する
- worker は共有デーモンとして自前管理する
- SQLite を queue / memory / worker state の中心に据える

この設計により、長時間のコーディングセッションや複数プロジェクトをまたぐ作業でも、重要な判断と知見を継続的に再利用できる。
