# M15: Plugin + Release engineering (`release-packaging`)

## 概要

全機能実装済みの memoria に対して、以下を実装・整備する:

1. `memory search/get/list/stats` コマンドの実装（スタブ→本実装）
2. `doctor` 完全版（worker 状態 + config 検証チェックを追加）
3. Plugin manifest 最終版（`plugin/memoria/manifest.json`）
4. GoReleaser 設定（`.goreleaser.yml`）
5. GitHub Actions CI/CD（`.github/workflows/release.yml`, `.github/workflows/test.yml`）

## ハンドオフ状態

- `memory search/get/list/stats` は `Run()` が `"not implemented"` を返すスタブ
- `doctor` は DB 接続・スキーマバージョン・pragma・FTS テーブルの基本4チェックのみ
- `plugin/` ディレクトリ未作成
- `.goreleaser.yml` / `.github/workflows/` 未作成

## タスク詳細

### Task 1: memory search コマンド実装

**仕様**:
- `memoria memory search <query>` — FTS + vector で chunks を検索
- `--limit N` フラグ（デフォルト 10）
- `--project <project_id>` フラグ（省略時は全プロジェクト）
- `--kind <kind>` フラグ（省略時は全 kind）
- 出力: text/JSON 両対応（Result のリスト）
- DB DI が必要（`*db.DB` を Kong から注入）

**実装方針**:
- `retrieval.Retriever.FTSSearch()` を利用
- embedder は nil（degraded mode = FTS only）で動作させる
- `--project` 指定時は project boost を適用
- JSON 出力は `[]retrieval.Result` をそのままシリアライズ
- text 出力は `[kind] score: N | content...` 形式

### Task 2: memory get コマンド実装

**仕様**:
- `memoria memory get <chunk_id>` — chunk 詳細取得
- chunk_id で chunks テーブルを直接 SELECT
- 出力: text/JSON 両対応
- DB DI が必要

### Task 3: memory list コマンド実装

**仕様**:
- `memoria memory list` — 最近の chunks 一覧
- `--limit N` フラグ（デフォルト 20）
- `--project <project_id>` フィルタ（省略時は全プロジェクト）
- `--kind <kind>` フィルタ（省略時は全 kind）
- ORDER BY created_at DESC
- 出力: text/JSON 両対応
- DB DI が必要

### Task 4: memory stats コマンド実装

**仕様**:
- `memoria memory stats` — 統計情報
- 統計項目:
  - `chunks_total`: chunks テーブルの件数
  - `sessions_total`: sessions テーブルの件数
  - `jobs_pending`: jobs テーブルの queued 件数
  - `db_size_bytes`: DB ファイルのバイトサイズ
  - `db_path`: DB ファイルパス
- DB DI が必要
- config 情報から db_path を取得

### Task 5: doctor 完全版

既存チェック + 以下を追加:

- `ingest_worker`: worker_leases テーブルを確認。liveness を表示
- `embedding_worker`: UDS ソケットファイルの存在確認（health チェックは行わない。タイムアウトリスク回避）
- `config_valid`: config ファイルが存在するか + TOML パースが通るか
- `queue_depth`: jobs テーブルの queued 件数を表示（INFO チェック、件数多すぎでも OK）

### Task 6: Plugin manifest 最終版

`plugin/memoria/manifest.json` を作成:

```json
{
  "name": "memoria",
  "version": "0.1.0",
  "description": "Claude Code 向けプロジェクト認識型ローカル RAG メモリシステム",
  "hooks": {
    "SessionStart": {
      "command": "memoria hook session-start"
    },
    "UserPromptSubmit": {
      "command": "memoria hook user-prompt"
    },
    "Stop": {
      "command": "memoria hook stop"
    },
    "PreToolUse": null
  }
}
```

### Task 7: GoReleaser 設定

`.goreleaser.yml`:
- darwin/amd64, darwin/arm64, linux/amd64, linux/arm64 のクロスコンパイル
- ldflags で version/commit/date を埋め込む
- archives: tar.gz
- Homebrew tap: `youyo/homebrew-tap` リポジトリ向け

### Task 8: GitHub Actions CI/CD

`.github/workflows/test.yml`:
- push/PR で `go test ./... -timeout 120s`
- `go vet ./...`
- Go 1.23 (mise.toml に合わせる)

`.github/workflows/release.yml`:
- tag push (`v*.*.*`) でトリガー
- GoReleaser を実行
- GITHUB_TOKEN を使う

## TDD 方針

### Red フェーズ（テスト先行）

1. `memory_search_test.go` — search クエリが results を返すことを確認
2. `memory_get_test.go` — chunk_id で chunk を取得できることを確認
3. `memory_list_test.go` — list がフィルタ条件に従って返すことを確認
4. `memory_stats_test.go` — stats が正しい件数・サイズを返すことを確認
5. `doctor_test.go` に追加 — worker/config/queue チェックのテスト

### Green フェーズ（最小実装）

各コマンドの Run() に最小の実装を追加。

### Refactor フェーズ

- 重複クエリを関数として抽出
- エラーハンドリングの統一

## ファイル一覧（新規作成/変更）

| ファイル | 操作 |
|---|---|
| `internal/cli/memory.go` | 変更: search/get/list/stats を本実装 |
| `internal/cli/memory_search_test.go` | 新規 |
| `internal/cli/memory_get_test.go` | 新規 |
| `internal/cli/memory_list_test.go` | 新規 |
| `internal/cli/memory_stats_test.go` | 新規 |
| `internal/cli/doctor.go` | 変更: worker/config/queue チェックを追加 |
| `internal/cli/doctor_test.go` | 変更: 追加チェックのテストを追加 |
| `plugin/memoria/manifest.json` | 新規 |
| `.goreleaser.yml` | 新規 |
| `.github/workflows/test.yml` | 新規 |
| `.github/workflows/release.yml` | 新規 |
| `CLAUDE.md` | 変更: 全マイルストーン完了の記録 |
| `plans/memoria-roadmap.md` | 変更: M15 完了マーク |

## 完了基準

- [ ] `go test ./... -timeout 120s` が全 green
- [ ] `make build` 成功
- [ ] `make lint` 成功
- [ ] `plugin/memoria/manifest.json` が存在する
- [ ] `.goreleaser.yml` が存在する
- [ ] `.github/workflows/test.yml` と `release.yml` が存在する
- [ ] `memoria memory search "test"` が動作する（空結果でも OK）
- [ ] `memoria memory stats` が統計情報を返す
- [ ] `memoria doctor` が worker/config/queue チェックを含む
