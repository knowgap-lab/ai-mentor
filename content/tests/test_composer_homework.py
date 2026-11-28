"""Разбор списков типов и минут в нагрузке домашнего задания ct-2027."""

from __future__ import annotations

from decimal import Decimal

from composer.compose import (
    _BANK_REF_PLAIN,
    _BUDGET,
    _BUDGET_ALT,
    _budget_hours,
    _plain_type_numbers,
)


def _numbers(text: str) -> list[int]:
    match = _BANK_REF_PLAIN.search(text)
    assert match is not None
    return _plain_type_numbers(match.group(1))


def test_plain_bank_ref_keeps_and_and_range() -> None:
    assert _numbers("типы 6 и 7 банка M02") == [6, 7]
    assert _numbers("типы 6–7 банка M02") == [6, 7]
    assert _numbers("типы 6-7 банка M02") == [6, 7]
    assert _numbers("типы 6—7 банка M02") == [6, 7]


def test_budget_keeps_hours_and_minutes() -> None:
    six_twenty = Decimal(6) + Decimal(20) / Decimal(60)
    six_forty = Decimal(6) + Decimal(40) / Decimal(60)
    marked = _BUDGET.search("Ориентир ≈ 6 ч 20 мин")
    plain = _BUDGET_ALT.search("Ориентир 6 ч 20 мин за неделю")
    bold = _BUDGET_ALT.search("**Ориентир 6 ч 40 мин**")
    assert marked is not None and plain is not None and bold is not None
    assert _budget_hours(marked) == six_twenty
    assert _budget_hours(plain) == six_twenty
    assert _budget_hours(bold) == six_forty
    hours_only = _BUDGET.search("Нагрузка ≈ 1 ч")
    assert hours_only is not None
    assert _budget_hours(hours_only) == Decimal(1)
