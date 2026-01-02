#!/usr/bin/env python3
import argparse
import csv
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


def iter_orders(base_url, consumer_key, consumer_secret, status):
    page = 1
    while True:
        query = {
            "consumer_key": consumer_key,
            "consumer_secret": consumer_secret,
            "per_page": 100,
            "page": page,
        }
        if status:
            query["status"] = status
        url = build_url(base_url, "/wp-json/wc/v3/orders", query)
        orders = fetch_json(url)
        if not orders:
            break
        for order in orders:
            yield order
        page += 1


def billing_name(billing):
    if not billing:
        return ""
    first_name = billing.get("first_name", "") or ""
    last_name = billing.get("last_name", "") or ""
    full_name = f"{first_name} {last_name}".strip()
    return full_name


def export_active_orders(
    base_url, consumer_key, consumer_secret, status, output_path
):
    rows = []
    order_count = 0
    excluded_statuses = {"cancelled", "processing"}
    for order in iter_orders(base_url, consumer_key, consumer_secret, status):
        if order.get("status") in excluded_statuses:
            continue
        billing = order.get("billing") or {}
        rows.append(
            [
                order.get("id", ""),
                billing_name(billing),
                billing.get("email", "") or "",
            ]
        )
        order_count += 1

    with open(output_path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["order id", "billing name", "email"])
        writer.writerows(rows)
    print(f"處理排除取消與處理中的訂單: {order_count}")


def parse_args():
    parser = argparse.ArgumentParser(
        description="Export WooCommerce orders excluding cancelled and processing."
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
        default="",
        help="Optional order status filter (processing, completed, etc.)",
    )
    parser.add_argument(
        "--output",
        default="active_orders.csv",
        help="Output CSV path",
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

    export_active_orders(
        args.base_url,
        args.consumer_key,
        args.consumer_secret,
        args.status,
        args.output,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
