#!/usr/bin/env bash
# Чтение mise.toml и сверка с опубликованными SHA256. Файл подключают, не запускают.

toolchain_value() {
  local key="$1"
  local file="$2"
  local value
  value="$(awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" {
      line = $0
      sub(/^[^=]*=[[:space:]]*/, "", line)
      gsub(/\r/, "", line)
      gsub(/^"/, "", line)
      gsub(/".*$/, "", line)
      print line
      exit
    }
  ' "$file")"
  if [[ -z "$value" ]]; then
    echo "в ${file} нет ключа ${key}" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

sha256_file() {
  local file="$1"
  local sum="$2"
  echo "${sum}  ${file}" | sha256sum -c - >&2
}

published_go_sha256() {
  local version="$1"
  local json name sum
  json="$(mktemp)"
  curl -fsSL -o "$json" 'https://go.dev/dl/?mode=json&include=all'
  name="go${version}.linux-amd64.tar.gz"
  sum="$(python3 - "$json" "$name" <<'PY'
import json
import sys
data = json.load(open(sys.argv[1], encoding="utf-8"))
name = sys.argv[2]
for rel in data:
    for item in rel.get("files", []):
        if item.get("filename") == name and item.get("sha256"):
            print(item["sha256"])
            raise SystemExit(0)
raise SystemExit(1)
PY
)"
  rm -f "$json"
  if [[ ! "$sum" =~ ^[0-9a-f]{64}$ ]]; then
    echo "нет опубликованной суммы Go ${version}" >&2
    return 1
  fi
  printf '%s\n' "$sum"
}

published_name_sha256() {
  local sums="$1"
  local name="$2"
  local sum
  sum="$(awk -v name="$name" '$2 == name || $2 == "./" name { print $1; exit }' "$sums")"
  if [[ ! "$sum" =~ ^[0-9a-f]{64}$ ]]; then
    echo "нет опубликованной суммы ${name}" >&2
    return 1
  fi
  printf '%s\n' "$sum"
}
