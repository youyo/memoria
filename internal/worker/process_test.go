package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWritePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	if err := WritePID(pidPath, 12345); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	pid, err := ReadPID(pidPath)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected pid 12345, got %d", pid)
	}
}

func TestRemovePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	if err := WritePID(pidPath, 99); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	if err := RemovePID(pidPath); err != nil {
		t.Fatalf("RemovePID: %v", err)
	}

	// ファイルが消えていることを確認
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected pid file to be removed")
	}

	// 冪等性: 再度削除してもエラーにならない
	if err := RemovePID(pidPath); err != nil {
		t.Errorf("RemovePID(not exist) should be nil, got: %v", err)
	}
}

func TestReadPID_NotExist(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "nonexistent.pid")

	pid, err := ReadPID(pidPath)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent file, got: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected pid 0 for nonexistent file, got %d", pid)
	}
}

func TestAcquireLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	lock, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	// ロックファイルが存在することを確認
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file should exist: %v", err)
	}
}

func TestDoubleAcquireLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "double.lock")

	lock1, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock (first): %v", err)
	}
	defer lock1.Release()

	// 2つ目はロック取得に失敗するはず
	_, err = AcquireLock(lockPath)
	if err == nil {
		t.Fatal("expected error for double lock, got nil")
	}
	if err != ErrLockHeld {
		t.Errorf("expected ErrLockHeld, got: %v", err)
	}
}

func TestTouchFile(t *testing.T) {
	dir := t.TempDir()
	stopPath := filepath.Join(dir, "test.stop")

	if err := TouchFile(stopPath); err != nil {
		t.Fatalf("TouchFile: %v", err)
	}
	if !FileExists(stopPath) {
		t.Error("expected stop file to exist after TouchFile")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")

	if FileExists(path) {
		t.Error("expected false for non-existing file")
	}

	if err := TouchFile(path); err != nil {
		t.Fatalf("TouchFile: %v", err)
	}

	if !FileExists(path) {
		t.Error("expected true for existing file")
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "removeme.txt")

	if err := TouchFile(path); err != nil {
		t.Fatalf("TouchFile: %v", err)
	}

	if err := RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}

	if FileExists(path) {
		t.Error("expected file to be removed")
	}

	// 冪等性
	if err := RemoveFile(path); err != nil {
		t.Errorf("RemoveFile(not exist) should be nil: %v", err)
	}
}
