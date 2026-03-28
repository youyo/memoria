// Package fingerprint はプロジェクトの特性を抽出してフィンガープリントを生成する。
// ファイル拡張子、設定ファイルの存在、ディレクトリ構造から
// プロジェクトの言語・フレームワーク・種別を推定する。
package fingerprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Info はプロジェクトのフィンガープリント情報を保持する。
type Info struct {
	// RepoName はリポジトリ名（ルートディレクトリのベース名）。
	RepoName string `json:"repo_name"`
	// PrimaryLanguage は最も多く使われているプログラミング言語。
	PrimaryLanguage string `json:"primary_language"`
	// Frameworks はフレームワーク・ビルドツール・設定ファイルの存在に基づく識別子のリスト。
	Frameworks []string `json:"frameworks"`
	// ProjectKind はプロジェクトの種別（cli, web, library, infra, etc）。
	ProjectKind string `json:"project_kind"`
	// FingerprintText は embedding 対象の自然言語表現。
	FingerprintText string `json:"fingerprint_text,omitempty"`
	// FingerprintJSON は構造化表現の JSON 文字列。
	FingerprintJSON string `json:"fingerprint_json,omitempty"`
}

// extensionToLanguage は拡張子から言語名へのマッピング。
var extensionToLanguage = map[string]string{
	".go":    "Go",
	".py":    "Python",
	".ts":    "TypeScript",
	".tsx":   "TypeScript",
	".js":    "JavaScript",
	".jsx":   "JavaScript",
	".rs":    "Rust",
	".java":  "Java",
	".rb":    "Ruby",
	".cs":    "CSharp",
	".cpp":   "C++",
	".cc":    "C++",
	".cxx":   "C++",
	".c":     "C",
	".h":     "C",
	".swift": "Swift",
	".kt":    "Kotlin",
	".scala": "Scala",
	".php":   "PHP",
	".lua":   "Lua",
	".zig":   "Zig",
}

// frameworkIndicators はファイル/ディレクトリの存在からフレームワークを検出する。
// key: フレームワーク識別子、value: ルートからの相対パス（ディレクトリは末尾に "/"）
var frameworkIndicators = []struct {
	key  string
	path string
}{
	{"go-module", "go.mod"},
	{"node", "package.json"},
	{"python-pyproject", "pyproject.toml"},
	{"python-requirements", "requirements.txt"},
	{"rust-cargo", "Cargo.toml"},
	{"ruby-bundle", "Gemfile"},
	{"java-maven", "pom.xml"},
	{"java-gradle", "build.gradle"},
	{"dotnet", "*.csproj"},
	{"docker", "Dockerfile"},
	{"make", "Makefile"},
	{"goreleaser", ".goreleaser.yaml"},
	{"goreleaser", ".goreleaser.yml"},
	{"github-actions", ".github/workflows/"},
	{"claude-code", ".claude/"},
	{"claude-skills", "skills/"},
	{"terraform", "main.tf"},
	{"terraform", "terraform.tf"},
	{"helm", "Chart.yaml"},
	{"compose", "docker-compose.yml"},
	{"compose", "docker-compose.yaml"},
	{"mise", "mise.toml"},
	{"uv", "uv.lock"},
}

// DetectPrimaryLanguage はディレクトリを再帰的に走査し、
// 最も多く使われているプログラミング言語を返す。
// 言語が検出できない場合は "Unknown" を返す。
func DetectPrimaryLanguage(root string) string {
	langCount := make(map[string]int)

	walkFunc := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// 隠しディレクトリと vendor をスキップ
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if lang, ok := extensionToLanguage[ext]; ok {
			langCount[lang]++
		}
		return nil
	}

	// filepath.WalkDir の代わりに os.ReadDir ベースで実装（go 1.16+）
	walkDir(root, walkFunc)

	if len(langCount) == 0 {
		return "Unknown"
	}

	// 最も多い言語を選択（同数の場合はアルファベット順で先のもの）
	type pair struct {
		lang  string
		count int
	}
	pairs := make([]pair, 0, len(langCount))
	for lang, count := range langCount {
		pairs = append(pairs, pair{lang, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].lang < pairs[j].lang
	})
	return pairs[0].lang
}

// walkDir は root を再帰的に走査して fn を呼ぶ。
// filepath.WalkDir と互換の関数シグネチャを使用。
func walkDir(root string, fn func(path string, d os.DirEntry, err error) error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		fn(root, nil, err)
		return
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if err := fn(path, entry, nil); err != nil {
			if err == filepath.SkipDir {
				continue
			}
			return
		}
		if entry.IsDir() {
			walkDir(path, fn)
		}
	}
}

