"""Manual, paid API experiment. Synthetic inputs only; never logs credentials.

No third-party Python packages. An explicit HTTP CONNECT proxy is optional.
This is a benchmark client, NOT a production DNS gate or hard-deadline transport.
"""
import argparse
import concurrent.futures
import http.client
import json
import math
import os
from pathlib import Path
import ssl
import time
from urllib.parse import urlsplit

HOST = "openrouter.ai"
MODEL = "typesafe/jev-1.13"
LABELS = {"DIRECT", "PROXY", "UNCERTAIN"}


def load_key(key_file=None):
    key = os.environ.get("OPENROUTER_API_KEY", "")
    if not key and key_file:
        for line in Path(key_file).read_text(encoding="utf-8-sig").splitlines():
            name, separator, value = line.strip().partition("=")
            if separator and name.strip() == "OPENROUTER_API_KEY":
                key = value.strip().strip("\"'")
                break
    if not key or key.startswith("${"):
        raise ValueError("OPENROUTER_API_KEY is missing")
    return key


def state_for(kind="missing", hostname="unknown.route-lab.test"):
    evidence = {
        "missing": {"direct_tls": "not_tested", "proxy_tls": "not_tested"},
        "direct": {"direct_tls": "verified_success", "proxy_tls": "not_tested"},
        "proxy": {"direct_tls": "repeated_failure", "proxy_tls": "verified_success"},
        "conflict": {"direct_tls": "conflicting_results", "proxy_tls": "conflicting_results"},
    }[kind]
    return {"hostname": hostname, "static_rules": "no_match",
            "fixture": "synthetic_not_live_network_measurement",
            "evidence": evidence}


def payload(state):
    return {"model": MODEL, "state": state, "questions": {"route": {
        "type": "choice",
        "instructions": "Choose a route only from supplied evidence. Domain spelling, TLD, brand knowledge and model memory are not current reachability evidence. Missing, untested or conflicting evidence requires UNCERTAIN.",
        "criteria": {
            "DIRECT": "Explicit verified successful direct TLS evidence, without conflicting direct results.",
            "PROXY": "Explicit repeated direct failures AND verified proxy TLS success, without conflicting results.",
            "UNCERTAIN": "Evidence is missing, untested, conflicting, or cannot establish DIRECT or PROXY. Never infer DIRECT from low PROXY probability.",
        }}}}


def validate(body):
    answer = body.get("answers", {}).get("route", {})
    scores = answer.get("probabilities", {})
    if answer.get("type") != "choice" or answer.get("choice") not in LABELS:
        raise ValueError("invalid_choice")
    if set(scores) != LABELS or any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) or not 0 <= v <= 1 for v in scores.values()):
        raise ValueError("invalid_probabilities")
    if abs(sum(scores.values()) - 1) > 0.02:
        raise ValueError("invalid_probability_sum")
    return answer


def accept(state, answer):
    """Evidence veto plus a provisional score threshold, not calibration."""
    evidence = state["evidence"]
    permitted = "UNCERTAIN"
    if evidence["direct_tls"] == "verified_success":
        permitted = "DIRECT"
    elif evidence["direct_tls"] == "repeated_failure" and evidence["proxy_tls"] == "verified_success":
        permitted = "PROXY"
    choice = answer["choice"]
    return choice if choice == permitted and answer["probabilities"][choice] >= 0.8 else "UNCERTAIN"


