"""Explicit hosted acceptance; default invocation refuses. Stdlib only.

prepare: no secrets, download/hash-check official core and build worker.
execute: protected manual job only, strict budget and inputs, owned processes.
Neither mode is invoked by ordinary CI. See docs/REAL_ACCEPTANCE.md.
"""
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


STAGES = set("arguments source gate budget root prepare_inputs download archive_hash archive_unpack core_hash build worker_hash platform once deadline node_schema api_key ports private_directory config_write core_spawn core_ready worker_spawn worker_wait worker_output cleanup prepared complete".split())
REASONS = set("none rejected unexpected_error invalid_mode invalid_json duplicate_key nonfinite_json input_size missing_fields unknown_fields type_mismatch invalid_value process_start_failed child_exit timeout interrupted io_error hash_mismatch already_consumed deadline_expired core_exited core_not_ready worker_failed invalid_worker_output cleanup_failed".split())
TYPES = {type(None): "null", bool: "boolean", int: "integer", float: "number", str: "string", list: "array", dict: "object"}
CLEANUP_STATES = {"not_started", "verified", "failed", "unknown"}


class Refused(Exception):
    def __init__(self, reason="rejected", fields=(), expected="", actual=""):
        # Never use exception text for diagnostics, even for our own exceptions.
        self.reason, self.fields, self.expected, self.actual = reason, fields, expected, actual


def require(value, reason="rejected", fields=(), expected="", actual=""):
    if not value:
        raise Refused(reason, fields, expected, actual)


def exit_code(value):
    return value if type(value) is int and -(2**31) <= value < 2**32 else None


class Report:
    def __init__(self):
        self.stage = "arguments"
        self.reason = "none"
        self.fields = []
        self.expected = self.actual = ""
        self.status = "failed"
        self.worker_result = None
        self.model_attempts = 0
        self.attempts_state = "not_started"
        self.core_start_exit_code = self.worker_exit_code = None
        self.cleanup = {k: "not_started" for k in ("worker", "core", "private_files", "ports")}
        self.cleanup_exit_codes = {"worker": None, "core": None}

    def failure(self, error):
        if isinstance(error, Refused):
            self.reason = error.reason if error.reason in REASONS else "unexpected_error"
            self.fields = sorted({f for f in error.fields if f in FIELDS})
            self.expected = error.expected if error.expected in set(TYPES.values()) else ""
            self.actual = error.actual if error.actual in set(TYPES.values()) | {"other"} else ""
        elif isinstance(error, subprocess.TimeoutExpired):
            self.reason = "timeout"
        elif isinstance(error, subprocess.CalledProcessError):
            self.reason = "child_exit"
        elif isinstance(error, KeyboardInterrupt):
            self.reason = "interrupted"
        elif isinstance(error, OSError):
            self.reason = "process_start_failed" if self.stage in {"core_spawn", "worker_spawn"} else "io_error"
        else:
            self.reason = "unexpected_error"
        self.status = "failed"

    def payload(self):
        # Reconstruction, never dict(error), repr(error), or unknown worker fields.
        cleanup = {k: v if v in CLEANUP_STATES else "unknown" for k, v in self.cleanup.items()}
        return {"status": self.status, "diagnostic": {"stage": self.stage if self.stage in STAGES else "cleanup", "reason": self.reason if self.reason in REASONS else "unexpected_error", "fields": self.fields, "expected_type": self.expected, "actual_type": self.actual}, "model_attempts": self.model_attempts, "model_attempts_state": self.attempts_state, "core_start_exit_code": exit_code(self.core_start_exit_code), "worker_exit_code": exit_code(self.worker_exit_code), "cleanup": cleanup, "cleanup_exit_codes": {k: exit_code(v) for k,v in self.cleanup_exit_codes.items()}, "worker_result": self.worker_result, "routing_updated": False, "proxy_learning_tested": False, "tun_tested": False}


def strict_json(raw):
    def pairs(items):
        out = {}
        for key, value in items:
            require(key not in out, "duplicate_key")
            out[key] = value
        return out
    def invalid(_):
        raise Refused("nonfinite_json")
    require(isinstance(raw, str) and len(raw) <= 16384, "input_size")
    try:
        return json.loads(raw, object_pairs_hook=pairs, parse_constant=invalid)
    except (ValueError, RecursionError):
        raise Refused("invalid_json") from None


