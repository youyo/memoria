import os

import pytest

from app.lifecycle import LockManager, PidFileManager


def test_pid_file_write_and_cleanup(tmp_path):
    pid_file = tmp_path / "embed.pid"
    mgr = PidFileManager(str(pid_file))
    mgr.write()
    assert pid_file.exists()
    assert pid_file.read_text().strip() == str(os.getpid())
    mgr.cleanup()
    assert not pid_file.exists()


def test_pid_file_cleanup_noop_if_not_exists(tmp_path):
    pid_file = tmp_path / "nonexistent.pid"
    mgr = PidFileManager(str(pid_file))
    mgr.cleanup()  # エラーなし


def test_lock_manager_acquires(tmp_path):
    lock_file = tmp_path / "embed.lock"
    mgr = LockManager(str(lock_file))
    mgr.acquire()
    mgr.release()


def test_lock_manager_double_acquire_fails(tmp_path):
    lock_file = tmp_path / "embed.lock"
    mgr1 = LockManager(str(lock_file))
    mgr2 = LockManager(str(lock_file))
    mgr1.acquire()
    try:
        with pytest.raises(RuntimeError, match="failed to acquire lock"):
            mgr2.acquire()
    finally:
        mgr1.release()


def test_lock_manager_release_after_acquire(tmp_path):
    lock_file = tmp_path / "embed.lock"
    mgr = LockManager(str(lock_file))
    mgr.acquire()
    mgr.release()
    # 解放後は再取得できる
    mgr2 = LockManager(str(lock_file))
    mgr2.acquire()
    mgr2.release()


def test_lock_manager_release_without_acquire(tmp_path):
    lock_file = tmp_path / "embed.lock"
    mgr = LockManager(str(lock_file))
    # 取得せずに解放してもエラーなし
    mgr.release()


def test_pid_file_write_creates_parent_dirs(tmp_path):
    nested_dir = tmp_path / "nested" / "deep"
    pid_file = nested_dir / "embed.pid"
    mgr = PidFileManager(str(pid_file))
    mgr.write()
    assert pid_file.exists()
