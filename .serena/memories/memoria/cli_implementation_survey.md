# Memoriaプロジェクト CLIコマンド実装状態 徹底調査

調査日時: 2026-03-31
調査対象: `internal/cli/` ディレクトリ全体

## 概要

Memoriaプロジェクトは、Claude Code用のメモリ管理・ワーカー管理を行うCLIツール。Kong frameworkを使用してコマンドツリーを構築している。

### ファイル構成 (20ファイル)
- 実装ファイル: 12個
- テストファイル: 8個

---

## 各コマンドの実装状態

### 1. `completion` コマンドグループ

**ファイル**: `completion.go`

#### `completion bash`
- 状態: **スタブ実装**
- コード: Run()メソッドが "not implemented" を出力
- 線: 18-22
- Kong framework: struct tag `cmd:""` で登録

#### `completion zsh`
- 状態: **スタブ実装**
- コード: Run()メソッドが "not implemented" を出力
- フラグ: `--short` オプションあり
- 線: 25-33

#### `completion fish`
- 状態: **スタブ実装**
- コード: Run()メソッドが "not implemented" を出力
- 線: 35-42

**結論**: すべてのcompletion系コマンドはスタブ。実装が必要。

---

### 2. `plugin` コマンドグループ

**ファイル**: `plugin.go`

#### `plugin list`
- 状態: **スタブ実装**
- コード: Run()メソッドが "not implemented" を出力
- 線: 17-21

#### `plugin doctor`
- 状態: **スタブ実装**
- コード: Run()メソッドが "not implemented" を出力
- 線: 26-30

**結論**: プラグイン管理機能は未実装。

---

### 3. `memory` コマンドグループ

**ファイル**: `memory.go`

#### `memory search` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - 全文検索（FTS）による検索
  - embedder使用時は埋め込みベクトル検索対応（準備段階）
  - フィルタリング: kind, project ID
  - JSON/テキスト出力対応
  - 線: 24-96
- 依存: `retrieval` package

#### `memory get <id>` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - chunk_id を指定してメモリを取得
  - SQL クエリで chunk テーブルから検索
  - JSON/テキスト出力対応
  - not found エラーハンドリング
  - 線: 110-170
- テスト: `memory_get_test.go` で検証済み

#### `memory list` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - メモリ一覧表示
  - フィルタリング: kind, project ID
  - LIMIT オプション
  - JSON/テキスト出力対応
  - 線: 172-264

#### `memory stats` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - chunks総数、sessions総数、pending jobs、DBサイズを表示
  - JSON/テキスト出力対応
  - 線: 275-324

#### `memory reindex` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - chunk_embeddings テーブルの JSON blob を バイナリ float32 形式に変換
  - project_embeddings も対応
  - --dry-run オプションで予行実行可能
  - バッチ処理対応（デフォルト100件）
  - 線: 326-420
- テスト: `memory_reindex_test.go` で検証済み

**結論**: memory コマンドグループは全て完全実装。

---

### 4. `worker` コマンドグループ

**ファイル**: `worker.go`

#### `worker start` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - ingest worker と embedding worker を並列起動
  - health check実施
  - RunWithDB() メソッドでテスト可能実装
  - 線: 30-97
- テスト: `worker_test.go` で検証済み

#### `worker stop` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - embedding worker を先に停止
  - ingest worker に .stop ファイルを書き込み
  - Graceful shutdown: 最大5秒待機
  - SIGTERM -> SIGKILL escalation
  - RunWithDB() メソッドでテスト可能実装
  - 線: 99-205

#### `worker restart` ❌ スタブ実装
- 状態: **未実装**
- コード: Run()メソッドが "not implemented" を出力
- 線: 239-246
- **問題**: start + stop の組み合わせでの実装が必要

#### `worker status` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - ingest worker の liveness チェック（CheckLiveness()）
  - embedding worker の health チェック（UDS通信）
  - PID、uptime、heartbeat情報を表示
  - JSON/テキスト出力対応
  - RunWithDB() メソッドでテスト可能実装
  - 線: 269-375

**結論**: `worker restart` のみ未実装。他は完全実装。

---

### 5. `config` コマンドグループ

**ファイル**: `config.go`

