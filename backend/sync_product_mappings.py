#!/usr/bin/env python3
import argparse
import json
import os
import sys
import urllib.request


def post_sync(base_url, timeout):
    url = base_url.rstrip("/") + "/product-mappings/sync"
    req = urllib.request.Request(
        url,
        method="POST",
        data=b"{}",
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, body
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8")
        return exc.code, body


def parse_args():
    parser = argparse.ArgumentParser(
        description="Call product-mappings sync endpoint."
    )
    parser.add_argument(
        "--base-url",
        default=os.environ.get("API_BASE_URL", "http://localhost:8080"),
        help="API base URL (default: http://localhost:8080)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=30,
        help="Request timeout seconds (default: 30)",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    status, body = post_sync(args.base_url, args.timeout)
    print(f"HTTP {status}")
    if body:
        try:
            parsed = json.loads(body)
        except json.JSONDecodeError:
            print(body)
        else:
            print(json.dumps(parsed, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