class Client:
    def __init__(self, key, proxy=None, timeout=8):
        self.key, self.proxy, self.timeout = key, proxy, timeout
        self.connection = None

    def close(self):
        if self.connection:
            self.connection.close()
            self.connection = None

    def call(self, state):
        start = time.perf_counter()
        try:
            if self.connection is None:
                if self.proxy:
                    proxy = urlsplit(self.proxy)
                    if proxy.scheme != "http" or proxy.hostname not in {"127.0.0.1", "localhost"} or proxy.username:
                        raise ValueError("only_unauthenticated_loopback_http_proxy_supported")
                    self.connection = http.client.HTTPSConnection(proxy.hostname, proxy.port, timeout=self.timeout, context=ssl.create_default_context())
                    self.connection.set_tunnel(HOST, 443)
                else:
                    self.connection = http.client.HTTPSConnection(HOST, timeout=self.timeout, context=ssl.create_default_context())
            self.connection.request("POST", "/api/alpha/decisions", json.dumps(payload(state)).encode(),
                                    {"Authorization": "Bearer " + self.key, "Content-Type": "application/json"})
            response = self.connection.getresponse()
            raw = response.read(262145)
            elapsed = round((time.perf_counter() - start) * 1000, 3)
            if response.status != 200:
                self.close()
                return {"ok": False, "status": response.status, "ms": elapsed}
            if len(raw) > 262144:
                raise ValueError("oversized_response")
            body = json.loads(raw)
            answer = validate(body)
            return {"ok": True, "status": 200, "ms": elapsed, "answer": answer,
                    "accepted": accept(state, answer), "model": body.get("model"), "usage": body.get("usage", {})}
        except Exception as error:
            self.close()
            # Do not serialize error text: remote responses may contain private data.
            return {"ok": False, "error_type": type(error).__name__,
                    "ms": round((time.perf_counter() - start) * 1000, 3)}


def stats(rows):
    values = sorted(row["ms"] for row in rows if row["ok"])
    def percentile(p):
        return values[max(0, math.ceil(len(values) * p) - 1)] if values else None
    return {"attempted": len(rows), "success": len(values), "failures": len(rows) - len(values),
            "p50_ms": percentile(.5), "p95_ms": percentile(.95), "max_ms": max(values) if values else None}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--key-file", help="optional local dotenv; never copied to output")
    parser.add_argument("--proxy", help="explicit http://127.0.0.1:PORT; environment proxies ignored")
    parser.add_argument("--count", type=int, default=40)
    parser.add_argument("--probe-only", action="store_true")
    parser.add_argument("--output", default="local-evidence/jev-api.json")
    args = parser.parse_args()
    if not 1 <= args.count <= 100:
        parser.error("count must be 1..100")
    key = load_key(args.key_file)
    client = Client(key, args.proxy)
    result = {"date": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "requested_model": MODEL,
              "transport": "explicit_loopback_HTTP_CONNECT" if args.proxy else "no_explicit_HTTP_proxy_TUN_may_still_apply",
              "fixture_scope": "synthetic_contract_not_real_route_accuracy", "first": client.call(state_for())}
    print(json.dumps({"first": result["first"]}), flush=True)
    try:
        if result["first"]["ok"] and not args.probe_only:
            result["sequential"] = [client.call(state_for(hostname=f"unknown-{i}.route-lab.test")) for i in range(args.count)]
            print(json.dumps({"sequential": stats(result["sequential"])}), flush=True)
            cases = []
            for kind, expected in [("missing", "UNCERTAIN"), ("direct", "DIRECT"), ("proxy", "PROXY"), ("conflict", "UNCERTAIN")]:
                for i in range(3):
                    state = state_for(kind, f"{kind}-{i}.route-lab.test")
                    cases.append({"kind": kind, "expected": expected, **client.call(state)})
            result["cases"] = cases
            def burst(i):
                other = Client(key, args.proxy)
                try:
                    return other.call(state_for(hostname=f"burst-{i}.route-lab.test"))
                finally:
                    other.close()
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                result["burst"] = list(pool.map(burst, range(4)))
            result["summary"] = {"sequential": stats(result["sequential"]), "burst": stats(result["burst"]),
                "contract_raw_matches": sum(r["ok"] and r["answer"]["choice"] == r["expected"] for r in cases),
                "contract_accepted_matches": sum(r["ok"] and r["accepted"] == r["expected"] for r in cases), "contract_cases": len(cases)}
        rows = [result["first"], *result.get("sequential", []), *result.get("cases", []), *result.get("burst", [])]
        result["reported_cost_usd"] = sum(r.get("usage", {}).get("cost", 0) or 0 for r in rows)
        result["usage_cost_missing_count"] = sum(r["ok"] and "cost" not in r.get("usage", {}) for r in rows)
        path = Path(args.output)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(result, indent=2), encoding="utf-8")
        print(json.dumps({"summary": result.get("summary"), "reported_cost_usd": result["reported_cost_usd"]}), flush=True)
    finally:
        client.close()


if __name__ == "__main__":
    main()
