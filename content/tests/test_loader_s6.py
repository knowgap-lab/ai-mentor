"""S6: хеш каталога и вызов core.

ВОПРОС: адрес core загрузчик берёт из окружения, а в deploy/secrets.md есть
только CONTENT_API_TOKEN. Имени адреса нет. Варианты: не задавать адрес;
выдумать переменную. Не выбираю. LD-R11 и LD-R12 по сети не тестирую.

ОШИБКА ТЕСТА: test_loader.py сверяет data после приведения чисел, колонка
catalog.revision.data имеет тип jsonb. Точные байты D275 в этой колонке
новый тест не требует: смена типа разошлась бы с замороженным чтением.
"""

from __future__ import annotations

import hashlib
import re
import secrets
import shutil
import subprocess
import tarfile

import pytest

import catalog_artifact
import test_loader
from test_loader import (
    POSTGRES_IMAGE,
    SCHEMA,
    _docker,
    _published_port,
    _remove_database,
    _trim,
    _wait_ready,
    load_artifact,
    psql,
)


@pytest.fixture
def dsn():
    """Postgres 16.15. Нет Docker — пропуск."""
    if shutil.which("docker") is None:
        pytest.skip("docker unavailable: executable not found")
    info = _docker(["info", "--format", "{{.ServerVersion}}"], timeout=20)
    if info.returncode != 0:
        pytest.skip(f"docker unavailable: {_trim(info.stderr)}")
    image = _docker(["image", "inspect", POSTGRES_IMAGE], timeout=20)
    if image.returncode != 0:
        pytest.skip(f"image {POSTGRES_IMAGE} unavailable")

    name = "ai-mentor-test-" + secrets.token_hex(4)
    volume = name + "-data"
    container_id: str | None = None
    try:
        started = _docker(
            [
                "run", "-d", "--rm", "--pull", "never", "--name", name,
                "-v", f"{volume}:/var/lib/postgresql/data",
                "-e", "POSTGRES_HOST_AUTH_METHOD=trust",
                "-e", "POSTGRES_DB=ai_mentor_test",
                "-p", "127.0.0.1::5432",
                POSTGRES_IMAGE,
            ],
            timeout=30,
        )
        container_id = started.stdout.strip() or None
        if started.returncode != 0 or container_id is None:
            raise AssertionError(f"docker run: {_trim(started.stderr)}")
        port = _published_port(container_id)
        admin_dsn = (
            f"postgresql://postgres@127.0.0.1:{port}/ai_mentor_test"
            "?sslmode=disable&connect_timeout=2"
        )
        _wait_ready(container_id, admin_dsn)
        subprocess.run(
            ["psql", admin_dsn, "-v", "ON_ERROR_STOP=1", "-X", "-q", "-f", str(SCHEMA)],
            check=True,
            capture_output=True,
            text=True,
        )
        loader_dsn = (
            f"postgresql://loader@127.0.0.1:{port}/ai_mentor_test"
            "?sslmode=disable&connect_timeout=2"
        )
        version = psql(loader_dsn, "SHOW server_version")
        if not version.startswith("16.15"):
            raise AssertionError(f"server version {version}")
        yield loader_dsn
    finally:
        _remove_database(container_id, volume)


def test_ld_r11_s6_loader_calls_core() -> None:
    """LD-R11. Сеть к core не проверяется: нет имени адреса."""
    pytest.skip(
        "ВОПРОС: в deploy/secrets.md нет имени переменной с адресом core; "
        "токен — CONTENT_API_TOKEN"
    )


def test_ld_r12_s6_loader08_calls_recheck() -> None:
    """LD-R12, loader-08. Сеть к core не проверяется: нет имени адреса."""
    pytest.skip(
        "ВОПРОС: в deploy/secrets.md нет имени переменной с адресом core; "
        "токен — CONTENT_API_TOKEN"
    )


def test_d275_s6_content_hash_is_lower_hex(
    dsn: str, catalog_root, tmp_path
) -> None:
    """D275: content_hash — нижний hex SHA-256 канонических байтов, не байты в jsonb."""
    if "zst" not in tarfile.TarFile.OPEN_METH:
        pytest.skip("нужен Python 3.14: tarfile не умеет zst")
    out = tmp_path / "v1.tar.zst"
    test_loader.build(catalog_root, out, commit="commit-s6", catalog_version=1)
    verified = catalog_artifact.verify_artifact(out)
    load_artifact(dsn, out, environment="staging")
    rows = {row["entity_id"]: row for row in test_loader.revisions(dsn)}
    assert rows
    for entity in verified.entities:
        row = rows[entity.id]
        assert re.fullmatch(r"[0-9a-f]{64}", row["content_hash"])
        assert row["content_hash"] == entity.content_hash
        assert hashlib.sha256(entity.data).hexdigest() == row["content_hash"]
        assert isinstance(row["data"], dict)
        assert row["data"].get("id") == entity.id
