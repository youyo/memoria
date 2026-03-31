package logging

import (
	"os"
	"strings"
	"testing"
)

func TestLogf_Format(t *testing.T) {
	// stderr を一時的にキャプチャ
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	Logf("ERROR", "test message: %s\n", "hello")

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// タイムスタンプ形式の確認（ISO8601）
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("expected [ERROR] in output, got: %s", output)
	}
	if !strings.Contains(output, "test message: hello") {
		t.Errorf("expected message in output, got: %s", output)
	}
	// タイムスタンプが先頭にある（T と Z を含む ISO8601）
	if !strings.Contains(output[:25], "T") || !strings.Contains(output[:25], "Z") {
		t.Errorf("expected ISO8601 timestamp at start, got: %s", output[:25])
	}
}

func TestError(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	Error("something failed\n")

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("expected [ERROR] in output, got: %s", output)
	}
}

func TestNewLogf(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = w

	logf := NewLogf("INFO")
	logf("daemon started: pid=%d\n", 12345)

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "[INFO]") {
		t.Errorf("expected [INFO] in output, got: %s", output)
	}
	if !strings.Contains(output, "daemon started: pid=12345") {
		t.Errorf("expected message in output, got: %s", output)
	}
}
