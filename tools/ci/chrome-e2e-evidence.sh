#!/usr/bin/env bash
# Chrome smoke: тот же канал, что Playwright в tools/e2e (channel: chrome, browser-actions/setup-chrome).
set -euo pipefail

if ! command -v google-chrome-stable >/dev/null 2>&1; then
  echo "google-chrome-stable не найден (ожидается после browser-actions/setup-chrome@v1, briefs/r1-e1-browser.md)"
  exit 1
fi

google-chrome-stable --version