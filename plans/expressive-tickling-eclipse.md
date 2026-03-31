# feat: ログにタイムスタンプとレベルを追加

## Context

現状のログは `fmt.Fprintf(os.Stderr, "memoria hook ...: %v\n", err)` のみで:
- タイムスタンプなし → 問題発生時刻が特定できない
- ログレベルなし → エラーと情報の区別がつかない
- 成功ログなし → 正常動作の確認ができない（「PASS」だけ）

今セッションのデバッグでもログのタイムスタンプ不足が障害切り分けを困難にした。

## 修正方針

### 1. `internal/logging/log.go` — 共通ログパッケージ（新規）

軽量な構造化ログ関数を提供。外部依存なし（標準 `log` パッケージベース）。

```go
package logging

import (
    "fmt"
    "io"
    "os"
    "time"
)

// Logf は タイムスタンプ + レベル付きでログ出力する。
// 形式: 2026-03-31T01:06:50Z [ERROR] memoria hook user-prompt: ...
func Logf(level, format string, args ...any) {
    ts := time.Now().UTC().Format(time.RFC3339)
    msg := fmt.Sprintf(format, args...)
    fmt.Fprintf(os.Stderr, "%s [%s] %s", ts, level, msg)
}

func Error(format string, args ...any) { Logf("ERROR", format, args...) }
func Warn(format string, args ...any)  { Logf("WARN", format, args...) }
func Info(format string, args ...any)   { Logf("INFO", format, args...) }
func Debug(format string, args ...any)  { Logf("DEBUG", format, args...) }

// NewLogf は logf 関数変数用のファクトリ（daemon.logf 互換）。
func NewLogf(level string) func(string, ...any) {
    return func(format string, args ...any) {
        Logf(level, format, args...)
    }
}
```

出力形式: `2026-03-31T01:06:50Z [ERROR] memoria hook user-prompt: failed to decode stdin: EOF`

### 2. 各ファイルの変更

| ファイル | 変更箇所数 | 変更内容 |
|---|---|---|
| `internal/logging/log.go` | 新規 | 共通ログパッケージ |
| `internal/cli/hook.go` | 16箇所 | `fmt.Fprintf(os.Stderr, ...)` → `logging.Error(...)` + 成功時に `logging.Info("PASS\n")` |
| `internal/worker/daemon.go` | logf初期化 | `d.logf = logging.NewLogf("INFO")` |
| `internal/worker/ensure.go` | 9箇所 | `fmt.Fprintf(os.Stderr, ...)` → `logging.Error/Warn(...)` |
| `internal/worker/ensure_embedding.go` | 1箇所 | 同上 |
| `internal/cli/worker.go` | 2箇所 | 同上 |
| `internal/cli/daemon.go` | 1箇所 | 同上 |
| `internal/worker/heartbeat.go` | `logToStderr` | `logging.NewLogf("INFO")` で置換 |

### 3. ログレベルの分類

- **ERROR**: enqueue 失敗、DB エラー、decode 失敗など処理失敗
- **WARN**: 非致命的だが注意が必要（embedding 未起動時のフォールバック等）
- **INFO**: daemon 起動/停止、ジョブ処理開始/完了、hook 成功（PASS）

### 4. hook の成功ログ追加

現状 hook 成功時は `os.Stdout` に `PASS` のみ出力。stderr にも `logging.Info` で記録する。

## 対象ファイル一覧

| ファイル | 操作 |
|---|---|
| `internal/logging/log.go` | 新規作成 |
| `internal/logging/log_test.go` | 新規作成（出力形式テスト） |
| `internal/cli/hook.go` | 既存変更 |
| `internal/worker/daemon.go` | 既存変更 |
| `internal/worker/ensure.go` | 既存変更 |
| `internal/worker/ensure_embedding.go` | 既存変更 |
| `internal/cli/worker.go` | 既存変更 |
| `internal/cli/daemon.go` | 既存変更 |
| `internal/worker/heartbeat.go` | 既存変更 |

## 検証

1. `make test` — 全テスト green
2. `echo '{"session_id":"test","cwd":"/tmp","prompt":"hello"}' | memoria hook user-prompt 2>&1 | head` — stderr にタイムスタンプ付きログが出ること
3. `tail -5 ~/.local/state/memoria/logs/ingest.log` — daemon ログにもタイムスタンプが入ること
