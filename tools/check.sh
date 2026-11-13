#!/usr/bin/env bash
# Все проверки документации: контракты и примеры (D133). Нужны python3 с jsonschema, pyyaml, openapi-spec-validator.
set -e
cd "$(dirname "$0")/.."
python3 docs/contracts/check.py
python3 tools/examples/check_answer_rules.py
python3 tools/examples/check_scenarios.py
python3 tools/examples/check_schema_examples.py
python3 tools/examples/check_text_examples.py
python3 tools/examples/check_consent.py
python3 tools/lint_docs.py
(cd content && python3 -m pytest -q)
