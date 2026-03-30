package ingest

import "testing"

// inferScope スコアリングロジックの直接テスト（white-box）

// TestInferScope_TechContent は技術指標が高い場合に global / similarity_shareable になることを確認する。
func TestInferScope_TechContent(t *testing.T) {
	tests := []struct {
		id    string
		input string
		kind  string
		want  string
		desc  string
	}{
		{
			id:    "S1",
			input: "func handleError() で panic: を workaround した",
			kind:  "fact",
			want:  "global",
			desc:  "techScore=3 (func,panic:,workaround) > contentScore=0 → global",
		},
		{
			id:    "S5",
			input: "error: nil pointer を .go ファイルで修正",
			kind:  "failure",
			want:  "global",
			desc:  "techScore=3 (error:,.go,failure kind+1) > contentScore=0 → global",
		},
		{
			id:    "S6",
			input: "import して deploy する方法",
			kind:  "fact",
			want:  "similarity_shareable",
			desc:  "techScore=2 (import,deploy) > contentScore=0, techScore<3 → similarity_shareable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := inferScope(tc.input, tc.kind)
			if got != tc.want {
				t.Errorf("%s: inferScope(%q, %q) = %q, want %q (%s)",
					tc.id, tc.input, tc.kind, got, tc.want, tc.desc)
			}
		})
	}
}

// TestInferScope_DomainContent はドメイン・コンテンツ指標が高い場合に project になることを確認する。
func TestInferScope_DomainContent(t *testing.T) {
	tests := []struct {
		id    string
		input string
		kind  string
		want  string
		desc  string
	}{
		{
			id:    "S3",
			input: "stakeholder と議論し requirement を decided",
			kind:  "decision",
			want:  "project",
			desc:  "contentScore=5 (stakeholder,requirement,decided,decision+2) >= 3 → project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := inferScope(tc.input, tc.kind)
			if got != tc.want {
				t.Errorf("%s: inferScope(%q, %q) = %q, want %q (%s)",
					tc.id, tc.input, tc.kind, got, tc.want, tc.desc)
			}
		})
	}
}

// TestInferScope_KindBonus は kind ボーナスによる scope 判定を確認する。
func TestInferScope_KindBonus(t *testing.T) {
	tests := []struct {
		id    string
		input string
		kind  string
		want  string
		desc  string
	}{
		{
			id:    "S2",
			input: "github.com/gorilla/mux の使い方を学んだ",
			kind:  "pattern",
			want:  "global",
			desc:  "techScore=3 (github.com/,pattern kind+2) > contentScore=0 → global",
		},
		{
			id:    "S4",
			input: "認証機能の TODO を追加",
			kind:  "todo",
			want:  "project",
			desc:  "contentScore=3 (todo kind+3) >= 3 → project (コンテンツベトー)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := inferScope(tc.input, tc.kind)
			if got != tc.want {
				t.Errorf("%s: inferScope(%q, %q) = %q, want %q (%s)",
					tc.id, tc.input, tc.kind, got, tc.want, tc.desc)
			}
		})
	}
}

// TestInferScope_Legacy は旧キーワード（後方互換）による強シグナルを確認する。
func TestInferScope_Legacy(t *testing.T) {
	tests := []struct {
		id    string
		input string
		kind  string
		want  string
		desc  string
	}{
		{
			id:    "S7",
			input: "これは汎用的に使える",
			kind:  "fact",
			want:  "global",
			desc:  "techScore=3 (旧キーワード 汎用 +3) > contentScore=0 → global",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := inferScope(tc.input, tc.kind)
			if got != tc.want {
				t.Errorf("%s: inferScope(%q, %q) = %q, want %q (%s)",
					tc.id, tc.input, tc.kind, got, tc.want, tc.desc)
			}
		})
	}
}

// TestInferScope_Default はマッチなしの場合に project（安全デフォルト）になることを確認する。
func TestInferScope_Default(t *testing.T) {
	tests := []struct {
		id    string
		input string
		kind  string
		want  string
		desc  string
	}{
		{
			id:    "S8",
			input: "普通の会話テキスト",
			kind:  "fact",
			want:  "project",
			desc:  "techScore=0, contentScore=0 → デフォルト project",
		},
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := inferScope(tc.input, tc.kind)
			if got != tc.want {
				t.Errorf("%s: inferScope(%q, %q) = %q, want %q (%s)",
					tc.id, tc.input, tc.kind, got, tc.want, tc.desc)
			}
		})
	}
}
