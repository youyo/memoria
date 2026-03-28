package cli

import (
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/testutil"
)

var _ = testutil.OpenTestDBFull // suppress unused import warning

func TestWorkerStatus_NotRunning(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	stdout, _, err := parseForTestWithDB(t, []string{"worker", "status"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "not_running") {
		t.Errorf("expected 'not_running' in status output, got: %s", stdout)
	}
}

func TestWorkerStatus_JSON(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	stdout, _, err := parseForTestWithDB(t, []string{"--format", "json", "worker", "status"}, database)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, `"status"`) {
		t.Errorf("expected JSON output with 'status' field, got: %s", stdout)
	}
	if !strings.Contains(stdout, "not_running") {
		t.Errorf("expected 'not_running' in JSON output, got: %s", stdout)
	}
}

func TestWorkerStart_NotImplemented(t *testing.T) {
	// worker start は EnsureIngest を呼ぶが、テスト環境ではエラーなく終了するはず
	// DB がない場合も graceful に終了する
	stdout, _, err := parseForTest(t, []string{"worker", "start"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "started" または "already running" のいずれかが出力される
	if !strings.Contains(stdout, "started") && !strings.Contains(stdout, "already running") {
		t.Errorf("expected 'started' or 'already running', got: %s", stdout)
	}
}

func TestWorkerStop_NotRunning(t *testing.T) {
	// worker stop は起動していなければ "was not running" を出力する
	stdout, _, err := parseForTest(t, []string{"worker", "stop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "was not running") && !strings.Contains(stdout, "stopped") {
		t.Errorf("expected 'was not running' or 'stopped', got: %s", stdout)
	}
}

func TestWorkerRestart_NotImplemented(t *testing.T) {
	stdout, _, err := parseForTest(t, []string{"worker", "restart"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "not implemented") {
		t.Errorf("expected 'not implemented', got: %s", stdout)
	}
}
