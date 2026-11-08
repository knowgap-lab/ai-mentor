"""Публичная модель проблемы проверки каталога."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Problem:
    """Одна проблема в исходниках каталога."""

    code: str
    file: str
    pointer: str
    message: str
