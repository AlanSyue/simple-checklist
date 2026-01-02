#!/usr/bin/env python3
import argparse
import json
import os
import sys
import urllib.parse
import urllib.request


def build_url(base_url, path, query):
    base_url = base_url.rstrip("/")
    path = path.lstrip("/")
    return f"{base_url}/{path}?{urllib.parse.urlencode(query)}"


def fetch_json(url):
    req = urllib.request.Request(url)
    try:
        with urllib.request.urlopen(req) as resp:
            data = resp.read().decode("utf-8")
            return json.loads(data)
    except urllib.error.HTTPError as exc:
        sys.stderr.write(f"HTTP error {exc.code} for {url}\n")
        sys.stderr.write(exc.read().decode("utf-8") + "\n")
        raise


def summarize_meta_types(meta_list, only_non_string):
    summary = []
    for meta in meta_list or []:
        display_value = meta.get("display_value")
        if only_non_string and isinstance(display_value, str):
            continue
        summary.append(
            {
                "id": meta.get("id"),
                "key": meta.get("key"),
                "display_key": meta.get("display_key"),
                "display_value_type": type(display_value).__name__,
                "display_value": display_value,
            }
        )
    return summary


def inspect_orders(
    base_url,
    consumer_key,
    consumer_secret,
    status,
    per_page,
    output_path,
    only_non_string,
):
    query = {
        "consumer_key": consumer_key,
        "consumer_secret": consumer_secret,
        "per_page": per_page,
        "page": 1,
    }
    if status:
        query["status"] = status
    url = build_url(base_url, "/wp-json/wc/v3/orders", query)
    orders = fetch_json(url)
    output = []
    for order in orders:
        line_items = order.get("line_items", [])
        item_summaries = []
        for item in line_items:
            meta_summary = summarize_meta_types(
                item.get("meta_data", []),
                only_non_string,
            )
            if only_non_string and not meta_summary:
                continue
            item_summaries.append(
                {
                    "item_id": item.get("id"),
                    "name": item.get("name"),
                    "meta_data": meta_summary,
                }
            )
        if only_non_string and not item_summaries:
            continue
        output.append(
            {
                "order_id": order.get("id"),
                "status": order.get("status"),
                "line_items": item_summaries,
            }
        )

    if output_path:
        with open(output_path, "w", encoding="utf-8") as handle:
            json.dump(output, handle, ensure_ascii=False, indent=2)
        print(f"Wrote {len(output)} orders to {output_path}")
    else:
        print(json.dumps(output, ensure_ascii=False, indent=2))


def parse_args():
    parser = argparse.ArgumentParser(
        description="Inspect WooCommerce order meta_data display_value types."
    )
    parser.add_argument(
        "--base-url",
        default=os.environ.get("WOO_BASE_URL"),
        help="WooCommerce site base URL, e.g. https://example.com",
    )
    parser.add_argument(
        "--consumer-key",
        default=os.environ.get("WOO_API_KEY"),
        help="WooCommerce REST API consumer key",
    )
    parser.add_argument(
        "--consumer-secret",
        default=os.environ.get("WOO_API_SECRET"),
        help="WooCommerce REST API consumer secret",
    )
    parser.add_argument(
        "--status",
        default="processing",
        help="Order status filter (default: processing)",
    )
    parser.add_argument(
        "--per-page",
        type=int,
        default=5,
        help="Orders per page to inspect (default: 5)",
    )
    parser.add_argument(
        "--output",
        default="",
        help="Output JSON path (default: stdout)",
    )
    parser.add_argument(
        "--only-non-string",
        action="store_true",
        help="Only include meta_data with non-string display_value",
    )
    return parser.parse_args()


def main():
    args = parse_args()
    if not args.base_url:
        sys.stderr.write("Missing --base-url or WOO_BASE_URL\n")
        return 2
    if not args.consumer_key or not args.consumer_secret:
        sys.stderr.write("Missing --consumer-key/--consumer-secret or env vars\n")
        return 2

    inspect_orders(
        args.base_url,
        args.consumer_key,
        args.consumer_secret,
        args.status,
        args.per_page,
        args.output,
        args.only_non_string,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
