"""Single-target product learning acceptance. Default: inert plan, no I/O.

prepare/execute require an approved main launcher on a hosted Windows runner.
This is not the nonpublishing real-acceptance worker. Never run on Sera.
"""
import hashlib
import ipaddress
from decimal import Decimal
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import urllib.request

import real_acceptance as a

HOST = "example.com"
PORTS = (17897, 18797, 19097)
PROVIDERS = ("route-agent-tail-direct", "route-agent-tail-proxy")
POLICY = set("not_evaluated accepted answer_type_invalid choice_invalid probability_count_invalid probability_value_invalid probability_sum_invalid evidence_insufficient choice_evidence_mismatch probability_below_threshold".split())
STAGES = set("normalize collection dns path direct_tls proxy_tls model policy ownership publication".split())
ERRORS = set("none input_invalid collection_failed deadline_exceeded canceled model_transport_failed model_response_invalid model_http_error model_unavailable ownership_failed publication_failed evaluation_failed".split())
TIMES = set("deadline_ms elapsed_ms dns_ms path_ms direct_tls_ms proxy_tls_ms collection_ms model_ms publication_ms job_deadline_ms job_elapsed_ms".split())
DIAGNOSTIC = TIMES | set("version stage stage_error policy_reason choice decision evidence dns model_calls transport_attempts transport_attempts_known publication connection_target_match".split())


def plan():
    return dict(schema_version=1, mode="inert_plan", execution_enabled=False,
                environment="github-hosted/windows-2025", product="cmd/route-agent",
                target_count=1, max_model_transport_attempts=1, tun=False,
                preflight_ms=10000, dns_deadline_ms=11000, attempt_timeout_ms=1000,
                observer_seconds=70, hosted_job_minutes=20,
                requires_new_run_approval=True, secrets_read=False,
                controller_writes=False, sera_network_changes=False)


def diagnostic(value):
    # Fail closed, never echo unknown keys/values or raw exception text.
    a.require(type(value) is dict and set(value) == DIAGNOSTIC)
    a.require(type(value["version"]) is int and value["version"] == 1)
    a.require(value["stage"] in STAGES and value["stage_error"] in ERRORS and value["policy_reason"] in POLICY)
    a.require(value["choice"] in {"DIRECT", "PROXY", "UNCERTAIN"} and value["decision"] in {"DIRECT", "PROXY", "UNCERTAIN"})
    a.require(type(value["evidence"]) is dict and set(value["evidence"]) == {"direct_tls", "proxy_tls"})
    a.require(all(v in a.PROBE for v in value["evidence"].values()))
    a.require(value["dns"] in {"not_tested", "resolved", "unavailable", "protected_or_fake_ip"})
    a.require(all(type(value[k]) is int and 0 <= value[k] <= 600000 for k in TIMES))
    a.require(type(value["model_calls"]) is int and 0 <= value["model_calls"] <= 1)
    a.require(type(value["transport_attempts"]) is int and 0 <= value["transport_attempts"] <= 1)
    a.require(type(value["transport_attempts_known"]) is bool and type(value["connection_target_match"]) is bool)
    a.require(value["publication"] in {"not_attempted", "committed", "failed"})
    return json.loads(json.dumps(value))


def gate(env, head):
    a.require(sys.platform == "win32" and env.get("RUNNER_OS") == "Windows")
    a.require(env.get("EXECUTE_LEARNING_ACCEPTANCE") == "true")
    a.gate(env, head, workflow="learning-acceptance")
    a.require(env.get("REMAINING_BUDGET_CONFIRMED") == "true" and re.fullmatch(r"(?:0|[1-9][0-9]{0,3})(?:\.[0-9]{1,4})?", env.get("REMAINING_USD", "")) is not None)
    a.require(0 < Decimal(env["REMAINING_USD"]) <= Decimal(env["PROVIDER_CAP_USD"]))
    a.require(re.fullmatch(r"[1-9][0-9]{0,8}", env.get("DIRECT_INTERFACE_INDEX", "")) is not None)


def ports_free():
    for port in PORTS:
        for kind in (socket.SOCK_STREAM, socket.SOCK_DGRAM):
            with socket.socket(socket.AF_INET, kind) as sock:
                if hasattr(socket, "SO_EXCLUSIVEADDRUSE"):
                    sock.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
                sock.bind(("127.0.0.1", port))


