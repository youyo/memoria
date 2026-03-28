package queue

import "time"

// BackoffDelays は retry_count に対応する待機時間を定義する。
// SPEC 7.3: 5s -> 30s -> 300s
var BackoffDelays = []time.Duration{
	5 * time.Second,
	30 * time.Second,
	300 * time.Second,
}

// NextRunAfter は retry_count から次回実行可能時刻を計算する。
// retryCount が配列範囲を超えた場合は最大値を使う。
func NextRunAfter(retryCount int, now time.Time) time.Time {
	idx := retryCount
	if idx >= len(BackoffDelays) {
		idx = len(BackoffDelays) - 1
	}
	return now.Add(BackoffDelays[idx])
}
