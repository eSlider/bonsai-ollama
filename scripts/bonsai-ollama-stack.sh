#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export BONSAI_REPO_ROOT="$ROOT"

OLLAMA_BIN="${OLLAMA_BIN:-/usr/local/bin/ollama}"
PROXY_BIN="${PROXY_BIN:-$ROOT/bin/bonsai-ollama-proxy}"
BACKEND_PORT="${BACKEND_PORT:-11435}"

if [[ ! -x "$PROXY_BIN" ]]; then
  echo "Build the proxy first: (cd $ROOT/cmd/bonsai-ollama-proxy && go build -o $PROXY_BIN .)" >&2
  exit 1
fi

for p in 11434 "$BACKEND_PORT" 9988; do
  if command -v fuser >/dev/null 2>&1; then
    fuser -k "${p}/tcp" 2>/dev/null || true
  fi
done
sleep 1

export OLLAMA_HOST="127.0.0.1:${BACKEND_PORT}"
nohup "$OLLAMA_BIN" serve >>/tmp/ollama-bonsai-backend.log 2>&1 &
echo "Started ollama backend on ${BACKEND_PORT} (pid $!)"
sleep 2

export OLLAMA_HOST="127.0.0.1:11434"
export BONSAI_OLLAMA_BACKEND="http://127.0.0.1:${BACKEND_PORT}"
exec "$PROXY_BIN"