def node_input(raw):
    n = strict_json(raw)
    require(type(n) is dict, "type_mismatch", expected="object", actual=TYPES.get(type(n), "other"))
    require(FIELDS <= set(n), "missing_fields", FIELDS - set(n))
    # Unknown keys can themselves contain secrets. Never echo their names.
    require(set(n) <= FIELDS, "unknown_fields")
    for field in sorted(FIELDS):
        expected = int if field == "port" else str
        require(type(n[field]) is expected, "type_mismatch", [field], TYPES[expected], TYPES.get(type(n[field]), "other"))
    require(1 <= n["port"] <= 65535, "invalid_value", ["port"])
    try:
        ip = ipaddress.ip_address(n["server"])
    except ValueError:
        raise Refused("invalid_value", ["server"]) from None
    require(ip.is_global and not ip.is_multicast and not ip.is_reserved and str(ip) == n["server"], "invalid_value", ["server"])
    try:
        canonical = str(uuid.UUID(n["uuid"]))
    except ValueError:
        raise Refused("invalid_value", ["uuid"]) from None
    require(canonical == n["uuid"], "invalid_value", ["uuid"])
    host = n["servername"]
    require(len(host) <= 253 and host == host.lower() and "." in host and all(re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?", x) for x in host.split(".")) and not host.endswith((".local", ".localhost", ".internal", ".invalid", ".test")), "invalid_value", ["servername"])
    try:
        ipaddress.ip_address(host)
    except ValueError:
        pass
    else:
        raise Refused("invalid_value", ["servername"])
    pub = n["reality_public_key"]
    require(re.fullmatch(r"[A-Za-z0-9_-]{43}", pub) is not None, "invalid_value", ["reality_public_key"])
    decoded = base64.urlsafe_b64decode(pub + "=")
    require(len(decoded) == 32 and base64.urlsafe_b64encode(decoded).decode().rstrip("=") == pub, "invalid_value", ["reality_public_key"])
    require(re.fullmatch(r"(?:[0-9a-f]{2}){0,8}", n["short_id"]) is not None, "invalid_value", ["short_id"])
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
    require(type(r) is dict and set(r) == {"hosts", "model_attempts", "routing_updated", "tun_tested", "stage", "reason", "api_requests", "route_inventory"})
    require(r["stage"] in {"worker_inputs", "core_identity", "route_inventory", "physical_guard", "controller_contract", "model_setup", "normalize", "probe", "dns", "vless_tls", "path_guard", "model", "complete"})
    require(r["reason"] in {"none", "guard_rejected"})
    inventory = r["route_inventory"]
    if inventory is not None:
        require(type(inventory) is dict and set(inventory) == {"reason", "elapsed_ms", "budget_ms", "query_exit_code", "stderr_present"})
        require(inventory["reason"] in {"none", "unexpected_query_error", "query_start_failed", "query_timeout", "query_canceled", "query_io_failed", "query_output_limit", "query_exit_failed", "route_shape_rejected", "index_not_integer", "index_not_positive"})
        require(type(inventory["elapsed_ms"]) is int and 0 <= inventory["elapsed_ms"] <= 240000)
        require(type(inventory["budget_ms"]) is int and inventory["budget_ms"] == 15000)
        require(inventory["query_exit_code"] is None or exit_code(inventory["query_exit_code"]) is not None)
        require(inventory["stderr_present"] is None or type(inventory["stderr_present"]) is bool)
    require(r["routing_updated"] is False and r["tun_tested"] is False)
    require(type(r["model_attempts"]) is int and 0 <= r["model_attempts"] <= 2)
    require(type(r["hosts"]) is list and len(r["hosts"]) <= 2)
    require(type(r["api_requests"]) is list and len(r["api_requests"]) == r["model_attempts"])
    for i, request in enumerate(r["api_requests"]):
        require(type(request) is dict and set(request) == {"attempt", "outcome", "http_status", "request_id_presence"})
        require(type(request["attempt"]) is int and request["attempt"] == i + 1)
        require(request["outcome"] in {"unknown", "transport_failed", "http_response"})
        require(type(request["http_status"]) is int and (request["http_status"] == 0 or 100 <= request["http_status"] <= 599))
        require(request["request_id_presence"] in {"unknown", "present", "absent"})
    for i, row in enumerate(r["hosts"]):
        require(type(row) is dict and set(row) == {"index", "probe", "vless_tls", "model", "choice", "accepted", "stage", "reason"})
        require(row["stage"] in {"normalize", "probe", "dns", "vless_tls", "path_guard", "model", "complete"})
        require(row["reason"] in {"none", "guard_rejected", "dns_unavailable", "transport_unverified", "insufficient_evidence", "model_unavailable", "model_transport_failed", "model_response_invalid", "model_http_error", "canceled", "answer_invalid"})
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
    if process is None:
        return None
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)
    code = process.poll()
    require(type(code) is int, "cleanup_failed")
    return code



def prepare(root, env, report):
    report.stage = "prepare_inputs"
    require(not env.get("OPENROUTER_API_KEY") and not env.get("VLESS_NODE_JSON"))
    report.stage = "root"
    root.mkdir(mode=0o700)
    # Official immutable version + two SHA256 checks; HTTPS verification remains
    # enabled through redirects. No archive extraction paths are trusted.
    report.stage = "download"
    with urllib.request.urlopen(CORE_URL, timeout=60) as response:
        require(response.url.startswith("https://"))
        wire = response.read(64 * 1024 * 1024 + 1)
    report.stage = "archive_hash"
    require(len(wire) <= 64 * 1024 * 1024 and hashlib.sha256(wire).hexdigest() == ARCHIVE_HASH)
    report.stage = "archive_unpack"
    with zipfile.ZipFile(io.BytesIO(wire)) as archive:
        members = [n for n in archive.infolist() if n.filename.endswith(".exe")]
        require(len(members) == 1 and members[0].file_size <= 128 * 1024 * 1024)
        binary = archive.read(members[0])
    report.stage = "core_hash"
    require(hashlib.sha256(binary).hexdigest() == CORE_HASH)
    (root / "mihomo.exe").write_bytes(binary)
    report.stage = "build"
    subprocess.run(["go", "build", "-trimpath", "-o", str(root / "worker.exe"), "./cmd/real-acceptance"], check=True, timeout=180, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    report.stage = "worker_hash"
    (root / "worker.sha256").write_text(hashlib.sha256((root / "worker.exe").read_bytes()).hexdigest(), encoding="ascii")


def execute(root, env, report):
    private = core = worker = None
    check_ports = False
    try:
        report.stage = "platform"
        require(sys.platform == "win32")
        report.stage = "core_hash"
        require(hashlib.sha256((root / "mihomo.exe").read_bytes()).hexdigest() == CORE_HASH, "hash_mismatch")
        report.stage = "worker_hash"
        require(hashlib.sha256((root / "worker.exe").read_bytes()).hexdigest() == (root / "worker.sha256").read_text(encoding="ascii"), "hash_mismatch")
        report.stage = "once"
        try:
            with (root / ("attempt-" + env["GITHUB_RUN_ID"])).open("x", encoding="ascii") as f:
                f.write("consumed")
        except FileExistsError:
            raise Refused("already_consumed") from None
        report.stage = "deadline"
        require(0 <= time.time() - float(env["JOB_STARTED_AT"]) <= 600, "deadline_expired")
        report.stage = "node_schema"
        node = node_input(env.get("VLESS_NODE_JSON", ""))
        report.stage = "api_key"
        key = env.get("OPENROUTER_API_KEY", "")
        require(0 < len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key), "invalid_value")
        report.stage = "ports"
        ports_free()
        check_ports = True
        report.stage = "private_directory"
        private = Path(tempfile.mkdtemp(prefix="private-", dir=root))
        report.stage = "config_write"
        secret = secrets.token_urlsafe(32)
        config = private / "config.json"
        config.write_text(json.dumps(core_config(node, secret)), encoding="utf-8")
        report.stage = "core_spawn"
        report.cleanup["core"] = "unknown"
        core = subprocess.Popen([str(root / "mihomo.exe"), "-d", str(private), "-f", str(config)], env=child_env(env), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        report.stage = "core_ready"
        ready = False
        for _ in range(100):
            report.core_start_exit_code = core.poll()
            require(report.core_start_exit_code is None, "core_exited")
            try:
                with socket.create_connection(("127.0.0.1", 19097), timeout=.2):
                    ready = True
                    break
            except OSError:
                time.sleep(.1)
        require(ready, "core_not_ready")
        worker_env = child_env(env, model=True)
        worker_env.update(REAL_ACCEPTANCE_WORKER="1", REAL_CORE_PID=str(core.pid), REAL_CONTROLLER_SECRET=secret)
        report.stage = "worker_spawn"
        report.cleanup["worker"] = "unknown"
        report.model_attempts = None
        report.attempts_state = "unknown"
        worker = subprocess.Popen([str(root / "worker.exe")], env=worker_env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        report.stage = "worker_wait"
        stdout, _ = worker.communicate(timeout=270)
        report.worker_exit_code = worker.returncode
        report.stage = "worker_output"
        try:
            result = output_contract(stdout.decode("utf-8"))
        except Exception:
            raise Refused("invalid_worker_output") from None
        report.worker_result = result
        report.model_attempts = result["model_attempts"]
        report.attempts_state = "complete"
        require(worker.returncode == 0 and result["reason"] == "none", "worker_failed")
        report.stage = "complete"
        report.status = "completed" if len(result["hosts"]) == 2 and all(h["vless_tls"] == "verified_success" and h["model"] == "answered" for h in result["hosts"]) else "partial"
    except (Exception, KeyboardInterrupt) as error:
        report.failure(error)
    finally:
        # Each action runs independently, preserving the original failure.
        for name, process in (("worker", worker), ("core", core)):
            if process is not None:
                report.cleanup[name] = "unknown"
                try:
                    report.cleanup_exit_codes[name] = stop_owned(process)
                    report.cleanup[name] = "verified"
                except (Exception, KeyboardInterrupt):
                    report.cleanup[name] = "failed"
        if private is not None:
            report.cleanup["private_files"] = "unknown"
            try:
                require(private.parent.resolve() == root.resolve() and private.name.startswith("private-"))
                shutil.rmtree(private)
                require(not private.exists())
                report.cleanup["private_files"] = "verified"
            except (Exception, KeyboardInterrupt):
                report.cleanup["private_files"] = "failed"
        if check_ports:
            report.cleanup["ports"] = "unknown"
            try:
                ports_free()
                report.cleanup["ports"] = "verified"
            except (Exception, KeyboardInterrupt):
                report.cleanup["ports"] = "failed"
        if any(v in {"failed", "unknown"} for v in report.cleanup.values()) and report.status != "failed":
            report.stage, report.reason, report.status = "cleanup", "cleanup_failed", "failed"


def main(argv=None):
    report = Report()
    try:
        args = sys.argv[1:] if argv is None else argv
        require(len(args) == 1 and args[0] in {"prepare", "execute"}, "invalid_mode")
        env = dict(os.environ)
        report.stage = "source"
        head = subprocess.check_output(["git", "rev-parse", "HEAD"], stderr=subprocess.DEVNULL, timeout=10).decode("ascii").strip()
        report.stage = "budget"
        budget(env)
        report.stage = "gate"
        gate(env, head)
        report.stage = "root"
        root = Path(env["RUNNER_TEMP"]).resolve() / ("real-acceptance-" + env["GITHUB_RUN_ID"])
        if args[0] == "prepare":
            prepare(root, env, report)
            report.stage, report.status = "prepared", "prepared"
        else:
            execute(root, env, report)
    except (Exception, KeyboardInterrupt) as error:
        report.failure(error)
    finally:
        # Hard process/VM termination cannot run finally: absence of this record
        # means unknown, never a successful cleanup or zero model attempts.
        print(json.dumps(report.payload(), separators=(",", ":")), flush=True)
    return 1 if report.status == "failed" else 0


if __name__ == "__main__":
    sys.exit(main())
