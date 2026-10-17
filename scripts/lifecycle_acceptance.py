"""Explicit guest-only synthetic continuous lifecycle acceptance; default is inert."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import threading
import time
import urllib.request

LABEL = "accept-lifecycle-on-hosted-windows"


def plan():
    return {"status": "not_run", "approval_label": LABEL, "timeout_minutes": 20,
            "environment": "one disposable standard windows-2025 guest",
            "scope": "official standalone Mihomo, synthetic learning and process supervision",
            "app_tested": False, "tun_tested": False, "physical_path_tested": False,
            "external_target_requests": 0, "model_requests": 0,
            "checks": ["first fallback then learned TCP 443", "original priority and other port",
                       "maintenance clear and unchanged resume", "supervisor death drains child",
                       "publisher death triggers independent empty recovery", "owned cleanup"],
            "test_only": "separate Go test binaries inject synthetic evidence/path; product executable has no bypass",
            "host_changes": False, "system_tasks": False, "services": False}


def require_permission(allowed, environment=None, platform=None):
    env = os.environ if environment is None else environment
    platform = os.name if platform is None else platform
    required = {"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted",
                "RUNNER_OS": "Windows", "GITHUB_REPOSITORY": "Sonderjyf/mihomo-route-agent"}
    if not allowed or platform != "nt" or any(env.get(k) != v for k, v in required.items()) or not env.get("GITHUB_RUN_ID", "").isdigit():
        raise RuntimeError("explicit isolated lifecycle permission and owned hosted Windows guest required")


def wait_for(read, predicate, seconds=40):
    end = time.monotonic() + seconds
    last = None
    while time.monotonic() < end:
        try:
            last = read()
            if predicate(last):
                return last
        except (OSError, ValueError, KeyError):
            pass
        time.sleep(.2)
    raise RuntimeError(f"bounded readback failed: {last!r}")


def process_info(pid):
    script = f"$p=Get-Process -Id {int(pid)} -ErrorAction SilentlyContinue; if($p){{@{{pid=$p.Id;path=$p.Path}}|ConvertTo-Json -Compress}}else{{'null'}}"
    return json.loads(subprocess.check_output(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script], text=True, timeout=10))


def stop_owned_pid(pid, executable):
    info = process_info(pid)
    if info is None:
        return
    if Path(info["path"]).resolve() != Path(executable).resolve():
        raise RuntimeError("PID no longer belongs to our executable")
    subprocess.run(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
                    f"Stop-Process -Id {int(pid)} -ErrorAction Stop"], check=True, timeout=10)
    wait_for(lambda: process_info(pid), lambda v: v is None, 15)


def cleanup_attempt(errors, action):
    try:
        return action()
    except Exception as error:
        errors.append(str(error))
        return None


def execute(options):
    require_permission(options.allow_isolated_lifecycle)
    # Optional smoke dependency is imported only after the inert/permission gate.
    from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
    from smoke_go import TCPServer, UDPServer, DNSHandler, UDPHandler, EchoHandler, launch, stop, ensure_ports_available
    from smoke_observe import FixtureSOCKS, connect_fake, HOSTS, EXIT_HOST, PORTS
    from tun_acceptance import guest_snapshot, require_clean_guest

    head = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    if not options.expected_head or head != options.expected_head:
        raise RuntimeError("checkout SHA does not equal explicitly approved SHA")
    before = guest_snapshot()
    require_clean_guest(before)
    ensure_ports_available()
    folder = Path(options.workdir).resolve()
    folder.mkdir(parents=True, exist_ok=False)
    agent, core, worker, supervisor = [Path(x).resolve() for x in (options.agent, options.mihomo, options.worker, options.supervisor)]
    environment = os.environ.copy()
    environment.pop("OPENROUTER_API_KEY", None)
    secret = secrets.token_hex(24)
    environment.update(MIHOMO_SECRET=secret, ROUTE_AGENT_GUEST_ACCEPTANCE="1", ROUTE_AGENT_GUEST_WORKER=str(worker))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def api(path):
        request = urllib.request.Request("http://127.0.0.1:19091"+path, headers={"Authorization": "Bearer "+secret})
        with opener.open(request, timeout=4) as response:
            return json.load(response)

    def status():
        with opener.open("http://127.0.0.1:18765/status", timeout=4) as response:
            return json.load(response)

    def empty():
        providers = api("/providers/rules")["providers"]
        return all(providers["route-agent-tail-"+name]["ruleCount"] == 0 for name in ("direct", "proxy"))

    class EmptyProvider(BaseHTTPRequestHandler):
        def do_GET(self):
            body = b"payload: []\n"
            self.send_response(200); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
        def log_message(self, *_):
            pass

    processes, servers, connections, worker_pids, leases = [], [], [], [], []
    result = {**plan(), "tested_sha": head, "passed": False, "status": "running",
              "binaries": {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in (agent, core, worker, supervisor)}}
    bootstrap = None
    try:
        for server in (UDPServer(("127.0.0.1", 15356), UDPHandler), TCPServer(("127.0.0.1", 15356), DNSHandler),
                       TCPServer(("127.0.0.1", 18081), EchoHandler), TCPServer(("127.0.0.1", 17892), FixtureSOCKS)):
            servers.append(server); threading.Thread(target=server.serve_forever, daemon=True).start()
        source = {"mode": "rule", "mixed-port": 17891, "external-controller": "127.0.0.1:19091", "secret": secret,
                  "allow-lan": False, "bind-address": "127.0.0.1", "log-level": "error", "tun": {"enable": False},
                  "dns": {"enable": True, "listen": "127.0.0.1:15354", "enhanced-mode": "fake-ip", "fake-ip-range": "198.19.0.1/16",
                          "use-hosts": False, "use-system-hosts": False, "nameserver": ["udp://127.0.0.1:15356"], "default-nameserver": ["127.0.0.1:15356"]},
                  "proxies": [{"name": n, "type": "socks5", "server": "127.0.0.1", "port": 17892} for n in ("lab-base", "lab-learned")],
                  "proxy-groups": [{"name": "BASE", "type": "select", "proxies": ["lab-base"]}, {"name": "LEARNED", "type": "select", "proxies": ["lab-learned"]}],
                  "rules": ["DOMAIN,known.route-lab.test,BASE", "MATCH,BASE"]}
        profile = folder/"source.json"; profile.write_text(json.dumps(source), encoding="utf-8")
        candidate = json.loads(subprocess.check_output([str(agent), "tail-preview", "--profile", str(profile), "--proxy-target", "LEARNED"]))["candidate"]
        for name, provider in candidate["rule-providers"].items():
            provider.update(type="http", url=f"http://127.0.0.1:18765/rules/{name}.yaml", interval=3600, proxy="DIRECT")
        home = folder/"core"; home.mkdir()
        effective = home/"config.json"; effective.write_text(json.dumps(candidate), encoding="utf-8")
        bootstrap = ThreadingHTTPServer(("127.0.0.1", 18765), EmptyProvider)
        threading.Thread(target=bootstrap.serve_forever, daemon=True).start()
        core_process = launch([str(core), "-d", str(home), "-f", str(effective)], folder/"core.log", environment); processes.append(core_process)
        wait_for(lambda: api("/version"), lambda v: bool(v))
        wait_for(empty, bool)
        assert api("/configs")["tun"]["enable"] is False
        bootstrap.shutdown(); bootstrap.server_close(); bootstrap = None

        # Fixture time is computed only in the approved disposable guest, never
        # installed as the user's maintenance time or a system schedule.
        start = (datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=3)).replace(second=0,microsecond=0)
        config = {"mode": "async", "judge": "stub", "controller": "http://127.0.0.1:19091", "http_listen": "127.0.0.1:18765",
                  "proxy_group": "LEARNED", "max_api_requests": 8, "preflight_ms": 10000, "dns_deadline_ms": 11000,
                  "observation": {"dns": "127.0.0.1:15356", "proxy": "http://127.0.0.1:17892", "hosts": HOSTS+[EXIT_HOST], "attempt_timeout_ms": 1000},
                  "maintenance": {"start": start.strftime("%H:%M"), "timezone": "UTC", "duration_seconds": 12, "resume": "unchanged"}}
        config_path = folder/"agent.json"; config_path.write_text(json.dumps(config), encoding="utf-8")

        def start_supervisor(phase):
            ownership, state = folder/f"{phase}-ownership.json", folder/f"{phase}-state.json"
            leases.append(ownership)
            subprocess.run([str(agent), "prepare-apply", "--config", str(config_path), "--allow-lab-fixtures", "--exclusive-controller",
                            "--profile", str(effective), "--output", str(ownership)], env=environment, check=True, timeout=20)
            environment.update(ROUTE_AGENT_GUEST_CONFIG=str(config_path), ROUTE_AGENT_GUEST_OWNERSHIP=str(ownership), ROUTE_AGENT_GUEST_STATE=str(state))
            parent = launch([str(supervisor), "-test.run=^TestGuestLifecycleSupervisor$", "-test.timeout=10m"], folder/f"{phase}-supervisor.log", environment)
            processes.append(parent)
            wait_for(status, lambda v: v["observer_ready"], 50)
            record = json.loads(Path(str(ownership)+".lock").read_text(encoding="utf-8"))
            info = process_info(record["pid"])
            assert info and Path(info["path"]).resolve() == worker
            worker_pids.append(record["pid"])
            return parent, record["pid"], ownership, state

        parent, pid, ownership, state = start_supervisor("window")
        while datetime.datetime.now(datetime.timezone.utc) < start-datetime.timedelta(seconds=35):
            if parent.poll() is not None: raise RuntimeError("supervisor exited before fixture window")
            time.sleep(.5)
        if datetime.datetime.now(datetime.timezone.utc) >= start-datetime.timedelta(seconds=10):
            raise RuntimeError("missed fixture preparation deadline; do not reschedule silently")
        connections.append(connect_fake(HOSTS[0]))
        first = api("/connections")["connections"]
        assert any(c["metadata"].get("host") == HOSTS[0] and c["rule"] == "Match" for c in first)
        wait_for(status, lambda v: v["commits"] >= 1)
        connections.append(connect_fake(HOSTS[0])); connections.append(connect_fake(HOSTS[1])); connections.append(connect_fake(HOSTS[0],8443))
        records = api("/connections")["connections"]
        assert any(c["metadata"].get("host")==HOSTS[0] and c["rule"]=="AND" for c in records)
        assert any(c["metadata"].get("host")==HOSTS[1] and c["rule"]=="Domain" for c in records)
        assert any(c["metadata"].get("destinationPort")=="8443" and c["rule"]=="Match" for c in records)
        result["synthetic_learning_and_scope"] = True
        wait_for(status, lambda v: v["maintenance"]=="paused_empty", 60)
        assert empty()
        result["maintenance_verified_empty"] = True
        wait_for(status, lambda v: v["maintenance"]=="scheduled" and v["observer_ready"], 40)
        lease = json.loads(ownership.read_text(encoding="utf-8"))
        result["short_lease_expires_at"] = lease["expires_at"]
        result["unchanged_resume"] = True
        # Parent death closes the heartbeat pipe. The child must stop and clear.
        stop_owned_pid(parent.pid, supervisor)
        wait_for(lambda: process_info(pid), lambda v: v is None, 30)
        assert empty() and json.loads(state.read_text(encoding="utf-8"))["phase"]=="stopped"
        result["supervisor_death_drains_child"] = True

        parent, pid, ownership, state = start_supervisor("crash")
        connections.append(connect_fake(EXIT_HOST))
        wait_for(status, lambda v: v["learned_count"] > 0)
        stop_owned_pid(pid, worker)
        parent.wait(timeout=55)
        assert parent.returncode != 0  # a crash never becomes normal automatic restart
        assert empty() and json.loads(state.read_text(encoding="utf-8"))["phase"]=="recovered_stopped"
        result["independent_crash_recovery"] = True
        result["passed"] = True
    finally:
        cleanup_errors = []
        for connection in connections: cleanup_attempt(cleanup_errors, connection.close)
        for ownership in leases:
            lock=Path(str(ownership)+".lock")
            if lock.exists():
                record=cleanup_attempt(cleanup_errors, lambda: json.loads(lock.read_text(encoding="utf-8")))
                if record and record["pid"] not in worker_pids: worker_pids.append(record["pid"])
        for process in reversed(processes):
            if process.poll() is None: cleanup_attempt(cleanup_errors, lambda: stop_owned_pid(process.pid, supervisor if process is not core_process else core))
            if not process.lab_log.closed: cleanup_attempt(cleanup_errors, process.lab_log.close)
        for pid in worker_pids: cleanup_attempt(cleanup_errors, lambda: stop_owned_pid(pid, worker))
        if bootstrap is not None:
            cleanup_attempt(cleanup_errors, bootstrap.shutdown); cleanup_attempt(cleanup_errors, bootstrap.server_close)
        for server in servers:
            cleanup_attempt(cleanup_errors, server.shutdown); cleanup_attempt(cleanup_errors, server.server_close)
        result["all_recorded_pids_absent"] = cleanup_attempt(cleanup_errors, lambda: all(process_info(p.pid) is None for p in processes) and all(process_info(pid) is None for pid in worker_pids)) is True
        result["guest_network_restored"] = cleanup_attempt(cleanup_errors, guest_snapshot)==before
        port_list=",".join(str(p) for p in sorted(PORTS))
        script=f"$ports=@({port_list}); @(@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue|Where-Object {{$_.LocalPort -in $ports}}).Count,@(Get-NetUDPEndpoint -ErrorAction SilentlyContinue|Where-Object {{$_.LocalPort -in $ports}}).Count)|ConvertTo-Json -Compress"
        result["remaining_fixture_endpoints"] = cleanup_attempt(cleanup_errors, lambda: json.loads(subprocess.check_output(["powershell.exe","-NoProfile","-NonInteractive","-Command",script],text=True,timeout=15)))
        result["cleanup_errors"] = cleanup_errors
        result["passed"] = result["passed"] and not cleanup_errors and result["all_recorded_pids_absent"] and result["guest_network_restored"] and result["remaining_fixture_endpoints"]==[0,0]
        result["status"] = "passed" if result["passed"] else "failed"
        (folder/"result.json").write_text(json.dumps(result,indent=2),encoding="utf-8")
        print(json.dumps(result,indent=2))
    if not result["passed"]: raise RuntimeError("lifecycle acceptance failed")


if __name__ == "__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute",action="store_true")
    parser.add_argument("--allow-isolated-lifecycle",action="store_true")
    for name in ("agent","mihomo","worker","supervisor","workdir","expected-head"): parser.add_argument("--"+name)
    options=parser.parse_args()
    if not options.execute: print(json.dumps(plan(),indent=2))
    elif not all(getattr(options,n.replace("-","_")) for n in ("agent","mihomo","worker","supervisor","workdir","expected-head")): parser.error("explicit binaries, workdir and approved SHA required")
    else: execute(options)
