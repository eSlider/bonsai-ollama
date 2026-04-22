#!/usr/bin/env python3
"""Measure Bonsai token timings via Prism llama-server OpenAI API (non-stream).

Reads JSON from llama-server /v1/chat/completions and aggregates timings.predicted_per_second
(decode) and timings.prompt_per_second (prefill). Target must be running (e.g. ./bin/run.sh).

Usage:
  python3 bin/bench_llama_tokens.py
  BONSAI_LLAMA_URL=http://127.0.0.1:9988 python3 bin/bench_llama_tokens.py --json
"""
from __future__ import annotations

import argparse
import json
import os
import statistics
import sys
import time
import urllib.error
import urllib.request


def post_chat(base: str, prompt: str, max_tokens: int, temperature: float) -> dict:
    body = json.dumps(
        {
            "model": "bonsai",
            "messages": [{"role": "user", "content": prompt}],
            "max_tokens": max_tokens,
            "temperature": temperature,
            "stream": False,
        }
    ).encode()
    url = base.rstrip("/") + "/v1/chat/completions"
    req = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=600) as r:
        return json.loads(r.read().decode())


def run_once(base: str, prompt: str, max_tokens: int, temperature: float) -> dict:
    t0 = time.perf_counter()
    d = post_chat(base, prompt, max_tokens, temperature)
    wall = time.perf_counter() - t0
    t = d.get("timings") or {}
    u = d.get("usage") or {}
    return {
        "wall_s": wall,
        "predicted_n": t.get("predicted_n"),
        "predicted_per_second": t.get("predicted_per_second"),
        "prompt_n": t.get("prompt_n"),
        "prompt_per_second": t.get("prompt_per_second"),
        "prompt_ms": t.get("prompt_ms"),
        "predicted_ms": t.get("predicted_ms"),
        "completion_tokens": u.get("completion_tokens"),
        "prompt_tokens": u.get("prompt_tokens"),
    }


def summarize(xs: list[float]) -> dict:
    xs = [x for x in xs if x is not None and x > 0]
    if not xs:
        return {}
    return {
        "n": len(xs),
        "mean": statistics.mean(xs),
        "stdev": statistics.stdev(xs) if len(xs) > 1 else 0.0,
        "min": min(xs),
        "max": max(xs),
    }


def main() -> int:
    p = argparse.ArgumentParser(description="Benchmark llama-server token timings.")
    p.add_argument(
        "--base",
        default=os.environ.get("BONSAI_LLAMA_URL", "http://127.0.0.1:9988"),
        help="llama-server base URL",
    )
    p.add_argument("--runs", type=int, default=5, help="iterations per scenario")
    p.add_argument("--warmup", type=int, default=1, help="warmup requests (discarded)")
    p.add_argument(
        "--prefill-warmup",
        type=int,
        default=3,
        help="extra repeats of the long-prompt request before measuring prefill (stabilizes KV cache)",
    )
    p.add_argument("--json", action="store_true", help="print machine-readable summary only")
    args = p.parse_args()

    decode_prompt = (
        "You are benchmarking. Output continuous prose only, no lists or headings. "
        "Explain how rivers shape landscapes, erosion, deltas, and floodplains in detail."
    )
    prefill_prompt = (
        "Summarize the following article in one sentence under 30 words:\n\n"
        + ("Lorem ipsum dolor sit amet. " * 120)
    )

    try:
        post_chat(args.base, "ping", 2, 0.0)
    except urllib.error.URLError as e:
        print(f"Cannot reach {args.base}: {e}", file=sys.stderr)
        print("Start the stack first: ./bin/run.sh", file=sys.stderr)
        return 1

    for _ in range(args.warmup):
        post_chat(args.base, "Say OK.", 4, 0.1)

    decode_rates: list[float] = []
    prompt_rates: list[float] = []
    predicted_ns: list[int] = []
    walls: list[float] = []

    for _ in range(args.runs):
        r = run_once(args.base, decode_prompt, 256, 0.75)
        ps = r.get("predicted_per_second")
        if isinstance(ps, (int, float)):
            decode_rates.append(float(ps))
        pn = r.get("predicted_n")
        if isinstance(pn, int):
            predicted_ns.append(pn)
        walls.append(r["wall_s"])

    for _ in range(max(0, args.prefill_warmup)):
        _ = run_once(args.base, prefill_prompt, 4, 0.3)
    for _ in range(args.runs):
        r2 = run_once(args.base, prefill_prompt, 8, 0.3)
        pr = r2.get("prompt_per_second")
        if isinstance(pr, (int, float)):
            prompt_rates.append(float(pr))

    out = {
        "llama_url": args.base,
        "runs": args.runs,
        "decode": {
            "scenario": "max_tokens=256, long user prompt",
            "predicted_tokens_mean": statistics.mean(predicted_ns) if predicted_ns else None,
            "decode_tokens_per_s": summarize(decode_rates),
            "wall_s_per_run_mean": statistics.mean(walls) if walls else None,
        },
        "prefill": {
            "scenario": "~480-token article + short completion (max_tokens=8)",
            "warmup_repeats": args.prefill_warmup,
            "prompt_tokens_per_s": summarize(prompt_rates),
        },
    }

    if args.json:
        print(json.dumps(out, indent=2))
        return 0

    dsum = out["decode"]["decode_tokens_per_s"]
    psum = out["prefill"]["prompt_tokens_per_s"]
    print("llama-server:", args.base)
    print("Decode (server-reported predicted_per_second):", dsum)
    print("Prefill (server-reported prompt_per_second):", psum)
    if predicted_ns:
        print("Mean completion tokens (actual):", round(statistics.mean(predicted_ns), 1))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
