"""Synthetic Fake-IP/connection-observer smoke; never accesses installed FlClash.

Uses only the explicitly supplied separate core binary, local stub services,
empty/new work directory and synthetic names. No real model or TLS evidence.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import threading
import time

from smoke_go import (TCPServer, UDPServer, DNSHandler, UDPHandler, EchoHandler,
                      ensure_ports_available, get_json, launch, stop, wait_ready,
                      receive, query)

HOSTS = ["first.route-lab.test", "known.route-lab.test", "uncertain.route-lab.test"]
PORTS = {15354, 15355, 15356, 17891, 17892, 18081, 18765, 19091}


def connect_fake(host):
    answer = query(host)
    assert answer["rcode"] == 0
    fake = answer["answers"][0].split()[-1]
    assert fake.startswith("198.19."), fake
    connection = socket.create_connection(("127.0.0.1", 17891), timeout=3)
    try:
        connection.sendall(b"\x05\x01\x00")
        assert receive(connection, 2) == b"\x05\x00"
        connection.sendall(b"\x05\x01\x00\x01" + socket.inet_aton(fake) + (18081).to_bytes(2, "big"))
        response = receive(connection, 4)
        assert response[1] == 0
        if response[3] == 1:
            receive(connection, 6)
        elif response[3] == 4:
            receive(connection, 18)
        else:
            receive(connection, receive(connection, 1)[0] + 2)
        connection.sendall(b"tail-proof")
        assert receive(connection, 10) == b"tail-proof"
        return connection
    except BaseException:
        connection.close()
        raise


def run(options):
    if options.protected_ports:
        protected = set(json.loads(Path(options.protected_ports).read_text(encoding="utf-8")))
        if PORTS & protected:
            raise RuntimeError("lab would overlap protected configuration ports")
    ensure_ports_available()
    folder = Path(options.workdir).resolve()
    folder.mkdir(parents=True, exist_ok=False)
    agent, core = Path(options.agent).resolve(), Path(options.mihomo).resolve()
    environment = os.environ.copy()
    environment.pop("MIHOMO_SECRET", None)
    environment.pop("OPENROUTER_API_KEY", None)
    processes, servers, connections = [], [], []
    result = {"scope": "synthetic isolated core; no actual FlClash/TUN/API", "passed": False,
              "core_version": subprocess.check_output([str(core), "-v"], text=True, timeout=5).strip(),
              "core_sha256": hashlib.sha256(core.read_bytes()).hexdigest()}
    controller, status = "http://127.0.0.1:19091", "http://127.0.0.1:18765/status"
    source = {
        "mode": "rule", "mixed-port": 17891, "external-controller": "127.0.0.1:19091",
        "allow-lan": False, "bind-address": "127.0.0.1", "log-level": "error", "tun": {"enable": False},
        "dns": {"enable": True, "listen": "127.0.0.1:15354", "enhanced-mode": "fake-ip",
                "fake-ip-range": "198.19.0.1/16", "use-hosts": False, "use-system-hosts": False,
                "nameserver": ["udp://127.0.0.1:15356"], "default-nameserver": ["127.0.0.1:15356"]},
        "proxies": [{"name": name, "type": "socks5", "server": "127.0.0.1", "port": 17892}
                    for name in ["lab-base", "lab-learned"]],
        "proxy-groups": [{"name": "BASE", "type": "select", "proxies": ["lab-base"]},
                         {"name": "LEARNED", "type": "select", "proxies": ["lab-learned"]}],
        "rules": ["PROCESS-NAME,synthetic.exe,BASE", "IP-CIDR,192.0.2.0/24,BASE,no-resolve",
                  "DOMAIN,known.route-lab.test,BASE", "MATCH,BASE"]}
    profile = folder / "source.json"
    profile.write_text(json.dumps(source), encoding="utf-8")
    plan = json.loads(subprocess.check_output([str(agent), "tail-preview", "--profile", str(profile), "--proxy-target", "LEARNED"]))
    main = plan["candidate"]
    # Deliberate LAB-ONLY conversion of the empty file providers to this own
    # observer's HTTP endpoints. The normal tail-preview remains inert.
    for name, provider in main["rule-providers"].items():
        provider.update({"type": "http", "url": f"http://127.0.0.1:18765/rules/{name}.yaml", "interval": 3600})
    config = {"mode": "async", "judge": "stub", "controller": controller, "proxy_group": "LEARNED",
              "http_listen": "127.0.0.1:18765", "preflight_ms": 500, "dns_deadline_ms": 1000,
              "capacity": 16, "max_api_requests": 8,
              "lab_fixtures": {host: {"direct_tls": "repeated_failure", "proxy_tls": "verified_success"} for host in HOSTS[:2]}}
    config["lab_fixtures"][HOSTS[2]] = {"direct_tls": "not_tested", "proxy_tls": "not_tested"}
    config_path = folder / "agent.json"
    config_path.write_text(json.dumps(config), encoding="utf-8")
    try:
        for server in [UDPServer(("127.0.0.1", 15356), UDPHandler), TCPServer(("127.0.0.1", 15356), DNSHandler), TCPServer(("127.0.0.1", 18081), EchoHandler)]:
            servers.append(server)
            threading.Thread(target=server.serve_forever, daemon=True).start()

        def start_core(name, data):
            home = folder / name
            home.mkdir()
            (home / "config.json").write_text(json.dumps(data), encoding="utf-8")
            process = launch([str(core), "-d", str(home), "-f", str(home / "config.json")], home / "core.log", environment)
            processes.append(process)
            return process

        start_core("hop", {"mixed-port": 17892, "allow-lan": False, "bind-address": "127.0.0.1",
                           "mode": "rule", "log-level": "error", "tun": {"enable": False},
                           "rules": ["MATCH,DIRECT"], "hosts": {host: "127.0.0.1" for host in HOSTS}})
        observer = launch([str(agent), "observe-lab", "--config", str(config_path), "--allow-lab-fixtures"], folder / "observer.log", environment)
        processes.append(observer)
        wait_ready(status, process=observer)
        main_process = start_core("main", main)
        wait_ready(controller + "/version", process=main_process)
        wait_ready(status, "observer_ready", process=observer)

        def records(host):
            return [{key: c.get(key) for key in ["rule", "rulePayload", "chains"]}
                    for c in get_json(controller + "/connections")["connections"] if c["metadata"].get("host") == host]

        connections.append(connect_fake(HOSTS[0]))
        result["first"] = records(HOSTS[0])
        assert any(c["rule"] == "Match" and c["rulePayload"] == "" and c["chains"][-1] == "BASE" for c in result["first"])
        deadline = time.monotonic() + 5
        while get_json(status)["commits"] != 1 and time.monotonic() < deadline:
            time.sleep(.05)
        assert get_json(status)["commits"] == 1
        connections.append(connect_fake(HOSTS[0]))
        result["after_learning"] = records(HOSTS[0])
        assert any(c["rule"] == "RuleSet" and c["rulePayload"] == "route-agent-tail-proxy" and "lab-learned" in c["chains"] for c in result["after_learning"])
        assert any(c["rule"] == "Match" and c["chains"][-1] == "BASE" for c in result["after_learning"]), "first connection unexpectedly changed"
        for host in HOSTS[1:]:
            connections.append(connect_fake(host))
        deadline = time.monotonic() + 3
        while get_json(status)["judge_calls"] < 2 and time.monotonic() < deadline:
            time.sleep(.05)
        time.sleep(.2)
        result["known"] = records(HOSTS[1])
        result["uncertain"] = records(HOSTS[2])
        result["status"] = get_json(status)
        assert result["status"]["judge_calls"] == 2 and result["status"]["commits"] == 1 and result["status"]["learned_count"] == 1
        assert any(c["rule"] == "Domain" for c in result["known"])
        assert any(c["rule"] == "Match" for c in result["uncertain"])
        # Mutate ONLY this owned isolated core to prove fail-closed mode drift.
        assert get_json(controller + "/configs", "PATCH", {"mode": "direct"}) == 204
        observer.wait(timeout=4)
        assert observer.returncode != 0
        result["mode_drift_stopped_observer"] = True
        result["passed"] = True
    finally:
        for connection in connections:
            connection.close()
        for process in reversed(processes):
            if not process.lab_log.closed:
                stop(process)
        for server in servers:
            server.shutdown()
            server.server_close()
        (folder / "result.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", required=True)
    parser.add_argument("--mihomo", required=True)
    parser.add_argument("--workdir", required=True)
    parser.add_argument("--protected-ports")
    run(parser.parse_args())
