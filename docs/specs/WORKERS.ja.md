# memoria Worker 詳細設計

## Worker 構成

memoria は 2 種類の共有 worker を持つ。

### 1. ingest worker

- 実装: Go
- 役割: queue 処理、chunk 化、LLM enrichment、DB 書き込み統制
- 共有単位: 全 Claude Code セッション共通の 1 プロセス

### 2. embedding worker

- 実装: Python
- 役割: sentence-transformers / Ruri v3 による embedding 生成
- 実行方式: `uv run`
- 通信: UDS デフォルト
- 共有単位: 全 Claude Code セッション共通の 1 プロセス

## ディレクトリ配置

固定パス:

```text
~/.config/memoria/
~/.local/share/memoria/
~/.local/state/memoria/
```

実行時ファイル:

```text
~/.local/state/memoria/
  run/
    ingest.lock
    embed.lock
    ingest.pid
    embed.pid
    embedding.sock
    ingest.stop
  logs/
    ingest.log
    embedding.log
```

## 起動方針

### embedding worker

- `uv run worker.py --uds ~/.local/state/memoria/run/embedding.sock --preload`
- UDS health により生存確認
- idle timeout: 600 秒

### ingest worker

- `memoria daemon ingest` で self-spawn
- SQLite `worker_leases` heartbeat により生存確認
- idle timeout: 60 秒

## 起動責任

- `memoria worker start` で両方起動
- 各 hook / retrieval / ingest 実行時にも `ensureWorker()` で自己防衛
- SessionStart は先行起動ポイントとして使うが、唯一の起動ポイントにはしない

## 生存確認

### ingest worker

主判定は `worker_leases.last_heartbeat_at`。

- alive: 3 秒以内
- suspect: 3〜10 秒
- stale: 10 秒超

suspect 時は SQLite probe による stronger check を行う。

### embedding worker

- UDS socket の存在
- `/health` endpoint の応答

## status について

`status` 文字列は真実のソースにしない。異常終了時に stale な値が残るため、表示用途の補助情報に留める。

## 停止方針

### ingest worker

- `ingest.stop` ファイルを検知して graceful stop
- 失敗時のみ PID を TERM

### embedding worker

- まず TERM
- 必要なら KILL
- 将来的に `/shutdown` API を追加してもよいが、MVP では不要

## liveness の強い確認

heartbeat が遅延した場合のみ、SQLite 上の `worker_probes` を使って「DB に触れているか」を検証する。

## 共有設計の理由

- SQLite write contention を抑える
- queue retry / dedupe を一元管理する
- Claude Code の複数セッションから自然に共有できる
