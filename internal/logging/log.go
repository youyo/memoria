package logging

import (
	"fmt"
	"os"
	"time"
)

// Logf はタイムスタンプ + レベル付きで stderr にログ出力する。
// 形式: 2026-03-31T01:06:50Z [ERROR] memoria hook user-prompt: ...
func Logf(level, format string, args ...any) {
	ts := time.Now().UTC().Format(time.RFC3339)
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s [%s] %s", ts, level, msg)
}

// Error はエラーレベルのログを出力する。
func Error(format string, args ...any) { Logf("ERROR", format, args...) }

// Warn は警告レベルのログを出力する。
func Warn(format string, args ...any) { Logf("WARN", format, args...) }

// Info は情報レベルのログを出力する。
func Info(format string, args ...any) { Logf("INFO", format, args...) }

// NewLogf は daemon.logf 互換の logf 関数を生成する。
func NewLogf(level string) func(string, ...any) {
	return func(format string, args ...any) {
		Logf(level, format, args...)
	}
}
