#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export BONSAI_REPO_ROOT="$ROOT"

if [[ ! -f "$ROOT/go.mod" ]]; then
  echo "Missing $ROOT/go.mod — run this script from the bonsai-ollama checkout (bin/ is under the repo root)." >&2
  exit 1
fi
if ! command -v go >/dev/null 2>&1; then
  echo "Go is not on PATH (need 1.22+). Install from https://go.dev/dl/ or use ./bin/setup.sh after installing Go." >&2
  exit 1
fi

# Rebuild when the binary is missing, not executable, or older than any .go in the package.
need_go_build() {
  local out="$1"
  local pkg="$2"
  if [[ ! -e "$out" || ! -x "$out" ]]; then
    return 0
  fi
  find "$ROOT/cmd/$pkg" -type f -name '*.go' -newer "$out" 2>/dev/null | head -1 | grep -q .
}

PROXY="$ROOT/bin/bonsai-ollama-proxy"
if need_go_build "$PROXY" bonsai-ollama-proxy; then
  echo "Building bonsai-ollama-proxy -> $PROXY" >&2
  (cd "$ROOT" && go build -o "$PROXY" ./cmd/bonsai-ollama-proxy)
fi

build_tool() {
  local out="$1"
  local pkg="$2"
  if need_go_build "$out" "$pkg"; then
    echo "Building $(basename "$out") -> $out" >&2
    (cd "$ROOT" && go build -o "$out" "./cmd/$pkg")
  fi
}
build_tool "$ROOT/bin/bench_llama_tokens" bench-llama-tokens
build_tool "$ROOT/bin/verify_stream" verify-stream
build_tool "$ROOT/bin/publish_ollama_hub_readme" publish-ollama-hub-readme

exec "$ROOT/bin/bonsai-ollama-stack.sh"
