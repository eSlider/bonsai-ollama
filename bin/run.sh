#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export BONSAI_REPO_ROOT="$ROOT"

PROXY="$ROOT/bin/bonsai-ollama-proxy"
if [[ ! -x "$PROXY" ]]; then
  echo "Building bonsai-ollama-proxy -> $PROXY" >&2
  (cd "$ROOT/cmd/bonsai-ollama-proxy" && go build -o "$PROXY" .)
fi

exec "$ROOT/scripts/bonsai-ollama-stack.sh"
