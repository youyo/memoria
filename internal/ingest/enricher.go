package ingest

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// EnrichedChunk はヒューリスティック enrichment の結果。
type EnrichedChunk struct {
	Kind         string  // decision | constraint | todo | failure | fact | preference | pattern
	Importance   float64 // 0.0 ~ 1.0
	Scope        string  // project | similarity_shareable | global
	KeywordsJSON string  // JSON 配列文字列 ["kw1", "kw2", ...]
	Summary      string  // 先頭 100 文字（rune 単位）
}

// コンパイル済み正規表現（summary クリーニング用）
var (
	reXMLTag     = regexp.MustCompile(`<[^>]+>`)
	reToolLine   = regexp.MustCompile(`(?m)^\[Tool: [^\]]*\]\s*$`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

// Enrich はコンテンツに対してヒューリスティック enrichment を行い、EnrichedChunk を返す。
// LLM 呼び出しは行わず、キーワードマッチと統計的ヒューリスティックで推定する。
func Enrich(content string) EnrichedChunk {
	lower := strings.ToLower(content)

	// system metadata は低 importance で保存
	if isSystemMetadata(lower) {
		return EnrichedChunk{
			Kind:         "fact",
			Importance:   0.1,
			Scope:        "project",
			KeywordsJSON: "[]",
			Summary:      makeSummary(content),
		}
	}

	kind := inferKind(lower)
	importance := inferImportance(lower, kind, content)
	scope := inferScope(lower, kind)
	keywords := extractKeywords(content)
	summary := makeSummary(content)

	kwJSON := "[]"
	if len(keywords) > 0 {
		b, _ := json.Marshal(keywords)
		kwJSON = string(b)
	}

	return EnrichedChunk{
		Kind:         kind,
		Importance:   importance,
		Scope:        scope,
		KeywordsJSON: kwJSON,
		Summary:      summary,
	}
}

// kindKeywords は kind ごとのキーワードパターン（優先順位順）。
var kindPatterns = []struct {
	kind     string
	keywords []string
}{
	{"decision", []string{"decided", "決定", "採用", "chose", "選択", "we will", "にする", "方針", "choose to"}},
	{"constraint", []string{"must not", "禁止", "してはいけない", "制約", "cannot", "prohibited", "制限", "must never"}},
	{"todo", []string{"todo", "やること", "残作業", "あとで", "later", "fix later"}},
	{"failure", []string{
		"failed to", "test failed", "build failed",
		"失敗", "エラーが", "エラー発生",
		"got error", "error occurred", "error:",
		"バグ", "bug", "crash", "不具合", "exception",
	}},
	{"preference", []string{"prefer", "好み", "したい", "使いたい", "気に入って", "like to"}},
	{"pattern", []string{"pattern", "パターン", "再利用", "template", "テンプレート", "abstraction"}},
}

// inferKind はコンテンツ（小文字化済み）から kind を推定する。
func inferKind(lower string) string {
	for _, kp := range kindPatterns {
		for _, kw := range kp.keywords {
			if strings.Contains(lower, kw) {
				return kp.kind
			}
		}
	}
	return "fact"
}

// inferImportance はコンテンツから importance (0.0 ~ 1.0) を推定する。
func inferImportance(lower, kind, original string) float64 {
	score := 0.3 // base score

	switch kind {
	case "decision", "constraint":
		score += 0.3
	case "failure":
		score += 0.2
	case "todo":
		score += 0.1
	}

	// 感嘆符
	if strings.Contains(original, "!") || strings.Contains(original, "！") {
		score += 0.1
	}

	// 重要ワード
	importantWords := []string{"重要", "critical", "important", "必須"}
	for _, w := range importantWords {
		if strings.Contains(lower, w) {
			score += 0.2
			break
		}
	}

	// FIXME/HACK/XXX
	for _, w := range []string{"fixme", "hack", "xxx"} {
		if strings.Contains(lower, w) {
			score += 0.15
			break
		}
	}

	// content 長 > 500 文字
	if len(original) > 500 {
		score += 0.05
	}

	// クランプ
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// inferScope はコンテンツ（小文字化済み）と kind から scope を推定する。
// 「コンテンツは閉じる、技術は開く」原則に基づいたスコアリング方式。
func inferScope(lower string, kind string) string {
	techScore := 0
	contentScore := 0

	// 技術指標 (各 +1)
	techKeywords := []string{"func ", "def ", "class ", "import ", "require(", "package ", "interface ", "struct "}
	errorKeywords := []string{"error:", "panic:", "stack trace", "exception", "traceback", "workaround"}
	libKeywords := []string{"github.com/", "npm:", "pip install", "go get", "cargo add", "brew install"}
	patternKeywords := []string{"pattern", "anti-pattern", "best practice", "idiom"}
	techVerbs := []string{"refactor", "optimize", "migrate", "deploy", "configure", "benchmark"}
	fileExts := []string{".go", ".py", ".ts", ".js", ".sql", ".yaml", ".toml"}

	for _, list := range [][]string{techKeywords, errorKeywords, libKeywords, patternKeywords, techVerbs, fileExts} {
		for _, kw := range list {
			if strings.Contains(lower, kw) {
				techScore++
			}
		}
	}

	// kind ボーナス（技術指標）
	switch kind {
	case "pattern":
		techScore += 2
	case "failure":
		techScore += 1
	}

	// 後方互換 (旧キーワード、強シグナル +3)
	legacyGlobal := []string{"global", "汎用", "どこでも使える", "universally"}
	for _, kw := range legacyGlobal {
		if strings.Contains(lower, kw) {
			techScore += 3
		}
	}

	// コンテンツ指標 (各 +1)
	decisionKeywords := []string{"decided", "chose", "requirement", "stakeholder", "にする", "にした"}
	domainKeywords := []string{"business", "仕様", "要件", "ドメイン", "policy"}
	for _, list := range [][]string{decisionKeywords, domainKeywords} {
		for _, kw := range list {
			if strings.Contains(lower, kw) {
				contentScore++
			}
		}
	}

	// kind ボーナス（コンテンツ指標）
	switch kind {
	case "decision":
		contentScore += 2
	case "constraint":
		contentScore += 2
	case "todo":
		contentScore += 3 // TODO は常に project
	case "preference":
		contentScore += 1
	}

	// 判定
	if contentScore >= 3 {
		return "project" // コンテンツベトー
	}
	if techScore > contentScore && techScore >= 3 {
		return "global"
	}
	if techScore > contentScore && techScore >= 2 {
		return "similarity_shareable"
	}
	return "project" // 安全デフォルト
}

// stopWords は除去するストップワード（英語 + 日本語）。
var stopWords = map[string]bool{
	// 英語
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "shall": true,
	"to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "through": true, "that": true, "this": true,
	"and": true, "or": true, "but": true, "not": true, "so": true,
	"if": true, "it": true, "its": true, "we": true, "you": true,
	"he": true, "she": true, "they": true, "our": true, "their": true,
	"use": true, "using": true, "used": true,
	// 日本語助詞・助動詞
	"が": true, "を": true, "の": true, "に": true, "は": true,
	"で": true, "と": true, "も": true, "や": true, "へ": true,
	"から": true, "まで": true, "より": true, "など": true,
	"です": true, "ます": true, "した": true, "する": true, "ている": true,
}

// extractKeywords はコンテンツからキーワードを抽出する（最大 10 件）。
func extractKeywords(content string) []string {
	// トークン化
	tokens := tokenize(content)

	// 出現回数カウント
	freq := make(map[string]int)
	for _, t := range tokens {
		lower := strings.ToLower(t)
		// ストップワード除外、3 文字以上（英語）または 2 文字以上（日本語）
		if stopWords[lower] {
			continue
		}
		runeLen := len([]rune(t))
		if isASCII(t) && runeLen < 3 {
			continue
		}
		if !isASCII(t) && runeLen < 2 {
			continue
		}
		freq[lower]++
	}

	// 出現回数でソート
	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, v := range freq {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].key < sorted[j].key
	})

	// 上位 10 件
	result := make([]string, 0, 10)
	for i, kv := range sorted {
		if i >= 10 {
			break
		}
		result = append(result, kv.key)
	}
	return result
}

// tokenize はコンテンツをトークンに分割する。
func tokenize(content string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range content {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// isASCII はトークンが ASCII 文字のみで構成されているか確認する。
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// isSystemMetadata はコンテンツが system metadata（Claude Code の内部タグ）かどうかを判定する。
func isSystemMetadata(lower string) bool {
	return strings.Contains(lower, "<task-notification>") ||
		strings.Contains(lower, "<task-id>") ||
		strings.Contains(lower, "<system-reminder>")
}

// makeSummary はコンテンツからノイズを除去し、先頭 100 文字（rune 単位）を summary として返す。
func makeSummary(content string) string {
	s := content
	// XML タグ除去
	s = reXMLTag.ReplaceAllString(s, "")
	// プレフィックス除去
	s = strings.TrimPrefix(s, "User: ")
	s = strings.TrimPrefix(s, "Assistant: ")
	s = strings.TrimPrefix(s, "A: ")
	// Tool 行除去
	s = reToolLine.ReplaceAllString(s, "")
	// 空白正規化
	s = reWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")

	runes := []rune(s)
	if len(runes) <= 100 {
		return s
	}
	return string(runes[:100]) + "..."
}
