#!/bin/sh
set -eu
cd "$(dirname "$0")"

if [ -f bin/phlo ]; then
  chmod +x bin/phlo 2>/dev/null || true
fi
if [ ! -x bin/phlo ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "Build Phlo first:"
    echo "  go build -o bin/phlo ./cmd/phlo"
    exit 1
  fi
  echo "Building Phlo. The first build can take a minute."
  mkdir -p bin
  go build -o bin/phlo ./cmd/phlo || exit 1
  chmod +x bin/phlo
fi

if [ ! -f config/phlo.json ] && [ -f config/phlo.json.example ]; then
  cp config/phlo.json.example config/phlo.json
fi

PORT=7700
if command -v python3 >/dev/null 2>&1; then
  PORT=$(python3 -c 'import json; print(json.load(open("config/phlo.json"))["dashboardPort"])')
fi

url="http://127.0.0.1:${PORT}/"

open_browser() {
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 || true
  elif command -v open >/dev/null 2>&1; then
    open "$url"
  else
    echo "$url"
  fi
}

if curl -fsS -o /dev/null --max-time 1 "${url}api/health"; then
  open_browser
  exit 0
fi

(
  tries=0
  while [ "$tries" -lt 30 ]; do
    if curl -fsS -o /dev/null --max-time 1 "${url}api/health"; then
      open_browser
      exit 0
    fi
    tries=$((tries + 1))
    sleep 1
  done
  echo "Phlo did not start." >&2
) &

exec bin/phlo start
