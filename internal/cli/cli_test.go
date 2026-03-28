package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/youyo/memoria/internal/config"
)

// parseForTest は Kong パーサーをテスト用に構成し、stdout をキャプチャして返す。
// Kong の os.Exit を回避するため kong.New + parser.Parse を使用する。
func parseForTest(t *testing.T, args []string) (stdout string, cli *CLI, err error) {
	t.Helper()

	var c CLI
	var buf bytes.Buffer

	info := &VersionInfo{
		Version: "0.1.0",
		Commit:  "abc1234",
		Date:    "2026-03-28",
	}

	w := io.Writer(&buf)
	cfg := config.DefaultConfig()

	parser, newErr := kong.New(&c,
		kong.Name("memoria"),
		kong.Description("Claude Code 向けプロジェクト認識型ローカル RAG メモリシステム"),
		kong.Writers(&buf, &buf),
		kong.Bind(info),
		kong.Bind(&w),
		kong.Bind(cfg),
		kong.Exit(func(code int) {
			// テスト中は os.Exit しない
		}),
	)
	if newErr != nil {
		return "", nil, newErr
	}

	ctx, parseErr := parser.Parse(args)
	if parseErr != nil {
		return buf.String(), &c, parseErr
	}

	runErr := ctx.Run(&c.Globals)
	return buf.String(), &c, runErr
}

func TestVersionCommand_Text(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "memoria v0.1.0") {
		t.Errorf("expected version string, got: %s", stdout)
	}
	if !strings.Contains(stdout, "abc1234") {
		t.Errorf("expected commit hash, got: %s", stdout)
	}
}

func TestVersionCommand_JSON(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"--format", "json", "version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected valid JSON, got error: %v, output: %s", jsonErr, stdout)
	}
	if result["version"] != "0.1.0" {
		t.Errorf("expected version 0.1.0, got: %s", result["version"])
	}
	if result["commit"] != "abc1234" {
		t.Errorf("expected commit abc1234, got: %s", result["commit"])
	}
	if result["date"] != "2026-03-28" {
		t.Errorf("expected date 2026-03-28, got: %s", result["date"])
	}
}

func TestGlobalFlagHelp(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"--help"})
	// Kong の --help は error を返す場合があるのでエラーは無視し、出力を確認
	_ = err

	expectedCommands := []string{"hook", "worker", "memory", "config", "doctor", "version"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("expected help to contain %q, got: %s", cmd, stdout)
		}
	}
}

func TestHookSubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"hook", "--help"})
	_ = err

	expectedSubs := []string{"session-start", "user-prompt", "stop", "session-end"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected hook help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestWorkerSubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"worker", "--help"})
	_ = err

	expectedSubs := []string{"start", "stop", "restart", "status"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected worker help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestMemorySubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"memory", "--help"})
	_ = err

	expectedSubs := []string{"search", "get", "list", "stats", "reindex"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected memory help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestConfigSubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"config", "--help"})
	_ = err

	expectedSubs := []string{"init", "show", "path", "print-hook"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected config help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestCompletionSubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"completion", "--help"})
	_ = err

	expectedSubs := []string{"bash", "zsh", "fish"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected completion help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestPluginSubcommands(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"plugin", "--help"})
	_ = err

	expectedSubs := []string{"list", "doctor"}
	for _, sub := range expectedSubs {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected plugin help to contain %q, got: %s", sub, stdout)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	_, _, err := parseForTest(t, []string{"nonexistent"})
	if err == nil {
		t.Error("expected error for unknown command, got nil")
	}
}

func TestVerboseFlag(t *testing.T) {
	_, c, err := parseForTest(t, []string{"--verbose", "version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Globals.Verbose {
		t.Error("expected Verbose to be true")
	}
}

func TestNoColorFlag(t *testing.T) {
	_, c, err := parseForTest(t, []string{"--no-color", "version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Globals.NoColor {
		t.Error("expected NoColor to be true")
	}
}

func TestNotImplementedCommands(t *testing.T) {
	// config init/show/path は M02 で実装済みのため除外
	// config print-hook は M12 で実装予定
	commands := [][]string{
		{"doctor"},
		{"hook", "session-start"},
		{"hook", "user-prompt"},
		{"hook", "stop"},
		{"hook", "session-end"},
		{"worker", "start"},
		{"worker", "stop"},
		{"worker", "restart"},
		{"worker", "status"},
		{"memory", "list"},
		{"memory", "stats"},
		{"memory", "reindex"},
		{"config", "print-hook"},
		{"completion", "bash"},
		{"completion", "zsh"},
		{"completion", "fish"},
		{"plugin", "list"},
		{"plugin", "doctor"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			stdout, _, err := parseForTest(t, args)
			if err != nil {
				t.Fatalf("unexpected error for %v: %v", args, err)
			}
			if !strings.Contains(stdout, "not implemented") {
				t.Errorf("expected 'not implemented' for %v, got: %s", args, stdout)
			}
		})
	}
}
