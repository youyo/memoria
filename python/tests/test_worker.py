import sys
import os

# worker.py のあるディレクトリを sys.path に追加
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from worker import parse_args


def test_parse_args_uds():
    args = parse_args(["--uds", "/tmp/test.sock"])
    assert args.uds == "/tmp/test.sock"
    assert args.preload is False


def test_parse_args_preload():
    args = parse_args(["--uds", "/tmp/test.sock", "--preload"])
    assert args.preload is True


def test_parse_args_model():
    args = parse_args(["--uds", "/tmp/test.sock", "--model", "cl-nagoya/ruri-v3-30m"])
    assert args.model == "cl-nagoya/ruri-v3-30m"


def test_parse_args_defaults():
    args = parse_args(["--uds", "/tmp/test.sock"])
    assert args.model == "cl-nagoya/ruri-v3-30m"


def test_parse_args_pid_file():
    args = parse_args(["--uds", "/tmp/test.sock", "--pid-file", "/tmp/test.pid"])
    assert args.pid_file == "/tmp/test.pid"


def test_parse_args_lock_file():
    args = parse_args(["--uds", "/tmp/test.sock", "--lock-file", "/tmp/test.lock"])
    assert args.lock_file == "/tmp/test.lock"


def test_parse_args_no_pid_file_default():
    args = parse_args(["--uds", "/tmp/test.sock"])
    assert args.pid_file is None


def test_parse_args_no_lock_file_default():
    args = parse_args(["--uds", "/tmp/test.sock"])
    assert args.lock_file is None
