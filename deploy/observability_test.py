"""OP-R16, OP-R17 и OP-R18 без секретов и без Docker.

Строки примеров: диск 85 % — тревога один раз; три неудачи подряд — тревога;
журнал старше 30 дней удалён.
"""

from __future__ import annotations

import os
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

DEPLOY = Path(__file__).resolve().parent


def _run(script: str, *args: str) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env.pop("TELEGRAM_BOT_TOKEN", None)
    env.pop("OPS_CHAT_ID", None)
    return subprocess.run(
        [str(DEPLOY / script), *args],
        check=False,
        capture_output=True,
        text=True,
        env=env,
    )


def test_op_r16_disk_85_alerts_once(tmp_path: Path) -> None:
    state = tmp_path / "disk"
    first = _run("disk-check.sh", "85", str(state))
    second = _run("disk-check.sh", "85", str(state))
    if first.returncode != 0 or second.returncode != 0:
        raise AssertionError(first.stderr + second.stderr)
    if first.stdout != "диск > 80 %\n":
        raise AssertionError(first.stdout)
    if second.stdout != "":
        raise AssertionError(second.stdout)
    at_limit = _run("disk-check.sh", "80", str(state))
    over = _run("disk-check.sh", "81", str(state))
    if at_limit.stdout != "":
        raise AssertionError(at_limit.stdout)
    if over.stdout != "диск > 80 %\n":
        raise AssertionError(over.stdout)


class _Status(BaseHTTPRequestHandler):
    code = 200

    def do_GET(self) -> None:
        self.send_response(self.code)
        self.end_headers()

    def log_message(self, format: str, *args: object) -> None:
        return


def _serve(code: int) -> tuple[ThreadingHTTPServer, str]:
    handler = type("Status", (_Status,), {"code": code})
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    host, port = server.server_address[:2]
    return server, f"http://{host}:{port}/status"


def test_alert_send_curl_has_timeouts() -> None:
    text = (DEPLOY / "alert-send.sh").read_text(encoding="utf-8")
    if "--connect-timeout" not in text or "--max-time" not in text:
        raise AssertionError("curl без ограничения времени")


def test_op_r17_status_url_trailing_slash(tmp_path: Path) -> None:
    up, up_url = _serve(200)
    state = tmp_path / "status"
    try:
        result = _run("status-check.sh", f"{up_url}/", str(state))
        if result.returncode != 0:
            raise AssertionError(result.stderr or result.stdout)
        if state.read_text(encoding="utf-8") != "0\n":
            raise AssertionError(state.read_text(encoding="utf-8"))
    finally:
        up.shutdown()


def test_op_r17_three_failures_alert(tmp_path: Path) -> None:
    down, down_url = _serve(503)
    up, up_url = _serve(200)
    state = tmp_path / "status"
    try:
        outputs = [
            _run("status-check.sh", down_url, str(state)).stdout for _ in range(4)
        ]
        if outputs != ["", "", "сервис недоступен\n", ""]:
            raise AssertionError(outputs)
        reset = _run("status-check.sh", up_url, str(state))
        if reset.returncode != 0 or reset.stdout != "":
            raise AssertionError(reset.stdout or reset.stderr)
        again = [_run("status-check.sh", down_url, str(state)).stdout for _ in range(3)]
        if again != ["", "", "сервис недоступен\n"]:
            raise AssertionError(again)
    finally:
        down.shutdown()
        up.shutdown()


def test_op_r16_service_chat_alerts() -> None:
    expected = {
        "catalog": "неудачная загрузка каталога\n",
        "recheck": "неудачный пакет перепроверки\n",
        "job": "задание в failed\n",
        "not-me": "Это не я\n",
    }
    for reason, text in expected.items():
        result = _run("ops-alert.sh", reason)
        if result.returncode != 0 or result.stdout != text:
            raise AssertionError(f"{reason}: {result.stdout!r} {result.stderr}")


def test_op_r18_log_older_than_30_days_deleted(tmp_path: Path) -> None:
    old = tmp_path / "old.log"
    fresh = tmp_path / "fresh.log"
    old.write_text("line\n", encoding="utf-8")
    fresh.write_text("line\n", encoding="utf-8")
    now = time.time()
    day = 24 * 60 * 60
    os.utime(old, (now - 31 * day, now - 31 * day))
    os.utime(fresh, (now - day, now - day))
    result = _run("log-retain.sh", str(tmp_path))
    if result.returncode != 0:
        raise AssertionError(result.stderr)
    if old.exists():
        raise AssertionError("журнал старше 30 дней остался")
    if fresh.read_text(encoding="utf-8") != "line\n":
        raise AssertionError("свежий журнал изменён")
