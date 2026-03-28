package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/config"
)

// configInitParseForTest は --config フラグ付きで parseForTest を呼ぶヘルパー。
func configInitParseForTest(t *testing.T, configPath string, args []string) (string, *CLI, error) {
	t.Helper()
	allArgs := append([]string{"--config", configPath}, args...)
	return parseForTest(t, allArgs)
}

// --- config init ---

func TestConfigInit_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	stdout, _, err := configInitParseForTest(t, cfgPath, []string{"config", "init"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("expected config file to exist at %s: %v", cfgPath, statErr)
	}

	if !strings.Contains(stdout, cfgPath) {
		t.Errorf("expected output to contain path %q, got: %s", cfgPath, stdout)
	}
}

func TestConfigInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// 事前にファイルを作成
	if err := config.Save(cfgPath, config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, _, err := configInitParseForTest(t, cfgPath, []string{"config", "init"})
	if err == nil {
		t.Error("expected error when config file already exists, got nil")
	}
}

func TestConfigInit_Force(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// 事前にファイルを作成
	if err := config.Save(cfgPath, config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, _, err := configInitParseForTest(t, cfgPath, []string{"config", "init", "--force"})
	if err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}
}

// --- config show ---

func TestConfigShow_DefaultOutput(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	// config ファイルなし → デフォルト値が TOML で出力される

	stdout, _, err := configInitParseForTest(t, cfgPath, []string{"config", "show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "level") {
		t.Errorf("expected TOML output to contain 'level', got: %s", stdout)
	}
	if !strings.Contains(stdout, "info") {
		t.Errorf("expected TOML output to contain default level 'info', got: %s", stdout)
	}
}

func TestConfigShow_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	stdout, _, err := configInitParseForTest(t, cfgPath, []string{"--format", "json", "config", "show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("expected JSON output to start with '{', got: %s", stdout)
	}
}

func TestConfigShow_CustomConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// カスタム設定を保存
	cfg := config.DefaultConfig()
	cfg.Log.Level = "debug"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stdout, _, err := configInitParseForTest(t, cfgPath, []string{"config", "show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "debug") {
		t.Errorf("expected output to contain custom level 'debug', got: %s", stdout)
	}
}

// --- config path ---

func TestConfigPath_Output(t *testing.T) {
	// デフォルトパスが出力される
	// --config を指定しない場合は config.ConfigFile() のパスが表示される
	stdout, _, err := parseForTest(t, []string{"config", "path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "config.toml") {
		t.Errorf("expected output to contain 'config.toml', got: %s", stdout)
	}
}

func TestConfigPath_CustomPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.toml")

	stdout, _, err := configInitParseForTest(t, cfgPath, []string{"config", "path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, cfgPath) {
		t.Errorf("expected output to contain %q, got: %s", cfgPath, stdout)
	}
}

func TestConfigPath_JSONOutput(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"--format", "json", "config", "path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Errorf("expected JSON output, got: %s", stdout)
	}
}
