// Пишет web/src/design/theme.css из темы (DS-R5). `--check` — сверить и упасть при расхождении.
// Mantine не вставляет переменные во время работы: CSP запрещает встроенные стили (D167).
import { readFileSync, writeFileSync } from "node:fs";
import process from "node:process";
import { URL, fileURLToPath } from "node:url";
import {
  DEFAULT_THEME,
  convertCssVariables,
  defaultCssVariablesResolver,
  mergeMantineTheme,
} from "@mantine/core";
import { theme } from "../src/design/theme.ts";

const target = fileURLToPath(
  new URL("../src/design/theme.css", import.meta.url),
);
const vars = defaultCssVariablesResolver(
  mergeMantineTheme(DEFAULT_THEME, theme),
);
const body = convertCssVariables(
  { variables: { ...vars.variables, ...vars.light }, light: {}, dark: {} },
  ":root",
);
const css = `/* Сгенерировано: npm run theme:css (docs/platform/design.md, DS-R5). Не править руками. */\n${body}\n`;

if (process.argv.includes("--check")) {
  let current = "";
  try {
    current = readFileSync(target, "utf8");
  } catch {
    // нет файла — тоже расхождение
  }
  if (current !== css) {
    process.stderr.write(
      "web/src/design/theme.css устарел: запустите npm run theme:css\n",
    );
    process.exit(1);
  }
} else {
  writeFileSync(target, css);
}
