package ingest_test

import (
	"encoding/json"
	"testing"

	"github.com/youyo/memoria/internal/ingest"
)

// --- kind 推定テスト ---

func TestEnrichKindDecision(t *testing.T) {
	result := ingest.Enrich("We decided to use Go for the backend")
	if result.Kind != "decision" {
		t.Errorf("expected kind=decision, got %s", result.Kind)
	}
}

func TestEnrichKindDecisionJP(t *testing.T) {
	result := ingest.Enrich("Go を採用することに決定した")
	if result.Kind != "decision" {
		t.Errorf("expected kind=decision, got %s", result.Kind)
	}
}

func TestEnrichKindConstraint(t *testing.T) {
	result := ingest.Enrich("You must not use global variables")
	if result.Kind != "constraint" {
		t.Errorf("expected kind=constraint, got %s", result.Kind)
	}
}

func TestEnrichKindConstraintJP(t *testing.T) {
	result := ingest.Enrich("グローバル変数の使用は禁止されている")
	if result.Kind != "constraint" {
		t.Errorf("expected kind=constraint, got %s", result.Kind)
	}
}

func TestEnrichKindTodo(t *testing.T) {
	result := ingest.Enrich("TODO: refactor the database layer")
	if result.Kind != "todo" {
		t.Errorf("expected kind=todo, got %s", result.Kind)
	}
}

func TestEnrichKindFailure(t *testing.T) {
	result := ingest.Enrich("The test failed with error: connection refused")
	if result.Kind != "failure" {
		t.Errorf("expected kind=failure, got %s", result.Kind)
	}
}

func TestEnrichKindFailureJP(t *testing.T) {
	result := ingest.Enrich("データベース接続が失敗した")
	if result.Kind != "failure" {
		t.Errorf("expected kind=failure, got %s", result.Kind)
	}
}

func TestEnrichKindPreference(t *testing.T) {
	result := ingest.Enrich("I prefer to use interfaces over concrete types")
	if result.Kind != "preference" {
		t.Errorf("expected kind=preference, got %s", result.Kind)
	}
}

func TestEnrichKindPattern(t *testing.T) {
	result := ingest.Enrich("This is a useful pattern for dependency injection")
	if result.Kind != "pattern" {
		t.Errorf("expected kind=pattern, got %s", result.Kind)
	}
}

func TestEnrichKindFact(t *testing.T) {
	// マッチなし → fact (default)
	result := ingest.Enrich("The weather is nice today")
	if result.Kind != "fact" {
		t.Errorf("expected kind=fact (default), got %s", result.Kind)
	}
}

// --- importance 推定テスト ---

func TestEnrichImportanceBase(t *testing.T) {
	result := ingest.Enrich("just a simple statement")
	if result.Importance < 0.3 || result.Importance > 0.4 {
		t.Errorf("expected importance around 0.3 (base), got %f", result.Importance)
	}
}

func TestEnrichImportanceDecision(t *testing.T) {
	result := ingest.Enrich("We decided to use Kong for CLI")
	if result.Importance < 0.3 {
		t.Errorf("expected importance >= 0.3 for decision, got %f", result.Importance)
	}
}

func TestEnrichImportanceCritical(t *testing.T) {
	result := ingest.Enrich("critical: must not use global state, this is important")
	if result.Importance < 0.8 {
		t.Errorf("expected high importance (>= 0.8) for critical constraint, got %f", result.Importance)
	}
}

func TestEnrichImportanceMax(t *testing.T) {
	result := ingest.Enrich("critical! must not! important! decided! constraint!")
	if result.Importance > 1.0 {
		t.Errorf("importance must not exceed 1.0, got %f", result.Importance)
	}
}

func TestEnrichImportanceFIXME(t *testing.T) {
	result := ingest.Enrich("FIXME: this is a workaround")
	if result.Importance < 0.44 {
		t.Errorf("expected importance >= 0.44 for FIXME, got %f", result.Importance)
	}
}

// --- scope 推定テスト ---

func TestEnrichScopeGlobal(t *testing.T) {
	result := ingest.Enrich("This pattern is globally applicable to all projects")
	if result.Scope != "global" {
		t.Errorf("expected scope=global, got %s", result.Scope)
	}
}

func TestEnrichScopeSimilarity(t *testing.T) {
	// techScore=2 (import, deploy) > contentScore=0 → similarity_shareable
	result := ingest.Enrich("import して deploy する方法")
	if result.Scope != "similarity_shareable" {
		t.Errorf("expected scope=similarity_shareable, got %s", result.Scope)
	}
}

func TestEnrichScopeDefault(t *testing.T) {
	result := ingest.Enrich("this is specific to the current project setup")
	if result.Scope != "project" {
		t.Errorf("expected scope=project (default), got %s", result.Scope)
	}
}

// --- keywords テスト ---

func TestEnrichKeywords(t *testing.T) {
	content := "Go programming language database sqlite connection pool retry"
	result := ingest.Enrich(content)

	var keywords []string
	if err := json.Unmarshal([]byte(result.KeywordsJSON), &keywords); err != nil {
		t.Fatalf("failed to parse keywords_json: %v", err)
	}
	if len(keywords) == 0 {
		t.Error("expected at least 1 keyword")
	}
	if len(keywords) > 10 {
		t.Errorf("expected at most 10 keywords, got %d", len(keywords))
	}
}

func TestEnrichKeywordsStopWords(t *testing.T) {
	content := "the a is are in of the to and for"
	result := ingest.Enrich(content)

	var keywords []string
	if err := json.Unmarshal([]byte(result.KeywordsJSON), &keywords); err != nil {
		t.Fatalf("failed to parse keywords_json: %v", err)
	}
	// ストップワードは除去されているはず
	for _, kw := range keywords {
		switch kw {
		case "the", "a", "is", "are", "in", "of", "to", "and", "for":
			t.Errorf("stop word %q should not appear in keywords", kw)
		}
	}
}

// --- summary テスト ---

func TestEnrichSummaryShort(t *testing.T) {
	content := "short content"
	result := ingest.Enrich(content)
	if result.Summary != content {
		t.Errorf("expected summary=%q, got %q", content, result.Summary)
	}
}

func TestEnrichSummaryLong(t *testing.T) {
	// 100 文字を超えるコンテンツ
	content := "あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわをんあいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよ"
	result := ingest.Enrich(content)
	if !isValidSummary(result.Summary, content) {
		t.Errorf("summary should be truncated with '...', got: %q", result.Summary)
	}
}

func isValidSummary(summary, original string) bool {
	if len([]rune(original)) <= 100 {
		return summary == original
	}
	// 100 文字超の場合は "..." で終わること
	runes := []rune(summary)
	if len(runes) > 103 { // 100 + "..."
		return false
	}
	return true
}
