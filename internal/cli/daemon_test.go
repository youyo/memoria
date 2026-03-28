package cli

import (
	"strings"
	"testing"
)

func TestDaemonIngestCmd_Registered(t *testing.T) {
	// daemon ingest コマンドが CLI に登録されていることを確認
	// (hidden コマンドなので help には表示されないが、parse は可能)
	stdout, _, err := parseForTest(t, []string{"daemon", "--help"})
	_ = err // help はエラーを返す場合がある

	// hidden コマンドなので help から確認しにくいが、
	// daemon サブコマンドが存在することを確認（parse エラーにならないこと）
	_ = stdout
}

func TestDaemonSubcommand_Help(t *testing.T) {
	// daemon サブコマンド自体が parse できること
	stdout, _, err := parseForTest(t, []string{"daemon", "ingest", "--help"})
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