def template(node, secret, target_ip=None):
    core = a.core_config(node, secret)
    if target_ip is not None:
        address = ipaddress.ip_address(target_ip)
        a.require(address.version == 4 and address.is_global and not address.is_multicast and not address.is_reserved and str(address) == target_ip)
        core["hosts"] = {HOST: target_ip}
    core["rule-providers"] = {
        name: dict(type="http", behavior="classical", format="yaml",
                   url=f"http://127.0.0.1:18797/rules/{name}.yaml",
                   path=f"./route-agent/{name}.yaml", interval=3600, proxy="DIRECT")
        for name in PROVIDERS}
    core["rules"] = [
        "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-direct)),DIRECT",
        "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-proxy)),acceptance-vless",
        "MATCH,acceptance-vless"]
    agent = dict(mode="async", judge="jev", controller="http://127.0.0.1:19097",
                 http_listen="127.0.0.1:18797", proxy_group="acceptance-vless",
                 api_proxy="http://127.0.0.1:17897", preflight_ms=10000,
                 dns_deadline_ms=11000, max_api_requests=1, capacity=1,
                 learned_ttl_seconds=60, observation=dict(dns="1.1.1.1:53",
                 proxy="http://127.0.0.1:17897", hosts=[HOST], attempt_timeout_ms=1000))
    return core, agent


def pin_target(env):
    # Real same-run DNS only, no caller-supplied address or synthetic override.
    # Numeric mapping keeps hostname metadata while Mihomo sends the pinned IP
    # through VLESS. Collector independently re-resolves and must match it.
    script = "$ErrorActionPreference='Stop'; $ip=@(Resolve-DnsName -Name 'example.com' -Server '1.1.1.1' -Type A -DnsOnly | Where-Object {$_.Type -eq 'A'} | Select-Object -ExpandProperty IPAddress); if($ip.Count -lt 1){exit 7}; $ip[0] | ConvertTo-Json -Compress"
    raw = subprocess.check_output(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script], env=a.child_env(env), stderr=subprocess.DEVNULL, timeout=15)
    ip = a.strict_json(raw.decode("utf-8-sig"))
    a.require(type(ip) is str)
    address = ipaddress.ip_address(ip)
    a.require(address.version == 4 and address.is_global and not address.is_multicast and not address.is_reserved and str(address) == ip)
    return ip


class EmptyProviders(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in {f"/rules/{n}.yaml" for n in PROVIDERS}:
            self.send_error(404); return
        body = b"payload: []\n"
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *args):
        pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise a.Refused()


def api(path, secret="", method="GET", token=None, agent=False):
    port = 18797 if agent else 19097
    data = json.dumps({"token": token}).encode() if token else None
    req = urllib.request.Request(f"http://127.0.0.1:{port}" + path, data=data, method=method)
    if secret: req.add_header("Authorization", "Bearer " + secret)
    if data: req.add_header("Content-Type", "application/json")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(req, timeout=2) as response:
        raw = response.read(16385)
    return a.strict_json(raw.decode("utf-8"))


def wait_for(get, accept, process, seconds=25):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        a.require(process.poll() is None)
        try:
            value = get()
            if accept(value): return value
        except (OSError, ValueError):
            pass
        time.sleep(.1)
    raise a.Refused("timeout")


def tls_connection():
    conn = socket.create_connection(("127.0.0.1", 17897), timeout=5)
    try:
        # CONNECT supplies a hostname for the ordinary core connection snapshot;
        # TLS verifies that hostname. No destination HTTP request is sent.
        conn.sendall(f"CONNECT {HOST}:443 HTTP/1.1\r\nHost: {HOST}:443\r\n\r\n".encode("ascii"))
        response = bytearray()
        while not response.endswith(b"\r\n\r\n"):
            part = conn.recv(1)
            a.require(part and len(response) < 8192)
            response.extend(part)
        a.require(bytes(response).split(b"\r\n", 1)[0].split(b" ")[1] == b"200")
        tls = ssl.create_default_context().wrap_socket(conn, server_hostname=HOST)
        a.require(tls.getpeercert())
        return tls
    except BaseException:
        conn.close(); raise


def connection_record(snapshot, conn):
    source_port = str(conn.getsockname()[1])
    records = [c for c in snapshot["connections"] if c["metadata"].get("sourcePort") == source_port
               and c["metadata"].get("host") == HOST and c["metadata"].get("network") == "tcp"
               and c["metadata"].get("destinationPort") == "443"]
    a.require(len(records) == 1 and records[0].get("id"))
    return records[0]


