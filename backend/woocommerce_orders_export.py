#!/usr/bin/env python3
import argparse
import csv
import json
import os
import sys
import urllib.parse
import urllib.request
from decimal import Decimal, InvalidOperation


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


def parse_variation_name(meta_data):
    parts = []
    for meta in meta_data or []:
        display_key = meta.get("display_key")
        display_value = meta.get("display_value")
        if display_key and display_value:
            parts.append(f"{display_key}: {display_value}")
        else:
            key = meta.get("key")
            value = meta.get("value")
            if key and value and key.startswith("pa_"):
                parts.append(f"{key[3:]}: {value}")
            elif key and value and key.startswith("attribute_"):
                parts.append(f"{key[10:]}: {value}")
    return ", ".join(parts)


def unit_price_from_item(item):
    price = item.get("price")
    if price is not None:
        return price
    quantity = item.get("quantity") or 0
    subtotal = item.get("subtotal")
    if quantity and subtotal is not None:
        try:
            return str(Decimal(subtotal) / Decimal(quantity))
        except (InvalidOperation, ZeroDivisionError):
            return ""
    return ""


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


def export_orders(base_url, consumer_key, consumer_secret, status, output_path):
    rows = []
    order_count = 0
    line_count = 0
    for order in iter_orders(base_url, consumer_key, consumer_secret, status):
        if order.get("status") == "cancelled":
            continue
        order_count += 1
        for item in order.get("line_items", []):
            product_name = item.get("name", "")
            variation_name = ""
            if item.get("variation_id"):
                variation_name = parse_variation_name(item.get("meta_data"))
            quantity = item.get("quantity", "")
            unit_price = unit_price_from_item(item)
            rows.append([product_name, variation_name, quantity, unit_price])
            line_count += 1

    with open(output_path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["product name", "variation name", "數量", "單價"])
        writer.writerows(rows)
    print(f"處理訂單: {order_count}，明細: {line_count}")


def parse_args():
    parser = argparse.ArgumentParser(
        description="Export WooCommerce order line items to CSV."
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
        default="orders.csv",
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

    export_orders(
        args.base_url,
        args.consumer_key,
        args.consumer_secret,
        args.status,
        args.output,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
