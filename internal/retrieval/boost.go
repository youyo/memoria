package retrieval

import "sort"

const (
	// sameProjectBoost は同一プロジェクトへのスコア加算値。
	sameProjectBoost = 2.0
	// similarProjectBoost は類似プロジェクトへのスコア加算値。
	similarProjectBoost = 1.0
	// similarProjectThreshold は類似プロジェクトとみなす最小類似度。
	similarProjectThreshold = 0.5
)

// ApplyProjectBoost は results に project boost を適用してスコアを再計算し、
// スコア降順でソートした新しいスライスを返す。
// currentProjectID は現在のプロジェクト ID。
// similarProjects は similar_project_id -> similarity スコアのマップ（nil 可）。
func ApplyProjectBoost(results []RankedResult, currentProjectID string, similarProjects map[string]float64) []RankedResult {
	if len(results) == 0 {
		return results
	}

	boosted := make([]RankedResult, len(results))
	for i, rr := range results {
		boost := 0.0
		if rr.ProjectID == currentProjectID {
			boost = sameProjectBoost
		} else if sim, ok := similarProjects[rr.ProjectID]; ok && sim >= similarProjectThreshold {
			boost = similarProjectBoost
		}
		rr.Score += boost
		boosted[i] = rr
	}

	sort.Slice(boosted, func(i, j int) bool {
		return boosted[i].Score > boosted[j].Score
	})

	return boosted
}
