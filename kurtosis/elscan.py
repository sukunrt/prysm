#!/usr/bin/env python3
"""Read an execution layer's blocks and report the blob traffic in them.

    kurtosis/elscan.py <rpc-url> [--from N] [--to N]

One row per block: transactions, gas, blob gas, and the running total of blobs
(blob gas / 131072). A run whose payloads are empty prints zeros, which is the
difference between the column plumbing being up and the columns being real.
"""

import argparse
import json
import urllib.request

BLOB_GAS_PER_BLOB = 131072


def rpc(url, method, params):
    req = urllib.request.Request(
        url, data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method,
                              "params": params}).encode(),
        headers={"content-type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        body = json.load(resp)
    if "error" in body:
        raise RuntimeError(body["error"])
    return body["result"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("url")
    ap.add_argument("--from", dest="first", type=int, default=1)
    ap.add_argument("--to", dest="last", type=int, default=None)
    args = ap.parse_args()

    last = args.last
    if last is None:
        last = int(rpc(args.url, "eth_blockNumber", []), 16)

    print("| block | txs | gas used | blob gas | blobs |")
    print("|---|---|---|---|---|")
    blocks_with_blobs = total_blobs = total_txs = 0
    for n in range(args.first, last + 1):
        b = rpc(args.url, "eth_getBlockByNumber", [hex(n), False])
        blob_gas = int(b.get("blobGasUsed") or "0x0", 16)
        blobs = blob_gas // BLOB_GAS_PER_BLOB
        txs = len(b["transactions"])
        total_txs += txs
        total_blobs += blobs
        blocks_with_blobs += 1 if blobs else 0
        print(f"| {n} | {txs} | {int(b['gasUsed'], 16)} | {blob_gas} | "
              f"{blobs} |")
    span = last - args.first + 1
    print(f"\n{span} blocks, {total_txs} transactions, {total_blobs} blobs in "
          f"{blocks_with_blobs} blocks "
          f"({100 * blocks_with_blobs / max(1, span):.0f}% of blocks).")


if __name__ == "__main__":
    main()
