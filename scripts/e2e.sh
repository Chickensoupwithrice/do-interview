#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR=$(dirname "$(dirname "$(realpath "$0")")")
PORT=${PORT:-18080}
BASE_URL=${BASE_URL:-"http://127.0.0.1:${PORT}"}
DB_PATH=$(mktemp "${TMPDIR:-/tmp}/url-shortener-e2e-XXXXXX.db")
LOG_PATH=$(mktemp "${TMPDIR:-/tmp}/url-shortener-e2e-log-XXXXXX")

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -f "${DB_PATH}" "${LOG_PATH}"
}

trap cleanup EXIT

ADDR=":${PORT}" BASE_URL="${BASE_URL}" DATABASE_PATH="${DB_PATH}" \
  go run ./cmd/api >"${LOG_PATH}" 2>&1 &
SERVER_PID=$!

for _ in $(seq 1 50); do
  if curl --silent --fail "${BASE_URL}/healthz" >/dev/null; then
    break
  fi
  sleep 0.2
done

if ! curl --silent --fail "${BASE_URL}/healthz" >/dev/null; then
  printf 'server failed to start\n' >&2
  cat "${LOG_PATH}" >&2
  exit 1
fi

create_response=$(curl --silent --show-error --fail \
  -X POST "${BASE_URL}/api/urls" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com/docs","alias":"docs-e2e","ttl_seconds":60}')

short_url=$(printf '%s' "${create_response}" | jq -r '.short_url')
if [[ "${short_url}" != "${BASE_URL}/docs-e2e" ]]; then
  printf 'unexpected short_url: %s\n' "${short_url}" >&2
  exit 1
fi

redirect_location=$(curl --silent --output /dev/null --write-out '%{redirect_url}' "${BASE_URL}/docs-e2e")
if [[ "${redirect_location}" != "https://example.com/docs" ]]; then
  printf 'unexpected redirect location: %s\n' "${redirect_location}" >&2
  exit 1
fi

metadata_response=$(curl --silent --show-error --fail "${BASE_URL}/api/urls/docs-e2e")
access_count=$(printf '%s' "${metadata_response}" | jq -r '.access_count')
if [[ "${access_count}" != "1" ]]; then
  printf 'unexpected access_count: %s\n' "${access_count}" >&2
  exit 1
fi

printf 'e2e passed\n'