#### `config init` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - 設定ファイルを XDG パスに初期化
  - -f/--force オプションで上書き可能
  - 線: 30-52

#### `config show` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - 現在の設定を TOML 形式で表示
  - JSON 形式にも対応
  - 線: 54-78

#### `config path` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - 設定ファイルのパスを出力
  - JSON/テキスト形式対応
  - 線: 80-96

#### `config print-hook` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - Claude Code の settings.json に貼り付ける hook 設定を JSON で生成
  - SessionStart, UserPromptSubmit, Stop, SessionEnd の4つの hook を定義
  - 線: 98-138
- テスト: `config_print_hook_test.go` で検証済み

**結論**: config コマンドグループは全て完全実装。

---

### 6. `hook` コマンドグループ

**ファイル**: `hook.go`

#### `hook session-start` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - Claude Code からのセッション開始フック
  - Project 解決（cwd から git root を特定）
  - Similar projects を取得（M13）
  - FTS + 埋め込み vector 検索で retrieval
  - FireAndForget: embedding worker 起動
  - RunWithReader() でテスト可能実装
  - 線: 59-124
- 依存: `retrieval`, `project`, `worker` packages

#### `hook user-prompt` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - ユーザープロンプト送信時の hook
  - Project 解決
  - Similar projects 取得
  - Prompt に基づく retrieval
  - 非同期 ingest: user_prompt_ingest ジョブをキューに追加
  - RunWithReader() でテスト可能実装
  - 線: 142-217

#### `hook stop` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - レスポンス完了時の hook
  - Project 解決
  - checkpoint_ingest ジョブをキューに追加
  - RunWithReader() でテスト可能実装
  - 線: 255-314
- テスト: `hook_stop_test.go` で検証済み

#### `hook session-end` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - セッション終了時の hook
  - Project 解決
  - session_end_ingest ジョブをキューに追加
  - RunWithReader() でテスト可能実装
  - 線: 334-395

**結論**: hook コマンドグループは全て完全実装。

---

### 7. `doctor` コマンド

**ファイル**: `doctor.go`

#### `doctor` ✅ 完全実装
- 状態: **完全実装**
- 機能チェック項目:
  1. DB 接続確認
  2. パス確認 (config_path, data_dir, state_dir, db_file)
  3. Schema version確認
  4. Pragma 確認 (WAL, foreign_keys)
  5. FTS テーブル確認 (chunks_fts)
  6. Ingest worker 状態確認 (liveness check)
  7. Embedding worker 状態確認 (socket existence)
  8. Config ファイル検証
  9. Queue depth 確認
- JSON/テキスト出力対応
- 線: 32-231

**結論**: doctor は詳細な診断機能を完全実装。

---

### 8. `version` コマンド

**ファイル**: `version.go`

#### `version` ✅ 完全実装
- 状態: **完全実装**
- 機能:
  - バージョン、コミット、ビルド日時を表示
  - ビルド時 -ldflags で値を注入
  - JSON/テキスト形式対応
  - 線: 19-28

**結論**: version は完全実装。

---

### 9. `daemon` コマンドグループ

**ファイル**: `daemon.go`

#### `daemon ingest` ✅ 完全実装（内部コマンド）
- 状態: **完全実装**
- 機能:
  - Self-spawn で起動される内部デーモンプロセス
  - ingest worker + embedding worker を実行
  - ユーザー直接実行は非推奨（hidden:""）
  - 線: 20-40

**結論**: daemon は内部実装。

---

### 10. `root.go` - Kong framework 設定

**ファイル**: `root.go`

- CLI 構造体で全サブコマンドを登録
- Globals に --config, -v, --no-color, --format フラグ
- Kong の `cmd:""` タグで自動ツリー構築

---

### 11. `lazydb.go` - Lazy DB 初期化

**ファイル**: `lazydb.go`

- DB を初回アクセス時にのみ開く遅延初期化パターン
- Version コマンドなど DB 不要なコマンドは DB を開かない
- テスト用 DI: NewLazyDBFromDB() でモック DB を注入可能

---

## Kong Framework の活用

### 概要
各コマンドは struct として定義され、Kong が struct tag から自動的にサブコマンドツリーを構築。

