import argparse
import logging
import os
import signal
import sys
import time

# python/ ディレクトリを sys.path に追加（uv run python/worker.py 経由での実行対応）
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# UTC タイムスタンプを使用するよう設定
logging.Formatter.converter = time.gmtime

# Go 側 ingest worker と同じログフォーマットで統一
UVICORN_LOG_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "default": {
            "()": "uvicorn.logging.DefaultFormatter",
            "fmt": "%(asctime)s [%(levelname)s] memoria embedding: %(message)s",
            "datefmt": "%Y-%m-%dT%H:%M:%SZ",
            "use_colors": False,
        },
        "access": {
            "()": "uvicorn.logging.AccessFormatter",
            "fmt": "%(asctime)s [%(levelname)s] memoria embedding: %(message)s",
            "datefmt": "%Y-%m-%dT%H:%M:%SZ",
            "use_colors": False,
        },
    },
    "handlers": {
        "default": {
            "formatter": "default",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
        "access": {
            "formatter": "access",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
    },
    "loggers": {
        "uvicorn": {"handlers": ["default"], "level": "INFO"},
        "uvicorn.error": {"level": "INFO"},
        "uvicorn.access": {"handlers": ["access"], "level": "INFO", "propagate": False},
    },
}


def parse_args(argv=None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="memoria embedding worker")
    parser.add_argument("--uds", required=True, help="Unix Domain Socket path")
    parser.add_argument(
        "--model",
        default="cl-nagoya/ruri-v3-30m",
        help="sentence-transformers model name",
    )
    parser.add_argument(
        "--preload",
        action="store_true",
        help="preload model on startup",
    )
    parser.add_argument(
        "--pid-file",
        default=None,
        help="PID file path (overrides default)",
    )
    parser.add_argument(
        "--lock-file",
        default=None,
        help="lock file path (overrides default)",
    )
    return parser.parse_args(argv)


def main() -> None:
    args = parse_args()

    run_dir = os.path.expanduser("~/.local/state/memoria/run")
    os.makedirs(run_dir, exist_ok=True)

    lock_file = args.lock_file or os.path.join(run_dir, "embed.lock")
    pid_file = args.pid_file or os.path.join(run_dir, "embed.pid")

    from app.lifecycle import LockManager, PidFileManager

    lock = LockManager(lock_file)
    pid_mgr = PidFileManager(pid_file)

    lock.acquire()
    pid_mgr.write()

    # 既存の UDS ファイルを削除（前回の異常終了対策）
    if os.path.exists(args.uds):
        os.unlink(args.uds)

    def _cleanup():
        pid_mgr.cleanup()
        lock.release()
        if os.path.exists(args.uds):
            os.unlink(args.uds)

    def _on_sigterm(signum, frame):
        _cleanup()
        raise SystemExit(0)

    signal.signal(signal.SIGTERM, _on_sigterm)

    from app.main import create_app
    import uvicorn

    app = create_app(
        model_name=args.model,
        preload=args.preload,
    )

    try:
        uvicorn.run(app, uds=args.uds, log_config=UVICORN_LOG_CONFIG)
    finally:
        _cleanup()


if __name__ == "__main__":
    main()
