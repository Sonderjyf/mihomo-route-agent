"""Explicit hosted acceptance; default invocation refuses. Stdlib only.

prepare: no secrets, download/hash-check official core and build worker.
execute: protected manual job only, strict budget and inputs, owned processes.
Neither mode is invoked by ordinary CI. See docs/REAL_ACCEPTANCE.md.
"""
import argparse
import base64
from decimal import Decimal
import hashlib
import io
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid
import zipfile

REPO = "Sonderjyf/mihomo-route-agent"
CORE_URL = "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.32/mihomo-windows-amd64-v1.19.32.zip"
ARCHIVE_HASH = "1ac84e795b5b915446e20139677fb6cc013a0778e876e8d4c96f994750b4c4a3"
CORE_HASH = "beb9878924d7bd38c67176b441ab5e668cc378bfe751c8dc8fef3eb5aaf566d5"
FIELDS = {"server", "port", "uuid", "servername", "reality_public_key", "short_id"}
PROBE = {"not_tested", "verified_success", "repeated_failure"}
OUTCOMES = {"not_tested", "verified_success", "connect_error", "timeout", "proxy_connect_error", "proxy_protocol_error", "proxy_rejected", "certificate_error", "tls_error", "chain_unverified"}


class Refused(Exception):
    """No input-derived exception text is ever printed."""


def require(value):
    if not value:
        raise Refused()


def strict_json(raw):
    def pairs(items):
        out = {}
        for key, value in items:
            require(key not in out)
            out[key] = value
        return out
    def invalid(_):
        raise Refused()
    require(isinstance(raw, str) and len(raw) <= 16384)
    return json.loads(raw, object_pairs_hook=pairs, parse_constant=invalid)


