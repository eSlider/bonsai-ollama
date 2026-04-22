# eslider/bonsai-1.7b

**PrismML [Bonsai 1.7B](https://huggingface.co/prism-ml/Bonsai-1.7B-gguf)** in GGUF **`Q1_0`** (1-bit, Apache-2.0 weights on Hugging Face).

## Important: stock `ollama run` will fail (for now)

This GGUF uses GGML tensor type **`Q1_0`**, which the Ollama runner’s bundled `ggml` does not load yet (`model failed to load` / HTTP **500**). That is a **missing quantizer in Ollama**, not a RAM issue.

**To actually run this model** with the normal Ollama CLI and `/api/chat` / `/api/generate`, use the companion project **[bonsai-ollama](https://github.com/eSlider/bonsai-ollama)** (Go reverse proxy + Prism **`llama-server`**). Same model name; traffic is routed to Prism for Bonsai and to your usual `ollama serve` for everything else.

| Resource | URL |
|----------|-----|
| **GitHub (setup, scripts, CI)** | https://github.com/eSlider/bonsai-ollama |
| **Latest release** | https://github.com/eSlider/bonsai-ollama/releases/latest |
| **Weights (HF)** | https://huggingface.co/prism-ml/Bonsai-1.7B-gguf |
| **Prism `llama-server` builds** | https://github.com/PrismML-Eng/llama.cpp/releases |
| **Bonsai demo / integration ideas** | https://github.com/PrismML-Eng/Bonsai-demo |

## How to run (recommended path)

1. **Install [Ollama](https://ollama.com/download)** (backend still used for the library, pulls, and non-Bonsai models).

2. **Clone and set up the proxy stack** (downloads GGUF + pinned Prism Ubuntu CPU tarball and builds the proxy):

   ```bash
   git clone https://github.com/eSlider/bonsai-ollama.git
   cd bonsai-ollama
   ./bin/setup.sh
   ```

3. **Start the stack** (frees ports `11434` / `11435` / `9988`, starts backend Ollama + proxy):

   ```bash
   ./bin/run.sh
   ```

4. **Point clients at the proxy** and pull this model if needed:

   ```bash
   export OLLAMA_HOST=http://127.0.0.1:11434
   ollama pull eslider/bonsai-1.7b
   ollama run eslider/bonsai-1.7b "Say hello in one sentence."
   ```

5. **Optional:** CPU throughput numbers and a small benchmark script live in the GitHub README under *Performance (CPU benchmarks)* (`scripts/bench_llama_tokens.py`).

## What you get

- **248 MB** GGUF, **32K** context (see `Modelfile` `PARAMETER num_ctx`).
- **Streaming** supported through the proxy (OpenAI SSE from Prism → Ollama NDJSON).

## License

- **Weights / upstream:** Apache-2.0 on Hugging Face — follow PrismML terms when redistributing GGUF files.
- **bonsai-ollama proxy & scripts:** MIT in the GitHub repository.
