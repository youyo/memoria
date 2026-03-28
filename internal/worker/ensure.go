package worker

import (
	"context"
	"fmt"
	"os"
)

// EnsureIngest は ingest worker が起動していることを確認する。
// M05 ではスタブ実装（ログ出力のみ）。M07 で本実装に置き換える。
func EnsureIngest(ctx context.Context) {
	fmt.Fprintln(os.Stderr, "memoria: ensureWorker: not implemented (M07)")
}
