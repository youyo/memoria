package worker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// FileLock は syscall.Flock を使ったファイルロックを表す。
// プロセス終了時にカーネルが自動解放する。
type FileLock struct {
	path string
	fd   *os.File
}

// AcquireLock は path に対して排他ロック（LOCK_EX | LOCK_NB）を取得する。
// 既に別プロセスがロックを保持している場合は ErrLockHeld を返す。
func AcquireLock(path string) (*FileLock, error) {
	// ファイルを作成または開く
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// 非ブロッキング排他ロック
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, ErrLockHeld
		}
		return nil, fmt.Errorf("flock: %w", err)
	}

	return &FileLock{path: path, fd: f}, nil
}

// Release はファイルロックを解放する（fd クローズで自動解放）。
func (l *FileLock) Release() error {
	return l.fd.Close()
}

// ErrLockHeld はファイルロックが既に保持されているエラー。
var ErrLockHeld = fmt.Errorf("lock already held by another process")

// WritePID は path に pid を書き込む。
func WritePID(path string, pid int) error {
	content := strconv.Itoa(pid) + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

// ReadPID は path から PID を読み取る。
// ファイルが存在しない場合は 0, nil を返す。
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, nil
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("parse pid %q: %w", pidStr, err)
	}
	return pid, nil
}

// RemovePID は PID ファイルを削除する。
// ファイルが存在しない場合は nil を返す（冪等）。
func RemovePID(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RemoveFile はファイルを削除する（stop ファイルなどの汎用削除）。
// ファイルが存在しない場合は nil を返す（冪等）。
func RemoveFile(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// TouchFile はファイルを作成する（stop シグナルファイルなど）。
func TouchFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("touch file %q: %w", path, err)
	}
	return f.Close()
}

// FileExists はファイルの存在を確認する。
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
