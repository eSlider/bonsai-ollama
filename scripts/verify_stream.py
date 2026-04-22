#!/usr/bin/env python3
"""Read Ollama NDJSON from stdin; check streaming is active and chunks arrive without long stalls."""
import json
import sys
import time

prev = time.perf_counter()
max_gap_ms = 0.0
non_done = 0
max_cp = 0
chunks = []

for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    now = time.perf_counter()
    gap_ms = (now - prev) * 1000.0
    max_gap_ms = max(max_gap_ms, gap_ms)
    prev = now
    try:
        o = json.loads(raw)
    except json.JSONDecodeError as e:
        print("BAD_JSON", e, raw[:120], file=sys.stderr)
        sys.exit(2)
    if o.get("done"):
        continue
    c = (o.get("message") or {}).get("content") or o.get("response") or ""
    ncp = len(c)
    max_cp = max(max_cp, ncp)
    non_done += 1
    chunks.append((ncp, c[:40]))

print("non_done_chunks", non_done)
print("max_codepoints_per_chunk", max_cp)
print("max_inter_chunk_gap_ms", round(max_gap_ms, 3))
if chunks[:5]:
    print("first_chunks_preview", chunks[:5])

if non_done == 0:
    print("FAIL: no streaming chunks received", file=sys.stderr)
    sys.exit(1)
if max_gap_ms > 8000:
    print("FAIL: long stall between chunks (ms)", max_gap_ms, file=sys.stderr)
    sys.exit(1)
print("OK")
