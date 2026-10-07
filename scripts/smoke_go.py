"""Manual Go/Mihomo loopback smoke. Never changes Windows or installed FlClash.

Requires dnspython, an already built Agent, and a separately downloaded Mihomo.
Jev mode sends two synthetic inputs to a paid API; stub mode is fully offline.
"""
import argparse
import concurrent.futures
import ctypes
from ctypes import wintypes
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import socket
import socketserver
import subprocess
import threading
import time
import urllib.request

import dns.message
import dns.query
import dns.rrset

from verify_jev import load_key

HOST = "first.route-lab.test"
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def get_json(url, method="GET", body=None):
    request = urllib.request.Request(url, method=method, data=json.dumps(body).encode() if body is not None else None,
                                     headers={"Content-Type": "application/json"})
    with OPENER.open(request, timeout=2) as response:
        raw = response.read()
        return json.loads(raw) if raw else response.status


def wait_ready(url, field=None):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        try:
            value = get_json(url)
            if field is None or value.get(field):
                return value
        except Exception:
            pass
        time.sleep(.1)
    raise RuntimeError("isolated service did not become ready")


def answer(wire):
    query = dns.message.from_wire(wire)
    response = dns.message.make_response(query)
    question = query.question[0]
    if question.rdtype == 1:
        address = "127.0.0.1" if str(question.name).startswith("private.") else "198.51.100.7"
        response.answer = [dns.rrset.from_text(str(question.name), 1, "IN", "A", address)]
    return response.to_wire()


class UDPHandler(socketserver.BaseRequestHandler):
    def handle(self):
        wire, sender = self.request
        sender.sendto(answer(wire), self.client_address)


