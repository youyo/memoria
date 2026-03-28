import fcntl
import os
import signal
import threading
import time


class IdleTimer:
    """指定された秒数リクエストがない場合に SIGTERM を発火する。"""

    def __init__(self, timeout_seconds: int = 600):
        self._timeout = timeout_seconds
        self._last_request_at = time.monotonic()
        self._lock = threading.Lock()

    def touch(self) -> None:
        """リクエスト受信時に呼ぶ。"""
        with self._lock:
            self._last_request_at = time.monotonic()

    def is_timed_out(self) -> bool:
        with self._lock:
            return time.monotonic() - self._last_request_at > self._timeout

    def start(self) -> None:
        """バックグラウンドスレッドを起動する。daemon=True で本体終了時に自動終了。"""
        t = threading.Thread(target=self._check_loop, daemon=True)
        t.start()

    def _check_loop(self) -> None:
        """10秒おきにタイムアウトをチェックし、超過時に SIGTERM を発火。"""
        while True:
            time.sleep(10)
            if self.is_timed_out():
                os.kill(os.getpid(), signal.SIGTERM)
                break


class PidFileManager:
    """PID ファイルの作成と削除を管理する。"""

    def __init__(self, pid_file: str):
        self._pid_file = pid_file

    def write(self) -> None:
        """現在のプロセスの PID をファイルに書き込む。"""
        os.makedirs(os.path.dirname(self._pid_file), exist_ok=True)
        with open(self._pid_file, "w") as f:
            f.write(str(os.getpid()))

    def cleanup(self) -> None:
        """PID ファイルを削除する。存在しない場合は何もしない。"""
        try:
            os.unlink(self._pid_file)
        except FileNotFoundError:
            pass


class LockManager:
    """fcntl.flock を使った排他ロックによる多重起動防止。"""

    def __init__(self, lock_file: str):
        self._lock_file = lock_file
        self._fd: int | None = None

    def acquire(self) -> None:
        """排他ロックを取得する。失敗時は RuntimeError を発生させる。"""
        os.makedirs(os.path.dirname(self._lock_file), exist_ok=True)
        fd = os.open(self._lock_file, os.O_CREAT | os.O_WRONLY)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError as e:
            os.close(fd)
            raise RuntimeError(f"failed to acquire lock: {self._lock_file}") from e
        self._fd = fd

    def release(self) -> None:
        """ロックを解放する。"""
        if self._fd is not None:
            try:
                fcntl.flock(self._fd, fcntl.LOCK_UN)
                os.close(self._fd)
            except OSError:
                pass
            self._fd = None
