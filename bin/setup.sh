#!/usr/bin/env bash
# One-shot setup from README: GGUF, Prism llama-server (Ubuntu x64), Go proxy binary.
# Does not start services — use ./bin/run.sh after Ollama is installed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Pinned assets (keep in sync with README.md).
GGUF_URL="${BONSAI_SETUP_GGUF_URL:-https://huggingface.co/prism-ml/Bonsai-1.7B-gguf/resolve/main/Bonsai-1.7B-Q1_0.gguf}"
PRISM_TAR_URL="${BONSAI_SETUP_PRISM_TAR_URL:-https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b8846-d104cf1/llama-prism-b8846-d104cf1-bin-ubuntu-x64.tar.gz}"

GGUF_PATH="${BONSAI_SETUP_GGUF_PATH:-$ROOT/models/bonsai-1.7b/Bonsai-1.7B-Q1_0.gguf}"
PRISM_VENDOR="$ROOT/vendor/prism-llama"
PRISM_TAR="$PRISM_VENDOR/prism.tar.gz"
PRISM_DIR="$PRISM_VENDOR/llama-prism-b8846-d104cf1"
PROXY_OUT="$ROOT/bin/bonsai-ollama-proxy"

FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)
      FORCE=1
      shift
      ;;
    -h | --help)
      cat <<'EOF'
Usage: ./bin/setup.sh [--force]

  Downloads Bonsai GGUF and Prism llama-server (Ubuntu x64 CPU tarball),
  extracts Prism under vendor/prism-llama/, and builds bin/bonsai-ollama-proxy.

  --force     Re-download GGUF / Prism archive and re-extract (removes existing
              GGUF file and Prism extract directory first).

Environment (optional overrides):
  BONSAI_SETUP_GGUF_URL       URL for the GGUF (default: Hugging Face resolve URL)
  BONSAI_SETUP_PRISM_TAR_URL  URL for Prism Ubuntu x64 tarball
  BONSAI_SETUP_GGUF_PATH      Destination path for the GGUF file

Next: ./bin/run.sh
Requires: curl, tar, Go 1.22+
EOF
      exit 0
      ;;
    *)
      echo "Unknown option: $1 (try --help)" >&2
      exit 2
      ;;
  esac
done

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing dependency: $1" >&2
    exit 1
  }
}
need curl
need tar

if ! command -v go >/dev/null 2>&1; then
  echo "Go is not installed or not on PATH (need 1.22+). See https://go.dev/dl/" >&2
  exit 1
fi

if [[ "$FORCE" -eq 1 ]]; then
  echo "Force: removing existing GGUF / Prism extract (if present)" >&2
  rm -f "$GGUF_PATH"
  rm -rf "$PRISM_DIR"
  rm -f "$PRISM_TAR"
fi

echo "==> GGUF -> $GGUF_PATH" >&2
mkdir -p "$(dirname "$GGUF_PATH")"
if [[ -f "$GGUF_PATH" ]]; then
  echo "    Already present (skip download). Use --force to re-fetch." >&2
else
  curl -fL --progress-bar -o "$GGUF_PATH" "$GGUF_URL"
fi

echo "==> Prism llama-server -> $PRISM_DIR" >&2
mkdir -p "$PRISM_VENDOR"
if [[ -x "$PRISM_DIR/llama-server" ]]; then
  echo "    llama-server already present (skip download/extract). Use --force to refresh." >&2
else
  if [[ ! -f "$PRISM_TAR" ]]; then
    curl -fL --progress-bar -o "$PRISM_TAR" "$PRISM_TAR_URL"
  fi
  tar -xzf "$PRISM_TAR" -C "$PRISM_VENDOR"
fi

if [[ ! -x "$PRISM_DIR/llama-server" ]]; then
  echo "Expected llama-server at $PRISM_DIR/llama-server — extract layout may have changed." >&2
  echo "Set BONSAI_PRISM_LIB_DIR to the folder that contains llama-server (see README)." >&2
  exit 1
fi

echo "==> Build Go tools -> $ROOT/bin" >&2
mkdir -p "$ROOT/bin"
(
  cd "$ROOT"
  go build -o "$PROXY_OUT" ./cmd/bonsai-ollama-proxy
  go build -o "$ROOT/bin/bench_llama_tokens" ./cmd/bench-llama-tokens
  go build -o "$ROOT/bin/verify_stream" ./cmd/verify-stream
  go build -o "$ROOT/bin/publish_ollama_hub_readme" ./cmd/publish-ollama-hub-readme
)

echo "Setup complete." >&2
echo "  GGUF:     $GGUF_PATH" >&2
echo "  Prism:    $PRISM_DIR" >&2
echo "  Proxy:    $PROXY_OUT" >&2
echo "Next: install Ollama if needed, then ./bin/run.sh" >&2