class DNSHandler(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.settimeout(2)
        length = int.from_bytes(receive(self.request, 2), "big")
        response = answer(receive(self.request, length))
        self.request.sendall(len(response).to_bytes(2, "big") + response)


def receive(connection, size):
    result = b""
    while len(result) < size:
        chunk = connection.recv(size - len(result))
        if not chunk:
            raise OSError("connection ended early")
        result += chunk
    return result


class EchoHandler(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.settimeout(10)
        try:
            self.request.sendall(self.request.recv(100))
            self.request.recv(1)
        except OSError:
            pass


class UDPServer(socketserver.ThreadingUDPServer):
    daemon_threads = True


class TCPServer(socketserver.ThreadingTCPServer):
    daemon_threads = True


def launch(arguments, log, environment=None):
    stream = log.open("w", encoding="utf-8")
    try:
        process = subprocess.Popen(arguments, stdout=stream, stderr=stream, env=environment,
                                   creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    except Exception:
        stream.close()
        raise
    process.lab_log = stream
    return process


def stop(process):
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
    process.lab_log.close()


def process_metrics(pid):
    if os.name != "nt":
        return None
    class Counters(ctypes.Structure):
        _fields_ = [("cb", wintypes.DWORD), ("faults", wintypes.DWORD),
                    *[(name, ctypes.c_size_t) for name in ["peak_working", "working", "peak_paged", "paged", "peak_nonpaged", "nonpaged", "pagefile", "peak_pagefile"]]]
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    psapi = ctypes.WinDLL("psapi", use_last_error=True)
    kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    kernel.OpenProcess.restype = wintypes.HANDLE
    kernel.CloseHandle.argtypes = [wintypes.HANDLE]
    kernel.GetProcessTimes.argtypes = [wintypes.HANDLE, *([ctypes.POINTER(wintypes.FILETIME)] * 4)]
    psapi.GetProcessMemoryInfo.argtypes = [wintypes.HANDLE, ctypes.POINTER(Counters), wintypes.DWORD]
    handle = kernel.OpenProcess(0x0400 | 0x0010, False, pid)
    if not handle:
        return None
    try:
        counters = Counters()
        counters.cb = ctypes.sizeof(counters)
        if not psapi.GetProcessMemoryInfo(handle, ctypes.byref(counters), counters.cb):
            return None
        times = [wintypes.FILETIME() for _ in range(4)]
        cpu = None
        if kernel.GetProcessTimes(handle, *(ctypes.byref(value) for value in times)):
            cpu = sum((value.dwHighDateTime << 32) + value.dwLowDateTime for value in times[2:]) / 10_000_000
        return {"working_set_bytes": counters.working, "peak_working_set_bytes": counters.peak_working, "cpu_seconds": cpu}
    finally:
        kernel.CloseHandle(handle)


def query(host, qtype="A", transport="udp"):
    start = time.perf_counter()
    response = getattr(dns.query, transport)(dns.message.make_query(host, qtype), "127.0.0.1", port=15354, timeout=4)
    return {"qtype": qtype, "transport": transport, "ms": round((time.perf_counter() - start) * 1000, 3),
            "received_unix_ns": time.time_ns(), "rcode": response.rcode(), "answers": [str(item) for item in response.answer]}


def connect_socks(host=HOST):
    connection = socket.create_connection(("127.0.0.1", 17891), timeout=3)
    connection.sendall(b"\x05\x01\x00")
    assert receive(connection, 2) == b"\x05\x00"
    encoded = host.encode()
    connection.sendall(b"\x05\x01\x00\x03" + bytes([len(encoded)]) + encoded + (18081).to_bytes(2, "big"))
    response = receive(connection, 4)
    assert response[1] == 0
    if response[3] == 1:
        receive(connection, 6)
    elif response[3] == 4:
        receive(connection, 18)
    else:
        receive(connection, receive(connection, 1)[0] + 2)
    connection.sendall(b"go-route-agent-proof")
    assert receive(connection, len(b"go-route-agent-proof")) == b"go-route-agent-proof"
    return connection


def run(options):
    folder = Path(options.workdir).resolve()
    folder.mkdir(parents=True, exist_ok=True)
    agent_exe, core_exe = Path(options.agent).resolve(), Path(options.mihomo).resolve()
    config = {"dns_listen": "127.0.0.1:15355", "http_listen": "127.0.0.1:18765", "upstream": "127.0.0.1:15356",
              "controller": "http://127.0.0.1:19091", "mode": "bounded-preflight", "judge": options.judge,
              "api_proxy": options.proxy, "preflight_ms": 1500, "dns_deadline_ms": 2500, "max_api_requests": 10,
              "rules": [{"type": "DOMAIN", "value": "known.route-lab.test", "route": "DIRECT", "source": "manual"}],
              "lab_fixtures": {HOST: {"direct_tls": "repeated_failure", "proxy_tls": "verified_success"},
                               "deadline.route-lab.test": {"direct_tls": "repeated_failure", "proxy_tls": "verified_success"}}}
    config_path = folder / "agent.json"
    config_path.write_text(json.dumps(config), encoding="utf-8")
    environment = os.environ.copy()
    if options.judge == "jev":
        environment["OPENROUTER_API_KEY"] = load_key(options.key_file)
    fragment = json.loads(subprocess.check_output([str(agent_exe), "render", "--config", str(config_path), "--allow-lab-fixtures"]))
    controller = "http://127.0.0.1:19091"
    status_url = "http://127.0.0.1:18765/status"
    processes, servers = [], []
    memory_samples, sampling_stop = [], threading.Event()
    result = {"date": datetime.now(timezone(timedelta(hours=8))).isoformat(), "kind": "go_isolated_not_live_flclash_or_TUN",
              "backend": options.judge, "synthetic_evidence": True, "connection_identity": "SOCKS5 hostname"}
    try:
        for server in [UDPServer(("127.0.0.1", 15356), UDPHandler), TCPServer(("127.0.0.1", 15356), DNSHandler), TCPServer(("127.0.0.1", 18081), EchoHandler)]:
            servers.append(server)
            threading.Thread(target=server.serve_forever, daemon=True).start()
        def core(name, data):
            home = folder / name
            home.mkdir(exist_ok=True)
            path = home / "config.json"
            path.write_text(json.dumps(data), encoding="utf-8")
            process = launch([str(core_exe), "-d", str(home), "-f", str(path)], home / "core.log")
            processes.append(process)
            return path
        core("hop", {"mixed-port": 17892, "allow-lan": False, "bind-address": "127.0.0.1", "mode": "rule", "log-level": "error",
                     "tun": {"enable": False}, "rules": ["MATCH,DIRECT"], "hosts": {HOST: "127.0.0.1"}})
        agent = launch([str(agent_exe), "serve", "--config", str(config_path), "--allow-lab-fixtures"], folder / "agent.log", environment)
        processes.append(agent)
        wait_ready(status_url)
        main_config = {**fragment, "mixed-port": 17891, "allow-lan": False, "bind-address": "127.0.0.1", "mode": "rule",
                       "external-controller": "127.0.0.1:19091", "log-level": "error", "ipv6": True, "tun": {"enable": False},
                       "proxies": [{"name": "lab-hop", "type": "socks5", "server": "127.0.0.1", "port": 17892}],
                       "proxy-groups": [{"name": "PROXY", "type": "select", "proxies": ["lab-hop"]}]}
        main_config["dns"].update({"listen": "127.0.0.1:15354", "ipv6": True, "use-hosts": False, "use-system-hosts": False})
        main_path = core("main", main_config)
        wait_ready(controller + "/version")
        wait_ready(status_url, "core_ready")
        def sample():
            while not sampling_stop.wait(.02):
                value = process_metrics(agent.pid)
                if value:
                    memory_samples.append(value["working_set_bytes"])
        threading.Thread(target=sample, daemon=True).start()
        before = process_metrics(agent.pid)
        idle_start = time.perf_counter()
        time.sleep(1)
        idle_seconds = time.perf_counter() - idle_start
        after = process_metrics(agent.pid)
        result["agent_sha256"] = hashlib.sha256(agent_exe.read_bytes()).hexdigest()
        result["resource"] = {"executable_bytes": agent_exe.stat().st_size, "idle_seconds": idle_seconds,
                              "idle_working_set_bytes": after["working_set_bytes"] if after else None}
        if before and after and after["cpu_seconds"] is not None:
            result["resource"]["idle_cpu_seconds"] = after["cpu_seconds"] - before["cpu_seconds"]
            result["resource"]["idle_cpu_percent_machine"] = 100 * result["resource"]["idle_cpu_seconds"] / idle_seconds / os.cpu_count()
        baseline = get_json(status_url)
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
            result["first_dns"] = list(pool.map(lambda kind: query(HOST, kind), ["A", "AAAA", "HTTPS"]))
        applied = get_json(status_url)
        assert applied["judge_calls"] - baseline["judge_calls"] == 1
        assert applied["commits"] == 1 and applied["learned_count"] == 1
        assert all(item["rcode"] == 0 for item in result["first_dns"])
        result["connection_started_unix_ns"] = time.time_ns()
        with connect_socks():
            connections = get_json(controller + "/connections")["connections"]
            matches = [item for item in connections if str(item.get("metadata", {}).get("destinationPort")) == "18081" and item.get("metadata", {}).get("host") == HOST]
            result["connections"] = [{key: item.get(key) for key in ["rule", "rulePayload", "chains"]} for item in matches]
            assert any(item["rulePayload"] == "learned-proxy" and "lab-hop" in item["chains"] for item in matches)
        result["cached_dns_tcp"] = query(HOST, "A", "tcp")
        result["known_dns"] = query("known.route-lab.test")
        result["private_dns"] = query("private.route-lab.test")
        assert get_json(status_url)["judge_calls"] == applied["judge_calls"]
        with connect_socks("private.route-lab.test"):
            connections = get_json(controller + "/connections")["connections"]
            private = [item for item in connections if item.get("metadata", {}).get("host") == "private.route-lab.test"]
            result["private_connections"] = [{key: item.get(key) for key in ["rule", "rulePayload", "chains"]} for item in private]
            assert any("DIRECT" in item["chains"] and "lab-hop" not in item["chains"] for item in private)
        assert get_json(status_url)["judge_calls"] == applied["judge_calls"]
        result["uncertain_dns"] = query("uncertain.route-lab.test")
        result["status_after"] = get_json(status_url)
        assert result["status_after"]["commits"] == 1 and result["status_after"]["learned_count"] == 1
        result["resource"]["sampled_peak_working_set_bytes"] = max(memory_samples, default=0)
        events = [json.loads(line) for line in (folder / "agent.log").read_text(encoding="utf-8").splitlines() if line.startswith("{")]
        host_id = hashlib.sha256(HOST.encode()).hexdigest()[:12]
        selected = [item for item in events if item.get("host_id") == host_id]
        result["events"] = selected
        applied_event = next(item for item in selected if item.get("msg") == "provider_applied")
        applied_ns = int(datetime.fromisoformat(applied_event["time"]).timestamp() * 1_000_000_000)
        result["timing_order_passed"] = applied_ns < min(item["received_unix_ns"] for item in result["first_dns"]) < result["connection_started_unix_ns"]
        assert result["timing_order_passed"]

        # Restart only our own Agent with a deterministic timeout fixture. This
        # proves DNS fallback and discarded late results without another API bill.
        sampling_stop.set()
        stop(agent)
        config.update({"judge": "stub", "preflight_ms": 50})
        config_path.write_text(json.dumps(config), encoding="utf-8")
        agent = launch([str(agent_exe), "serve", "--config", str(config_path), "--allow-lab-fixtures"], folder / "timeout.log", environment)
        processes.append(agent)
        wait_ready(status_url, "core_ready")
        result["deadline_dns"] = query("deadline.route-lab.test")
        time.sleep(.2)
        result["deadline_status"] = get_json(status_url)
        assert result["deadline_dns"]["rcode"] == 0 and result["deadline_dns"]["ms"] < 500
        assert result["deadline_status"]["judge_calls"] == 1 and result["deadline_status"]["commits"] == 0
        assert result["deadline_status"]["learned_count"] == 0

        before_fake = get_json(status_url)["queries"]
        main_config["dns"].update({"enhanced-mode": "fake-ip", "fake-ip-range": "198.19.0.1/16"})
        main_path.write_text(json.dumps(main_config), encoding="utf-8")
        assert get_json(controller + "/configs?force=true", "PUT", {"path": str(main_path)}) == 204
        result["fake_dns"] = query("fake.route-lab.test")
        result["fake_gate_hits"] = get_json(status_url)["queries"] - before_fake
        assert result["fake_gate_hits"] == 0
        result["passed"] = True
    finally:
        sampling_stop.set()
        for process in reversed(processes):
            if not process.lab_log.closed:
                stop(process)
        for server in servers:
            server.shutdown()
            server.server_close()
        path = Path(options.output)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(result, indent=2), encoding="utf-8")
    print(json.dumps({"passed": result["passed"], "backend": options.judge, "first_dns": result["first_dns"],
                      "resource": result["resource"], "deadline_dns": result["deadline_dns"], "fake_gate_hits": result["fake_gate_hits"]}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", default="dist/route-agent.exe")
    parser.add_argument("--mihomo", required=True)
    parser.add_argument("--judge", choices=["stub", "jev"], default="stub")
    parser.add_argument("--key-file")
    parser.add_argument("--proxy", default="http://127.0.0.1:7888")
    parser.add_argument("--workdir", default="lab-work/go-smoke")
    parser.add_argument("--output", default="local-evidence/go-smoke.json")
    run(parser.parse_args())
