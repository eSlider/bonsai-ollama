#!/usr/bin/env python3
"""
Publish the Ollama Hub **summary** (≤255 chars) and **readme** for eslider/bonsai-1.7b.

Ollama.com expects POST https://ollama.com/eslider/bonsai-1.7b with
application/x-www-form-urlencoded bodies (same as the site UI). This requires
your logged-in **session cookie** — the local Ollama API key is not used here.

Usage:
  export OLLAMA_COM_COOKIE='session=...; ...'   # copy from browser DevTools → Application → Cookies → ollama.com
  python3 bin/publish_ollama_hub_readme.py [--dry-run]

  # optional overrides:
  OLLAMA_HUB_MODEL=eslider/bonsai-1.7b \\
  OLLAMA_HUB_README=models/bonsai-1.7b/README.md \\
  OLLAMA_HUB_SUMMARY='One line under 255 chars' \\
  python3 bin/publish_ollama_hub_readme.py
"""
from __future__ import annotations

import argparse
import os
import pathlib
import sys
import urllib.error
import urllib.parse
import urllib.request

DEFAULT_README = pathlib.Path(__file__).resolve().parent.parent / "models" / "bonsai-1.7b" / "README.md"
DEFAULT_SUMMARY = (
    "PrismML Bonsai 1.7B (GGUF Q1_0). Stock Ollama cannot load Q1_0 yet — run via "
    "bonsai-ollama proxy + Prism llama-server: https://github.com/eSlider/bonsai-ollama"
)


def post_form(url: str, fields: dict[str, str], cookie: str, dry_run: bool) -> tuple[int, str]:
    body = urllib.parse.urlencode(fields).encode()
    if dry_run:
        print(f"DRY-RUN POST {url}\n  fields: {list(fields.keys())}\n  body bytes: {len(body)}")
        return 0, ""

    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    req.add_header("Cookie", cookie)
    req.add_header(
        "User-Agent",
        "bonsai-ollama-publish-script (https://github.com/eSlider/bonsai-ollama)",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            text = resp.read().decode("utf-8", errors="replace")
            return resp.status, text
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace") if e.fp else ""
        return e.code, err_body or str(e)


def main() -> int:
    p = argparse.ArgumentParser(description="POST readme + summary to ollama.com model page.")
    p.add_argument("--dry-run", action="store_true", help="print actions only")
    args = p.parse_args()

    cookie = (os.environ.get("OLLAMA_COM_COOKIE") or "").strip()
    if not cookie and not args.dry_run:
        print(
            "Missing OLLAMA_COM_COOKIE.\n"
            "  1. Sign in at https://ollama.com in your browser (account must own eslider/bonsai-1.7b).\n"
            "  2. DevTools → Application → Cookies → https://ollama.com → copy the Cookie header value\n"
            "     (or export document.cookie from the console while on ollama.com).\n"
            "  3. export OLLAMA_COM_COOKIE='...'\n"
            "  4. Re-run this script.",
            file=sys.stderr,
        )
        return 1

    model = os.environ.get("OLLAMA_HUB_MODEL", "eslider/bonsai-1.7b").strip().strip("/")
    readme_path = pathlib.Path(os.environ.get("OLLAMA_HUB_README", str(DEFAULT_README)))
    summary = (os.environ.get("OLLAMA_HUB_SUMMARY") or DEFAULT_SUMMARY).strip()

    if len(summary) > 255:
        print(f"Summary is {len(summary)} chars; max 255 (Ollama hub limit).", file=sys.stderr)
        return 1

    if not readme_path.is_file():
        print(f"Readme file not found: {readme_path}", file=sys.stderr)
        return 1

    readme_text = readme_path.read_text(encoding="utf-8").strip()
    base = f"https://ollama.com/{model}"

    # Summary and readme use the same path with different form keys (matches site HTMX / JS).
    steps = [
        ("summary", {"summary": summary}),
        ("readme", {"readme": readme_text}),
    ]

    for label, fields in steps:
        code, body = post_form(base, fields, cookie, args.dry_run)
        if args.dry_run:
            continue
        if code != 200:
            print(f"POST {label} failed: HTTP {code}\n{body[:2000]}", file=sys.stderr)
            return 1
        print(f"OK: updated {label} ({len(fields[label])} chars)")

    if args.dry_run:
        print("Dry run complete.")
    else:
        print(f"Done. View: {base}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