// DetectFrameworks はルートディレクトリの特定ファイル/ディレクトリの存在から
// フレームワーク識別子のスライスを返す。
func DetectFrameworks(root string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, indicator := range frameworkIndicators {
		if seen[indicator.key] {
			continue
		}

		path := filepath.Join(root, filepath.FromSlash(indicator.path))

		// ディレクトリの場合（末尾に "/" を含む場合）
		if strings.HasSuffix(indicator.path, "/") {
			dirPath := strings.TrimSuffix(path, string(os.PathSeparator))
			if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
				seen[indicator.key] = true
				result = append(result, indicator.key)
			}
			continue
		}

		// ワイルドカード（*.csproj など）
		if strings.Contains(indicator.path, "*") {
			dir := filepath.Dir(path)
			pattern := filepath.Base(indicator.path)
			entries, err := os.ReadDir(dir)
			if err == nil {
				for _, entry := range entries {
					matched, _ := filepath.Match(pattern, entry.Name())
					if matched {
						seen[indicator.key] = true
						result = append(result, indicator.key)
						break
					}
				}
			}
			continue
		}

		// 通常ファイル
		if _, err := os.Stat(path); err == nil {
			seen[indicator.key] = true
			result = append(result, indicator.key)
		}
	}

	return result
}

// DetectProjectKind はプロジェクトの種別を推定する。
// 優先順位: infra > web > cli > library > unknown
func DetectProjectKind(root string) string {
	// インフラ系
	if hasFile(root, "main.tf") || hasFile(root, "terraform.tf") || hasFile(root, "Chart.yaml") {
		return "infra"
	}

	// Web系（package.json があれば web）
	if hasFile(root, "package.json") {
		return "web"
	}

	// CLI系（main.go が存在し、cmd/ ディレクトリがある、または main 関数がある）
	if hasFile(root, "main.go") || hasDir(root, "cmd") {
		return "cli"
	}

	// Go プロジェクトで main.go がない場合はライブラリ
	if hasFile(root, "go.mod") {
		return "library"
	}

	// Python
	if hasFile(root, "pyproject.toml") || hasFile(root, "setup.py") {
		return "library"
	}

	// Rust
	if hasFile(root, "Cargo.toml") {
		if hasFile(root, "src/main.rs") {
			return "cli"
		}
		return "library"
	}

	return "unknown"
}

// hasFile はルートディレクトリに指定ファイルが存在するか確認する。
func hasFile(root, relPath string) bool {
	_, err := os.Stat(filepath.Join(root, relPath))
	return err == nil
}

// hasDir はルートディレクトリに指定ディレクトリが存在するか確認する。
func hasDir(root, name string) bool {
	info, err := os.Stat(filepath.Join(root, name))
	return err == nil && info.IsDir()
}

// GenerateFingerprintText は Info から embedding 対象の自然言語テキストを生成する。
func GenerateFingerprintText(info Info) string {
	var parts []string

	// 言語
	if info.PrimaryLanguage != "" && info.PrimaryLanguage != "Unknown" {
		parts = append(parts, info.PrimaryLanguage)
	}

	// プロジェクト種別
	if info.ProjectKind != "" && info.ProjectKind != "unknown" {
		parts = append(parts, fmt.Sprintf("%s project", info.ProjectKind))
	}

	// フレームワーク/ツール
	if len(info.Frameworks) > 0 {
		parts = append(parts, "using "+strings.Join(info.Frameworks, ", "))
	}

	// リポジトリ名
	if info.RepoName != "" {
		parts = append(parts, "repository: "+info.RepoName)
	}

	if len(parts) == 0 {
		return "unknown project"
	}
	return strings.Join(parts, ". ") + "."
}

// GenerateFingerprintJSON は Info を JSON 文字列に変換する（FingerprintText/JSON フィールドを除く）。
func GenerateFingerprintJSON(info Info) (string, error) {
	// 循環参照を避けるためにシンプルな構造体を使う
	type jsonInfo struct {
		RepoName        string   `json:"repo_name"`
		PrimaryLanguage string   `json:"primary_language"`
		Frameworks      []string `json:"frameworks"`
		ProjectKind     string   `json:"project_kind"`
	}
	j := jsonInfo{
		RepoName:        info.RepoName,
		PrimaryLanguage: info.PrimaryLanguage,
		Frameworks:      info.Frameworks,
		ProjectKind:     info.ProjectKind,
	}
	if j.Frameworks == nil {
		j.Frameworks = []string{}
	}
	b, err := json.Marshal(j)
	if err != nil {
		return "", fmt.Errorf("marshal fingerprint: %w", err)
	}
	return string(b), nil
}

// Generate は rootPath を解析してフィンガープリント情報を生成する。
// git ls-files の代わりに os.ReadDir ベースの走査を使用するため、
// git がない環境でも動作する。
func Generate(rootPath string) (Info, error) {
	repoName := filepath.Base(rootPath)
	primaryLang := DetectPrimaryLanguage(rootPath)
	frameworks := DetectFrameworks(rootPath)
	projectKind := DetectProjectKind(rootPath)

	info := Info{
		RepoName:        repoName,
		PrimaryLanguage: primaryLang,
		Frameworks:      frameworks,
		ProjectKind:     projectKind,
	}

	text := GenerateFingerprintText(info)
	info.FingerprintText = text

	jsonStr, err := GenerateFingerprintJSON(info)
	if err != nil {
		return info, fmt.Errorf("generate fingerprint json: %w", err)
	}
	info.FingerprintJSON = jsonStr

	return info, nil
}
