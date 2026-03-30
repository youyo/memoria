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
// isolated=true の場合は他プロジェクトのチャンクに -999 を設定して実質除外する。
func ApplyProjectBoost(results []RankedResult, currentProjectID string, similarProjects map[string]float64, isolated bool) []RankedResult {
	if len(results) == 0 {
		return results
	}

	boosted := make([]RankedResult, len(results))
	for i, rr := range results {
		if isolated {
			// isolated プロジェクト: 自プロジェクトのみ許可、他は実質除外
			if rr.ProjectID != currentProjectID {
				rr.Score = -999
			}
		} else {
			// 通常プロジェクト: scope-aware boost
			if rr.ProjectID == currentProjectID {
				// 同プロジェクト: 常に boost
				rr.Score += sameProjectBoost
			} else if sim, ok := similarProjects[rr.ProjectID]; ok && sim >= similarProjectThreshold {
				// 類似プロジェクト: scope に関わらず boost（global / similarity_shareable）
				// project scope のチャンクは類似プロジェクトからも高ペナルティ
				if rr.Scope == "global" || rr.Scope == "similarity_shareable" {
					rr.Score += similarProjectBoost
				} else {
					rr.Score -= 3.0 // project scope from similar project
				}
			} else {
				// その他プロジェクト: scope によって扱いが異なる
				switch rr.Scope {
				case "global":
					// boost なし、ペナルティなし
				case "similarity_shareable":
					rr.Score -= 1.0
				default: // "project"
					rr.Score -= 3.0
				}
			}
		}
		boosted[i] = rr
	}

	sort.Slice(boosted, func(i, j int) bool {
		return boosted[i].Score > boosted[j].Score
	})

	return boosted
}
