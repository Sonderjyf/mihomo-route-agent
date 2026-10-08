"""Offline FlClash configuration contract checker; never launches an application."""
import argparse
import json
from pathlib import Path


SOURCE_SHA = "68c71b8ef9b7486a224972eb371ff153c6b2de0f"
ARCHIVE_SHA256 = "9ff3a9315b51e6665669bfde9b0323ddb671a22d0a9647987b4cc5e574007f4f"
MANAGED = ("route-agent-tail-direct", "route-agent-tail-proxy")


def plan():
    return {
        "status": "not_run", "app_tested": False, "tun_tested": False,
        "application": "FlClash v0.8.99", "source_sha": SOURCE_SHA,
        "archive_sha256": ARCHIVE_SHA256,
        "archive_url": "https://github.com/chen08209/FlClash/releases/download/v0.8.99/FlClash-0.8.99-windows-amd64.zip",
        "next_scope": "one disposable hosted Windows guest; TUN disabled; synthetic loopback fixtures",
        "checks": ["actual app-generated config after script overwrite",
                   "app-driven profile refresh retains exactly one managed tail",
                   "runtime rules agree with generated config",
                   "app/core startup and graceful shutdown",
                   "guest proxy, DNS and routes unchanged"],
        "requires_new_permission": True,
        "execution_implemented": False,
        "blockers": ["release GUI automation and preference seeding not yet validated",
                     "no runtime SAFE_MODE switch; it is a compile-time define"],
    }


def check_config(original, effective, proxy_target):
    """Check parsed JSON snapshots, without authenticating their app provenance."""
    rules = original.get("rules")
    if not isinstance(rules, list) or not rules or not all(isinstance(r, str) for r in rules):
        raise ValueError("original rules must be a nonempty string list")
    if not rules[-1].startswith("MATCH,") or any(r.startswith("MATCH,") for r in rules[:-1]):
        raise ValueError("original must have one terminal MATCH")
    if "sub-rules" in original or any(r.startswith(("AND,", "OR,", "NOT,", "SUB-RULE,")) for r in rules):
        raise ValueError("conditional source rules require separate review")
    if not proxy_target or any(c in proxy_target for c in ",\r\n"):
        raise ValueError("invalid proxy target")
    names = [p.get("name") for key in ("proxies", "proxy-groups") for p in original.get(key, [])]
    if names.count(proxy_target) != 1:
        raise ValueError("proxy target must resolve exactly once")
    if any(name in json.dumps(original) for name in MANAGED):
        raise ValueError("reserved provider already present in original")
    added = [f"AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,{name})),{target}"
             for name, target in zip(MANAGED, ("DIRECT", proxy_target))]
    if effective.get("rules") != rules[:-1] + added + rules[-1:]:
        raise ValueError("effective rules lost order, scope, or exactly-once tail insertion")
    if effective.get("mode") != "rule" or effective.get("tun", {}).get("enable") is not False:
        raise ValueError("this app contract requires rule mode and explicit TUN disabled")
    if effective.get("allow-lan") is not False:
        raise ValueError("app fixture must disable LAN exposure")
    if effective.get("external-controller") != "127.0.0.1:9090":
        raise ValueError("pinned app controller must be explicitly enabled on loopback 9090")
    if original.get("dns") != effective.get("dns"):
        raise ValueError("DNS changed after overwrite; inspect app DNS overrides and appendSystemDns")
    for field in ("proxies", "proxy-groups"):
        if original.get(field) != effective.get(field):
            raise ValueError(f"{field} changed after overwrite")
    before = original.get("rule-providers", {})
    after = effective.get("rule-providers", {})
    if set(after) != set(before) | set(MANAGED):
        raise ValueError("provider inventory mismatch")
    rewritten = []
    for name, provider in before.items():
        actual = after[name]
        if {k: v for k, v in provider.items() if k != "path"} != {k: v for k, v in actual.items() if k != "path"}:
            raise ValueError("original provider changed beyond app-owned cache path")
        if provider.get("path") != actual.get("path"):
            rewritten.append(name)
    for name in MANAGED:
        p = after[name]
        expected = {"type": "http", "behavior": "classical", "format": "yaml", "proxy": "DIRECT",
                    "url": f"http://127.0.0.1:18765/rules/{name}.yaml", "interval": 3600}
        if {k: v for k, v in p.items() if k != "path"} != expected or not p.get("path"):
            raise ValueError("managed provider must use the owned loopback HTTP endpoint and explicit DIRECT")
    return {"config_contract_passed": True, "app_tested": False, "tun_tested": False,
            "original_rule_count": len(rules), "original_provider_paths_rewritten": rewritten,
            "limitations": ["input provenance is not verified", "provider files and runtime APIs are not read",
                            "refresh, startup, shutdown and network restoration remain untested"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--original-json")
    parser.add_argument("--effective-json")
    parser.add_argument("--proxy-target")
    args = parser.parse_args()
    supplied = (args.original_json, args.effective_json, args.proxy_target)
    if not any(supplied):
        print(json.dumps(plan(), indent=2))
        return
    if not all(supplied):
        parser.error("supply original-json, effective-json and proxy-target together")
    try:
        original = json.loads(Path(args.original_json).read_text(encoding="utf-8-sig"))
        effective = json.loads(Path(args.effective_json).read_text(encoding="utf-8-sig"))
        result = check_config(original, effective, args.proxy_target)
    except (OSError, ValueError, TypeError, AttributeError, KeyError) as exc:
        parser.exit(1, f"configuration contract failed: {exc}\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
