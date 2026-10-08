"""Explicit, disposable GitHub-hosted Windows TUN acceptance. Default: plan only."""
import argparse
import ctypes
import json
import os
from pathlib import Path
import subprocess

DEVICE = "RouteAgentAcceptance"
PREFIX = "198.19.0.0/16"
APPROVAL_LABEL = "accept-core-tun-on-hosted-windows"


def plan():
    return {
        "status": "not_run", "scope": "owned disposable Windows guest only",
        "core": "Mihomo v1.19.32", "device": DEVICE, "route_prefix": PREFIX,
        "network_effects": ["create temporary Wintun adapter", "route only synthetic Fake-IP prefix"],
        "targets": ["allowlisted .test fixtures and guest loopback services"],
        "external_target_requests": 0, "model_requests": 0,
        "host_flclash_changes": False, "system_proxy_changes": False,
        "requires_explicit_permission": True, "approval_label": APPROVAL_LABEL,
        "checks": ["native socket enters TUN", "first MATCH retains fallback",
                   "later TCP 443 uses learned AND rule", "TCP 8443 retains fallback",
                   "original rule precedence", "TTL/restart/exit cleanup", "guest route cleanup"],
        "does_not_prove": ["FlClash application/settings integration",
                           "independent external direct/proxy quality", "real-evidence rule publication"],
    }


def require_hosted_guest_permission(allowed, environment=None, platform=None, is_admin=None):
    env = os.environ if environment is None else environment
    platform = os.name if platform is None else platform
    if not allowed:
        raise RuntimeError("TUN execution requires explicit --allow-isolated-tun permission")
    expected = {"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted",
                "RUNNER_OS": "Windows", "GITHUB_REPOSITORY": "Sonderjyf/mihomo-route-agent"}
    if platform != "nt" or any(env.get(key) != value for key, value in expected.items()) or not env.get("GITHUB_RUN_ID", "").isdigit():
        raise RuntimeError("TUN execution is restricted to the owned disposable GitHub-hosted Windows guest")
    admin = ctypes.windll.shell32.IsUserAnAdmin() if is_admin is None else is_admin
    if not admin:
        raise RuntimeError("Guest administrator access is required; do not elevate a host session")


def tun_config():
    return {"enable": True, "device": DEVICE, "stack": "gvisor", "auto-route": True,
            "auto-detect-interface": True, "strict-route": False, "dns-hijack": [],
            "route-address": [PREFIX]}


def guest_snapshot():
    script = r"""
$ErrorActionPreference = 'Stop'
$processes = @(Get-Process -Name FlClash,FlClashCore,FlClashHelperService -ErrorAction SilentlyContinue)
$defaultRoutes = @(Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' | Sort-Object InterfaceIndex,NextHop | Select-Object InterfaceIndex,NextHop,RouteMetric)
$dns = @(Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object {$_.InterfaceAlias -ne 'RouteAgentAcceptance'} | Sort-Object InterfaceIndex | Select-Object InterfaceIndex,ServerAddresses)
$owned = @(Get-NetRoute -AddressFamily IPv4 | Where-Object {$_.InterfaceAlias -eq 'RouteAgentAcceptance'} | Select-Object DestinationPrefix,InterfaceIndex)
$interfaces = @(Get-NetIPInterface -AddressFamily IPv4 | Where-Object {$_.InterfaceAlias -eq 'RouteAgentAcceptance'} | Select-Object InterfaceIndex,ConnectionState)
$prefix = @(Get-NetRoute -AddressFamily IPv4 | Where-Object {$_.DestinationPrefix -eq '198.19.0.0/16'} | Select-Object InterfaceAlias,DestinationPrefix)
[ordered]@{unrelated_flclash_count=$processes.Count; default_routes=$defaultRoutes; dns=$dns; owned_routes=$owned; owned_interfaces=$interfaces; prefix_routes=$prefix} | ConvertTo-Json -Depth 5 -Compress
"""
    result = subprocess.run(["pwsh", "-NoProfile", "-NonInteractive", "-Command", script],
                            capture_output=True, text=True, timeout=15, check=True)
    return json.loads(result.stdout)


def require_clean_guest(snapshot):
    if snapshot["unrelated_flclash_count"] or snapshot["owned_routes"] or snapshot["owned_interfaces"] or snapshot["prefix_routes"] or not snapshot["default_routes"]:
        raise RuntimeError("Guest is not clean or synthetic route prefix is already in use")


def require_active_tun(before, active):
    if before["default_routes"] != active["default_routes"] or before["dns"] != active["dns"]:
        raise RuntimeError("Guest default route or existing-adapter DNS unexpectedly changed")
    if not any(route["InterfaceAlias"] == DEVICE for route in active["prefix_routes"]):
        raise RuntimeError("Synthetic prefix is not routed into the owned TUN")


def require_clean_exit(before, after):
    if before != after:
        raise RuntimeError("Guest route/DNS/process state did not return to its initial snapshot")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--allow-isolated-tun", action="store_true")
    parser.add_argument("--agent")
    parser.add_argument("--mihomo")
    parser.add_argument("--workdir")
    parser.add_argument("--lifecycle", action="store_true")
    options = parser.parse_args()
    if not options.execute:
        print(json.dumps(plan(), indent=2))
        return
    require_hosted_guest_permission(options.allow_isolated_tun)
    if not all([options.agent, options.mihomo, options.workdir]):
        parser.error("execution requires --agent, --mihomo and a new --workdir")
    if Path(options.workdir).exists():
        parser.error("work directory already exists")
    options.protected_ports = None
    from smoke_observe import run
    run(options, tun=True)


if __name__ == "__main__":
    main()