def node_input(raw):
    n = strict_json(raw)
    require(type(n) is dict and set(n) == FIELDS)
    require(type(n["port"]) is int and 1 <= n["port"] <= 65535)
    require(all(type(n[k]) is str for k in FIELDS - {"port"}))
    # Numeric global server avoids unapproved extra DNS resolver behavior.
    ip = ipaddress.ip_address(n["server"])
    require(ip.is_global and not ip.is_multicast and not ip.is_reserved and str(ip) == n["server"])
    require(str(uuid.UUID(n["uuid"])) == n["uuid"])
    host = n["servername"]
    require(len(host) <= 253 and host == host.lower() and "." in host)
    require(all(re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?", x) for x in host.split(".")))
    require(not host.endswith((".local", ".localhost", ".internal", ".invalid", ".test")))
    try:
        ipaddress.ip_address(host)
    except ValueError:
        pass
    else:
        raise Refused()
    pub = n["reality_public_key"]
    require(re.fullmatch(r"[A-Za-z0-9_-]{43}", pub) is not None)
    decoded = base64.urlsafe_b64decode(pub + "=")
    require(len(decoded) == 32 and base64.urlsafe_b64encode(decoded).decode().rstrip("=") == pub)
    require(re.fullmatch(r"(?:[0-9a-f]{2}){0,8}", n["short_id"]) is not None)
    return n


def budget(env):
    # Only fixed-point positive amounts, no NaN/Infinity/exponent/coercion.
    for name in ("APPROVED_USD", "PROVIDER_CAP_USD"):
        require(re.fullmatch(r"(?:0|[1-9][0-9]{0,3})(?:\.[0-9]{1,4})?", env.get(name, "")) is not None)
    approved, cap = Decimal(env["APPROVED_USD"]), Decimal(env["PROVIDER_CAP_USD"])
    require(0 < cap <= approved <= Decimal("10"))
    require(env.get("PROVIDER_CAP_CONFIRMED") == "true" and env.get("PROXY_COST_APPROVED") == "true")


def gate(env, head):
    expected = env.get("REVIEWED_SHA", "")
    require(re.fullmatch("[0-9a-f]{40}", expected) is not None and head == expected)
    # Trust anchor: the reviewed MAIN launcher supplies this literal allowlist,
    # never workflow input or repository-variable content.
    require(env.get("REVIEWED_SHA_ALLOWLIST") == expected)
    require(env.get("GITHUB_ACTIONS") == "true" and env.get("RUNNER_ENVIRONMENT") == "github-hosted")
    require(env.get("GITHUB_REPOSITORY") == REPO and env.get("GITHUB_EVENT_NAME") == "workflow_dispatch")
    require(env.get("GITHUB_REF") == "refs/heads/main" and env.get("GITHUB_RUN_ATTEMPT") == "1")
    require(env.get("GITHUB_WORKFLOW_REF") == REPO + "/.github/workflows/real-acceptance.yml@refs/heads/main")
    require(re.fullmatch(r"[0-9]+", env.get("GITHUB_RUN_ID", "")) is not None)
    require(env.get("APPROVAL_RECORD", "").strip() != "")
    budget(env)


def child_env(env, model=False):
    # An allowlist avoids accidentally forwarding credentials from the runner.
    names = {"SYSTEMROOT", "WINDIR", "SYSTEMDRIVE", "TEMP", "TMP", "PATH", "PATHEXT"}
    out = {k: v for k, v in env.items() if k.upper() in names}
    if model:
        out["OPENROUTER_API_KEY"] = env["OPENROUTER_API_KEY"]
    return out


def core_config(node, secret):
    proxy = {"name": "acceptance-vless", "type": "vless", "server": node["server"], "port": node["port"], "uuid": node["uuid"], "network": "tcp", "tls": True, "skip-cert-verify": False, "flow": "xtls-rprx-vision", "servername": node["servername"], "client-fingerprint": "chrome", "udp": False, "reality-opts": {"public-key": node["reality_public_key"], "short-id": node["short_id"]}}
    return {"mixed-port": 17897, "allow-lan": False, "bind-address": "127.0.0.1", "external-controller": "127.0.0.1:19097", "secret": secret, "mode": "rule", "log-level": "silent", "ipv6": False, "tun": {"enable": False}, "dns": {"enable": False}, "sniffer": {"enable": False}, "profile": {"store-selected": False, "store-fake-ip": False}, "geo-auto-update": False, "proxies": [proxy], "proxy-groups": [], "rules": ["MATCH,acceptance-vless"]}


def output_contract(raw):
    r = strict_json(raw)
    require(type(r) is dict and set(r) == {"hosts", "model_attempts", "routing_updated", "tun_tested"})
    require(r["routing_updated"] is False and r["tun_tested"] is False)
    require(type(r["model_attempts"]) is int and 0 <= r["model_attempts"] <= 2)
    require(type(r["hosts"]) is list and len(r["hosts"]) == 2)
    for i, row in enumerate(r["hosts"]):
        require(type(row) is dict and set(row) == {"index", "probe", "vless_tls", "model", "choice", "accepted"})
        require(type(row["index"]) is int and row["index"] == i)
        require(row["vless_tls"] in OUTCOMES and row["model"] in {"not_attempted", "unavailable", "invalid", "answered"})
        require(row["choice"] in {"DIRECT", "PROXY", "UNCERTAIN"} and row["accepted"] in {"DIRECT", "PROXY", "UNCERTAIN"})
        p = row["probe"]
        require(type(p) is dict and set(p) == {"evidence", "dns", "direct_attempts", "proxy_attempt"})
        require(p["dns"] in {"not_tested", "unavailable", "protected_or_fake_ip", "resolved"})
        require(type(p["evidence"]) is dict and set(p["evidence"]) == {"direct_tls", "proxy_tls"} and all(v in PROBE for v in p["evidence"].values()))
        require(type(p["direct_attempts"]) is list and len(p["direct_attempts"]) <= 2 and all(v in OUTCOMES for v in p["direct_attempts"]))
        require(p["proxy_attempt"] in OUTCOMES)
    return r


def ports_free():
    for port in (17897, 19097):
        with socket.socket() as s:
            s.bind(("127.0.0.1", port))


def stop_owned(process):
    if process is not None and process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)


def prepare(root, env):
    require(not env.get("OPENROUTER_API_KEY") and not env.get("VLESS_NODE_JSON"))
    root.mkdir(mode=0o700)
    # Official immutable version + two SHA256 checks; HTTPS verification remains
    # enabled through redirects. No archive extraction paths are trusted.
    with urllib.request.urlopen(CORE_URL, timeout=60) as response:
        require(response.url.startswith("https://"))
        wire = response.read(64 * 1024 * 1024 + 1)
    require(len(wire) <= 64 * 1024 * 1024 and hashlib.sha256(wire).hexdigest() == ARCHIVE_HASH)
    with zipfile.ZipFile(io.BytesIO(wire)) as archive:
        members = [n for n in archive.infolist() if n.filename.endswith(".exe")]
        require(len(members) == 1 and members[0].file_size <= 128 * 1024 * 1024)
        binary = archive.read(members[0])
    require(hashlib.sha256(binary).hexdigest() == CORE_HASH)
    (root / "mihomo.exe").write_bytes(binary)
    subprocess.run(["go", "build", "-trimpath", "-o", str(root / "worker.exe"), "./cmd/real-acceptance"], check=True, timeout=180, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    (root / "worker.sha256").write_text(hashlib.sha256((root / "worker.exe").read_bytes()).hexdigest(), encoding="ascii")


def execute(root, env):
    require(sys.platform == "win32")
    require(hashlib.sha256((root / "mihomo.exe").read_bytes()).hexdigest() == CORE_HASH)
    require(hashlib.sha256((root / "worker.exe").read_bytes()).hexdigest() == (root / "worker.sha256").read_text(encoding="ascii"))
    # Refuse reruns before reading secrets or starting a process.
    with (root / ("attempt-" + env["GITHUB_RUN_ID"])).open("x", encoding="ascii") as f:
        f.write("consumed")
    require(0 <= time.time() - float(env["JOB_STARTED_AT"]) <= 600)
    node = node_input(env.get("VLESS_NODE_JSON", ""))
    key = env.get("OPENROUTER_API_KEY", "")
    require(0 < len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key))
    ports_free()
    private = Path(tempfile.mkdtemp(prefix="private-", dir=root))
    core = None
    try:
        secret = secrets.token_urlsafe(32)
        config = private / "config.json"
        config.write_text(json.dumps(core_config(node, secret)), encoding="utf-8")
        core = subprocess.Popen([str(root / "mihomo.exe"), "-d", str(private), "-f", str(config)], env=child_env(env), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        # Bounded loopback readiness only; no model availability request.
        ready = False
        for _ in range(100):
            require(core.poll() is None)
            try:
                with socket.create_connection(("127.0.0.1", 19097), timeout=.2):
                    ready = True
                    break
            except OSError:
                time.sleep(.1)
        require(ready)
        worker_env = child_env(env, model=True)
        worker_env.update(REAL_ACCEPTANCE_WORKER="1", REAL_CORE_PID=str(core.pid), REAL_CONTROLLER_SECRET=secret)
        completed = subprocess.run([str(root / "worker.exe")], env=worker_env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=270, check=True, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        result = output_contract(completed.stdout.decode("utf-8"))
    finally:
        try:
            stop_owned(core)
        finally:
            # Literal owned mkdtemp directory only; no user-controlled paths.
            require(private.parent.resolve() == root.resolve() and private.name.startswith("private-"))
            shutil.rmtree(private)
    ports_free()
    require(not private.exists())
    result["cleanup"] = "verified"
    result["status"] = "completed" if all(h["vless_tls"] == "verified_success" and h["model"] == "answered" for h in result["hosts"]) else "partial"
    result["proxy_learning_tested"] = False
    return result


def main(argv=None):
    # All exceptions, including malformed JSON/key material/OS errors, are
    # reduced to a fixed status; never print traceback, command, path or body.
    try:
        parser = argparse.ArgumentParser()
        parser.add_argument("mode", choices=("prepare", "execute"))
        args = parser.parse_args(argv)
        env = dict(os.environ)
        head = subprocess.check_output(["git", "rev-parse", "HEAD"], stderr=subprocess.DEVNULL, timeout=10).decode("ascii").strip()
        gate(env, head)
        root = Path(env["RUNNER_TEMP"]).resolve() / ("real-acceptance-" + env["GITHUB_RUN_ID"])
        if args.mode == "prepare":
            prepare(root, env)
            print('{"stage":"prepared","secrets_used":false}')
        else:
            print(json.dumps(execute(root, env), separators=(",", ":")))
        return 0
    except (Exception, KeyboardInterrupt):
        print('{"status":"refused_or_failed","details":"redacted"}')
        return 1


if __name__ == "__main__":
    sys.exit(main())
