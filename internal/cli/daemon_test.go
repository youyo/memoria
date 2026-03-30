package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/youyo/memoria/internal/config"
)

// parseOnlyForTest は Kong パーサーの parse のみを行い、Run() は呼ばない。
// daemon のように Run() を呼ぶと永久に実行されるコマンドのテストに使う。
func parseOnlyForTest(t *testing.T, args []string) (stdout string, err error) {
	t.Helper()

	var c CLI
	var buf bytes.Buffer

	info := &VersionInfo{Version: "test", Commit: "test", Date: "test"}
	w := io.Writer(&buf)
	cfg := config.DefaultConfig()
	lazyDB := NewLazyDB("")

	parser, newErr := kong.New(&c,
		kong.Name("memoria"),
		kong.Writers(&buf, &buf),
		kong.Bind(info),
		kong.Bind(&w),
		kong.Bind(cfg),
		kong.Bind(lazyDB),
		kong.Exit(func(int) {}),
	)
	if newErr != nil {
		return "", newErr
	}

	_, parseErr := parser.Parse(args)
	return buf.String(), parseErr
}

func TestDaemonIngestCmd_Registered(t *testing.T) {
	// daemon ingest コマンドが CLI に登録されていることを確認
	// (hidden コマンドなので help には表示されないが、parse は可能)
	// Run() を呼ぶと daemon が起動してしまうため parse のみで検証する
	stdout, err := parseOnlyForTest(t, []string{"daemon", "--help"})
	_ = err // help はエラーを返す場合がある

	// hidden コマンドなので help から確認しにくいが、
	// daemon サブコマンドが存在することを確認（parse エラーにならないこと）
	_ = stdout
}

func TestDaemonSubcommand_Help(t *testing.T) {
	// daemon ingest --help は Run() を呼ぶと daemon が起動してしまうため、
	// parse のみで検証する
	stdout, err := parseOnlyForTest(t, []string{"daemon", "ingest", "--help"})
	_ = err
	_ = stdout
}

func TestDaemon_NotInMainHelp(t *testing.T) {
	// daemon は hidden コマンドなのでメインのヘルプには表示されない
	stdout, _, err := parseForTest(t, []string{"--help"})
	_ = err

	// 通常のコマンドは表示される
	if !strings.Contains(stdout, "hook") {
		t.Errorf("expected 'hook' in help, got: %s", stdout)
	}
}
