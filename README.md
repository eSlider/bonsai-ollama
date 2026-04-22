# bonsai-ollama

[![CI](https://github.com/eSlider/bonsai-ollama/actions/workflows/ci.yml/badge.svg)](https://github.com/eSlider/bonsai-ollama/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/eSlider/bonsai-ollama)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)

**Repository:** [github.com/eSlider/bonsai-ollama](https://github.com/eSlider/bonsai-ollama) · **Latest release:** [v0.1.2](https://github.com/eSlider/bonsai-ollama/releases/tag/v0.1.2)

**Run [PrismML Bonsai 1.7B](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf) (GGUF `Q1_0`) with the [Ollama](https://ollama.com) CLI and HTTP API** even though the stock Ollama engine cannot load this quantization yet. This repository ships a small **Go reverse proxy** that forwards Bonsai traffic to [PrismML’s `llama-server`](https://github.com/PrismML-Eng/llama.cpp/releases) and everything else to a normal `ollama serve`.

**Upstream weights & paper trail:** [Hugging Face — `prism-ml/Bonsai-1.7B-gguf`](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf) (Apache-2.0) · [Bonsai-demo](https://github.com/PrismML-Eng/Bonsai-demo) · [Ollama import docs](https://docs.ollama.com/import)

**Registry (same GGUF; still needs this proxy until Ollama supports `Q1_0`):** [ollama.com/eslider/bonsai-1.7b](https://ollama.com/eslider/bonsai-1.7b) — hub-facing copy lives in [`models/bonsai-1.7b/README.md`](models/bonsai-1.7b/README.md); maintainers can push summary + readme with [`bin/publish_ollama_hub_readme`](bin/publish_ollama_hub_readme) (needs `OLLAMA_COM_COOKIE` from a signed-in browser session; build via `./bin/setup.sh` or `./bin/run.sh`).

---

## Table of contents

- [Why this exists](#why-this-exists)
- [How it works](#how-it-works)
- [Quick start](#quick-start)
- [Full setup](#full-setup)
- [Daily use](#daily-use)
- [HTTP API examples](#http-api-examples)
- [Streaming](#streaming)
- [Performance (CPU benchmarks)](#performance-cpu-benchmarks)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)
- [Repository layout](#repository-layout)
- [Contributing](#contributing)
- [License](#license)

---

## Why this exists

Bonsai ships in **GGML `Q1_0`** (1-bit, g128 scales). Ollama’s bundled [`ggml`](https://github.com/ollama/ollama) build currently ends its type enum **before** `Q1_0`, so the runner fails while loading tensors (`model failed to load` / HTTP **500**). That is a **missing tensor type in Ollama**, not an out-of-memory error.

PrismML’s **`llama-server`** builds include the kernels this GGUF needs. This project sits **in front of** Ollama: it runs `llama-server` locally and **translates** OpenAI-style [SSE streaming](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events) from Prism into [Ollama’s streaming JSON API](https://docs.ollama.com/api) for Bonsai, while **reverse-proxying** all other routes to your existing Ollama daemon.

---

## How it works

```mermaid
flowchart LR
  subgraph clients [Clients]
    CLI[ollama CLI]
    API[HTTP clients]
  end
  subgraph proxy [bonsai-ollama-proxy :11434]
    R[Router]
  end
  subgraph backends [Backends]
    LS[llama-server :9988]
    OS[ollama serve :11435]
  end
  CLI --> R
  API --> R
  R -->|"bonsai-1.7b chat/generate"| LS
  R -->|"everything else"| OS
```

| Port | Process | Role |
|------|---------|------|
| **11434** | `bonsai-ollama-proxy` | What you point `OLLAMA_HOST` at |
| **11435** | `ollama serve` | Normal models, `ollama pull`, etc. |
| **9988** | `llama-server` | Loads `Bonsai-1.7B-Q1_0.gguf`, OpenAI-compatible `/v1/chat/completions` |

---

## Quick start

```bash
git clone https://github.com/eSlider/bonsai-ollama.git
cd bonsai-ollama

./bin/setup.sh   # GGUF + Prism llama-server + go build (see [Full setup](#full-setup))

./bin/run.sh     # frees 11434/11435/9988, starts backend Ollama + proxy

export OLLAMA_HOST=http://127.0.0.1:11434
ollama run eslider/bonsai-1.7b "Say hello in one sentence."
```

---

## Full setup

### One-command setup (recommended)

```bash
./bin/setup.sh
```

[`bin/setup.sh`](bin/setup.sh) performs the download / extract / build steps documented below: Hugging Face **GGUF**, pinned **Prism Ubuntu x64** `llama-server` tarball under `vendor/prism-llama/`, and `go build` → `bin/bonsai-ollama-proxy`. It does **not** install [Ollama](https://ollama.com/download) (install that separately), and it does **not** start daemons — use [`./bin/run.sh`](#4-run-the-stack) after setup.

- **`./bin/setup.sh --force`** — re-download the GGUF, re-fetch the tarball, remove the old Prism extract, and rebuild.
- **Overrides** — `BONSAI_SETUP_GGUF_URL`, `BONSAI_SETUP_PRISM_TAR_URL`, `BONSAI_SETUP_GGUF_PATH` (see `./bin/setup.sh --help`).

### Prerequisites

| Requirement | Notes |
|---------------|--------|
| [Go](https://go.dev/dl/) **1.22+** | Build `bonsai-ollama-proxy` (`setup.sh` and `run.sh`) |
| `curl`, `tar` | Used by `bin/setup.sh` |
| [Ollama](https://ollama.com/download) | Backend on port `11435` (not installed by `setup.sh`) |
| `fuser` (optional) | From [`psmisc`](https://gitlab.com/psmisc/psmisc) on Debian/Ubuntu — `bin/bonsai-ollama-stack.sh` uses it to free ports |

### Manual setup (same as `./bin/setup.sh`)

### 1. Download the GGUF (~237 MiB)

Official file: [`Bonsai-1.7B-Q1_0.gguf`](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf/blob/main/Bonsai-1.7B-Q1_0.gguf)

```bash
mkdir -p models/bonsai-1.7b
curl -fL -o models/bonsai-1.7b/Bonsai-1.7B-Q1_0.gguf \
  "https://huggingface.co/prism-ml/Bonsai-1.7B-gguf/resolve/main/Bonsai-1.7B-Q1_0.gguf"
```

### 2. Download Prism `llama-server` (Ubuntu x64 CPU)

Release asset (pinned version in this repo): [`llama-prism-b8846-d104cf1-bin-ubuntu-x64.tar.gz`](https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b8846-d104cf1/llama-prism-b8846-d104cf1-bin-ubuntu-x64.tar.gz)

```bash
mkdir -p vendor/prism-llama && cd vendor/prism-llama
curl -fL -o prism.tar.gz \
  "https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b8846-d104cf1/llama-prism-b8846-d104cf1-bin-ubuntu-x64.tar.gz"
tar -xzf prism.tar.gz
cd ../..
```

Other platforms (**CUDA**, **Vulkan**, **macOS**, etc.) are on the [PrismML-Eng/llama.cpp releases](https://github.com/PrismML-Eng/llama.cpp/releases) page. Extract into `vendor/prism-llama/` and set `BONSAI_PRISM_LIB_DIR` to the folder that contains `llama-server` and its `.so` / `.dylib` files.

### 3. Build the proxy

Skip if you already ran `./bin/setup.sh` (it builds `bin/bonsai-ollama-proxy`).

```bash
cd /path/to/bonsai-ollama   # repository root (contains go.mod)
go build -o bin/bonsai-ollama-proxy ./cmd/bonsai-ollama-proxy
go build -o bin/bench_llama_tokens ./cmd/bench-llama-tokens
go build -o bin/verify_stream ./cmd/verify-stream
go build -o bin/publish_ollama_hub_readme ./cmd/publish-ollama-hub-readme
```

### 4. Run the stack

Stop anything already bound to **11434**, **11435**, and **9988** (or let the script try `fuser -k`).

```bash
./bin/run.sh
```

- Builds `bin/bonsai-ollama-proxy` if missing, then runs `bin/bonsai-ollama-stack.sh`.
- Backend logs: `/tmp/ollama-bonsai-backend.log`.

---

## Daily use

Point every Ollama client at the **proxy** (not the backend port):

```bash
export OLLAMA_HOST=http://127.0.0.1:11434
ollama list
ollama run eslider/bonsai-1.7b "Your prompt"
ollama run qwen3:4b "Other models still go to backend :11435"
```

`OLLAMA_HOST` is documented in the [Ollama FAQ / Linux](https://github.com/ollama/ollama/blob/main/docs/faq.md).

---

## HTTP API examples

**Chat (non-stream):**

```bash
curl -sS http://127.0.0.1:11434/api/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"eslider/bonsai-1.7b","messages":[{"role":"user","content":"Hi"}],"stream":false}'
```

**Generate (non-stream):**

```bash
curl -sS http://127.0.0.1:11434/api/generate \
  -H "Content-Type: application/json" \
  -d '{"model":"eslider/bonsai-1.7b","prompt":"Hello","stream":false}'
```

---

## Streaming

For each `data:` line from `llama-server`’s OpenAI SSE stream, the proxy emits **one Ollama NDJSON object** with that text delta and **`Flush`es immediately**, so backend token granularity is preserved.

```bash
curl -sS -N -X POST http://127.0.0.1:11434/api/chat \
  -H "Content-Type: application/json" \
  -d '{"model":"eslider/bonsai-1.7b","messages":[{"role":"user","content":"Count 1 2 3"}],"stream":true}' \
| ./bin/verify_stream
```

[`bin/verify_stream`](bin/verify_stream) checks that chunks arrive and that there are no multi-second stalls (built from [`cmd/verify-stream`](cmd/verify-stream)).

---

## Performance (CPU benchmarks)

Bonsai inference in this stack is executed by **Prism `llama-server`** (OpenAI-compatible `/v1/chat/completions`). The proxy’s job is routing and stream translation; end-to-end generation speed is dominated by **`llama-server` on your CPU** (or GPU build, if you use a Prism release with CUDA/Vulkan and set `BONSAI_PRISM_LIB_DIR` accordingly).

The table below uses the server’s built-in **`timings`** object (tokens per second for the prompt and prediction phases), which matches how [`llama.cpp` server](https://github.com/ggerganov/llama.cpp) reports throughput.

| Metric | Result |
|--------|--------|
| **Decode** (`max_tokens=256`, 5 runs, temperature 0.75) | **~72 tok/s** mean (σ ≈ 1.5; ~70–74 tok/s on this host) |
| **Prefill** (~480-token article prompt, `max_tokens=8`, **3× warmup** on the same prompt then 5 measured runs) | **~54 tok/s** mean (σ ≈ 4.7) |

**Measured environment (2026-04-22, re-run after `bin/setup.sh` / proxy rebuild):**

| | |
|--|--|
| **CPU** | AMD Ryzen 7 5800H (8 cores / 16 threads), x86_64 |
| **OS** | Linux `7.0.0-070000rc7-generic` |
| **GGUF** | `Bonsai-1.7B-Q1_0.gguf` ([HF](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf)) |
| **Binary** | Prism `llama-server` tarball **`llama-prism-b8846-d104cf1-bin-ubuntu-x64`** ([release](https://github.com/PrismML-Eng/llama.cpp/releases/download/prism-b8846-d104cf1/llama-prism-b8846-d104cf1-bin-ubuntu-x64.tar.gz)); API `system_fingerprint`: **`b8846-d104cf1b6`** |
| **Endpoint** | `http://127.0.0.1:9988` (same process the proxy supervises) |

Your numbers will differ with other CPUs, power/thermal limits, concurrent load, and thread / batch settings on `llama-server`. Throughput also drifts between runs on the same machine.

**Reproduce:**

```bash
./bin/setup.sh   # once per machine (GGUF + Prism + go build)
./bin/run.sh     # wait until llama-server answers on 9988
./bin/bench_llama_tokens --runs 5 --json
```

[`bin/bench_llama_tokens`](bin/bench_llama_tokens) prints a small JSON summary (mean / stdev / min / max); sources in [`cmd/bench-llama-tokens`](cmd/bench-llama-tokens). Point at another host or port with `BONSAI_LLAMA_URL=http://127.0.0.1:9988`.

---

## Configuration

Environment variables (optional). Full notes: [`models/bonsai-1.7b/OLLAMA.txt`](models/bonsai-1.7b/OLLAMA.txt).

| Variable | Default | Meaning |
|----------|---------|---------|
| `BONSAI_REPO_ROOT` | parent of proxy binary `../..` | Root for default GGUF / Prism paths |
| `BONSAI_GGUF` | `models/bonsai-1.7b/Bonsai-1.7B-Q1_0.gguf` under root | Path to GGUF |
| `BONSAI_PRISM_LIB_DIR` | `vendor/prism-llama/llama-prism-b8846-d104cf1` under root | Directory with `llama-server` + libs |
| `BONSAI_PROXY_LISTEN` | `127.0.0.1:11434` | Proxy listen address |
| `BONSAI_OLLAMA_BACKEND` | `http://127.0.0.1:11435` | Upstream Ollama |
| `BONSAI_LLAMA_PORT` | `9988` | `llama-server` port |
| `OLLAMA_BIN` | `/usr/local/bin/ollama` | Used by `bonsai-ollama-stack.sh` only |

---

## Troubleshooting

| Symptom | What to check |
|---------|----------------|
| **Address already in use** | Free `11434` / `11435` / `9988` or change ports via env vars. |
| **`llama-server` not found** | `BONSAI_PRISM_LIB_DIR` must contain the extracted Prism binaries. |
| **GGUF not found** | `BONSAI_GGUF` path; run `./bin/setup.sh` or the [manual GGUF download](#1-download-the-gguf-237-mib). |
| **`ollama run` hangs in CI** | Use a real TTY or call `/api/chat` with `curl` / your HTTP client. |
| **Stock Ollama still 500 on Bonsai** | You must talk to the **proxy** (`OLLAMA_HOST=…:11434`), not raw `:11435`. |

---

## Repository layout

| Path | Purpose |
|------|---------|
| [`cmd/bonsai-ollama-proxy/`](cmd/bonsai-ollama-proxy/) | Go source: proxy + `llama-server` supervisor |
| [`cmd/bench-llama-tokens/`](cmd/bench-llama-tokens/) | `llama-server` token benchmark (`bin/bench_llama_tokens`) |
| [`cmd/verify-stream/`](cmd/verify-stream/) | Streaming sanity reader (`bin/verify_stream`) |
| [`cmd/publish-ollama-hub-readme/`](cmd/publish-ollama-hub-readme/) | Ollama Hub readme/summary publisher |
| [`bin/bonsai-ollama-stack.sh`](bin/bonsai-ollama-stack.sh) | Starts backend Ollama + proxy |
| [`bin/verify_stream`](bin/verify_stream) | Quick streaming sanity check (built Go binary) |
| [`bin/bench_llama_tokens`](bin/bench_llama_tokens) | CPU token throughput (uses `llama-server` `timings`) |
| [`bin/setup.sh`](bin/setup.sh) | Full local setup: GGUF + Prism tarball + `go build` |
| [`bin/run.sh`](bin/run.sh) | Build-if-needed + exec stack |
| [`models/bonsai-1.7b/Modelfile`](models/bonsai-1.7b/Modelfile) | `ollama create` recipe (weights not in git) |
| [`models/bonsai-1.7b/OLLAMA.txt`](models/bonsai-1.7b/OLLAMA.txt) | Extra operational notes |
| [`models/bonsai-1.7b/README.md`](models/bonsai-1.7b/README.md) | Text for [Ollama Hub](https://ollama.com/eslider/bonsai-1.7b) (summary + readme) |
| [`bin/publish_ollama_hub_readme`](bin/publish_ollama_hub_readme) | POST hub summary/readme (`OLLAMA_COM_COOKIE`) |

CLI programs under `cmd/*/main.go` start with a **`///usr/bin/true; exec /usr/bin/env go run "$0" "$@"`** line comment (it is valid Go: `//` begins the line comment). A real **`#!`** shebang cannot be the first bytes of a `.go` file because **`go build` / `go vet` reject `#`**. Day-to-day use: **`./bin/<tool>`** (compiled by `./bin/setup.sh` / `./bin/run.sh`) or **`go run ./cmd/…`**. The `main.go` sources are stored as **executable (`100755`)** in git for environments that run them via a shell wrapper around that idiom.

---

## Importing without the proxy (expect failure on `run`)

From `models/bonsai-1.7b/`:

```bash
ollama create bonsai-1.7b -f Modelfile
```

This registers the blob, but **`ollama run` keeps failing** until Ollama ships `Q1_0` support. Use the proxy stack for inference today.

---

## Optional: run Prism only

```bash
cd vendor/prism-llama/llama-prism-b8846-d104cf1
LD_LIBRARY_PATH="$PWD" ./llama-server \
  -m ../../../models/bonsai-1.7b/Bonsai-1.7B-Q1_0.gguf \
  --host 127.0.0.1 --port 9988
```

Then use OpenAI-compatible [`POST /v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat/create). See [Bonsai-demo](https://github.com/PrismML-Eng/Bonsai-demo) for more integration examples.

---

## Contributing

Issues and PRs are welcome. When changing Go code, from the **repository root** (where `go.mod` lives), run:

```bash
go vet ./... && go test ./...
```

(`go test` is a no-op until tests exist; `go vet` should be clean.)

CI runs the same vet + build on every push to `main` (see [`.github/workflows/ci.yml`](.github/workflows/ci.yml)). If `git push` asks for credentials in a headless environment, run `gh auth setup-git` once (requires the [GitHub CLI](https://cli.github.com/) logged in).

---

## License

- **This repository** (Go proxy, scripts, docs): [MIT](LICENSE).
- **Bonsai weights & Prism upstream**: [Apache-2.0](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf) on Hugging Face; follow their attribution and license terms when redistributing GGUF files.
