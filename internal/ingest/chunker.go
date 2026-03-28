package ingest

import (
	"fmt"
	"strings"
)

// MaxChunkBytes はチャンクの最大バイト数（16 KiB）。
const MaxChunkBytes = 16 * 1024

// ChunkInput はチャンク化の入力。
type ChunkInput struct {
	Turns     []Turn
	SessionID string
	ProjectID string
}

// RawChunk はチャンク化の出力（enrichment 前）。
type RawChunk struct {
	Content      string
	TurnStartIdx int // turns スライス内の開始インデックス
	TurnEndIdx   int // turns スライス内の終了インデックス（inclusive）
}

// Chunk はターンリストをチャンクに変換する。
// user + assistant のペアを 1 chunk の基本単位とする。
// tool ターンは直前の user/assistant ペアに包含する。
// MaxChunkBytes を超える場合は ParagraphSplit で分割する。
func Chunk(input ChunkInput) []RawChunk {
	if len(input.Turns) == 0 {
		return nil
	}

	var chunks []RawChunk
	turns := input.Turns
	i := 0

	for i < len(turns) {
		// user ターンを探す
		if turns[i].Role != "user" {
			i++
			continue
		}

		startIdx := i
		i++

		// tool ターンと assistant ターンを収集
		var parts []string
		parts = append(parts, "User: "+turns[startIdx].Content)

		endIdx := startIdx
		for i < len(turns) && turns[i].Role != "user" {
			if turns[i].Role == "tool" {
				parts = append(parts, turns[i].Content)
			} else if turns[i].Role == "assistant" {
				parts = append(parts, "Assistant: "+turns[i].Content)
			}
			endIdx = i
			i++
		}

		content := strings.Join(parts, "\n\n")

		// MaxChunkBytes を超える場合は分割
		if len(content) > MaxChunkBytes {
			subChunks := splitContent(content, startIdx, endIdx)
			chunks = append(chunks, subChunks...)
		} else {
			chunks = append(chunks, RawChunk{
				Content:      content,
				TurnStartIdx: startIdx,
				TurnEndIdx:   endIdx,
			})
		}
	}

	return chunks
}

// SplitLongContent は単一コンテンツを MaxChunkBytes 以内に分割する（checkpoint 用）。
func SplitLongContent(content string) []string {
	if len(content) <= MaxChunkBytes {
		return []string{content}
	}
	chunks := splitContentIntoStrings(content)
	return chunks
}

// splitContent はコンテンツを MaxChunkBytes 以内に分割して RawChunk を返す。
func splitContent(content string, startIdx, endIdx int) []RawChunk {
	parts := splitContentIntoStrings(content)
	chunks := make([]RawChunk, len(parts))
	for i, p := range parts {
		chunks[i] = RawChunk{
			Content:      p,
			TurnStartIdx: startIdx,
			TurnEndIdx:   endIdx,
		}
	}
	return chunks
}

// splitContentIntoStrings はコンテンツを文字列スライスに分割する。
// 1. \n\n（段落）で分割を試みる
// 2. 段落でも超える場合は \n（改行）で分割
// 3. それでも超える場合はハードカット（rune 単位）
func splitContentIntoStrings(content string) []string {
	if len(content) <= MaxChunkBytes {
		return []string{content}
	}

	// まず段落区切りで分割
	paragraphs := strings.Split(content, "\n\n")
	return mergeIntoChunks(paragraphs, "\n\n")
}

// mergeIntoChunks は要素を MaxChunkBytes 以内にマージしてチャンクのスライスを返す。
func mergeIntoChunks(parts []string, sep string) []string {
	var chunks []string
	var current strings.Builder

	for _, part := range parts {
		// part 自体が MaxChunkBytes を超える場合はさらに分割
		if len(part) > MaxChunkBytes {
			// まず現在のバッファをフラッシュ
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			// \n で分割を試みる
			if sep == "\n\n" {
				subParts := strings.Split(part, "\n")
				subChunks := mergeIntoChunks(subParts, "\n")
				chunks = append(chunks, subChunks...)
			} else {
				// ハードカット（rune 単位）
				runes := []rune(part)
				for len(runes) > 0 {
					// MaxChunkBytes を rune 数に変換（保守的に MaxChunkBytes/4）
					maxRunes := MaxChunkBytes / 4
					if maxRunes > len(runes) {
						maxRunes = len(runes)
					}
					// バイト数が MaxChunkBytes 以内になるまで削る
					for maxRunes > 0 && len(string(runes[:maxRunes])) > MaxChunkBytes {
						maxRunes--
					}
					if maxRunes == 0 {
						maxRunes = 1
					}
					chunks = append(chunks, string(runes[:maxRunes]))
					runes = runes[maxRunes:]
				}
			}
			continue
		}

		// current + sep + part がMaxChunkBytes を超えるか確認
		addition := part
		if current.Len() > 0 {
			addition = sep + part
		}

		if current.Len()+len(addition) > MaxChunkBytes {
			// フラッシュして新しいチャンクへ
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			current.WriteString(part)
		} else {
			if current.Len() > 0 {
				fmt.Fprintf(&current, "%s%s", sep, part)
			} else {
				current.WriteString(part)
			}
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