### 活用パターン:
- `cmd:""` : サブコマンドとして登録
- `help:"..."` : ヘルプテキスト
- `short:"X"` : 単字フラグ
- `default:"..."` : デフォルト値
- `enum:"..."` : 列挙値制限
- `name:"..."` : コマンド名をカスタマイズ
- `hidden:""` : ヘルプから隠す（daemon など）
- `optional:""` : オプション引数化

### Kong completion 機能の利用
- 現在、Kong の completion 機能は未利用
- completion bash/zsh/fish コマンドがスタブなので、
  Kong が提供する completion 生成ロジックを活用する手段がない
- 実装時には Kong の ctx.Plugins や completion registry を活用可能

---

## テスト状態

実装済みテスト:
- `memory_get_test.go` : memory get コマンド
- `memory_list_test.go` : memory list コマンド
- `memory_stats_test.go` : memory stats コマンド
- `memory_search_test.go` : memory search コマンド
- `memory_reindex_test.go` : memory reindex コマンド
- `hook_session_start_test.go` : hook session-start コマンド
- `hook_stop_test.go` : hook stop コマンド
- `hook_session_end_test.go` : hook session-end コマンド
- `config_print_hook_test.go` : config print-hook コマンド
- `doctor_test.go` : doctor コマンド
- `worker_test.go` : worker コマンド
- `config_test.go` : config コマンド
- `daemon_test.go` : daemon コマンド
- `cli_test.go` : 統合テスト

**未テスト**:
- completion bash/zsh/fish（スタブなので test 不可）
- plugin list/doctor（スタブなので test 不可）
- worker restart（未実装なので test 不可）
- version コマンド（単純なため test 省略の可能性）

---

## 実装度サマリー

| カテゴリ | 完全実装 | スタブ | 未実装 |
|---------|--------|--------|--------|
| completion | - | 3 | - |
| plugin | - | 2 | - |
| memory | 5 | - | - |
| worker | 3 | - | 1 |
| config | 4 | - | - |
| hook | 4 | - | - |
| doctor | 1 | - | - |
| version | 1 | - | - |
| daemon | 1 | - | - |
| **合計** | **22** | **5** | **1** |

---

## 問題点と改善提案

### 緊急対応が必要:
1. **worker restart** (1)
   - 簡単な実装: stop() + start() の順序呼び出し
   - または既存の実装パターンで start/stop を統合

### 実装推奨:
2. **completion bash/zsh/fish** (3)
   - Kong の completion サポートを活用
   - または generate scripts を外部実装

3. **plugin list/doctor** (2)
   - プラグインシステムの要件定義が必要
   - 現在は placeholder

### テスト改善:
4. 新規コマンド実装時のテスト追加
5. 統合テスト（cli_test.go）の充実

---

## 実装パターン分析

### RunWithDB / RunWithReader パターン
すべての worker, hook, memory, config コマンドが
テスト可能な実装を提供（DI対応）:
- `worker` : RunWithDB(sqlDB, runDir)
- `hook` : RunWithReader(reader, sqlDB, embedder)
- これにより unit test が容易

### エラーハンドリング
- Hook コマンドは non-fatal errors で exit 0（block しない設計）
- Worker stop は graceful shutdown + escalation (SIGTERM -> SIGKILL)
- Doctor はチェック失敗してもレポートを返す

### 非同期処理
- hook user-prompt / session-end : go routine で async ingest
- worker start : embedding worker を fire-and-forget で起動

---

## Kong framework との関係

### 現在の使い方:
- Struct tag ベースの自動ツリー構築
- Globals フラグの共有
- DI: Context に値を注入（Config, LazyDB など）

### 未利用機能:
- Kong の completion 生成ロジック
  → completion bash/zsh/fish 実装時に活用可能

### その他の Kong 機能:
- Named positional arguments (`arg:""`)
- Enum validation (`enum:"..."`)
- Custom types (path, etc.)
  → 全てまだ活用可能

---

## まとめ

**実装完成度: 82% (22/27 コマンド)**

- メインの memory, hook, worker, config, doctor コマンドは完全実装
- completion 系 3 コマンドはスタブ
- plugin 系 2 コマンドはスタブ
- worker restart は未実装

あと **5 つのコマンド** を実装すれば CLI 機能は完成。
特に worker restart と completion は優先度が高い。
