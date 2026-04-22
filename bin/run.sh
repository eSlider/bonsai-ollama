#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export BONSAI_REPO_ROOT="$ROOT"

PROXY="$ROOT/bin/bonsai-ollama-proxy"
if [[ ! -x "$PROXY" ]]; then
  echo "Building bonsai-ollama-proxy -> $PROXY" >&2
  (cd "$ROOT" && go build -o "$PROXY" ./cmd/bonsai-ollama-proxy)
fi

build_tool() {
  local out="$1"
  local pkg="$2"
  if [[ ! -x "$out" ]]; then
    echo "Building $(basename "$out") -> $out" >&2
    (cd "$ROOT" && go build -o "$out" "./cmd/$pkg")
  fi
}
build_tool "$ROOT/bin/bench_llama_tokens" bench-llama-tokens
build_tool "$ROOT/bin/verify_stream" verify-stream
build_tool "$ROOT/bin/publish_ollama_hub_readme" publish-ollama-hub-readme

exec "$ROOT/bin/bonsai-ollama-stack.sh"
