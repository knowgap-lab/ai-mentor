#!/usr/bin/env python3
"""DS-R1 (docs/platform/design.md): цвета задаются только в web/src/design/.

Ищет в web/src (кроме web/src/design/ и тестов) шестнадцатеричные цвета, rgb(), hsl()
и именованные цвета CSS в значениях color, background и border. Код выхода 1 при находке.
"""
import re
import sys
from pathlib import Path

root = Path(__file__).resolve().parent.parent
src = root / "web" / "src"
allowed = src / "design"

HEX = re.compile(r"#[0-9a-fA-F]{3,8}\b")
FUNC = re.compile(r"\b(?:rgba?|hsla?|oklch|lab|lch)\(")
NAMED = re.compile(
    r"\b(?:color|background(?:-color)?|border(?:-color)?|fill|stroke)\s*:\s*['\"]?"
    r"(?:red|green|blue|black|white|gray|grey|orange|yellow|purple|pink|brown)\b",
    re.IGNORECASE,
)

problems = []
for path in sorted(src.rglob("*")):
    if path.suffix not in {".ts", ".tsx", ".css"} or allowed in path.parents:
        continue
    if ".test." in path.name:
        continue
    for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        code = line.split("//", 1)[0] if path.suffix != ".css" else line
        # Имена цветов темы в пропсах Mantine (color="red") разрешены; именованные цвета CSS
        # ищем только в CSS и во встроенных style.
        patterns = (HEX, FUNC, NAMED) if path.suffix == ".css" or "style" in code else (HEX, FUNC)
        for pattern in patterns:
            m = pattern.search(code)
            if m:
                problems.append(f"{path.relative_to(root)}:{n}: {m.group(0)}")
                break

if problems:
    print("Цвет вне web/src/design (DS-R1):")
    print("\n".join(problems))
    sys.exit(1)
print("DS-R1: OK")
