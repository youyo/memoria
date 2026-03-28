package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/youyo/memoria/internal/testutil"
)

var _ = testutil.OpenTestDBFull // suppress unused import warning

func TestWorkerStatus_NotRunning(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	cmd := &WorkerStatusCmd{}
	globals := &Globals{}
	var buf strings.Builder
	w := io.Writer(&buf)

	if err := cmd.RunWithDB(globals, &w, database.SQL()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "not_running") {
		t.Errorf("expected 'not_running' in status output, got: %s", buf.String())
	}
}

func TestWorkerStatus_JSON(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	cmd := &WorkerStatusCmd{}
	globals := &Globals{Format: "json"}
	var buf strings.Builder
	w := io.Writer(&buf)

	if err := cmd.RunWithDB(globals, &w, database.SQL()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `"status"`) {
		t.Errorf("expected JSON output with 'status' field, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "not_running") {
		t.Errorf("expected 'not_running' in JSON output, got: %s", buf.String())
	}
}

func TestWorkerStart_NotImplemented(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	cmd := &WorkerStartCmd{}
	globals := &Globals{}
	var buf strings.Builder
	w := io.Writer(&buf)

	if err := cmd.RunWithDB(globals, &w, database.SQL()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 空の DB では worker は起動していないので "started" が返る
	if !strings.Contains(buf.String(), "started") && !strings.Contains(buf.String(), "already running") {
		t.Errorf("expected 'started' or 'already running', got: %s", buf.String())
	}
}

func TestWorkerStop_NotRunning(t *testing.T) {
	database := testutil.OpenTestDBFull(t)
	runDir := t.TempDir()
	cmd := &WorkerStopCmd{}
	globals := &Globals{}
	var buf strings.Builder
	w := io.Writer(&buf)

	if err := cmd.RunWithDB(globals, &w, database.SQL(), runDir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "was not running") && !strings.Contains(buf.String(), "stopped") {
		t.Errorf("expected 'was not running' or 'stopped', got: %s", buf.String())
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