def baseline(record):
    return record.get("rule") == "Match" and record.get("rulePayload", "") == "" and record.get("chains", [])[-1:] == ["acceptance-vless"]


def empty(secret):
    providers = api("/providers/rules", secret)["providers"]
    return all(providers.get(n, {}).get("ruleCount") == 0 for n in PROVIDERS)


def result():
    return dict(schema_version=1, phase_status="failed", acceptance_status="incomplete",
                stage="gate", reason="rejected", target_index=0, evaluation=None,
                model_transport_attempts=0, model_transport_attempts_known=True,
                baseline_connection_verified=False, nonempty_publication_verified=False,
                new_connection_verified=False, existing_connection_preserved=False,
                cleanup={k: "not_started" for k in ("bootstrap", "agent", "providers", "core", "private_files", "ports")},
                tun_tested=False, sera_network_changes=False)


def final_summary(path):
    with path.open("rb") as stream:
        raw = stream.read(16385)
    a.require(len(raw) <= 16384)
    final = a.strict_json(raw.decode("utf-8"))
    a.require(type(final) is dict and set(final) == {"evaluation", "observer_stopped"}
              and type(final["observer_stopped"]) is bool)
    final["evaluation"] = diagnostic(final["evaluation"])
    return final


def execute(root, env, out):
    private = core = agent = bootstrap = None
    connections = []
    secret = ""
    checked_ports = False
    candidate_pass = False
    normal_stop_verified = False
    observer_output = None
    try:
        out["stage"] = "hashes"
        a.require(hashlib.sha256((root/"mihomo.exe").read_bytes()).hexdigest() == a.CORE_HASH, "hash_mismatch")
        a.require(hashlib.sha256((root/"agent.exe").read_bytes()).hexdigest() == (root/"agent.sha256").read_text(encoding="ascii"), "hash_mismatch")
        out["stage"] = "once"
        with (root/("attempt-" + env["GITHUB_RUN_ID"])).open("x", encoding="ascii") as f: f.write("consumed")
        a.require(0 <= time.time() - float(env["JOB_STARTED_AT"]) <= 600, "deadline_expired")
        node = a.node_input(env.get("VLESS_NODE_JSON", ""))
        key = env.get("OPENROUTER_API_KEY", "")
        a.require(0 < len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key))
        out["stage"] = "ports"
        ports_free(); checked_ports = True
        private = Path(tempfile.mkdtemp(prefix="private-", dir=root))
        out["cleanup"]["private_files"] = "unknown"
        secret = secrets.token_urlsafe(32)
        out["stage"] = "target_dns"
        config, agent_config = template(node, secret, pin_target(env))
        effective, settings = private/"core.json", private/"agent.json"
        # Validate the handwritten HTTP conversion against product tail-preview.
        original = private/"original.json"
        original.write_text(json.dumps(a.core_config(node, secret)), encoding="utf-8")
        clean = a.child_env(env); clean["MIHOMO_SECRET"] = secret
        preview = subprocess.check_output([str(root/"agent.exe"), "tail-preview", "--profile", str(original), "--proxy-target", "acceptance-vless"], env=clean, stderr=subprocess.DEVNULL, timeout=10)
        a.require(json.loads(preview)["candidate"]["rules"] == config["rules"])
        effective.write_text(json.dumps(config), encoding="utf-8")
        settings.write_text(json.dumps(agent_config), encoding="utf-8")
        bootstrap = ThreadingHTTPServer(("127.0.0.1", 18797), EmptyProviders)
        out["cleanup"]["bootstrap"] = "unknown"
        threading.Thread(target=bootstrap.serve_forever, daemon=True).start()
        out["stage"] = "core"
        out["cleanup"]["core"] = "unknown"
        core = subprocess.Popen([str(root/"mihomo.exe"), "-d", str(private), "-f", str(effective)], env=a.child_env(env), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        wait_for(lambda: api("/version", secret), lambda v: bool(v.get("version")), core)
        wait_for(lambda: empty(secret), bool, core)
        a.require(api("/configs", secret)["tun"]["enable"] is False)
        bootstrap.shutdown(); bootstrap.server_close(); bootstrap = None
        out["cleanup"]["bootstrap"] = "verified"
        ownership, journal = private/"ownership.json", private/"state.json"
        out["stage"] = "ownership"
        subprocess.run([str(root/"agent.exe"), "prepare-apply", "--config", str(settings), "--exclusive-controller", "--profile", str(effective), "--output", str(ownership)], env=clean, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=20)
        # Only the real product observer receives the model key. Core and
        # preview/ownership steps never receive it. No continuous mode or retry.
        worker_env = a.child_env(env, model=True); worker_env["MIHOMO_SECRET"] = secret
        out["stage"] = "observer"
        out["model_transport_attempts"], out["model_transport_attempts_known"] = None, False
        out["cleanup"]["agent"] = out["cleanup"]["providers"] = "unknown"
        final_path = private/"observer-result.json"
        observer_output = final_path.open("xb")
        agent = subprocess.Popen([str(root/"agent.exe"), "observe-apply", "--config", str(settings), "--allow-external-probes", "--allow-model-api", "--allow-controlled-apply", "--exclusive-controller", "--ownership", str(ownership), "--state-file", str(journal), "--direct-interface-index", env["DIRECT_INTERFACE_INDEX"], "--run-for", "70s"], env=worker_env, stdin=subprocess.DEVNULL, stdout=observer_output, stderr=subprocess.DEVNULL, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
        wait_for(lambda: api("/status", agent=True), lambda s: s["observer_ready"], agent)
        out["stage"] = "connection_a"
        first = tls_connection(); connections.append(first)
        record_a = connection_record(api("/connections", secret), first)
        a.require(baseline(record_a))
        out["baseline_connection_verified"] = True
        out["stage"] = "evaluation"
        status = wait_for(lambda: api("/status", agent=True), lambda s: s.get("evaluation") is not None and (s["evaluation"]["decision"] != "DIRECT" or s["commits"] == 1), agent, seconds=15)
        row = diagnostic(status["evaluation"]); out["evaluation"] = row
        out["model_transport_attempts"], out["model_transport_attempts_known"] = row["transport_attempts"], row["transport_attempts_known"]
        a.require(status["attempts"] == 1 and status["judge_calls"] == row["model_calls"] and row["transport_attempts_known"])
        if row["decision"] == "UNCERTAIN" and row["stage_error"] == "none":
            a.require(status["commits"] == 0 and empty(secret))
            out["phase_status"], out["acceptance_status"], out["reason"] = "completed", "no_go", "policy_refused"
        else:
            a.require(row["stage_error"] == "none" and row["decision"] == "DIRECT" and row["policy_reason"] == "accepted" and row["publication"] == "committed" and row["connection_target_match"])
            a.require(row["transport_attempts"] == 1 and status["commits"] == 1)
            providers = api("/providers/rules", secret)["providers"]
            a.require(providers[PROVIDERS[0]]["ruleCount"] == 1 and providers[PROVIDERS[1]]["ruleCount"] == 0)
            out["nonempty_publication_verified"] = True
            out["stage"] = "connection_b"
            second = tls_connection(); connections.append(second)
            snapshot = api("/connections", secret)
            record_b = connection_record(snapshot, second)
            a.require(record_b["id"] != record_a["id"] and record_b["metadata"]["destinationIP"] == record_a["metadata"]["destinationIP"])
            a.require(record_b["rule"] == "AND" and record_b["rulePayload"] == "((Network,tcp) && (DstPort,443) && (RuleSet,route-agent-tail-direct))" and record_b["chains"][-1:] == ["DIRECT"])
            out["new_connection_verified"] = True
            surviving = connection_record(snapshot, first)
            a.require(surviving["id"] == record_a["id"] and baseline(surviving))
            out["existing_connection_preserved"] = True
            candidate_pass = True
            out["phase_status"], out["reason"] = "completed", "none"
        out["stage"] = "stop"
        # Normal finite product exit runs empty synchronization while HTTP is
        # still alive. A forced Windows termination is not a normal cleanup.
        a.require(agent.wait(timeout=90) == 0)
        a.require(a.strict_json(journal.read_text(encoding="utf-8"))["phase"] == "stopped" and empty(secret))
        # Runtime status alone cannot prove a normal, fully reported stop.
        final = final_summary(final_path)
        a.require(final["observer_stopped"] and final["evaluation"] == out["evaluation"])
        normal_stop_verified = True
        out["cleanup"]["providers"] = "verified"
    except (Exception, KeyboardInterrupt):
        out["phase_status"], out["acceptance_status"], out["reason"] = "failed", "incomplete", "execution_failed"
    finally:
        for conn in connections:
            try: conn.close()
            except OSError: pass
        if bootstrap is not None:
            try:
                bootstrap.shutdown(); bootstrap.server_close(); out["cleanup"]["bootstrap"] = "verified"
            except (Exception, KeyboardInterrupt): out["cleanup"]["bootstrap"] = "failed"
        if agent is not None:
            try:
                a.stop_owned(agent); out["cleanup"]["agent"] = "verified"
                if out["evaluation"] is None and final_path.exists():
                    try:
                        row = final_summary(final_path)["evaluation"]
                        out["evaluation"] = row
                        out["model_transport_attempts"], out["model_transport_attempts_known"] = row["transport_attempts"], row["transport_attempts_known"]
                        if out["stage"] == "evaluation" and row["stage_error"] != "none" and row["transport_attempts_known"]:
                            out["phase_status"], out["acceptance_status"], out["reason"] = "completed", "no_go", "pipeline_failed"
                    except (Exception, KeyboardInterrupt):
                        pass  # Invalid output stays unknown; recovery still runs.
                if out["cleanup"]["providers"] != "verified":
                    # Recovery is empty-only, requires confirmed exit and the
                    # original unchanged owner. Never delete a lock to bypass it.
                    subprocess.run([str(root/"agent.exe"), "recover-apply", "--config", str(settings), "--allow-owned-recovery", "--exclusive-controller", "--ownership", str(ownership), "--state-file", str(journal)], env=clean, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=30)
                    a.require(empty(secret))
                    out["cleanup"]["providers"] = "verified"
            except (Exception, KeyboardInterrupt):
                if agent.poll() is None: out["cleanup"]["agent"] = "failed"
                out["cleanup"]["providers"] = "failed"
        if observer_output is not None:
            try: observer_output.close()
            except (Exception, KeyboardInterrupt):
                out["phase_status"], out["acceptance_status"], out["reason"] = "failed", "incomplete", "execution_failed"
        if core is not None:
            try: a.stop_owned(core); out["cleanup"]["core"] = "verified"
            except (Exception, KeyboardInterrupt): out["cleanup"]["core"] = "failed"
        if private is not None:
            try:
                a.require(private.parent.resolve() == root.resolve() and private.name.startswith("private-"))
                shutil.rmtree(private); a.require(not private.exists()); out["cleanup"]["private_files"] = "verified"
            except (Exception, KeyboardInterrupt): out["cleanup"]["private_files"] = "failed"
        if checked_ports:
            try: ports_free(); out["cleanup"]["ports"] = "verified"
            except (Exception, KeyboardInterrupt): out["cleanup"]["ports"] = "failed"
        if any(v in {"failed", "unknown"} for v in out["cleanup"].values()):
            out["phase_status"], out["acceptance_status"], out["reason"] = "failed", "incomplete", "cleanup_incomplete"
        elif candidate_pass and normal_stop_verified and out["phase_status"] == "completed" and all(v == "verified" for v in out["cleanup"].values()):
            out["acceptance_status"] = "pass"


def main(argv=None):
    args = sys.argv[1:] if argv is None else argv
    if not args or args == ["plan"]:
        print(json.dumps(plan(), separators=(",", ":"))); return 0
    out = result()
    try:
        a.require(args in (["prepare"], ["execute"]))
        env = dict(os.environ)
        # Check environment before even invoking git or looking at a Secret.
        gate(env, env.get("REVIEWED_SHA", ""))
        head = subprocess.check_output(["git", "rev-parse", "HEAD"], stderr=subprocess.DEVNULL, timeout=10).decode("ascii").strip()
        gate(env, head)
        # A reviewed commit must also be the entire clean checkout. No local
        # patch or untracked replacement may masquerade as that execution SHA.
        a.require(not subprocess.check_output(["git", "status", "--porcelain", "--untracked-files=normal"], stderr=subprocess.DEVNULL, timeout=10).strip())
        root = Path(env["RUNNER_TEMP"]).resolve() / ("learning-acceptance-" + env["GITHUB_RUN_ID"])
        if args == ["prepare"]:
            report = a.Report()
            a.prepare(root, env, report, "./cmd/route-agent", "agent.exe")
            out["stage"], out["phase_status"], out["acceptance_status"], out["reason"] = "prepared", "completed", "not_tested", "none"
        else:
            execute(root, env, out)
    except (Exception, KeyboardInterrupt):
        pass  # Fixed initial failure only; never print exception/env/argv.
    print(json.dumps(out, separators=(",", ":")), flush=True)
    return 0 if out["phase_status"] == "completed" else 1


if __name__ == "__main__":
    sys.exit(main())
