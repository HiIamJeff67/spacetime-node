#!/usr/bin/env python3
"""Small stdlib-only journey -> recommendation load test."""

import argparse
import json
import math
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from collections import Counter


USER_ID_HASH = "sha256:" + "a" * 64


def request_json(url, method="GET", payload=None, timeout=5):
    body = None if payload is None else json.dumps(payload).encode()
    request = urllib.request.Request(
        url,
        data=body,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, json.loads(response.read())
    except urllib.error.HTTPError as error:
        return error.code, {}


def percentile(values, fraction):
    if not values:
        return None
    values = sorted(values)
    return values[min(len(values) - 1, max(0, math.ceil(len(values) * fraction) - 1))]


def run_one(base_url, station_id, poll_timeout, poll_interval):
    started = time.perf_counter()
    trace_id = "load-test-" + uuid.uuid4().hex
    status, entry = request_json(
        base_url + "/v1/entry-events",
        method="POST",
        payload={
            "request_context": {"trace_id": trace_id},
            "user_id_hash": USER_ID_HASH,
            "station_id": station_id,
        },
    )
    if status != 200 or not entry.get("journey_id"):
        return {"ok": False, "error": f"entry_status_{status}"}

    journey_id = entry["journey_id"]
    deadline = time.perf_counter() + poll_timeout
    while time.perf_counter() < deadline:
        query = urllib.parse.urlencode({"journey_id": journey_id})
        status, recommendation = request_json(base_url + "/v1/recommendations/latest?" + query)
        if status == 200 and recommendation.get("recommendation_id"):
            return {
                "ok": True,
                "elapsed_ms": (time.perf_counter() - started) * 1000,
                "decision_latency_ms": recommendation.get("decision_latency_ms"),
                "copy_source": recommendation.get("copy_source", ""),
            }
        time.sleep(poll_interval)
    return {"ok": False, "error": "recommendation_timeout"}


def measure_qdrant(qdrant_url, station_id, samples, timeout):
    if not qdrant_url:
        return {"samples": 0, "p50_ms": None, "p95_ms": None}
    values = []
    payload = {
        "vector": [0.0] * 32,
        "limit": 10,
        "with_payload": True,
        "filter": {"must": [{"key": "station_ids", "match": {"any": [station_id]}}]},
    }
    for _ in range(samples):
        started = time.perf_counter()
        status, _ = request_json(
            qdrant_url.rstrip("/") + "/collections/offer_embeddings_v1/points/search",
            method="POST",
            payload=payload,
            timeout=timeout,
        )
        if status == 200:
            values.append((time.perf_counter() - started) * 1000)
    return {"samples": len(values), "p50_ms": percentile(values, 0.50), "p95_ms": percentile(values, 0.95)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default=os.getenv("GATEWAY_URL", "http://127.0.0.1:8000"))
    parser.add_argument("--station-id", default="R04")
    parser.add_argument("--requests", type=int, default=20)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--poll-timeout", type=float, default=5.0)
    parser.add_argument("--poll-interval", type=float, default=0.05)
    parser.add_argument("--qdrant-url", default=os.getenv("QDRANT_URL", ""))
    parser.add_argument("--qdrant-samples", type=int, default=10)
    parser.add_argument("--output", default="")
    args = parser.parse_args()
    if args.requests < 1 or args.concurrency < 1:
        parser.error("requests and concurrency must be positive")

    started = time.perf_counter()
    results = []
    with ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = [executor.submit(run_one, args.base_url.rstrip("/"), args.station_id, args.poll_timeout, args.poll_interval) for _ in range(args.requests)]
        for future in as_completed(futures):
            results.append(future.result())
    elapsed = time.perf_counter() - started
    successful = [result for result in results if result["ok"]]
    e2e = [result["elapsed_ms"] for result in successful]
    decision = [result["decision_latency_ms"] for result in successful if isinstance(result.get("decision_latency_ms"), (int, float))]
    fallback = [result for result in successful if result.get("copy_source") == "template"]
    report = {
        "target_p95_ms": 2000,
        "base_url": args.base_url,
        "requests": args.requests,
        "concurrency": args.concurrency,
        "elapsed_seconds": round(elapsed, 3),
        "successes": len(successful),
        "failures": len(results) - len(successful),
        "throughput_per_second": round(len(successful) / elapsed, 2) if elapsed else 0,
        "e2e_ms": {"p50": percentile(e2e, 0.50), "p95": percentile(e2e, 0.95), "max": max(e2e) if e2e else None},
        "decision_latency_ms": {"p50": percentile(decision, 0.50), "p95": percentile(decision, 0.95)},
        "qdrant_latency_ms": measure_qdrant(args.qdrant_url, args.station_id, args.qdrant_samples, 5),
        "fallback_rate": round(len(fallback) / len(successful), 3) if successful else None,
        "error_counts": dict(Counter(result.get("error", "unknown") for result in results if not result["ok"])),
        "kafka_lag": "capture with kafka-consumer-groups.sh after run",
    }
    rendered = json.dumps(report, indent=2, sort_keys=True)
    print(rendered)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as output:
            output.write(rendered + "\n")
    if report["failures"] or (report["e2e_ms"]["p95"] is not None and report["e2e_ms"]["p95"] > report["target_p95_ms"]):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
