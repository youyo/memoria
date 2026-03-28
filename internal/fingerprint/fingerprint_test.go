package fingerprint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/youyo/memoria/internal/fingerprint"
)

// setupDir はテスト用のディレクトリにファイルを作成するヘルパー。
func setupDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return dir
}

func TestDetectPrimaryLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantLang string
	}{
		{
			name: "Go project",
			files: map[string]string{
				"main.go":   "package main",
				"helper.go": "package main",
				"README.md": "# test",
			},
			wantLang: "Go",
		},
		{
			name: "Python project",
			files: map[string]string{
				"main.py":    "print('hello')",
				"utils.py":   "def foo(): pass",
				"models.py":  "class Bar: pass",
				"README.md":  "# test",
			},
			wantLang: "Python",
		},
		{
			name: "TypeScript project",
			files: map[string]string{
				"index.ts":   "const x = 1;",
				"app.ts":     "export default {}",
				"config.ts":  "const c = {}",
				"README.md":  "# test",
			},
			wantLang: "TypeScript",
		},
		{
			name: "JavaScript project",
			files: map[string]string{
				"index.js":  "const x = 1;",
				"app.js":    "module.exports = {}",
				"README.md": "# test",
			},
			wantLang: "JavaScript",
		},
		{
			name: "Rust project",
			files: map[string]string{
				"src/main.rs": "fn main() {}",
				"src/lib.rs":  "pub fn foo() {}",
			},
			wantLang: "Rust",
		},
		{
			name: "empty dir",
			files: map[string]string{
				"README.md": "# empty",
			},
			wantLang: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupDir(t, tt.files)
			lang := fingerprint.DetectPrimaryLanguage(dir)
			if lang != tt.wantLang {
				t.Errorf("DetectPrimaryLanguage() = %q, want %q", lang, tt.wantLang)
			}
		})
	}
}

func TestDetectFrameworks(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantKeys []string
	}{
		{
			name: "Go module",
			files: map[string]string{
				"go.mod": "module example.com/foo\ngo 1.21",
			},
			wantKeys: []string{"go-module"},
		},
		{
			name: "Node.js with package.json",
			files: map[string]string{
				"package.json": `{"name":"test"}`,
			},
			wantKeys: []string{"node"},
		},
		{
			name: "Python with pyproject.toml",
			files: map[string]string{
				"pyproject.toml": "[build-system]",
			},
			wantKeys: []string{"python-pyproject"},
		},
		{
			name: "Rust with Cargo.toml",
			files: map[string]string{
				"Cargo.toml": "[package]",
			},
			wantKeys: []string{"rust-cargo"},
		},
		{
			name: "Docker",
			files: map[string]string{
				"Dockerfile": "FROM ubuntu",
			},
			wantKeys: []string{"docker"},
		},
		{
			name: "Makefile",
			files: map[string]string{
				"Makefile": "build:",
			},
			wantKeys: []string{"make"},
		},
		{
			name: "GitHub Actions",
			files: map[string]string{
				".github/workflows/ci.yml": "name: CI",
			},
			wantKeys: []string{"github-actions"},
		},
		{
			name: "Claude project",
			files: map[string]string{
				".claude/CLAUDE.md": "# project",
			},
			wantKeys: []string{"claude-code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupDir(t, tt.files)
			frameworks := fingerprint.DetectFrameworks(dir)
			fwMap := make(map[string]bool, len(frameworks))
			for _, f := range frameworks {
				fwMap[f] = true
			}
			for _, key := range tt.wantKeys {
				if !fwMap[key] {
					t.Errorf("DetectFrameworks() missing %q, got %v", key, frameworks)
				}
			}
		})
	}
}

func TestGenerateFingerprintText(t *testing.T) {
	info := fingerprint.Info{
		RepoName:        "myproject",
		PrimaryLanguage: "Go",
		Frameworks:      []string{"go-module", "make", "docker"},
		ProjectKind:     "cli",
	}
	text := fingerprint.GenerateFingerprintText(info)
	if text == "" {
		t.Error("GenerateFingerprintText() returned empty string")
	}
	// テキストに主要情報が含まれていることを確認
	if !containsAny(text, "Go", "myproject", "cli") {
		t.Errorf("GenerateFingerprintText() = %q, expected to contain language/repo/kind", text)
	}
}

func TestGenerateFingerprintJSON(t *testing.T) {
	info := fingerprint.Info{
		RepoName:        "myproject",
		PrimaryLanguage: "Go",
		Frameworks:      []string{"go-module"},
		ProjectKind:     "cli",
	}
	jsonStr, err := fingerprint.GenerateFingerprintJSON(info)
	if err != nil {
		t.Fatalf("GenerateFingerprintJSON() error: %v", err)
	}
	if jsonStr == "" {
		t.Error("GenerateFingerprintJSON() returned empty string")
	}
	// JSON として有効であることを確認（簡易チェック）
	if jsonStr[0] != '{' {
		t.Errorf("GenerateFingerprintJSON() does not start with '{': %s", jsonStr)
	}
}

func TestDetectProjectKind(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantKind string
	}{
		{
			name: "CLI with main.go",
			files: map[string]string{
				"main.go":   "package main\nfunc main(){}",
				"go.mod":    "module example.com/foo",
				"cmd/root.go": "package cmd",
			},
			wantKind: "cli",
		},
		{
			name: "Library without main.go",
			files: map[string]string{
				"lib.go":  "package foo",
				"go.mod":  "module example.com/foo",
			},
			wantKind: "library",
		},
		{
			name: "Infrastructure with Terraform",
			files: map[string]string{
				"main.tf": "terraform {}",
			},
			wantKind: "infra",
		},
		{
			name: "Web with package.json",
			files: map[string]string{
				"package.json": `{"name":"web-app"}`,
				"src/index.ts": "const x = 1",
			},
			wantKind: "web",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupDir(t, tt.files)
			kind := fingerprint.DetectProjectKind(dir)
			if kind != tt.wantKind {
				t.Errorf("DetectProjectKind() = %q, want %q", kind, tt.wantKind)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"main.go":  "package main\nfunc main(){}",
		"go.mod":   "module example.com/myapp\ngo 1.21",
		"Makefile": "build:\n\tgo build ./...",
	})

	info, err := fingerprint.Generate(dir)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if info.PrimaryLanguage != "Go" {
		t.Errorf("PrimaryLanguage = %q, want Go", info.PrimaryLanguage)
	}
	if info.ProjectKind == "" {
		t.Error("ProjectKind is empty")
	}
	if info.FingerprintText == "" {
		t.Error("FingerprintText is empty")
	}
	if info.FingerprintJSON == "" {
		t.Error("FingerprintJSON is empty")
	}
	// RepoName はディレクトリ名から取得される
	if info.RepoName == "" {
		t.Error("RepoName is empty")
	}
}

// containsAny は text が words のいずれかを含むかどうかを返す。
func containsAny(text string, words ...string) bool {
	for _, w := range words {
		found := false
		for i := 0; i <= len(text)-len(w); i++ {
			if text[i:i+len(w)] == w {
				found = true
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}
