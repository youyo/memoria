package retrieval

import (
	"encoding/json"
	"fmt"
	"math"
)

// CosineSimilarity は 2 つのベクトルのコサイン類似度を計算する。
// ベクトルが零ベクトルの場合や長さが異なる場合は 0 を返す。
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// parseFloat32Slice は JSON 文字列から []float32 を復元する。
func parseFloat32Slice(s string) ([]float32, error) {
	var vec []float32
	if err := json.Unmarshal([]byte(s), &vec); err != nil {
		return nil, fmt.Errorf("parse float32 slice: %w", err)
	}
	return vec, nil
}
