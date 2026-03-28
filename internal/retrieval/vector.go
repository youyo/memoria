package retrieval

import (
	"encoding/binary"
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

// Float32SliceToBytes は []float32 を little-endian バイト列に変換する。
// 各要素は 4 バイト（IEEE 754 single precision）として直列化される。
func Float32SliceToBytes(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// BytesToFloat32Slice は little-endian バイト列を []float32 に変換する。
// バイト数が 4 の倍数でない場合はエラーを返す。
func BytesToFloat32Slice(b []byte) ([]float32, error) {
	if len(b) == 0 {
		return nil, nil
	}
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("bytes to float32: length %d is not a multiple of 4", len(b))
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec, nil
}
