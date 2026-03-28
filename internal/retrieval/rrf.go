package retrieval

import "sort"

// MergeRRF は複数の ranked lists を Reciprocal Rank Fusion で統合する。
// k は RRF の定数（通常 60）。
// 各リストのランクは 0-indexed（rank 0 が最上位）。
func MergeRRF(lists [][]RankedResult, k int) []RankedResult {
	if len(lists) == 0 {
		return nil
	}

	scores := make(map[string]float64)
	// ID → RankedResult のマップ（最初に見つかったものを使用）
	byID := make(map[string]RankedResult)

	for _, list := range lists {
		for rank, rr := range list {
			scores[rr.ID] += 1.0 / float64(k+rank+1)
			if _, exists := byID[rr.ID]; !exists {
				byID[rr.ID] = rr
			}
		}
	}

	// スコアを RankedResult にマージ
	results := make([]RankedResult, 0, len(scores))
	for id, score := range scores {
		rr := byID[id]
		rr.Score = score
		results = append(results, rr)
	}

	// スコア降順でソート
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
