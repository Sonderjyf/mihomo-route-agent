"""One explicitly approved hosted Windows FlClash run. Default is inert."""
import argparse
from contextlib import closing
import hashlib
import http.server
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import threading
import time
import urllib.request
import zipfile

from flclash_acceptance import ARCHIVE_SHA256, MANAGED, check_config, plan

SECRET = "synthetic-owned-app-fixture"
PROFILE_ID, SCRIPT_ID = 101, 201


def require_guest(allowed, environment=None, platform=None):
    env = os.environ if environment is None else environment
    if not allowed or (os.name if platform is None else platform) != "nt":
        raise RuntimeError("Explicit --allow-isolated-app on hosted Windows is required")
    expected = {"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted",
                "RUNNER_OS": "Windows", "GITHUB_REPOSITORY": "Sonderjyf/mihomo-route-agent"}
    if any(env.get(k) != v for k, v in expected.items()) or not env.get("GITHUB_RUN_ID", "").isdigit():
        raise RuntimeError("Refusing host or self-hosted application execution")


def preferences(profile_id=None):
    config = {
        "currentProfileId": profile_id, "overrideDns": False, "overrideNtp": False,
        "hotKeyActions": [], "excludeSSIDs": [],
        "appSettingProps": {"locale": "en", "autoRun": False, "autoLaunch": False,
                            "autoCheckUpdate": False, "silentLaunch": False,
                            "minimizeOnExit": False, "disclaimerAccepted": True,
                            "crashlytics": False, "crashlyticsTip": True,
                            "dashboardWidgets": [], "testUrl": "http://127.0.0.1:18766/health"},
        "networkProps": {"systemProxy": False, "autoSetSystemDns": False, "appendSystemDns": False},
        "vpnProps": {"enable": False, "systemProxy": False},
        "patchClashConfig": {"tun": {"enable": False}, "allow-lan": False,
                             "mixed-port": 17891, "external-controller": "127.0.0.1:9090",
                             "mode": "rule", "dns-override-keys": [], "ntp-override-keys": [],
                             "geo-auto-update": False, "port": 0, "socks-port": 0,
                             "redir-port": 0, "tproxy-port": 0},
    }
    return {"flutter.version": 1, "flutter.config": json.dumps(config)}


def validate_preferences(raw):
    if raw.get("flutter.version") != 1:
        raise RuntimeError("Unexpected preference migration version")
    config = json.loads(raw["flutter.config"])
    required = preferences()["flutter.config"]
    required = json.loads(required)
    for group in ("appSettingProps", "networkProps", "vpnProps", "patchClashConfig"):
        for key, value in required[group].items():
            actual = config[group].get(key)
            if isinstance(value, dict):
                if not isinstance(actual, dict) or any(actual.get(k) != v for k, v in value.items()):
                    raise RuntimeError(f"Unsafe app setting {group}.{key}")
            elif actual != value:
                raise RuntimeError(f"Unsafe app setting {group}.{key}")
    if config.get("overrideDns") is not False or config.get("overrideNtp") is not False:
        raise RuntimeError("App overrides unexpectedly enabled")
    return config


def profile(revision):
    rules = ["DOMAIN,known.route-lab.test,DIRECT"]
    if revision == "B":
        rules.insert(0, "DOMAIN,new.route-lab.test,DIRECT")
    return {"mode": "rule", "secret": SECRET, "allow-lan": False, "tun": {"enable": False},
            "dns": {"enable": True, "enhanced-mode": "fake-ip", "listen": "",
                    "nameserver": ["udp://127.0.0.1:15356"]},
            "proxy-groups": [{"name": "LAB", "type": "select", "proxies": ["DIRECT"]}],
            "rules": rules + ["MATCH,DIRECT"]}


def overwrite_script():
    return '''function main(config) {
  var names = ["route-agent-tail-direct", "route-agent-tail-proxy"];
  if (!config.rules || config.rules[config.rules.length-1] !== "MATCH,DIRECT") throw Error("unexpected fixture");
  var encoded = JSON.stringify(config);
  names.forEach(function(n) { if (encoded.indexOf(n) >= 0) throw Error("duplicate managed provider"); });
  config["rule-providers"] = config["rule-providers"] || {};
  names.forEach(function(n) {
    config["rule-providers"][n] = {type:"http",behavior:"classical",format:"yaml",proxy:"DIRECT",
      url:"http://127.0.0.1:18765/rules/"+n+".yaml",path:"./route-agent-tail/"+n+".yaml",interval:3600};
  });
  config.rules.splice(config.rules.length-1,0,
    "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-direct)),DIRECT",
    "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-proxy)),LAB");
  return config;
}'''


def seed_database(path):
    # Use the schema created by the actual first app launch, not a recreated DB.
    with closing(sqlite3.connect(path)) as db, db:
        seed_tables(db)


def seed_tables(db):
    if db.execute("PRAGMA user_version").fetchone()[0] != 10:
        raise RuntimeError("Unexpected release database schema")
    if db.execute("SELECT count(*) FROM profiles").fetchone()[0] or db.execute("SELECT count(*) FROM scripts").fetchone()[0]:
        raise RuntimeError("Guest database is not empty")
    db.execute('INSERT INTO scripts(id,label,last_update_time,url,"order") VALUES(?,?,?,?,?)',
               (SCRIPT_ID, "Owned acceptance overwrite", int(time.time()), None, 0))
    db.execute('''INSERT INTO profiles(id,label,url,last_update_date,overwrite_type,script_id,
               auto_update_duration_millis,auto_update,selected_map,unfold_set,"order")
               VALUES(?,?,?,?,?,?,?,?,?,?,?)''',
               (PROFILE_ID, "Owned acceptance profile", "http://127.0.0.1:18766/profile.yaml",
                int(time.time()), "script", SCRIPT_ID, 86400000, 0, "{}", "[]", 0))


def ps(action, executable=None, pid=0, diagnostic_path=None, allow_window_input=False):
    command = ["powershell.exe", "-NoProfile", "-NonInteractive", "-File",
               str(Path(__file__).with_name("flclash_guest.ps1")), "-Action", action, "-AllowIsolatedApp"]
    if executable:
        command += ["-Executable", str(executable)]
    if pid:
        command += ["-AppPid", str(pid)]
    if diagnostic_path:
        command += ["-DiagnosticPath", str(diagnostic_path)]
    if allow_window_input:
        command += ["-AllowWindowInput"]
    result = subprocess.run(command, capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(f"Guest {action} failed: {result.stderr[-1200:]}")
    return json.loads(result.stdout)


def wait_until(check, timeout=30):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (OSError, ValueError, RuntimeError) as exc:
            last = exc
        time.sleep(0.25)
    raise RuntimeError(f"Bounded app wait failed: {last}")


def read_fixture_profile(path):
    with closing(sqlite3.connect(Path(path).resolve().as_uri() + "?mode=ro", uri=True)) as db:
        row = db.execute('SELECT id,url,overwrite_type,script_id,auto_update,last_update_date FROM profiles WHERE id=?', (PROFILE_ID,)).fetchone()
    if not row or row[:5] != (PROFILE_ID, "http://127.0.0.1:18766/profile.yaml", "script", SCRIPT_ID, 0):
        raise RuntimeError("App database fixture identity changed")
    return {"id": row[0], "url": row[1], "overwrite_type": row[2], "script_id": row[3], "auto_update": row[4], "last_update_date": row[5]}


def validate_refresh_result(before, after, requests_before, requests_after):
    if requests_after <= requests_before or not isinstance(after["last_update_date"], int) or after["last_update_date"] <= before["last_update_date"]:
        raise RuntimeError("Refresh lacks a new subscription request and database update")


def validate_final_inventory(inventory, pid_checks):
    if any(not isinstance(inventory.get(key), list) for key in ("processes", "tcp_listeners", "udp_endpoints")):
        raise RuntimeError("Incomplete final process/listener inventory")
    if any(item["alive"] for item in pid_checks) or any(inventory.get(key) for key in ("processes", "tcp_listeners", "udp_endpoints")):
        raise RuntimeError("Recorded process, owned executable or fixture listener remains after cleanup")


def run(options):
    require_guest(options.allow_isolated_app)
    if not options.allow_window_input:
        raise RuntimeError("Explicit --allow-window-input required; UIA-only navigation is blocked")
    import yaml
    ps("clean")
    work = Path(options.workdir).resolve()
    work.mkdir(exist_ok=False)
    report = {"passed": False, "app_started": False, "app_tested": False, "tun_tested": False, "steps": [],
              "tested_head_sha": os.environ.get("ROUTE_AGENT_TESTED_SHA"), "run_id": os.environ.get("GITHUB_RUN_ID")}
    owned = []
    servers = []
    baseline = ps("snapshot")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    app = core = data_dir = None
    revision = {"value": "A", "requests": 0}

    def network_unchanged():
        if ps("snapshot") != baseline:
            raise RuntimeError("Guest network settings changed unexpectedly")

    def remember_children(pid):
        for child in ps("children", core, pid):
            pair = (child["ProcessId"], core)
            if pair not in owned:
                owned.append(pair)

    def launch():
        validate_preferences(json.loads((data_dir / "shared_preferences.json").read_text(encoding="utf-8")))
        pid = ps("start", app)["pid"]
        owned.append((pid, app))
        report["app_started"] = True
        wait_until(lambda: (data_dir / "database.sqlite").exists())
        wait_until(lambda: ps("children", core, pid))
        remember_children(pid)
        network_unchanged()
        return pid

    def close(pid):
        remember_children(pid)
        ps("close", app, pid)
        wait_until(lambda: not ps("alive", app, pid))
        for child_pid, executable in owned:
            if executable == core:
                wait_until(lambda: not ps("alive", executable, child_pid), timeout=15)
        network_unchanged()

    def ui(action, pid):
        diagnostic = work / "ui-failure.json"
        try:
            result = ps(action, app, pid, diagnostic, allow_window_input=True)
            if not isinstance(result, dict) or result.get("page_title_verified") != "Profiles":
                raise RuntimeError("Navigation lacks verified page evidence")
            report.setdefault("ui_actions", []).append(result)
        except Exception:
            if diagnostic.exists():
                report["ui_failure"] = json.loads(diagnostic.read_text(encoding="utf-8-sig"))
            else:
                report["ui_failure"] = {"error": "helper ended without diagnostics (including possible UIA timeout)"}
            try:
                stored = json.loads((data_dir / "shared_preferences.json").read_text(encoding="utf-8"))
                report["ui_failure"]["persisted_locale"] = json.loads(stored["flutter.config"])["appSettingProps"]["locale"]
            except Exception:
                report["ui_failure"]["persisted_locale"] = "unavailable"
            raise

    def api(path):
        req = urllib.request.Request("http://127.0.0.1:9090" + path,
                                     headers={"Authorization": "Bearer " + SECRET})
        with opener.open(req, timeout=2) as response:
            return json.load(response)

    def capture(expected_revision):
        actual = yaml.safe_load((data_dir / "config.yaml").read_text(encoding="utf-8"))
        checked = check_config(profile(expected_revision), actual, "LAB")
        configs, rules, providers = api("/configs"), api("/rules")["rules"], api("/providers/rules")["providers"]
        if configs.get("mode") != "rule" or configs.get("tun", {}).get("enable") is not False:
            raise RuntimeError("App runtime mode/TUN differs from expected")
        expected = [("Domain", r.split(",")[1], "DIRECT") for r in profile(expected_revision)["rules"][:-1]]
        expected += [("AND", "((Network,tcp) && (DstPort,443) && (RuleSet,"+n+"))", target)
                     for n, target in zip(MANAGED, ("DIRECT", "LAB"))] + [("Match", "", "DIRECT")]
        if [(r["type"], r["payload"], r["proxy"]) for r in rules] != expected:
            raise RuntimeError("Actual app-owned core rules differ from generated config")
        if any(providers.get(n, {}).get("ruleCount") != 0 or providers[n].get("vehicleType") != "HTTP" for n in MANAGED):
            raise RuntimeError("App-owned core provider mismatch")
        return {"revision": expected_revision, "contract": checked, "runtime_rules": rules,
                "providers_empty": True, "effective_sha256": hashlib.sha256(json.dumps(actual, sort_keys=True).encode()).hexdigest()}

    try:
        report["desktop_preflight"] = ps("preflight")
        if report["desktop_preflight"].get("ready") is not True:
            raise RuntimeError("blocked_interactive_desktop: active input desktop and English OCR required; app not started")
        archive = work / "flclash.zip"
        with opener.open(plan()["archive_url"], timeout=60) as response, archive.open("xb") as output:
            total = 0
            while block := response.read(1 << 20):
                total += len(block)
                if total > 70_000_000:
                    raise RuntimeError("Oversized app archive")
                output.write(block)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != ARCHIVE_SHA256:
            raise RuntimeError("Official portable archive checksum mismatch")
        unpack = work / "app"
        with zipfile.ZipFile(archive) as bundle:
            for name in bundle.namelist():
                if not (unpack / name).resolve().is_relative_to(unpack.resolve()):
                    raise RuntimeError("Unsafe archive path")
            bundle.extractall(unpack)
        matches = list(unpack.rglob("FlClash.exe"))
        if len(matches) != 1:
            raise RuntimeError("Unexpected portable layout")
        app = matches[0]
        core = app.with_name("FlClashCore.exe")
        if not core.is_file():
            raise RuntimeError("Bundled core missing")
        metadata = ps("metadata", app)
        data_dir = Path(metadata["roaming"]) / metadata["company"] / metadata["product"]
        data_dir.mkdir(parents=True, exist_ok=False)
        (data_dir / "shared_preferences.json").write_text(json.dumps(preferences()), encoding="utf-8")
        report.update({"archive_sha256": ARCHIVE_SHA256,
                       "app_sha256": hashlib.sha256(app.read_bytes()).hexdigest(),
                       "bundled_core_sha256": hashlib.sha256(core.read_bytes()).hexdigest()})

        class Fixture(http.server.BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_GET(self):
                if self.path == "/profile.yaml":
                    revision["requests"] += 1
                    body = yaml.safe_dump(profile(revision["value"])).encode()
                elif self.path in [f"/rules/{n}.yaml" for n in MANAGED]:
                    body = b"payload: []\n"
                elif self.path == "/health":
                    body = b"ok"
                else:
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        for port in (18765, 18766):
            server = http.server.ThreadingHTTPServer(("127.0.0.1", port), Fixture)
            servers.append(server)
            threading.Thread(target=server.serve_forever, daemon=True).start()
        first = launch()
        ui("profiles", first)
        close(first)
        validate_preferences(json.loads((data_dir / "shared_preferences.json").read_text(encoding="utf-8")))
        report["steps"].append("actual blank app startup, UI navigation and graceful exit")
        seed_database(data_dir / "database.sqlite")
        (data_dir / "profiles").mkdir(exist_ok=True)
        (data_dir / "scripts").mkdir(exist_ok=True)
        (data_dir / "profiles" / f"{PROFILE_ID}.yaml").write_text(yaml.safe_dump(profile("A")), encoding="utf-8")
        (data_dir / "scripts" / f"{SCRIPT_ID}.js").write_text(overwrite_script(), encoding="utf-8")
        (data_dir / "shared_preferences.json").write_text(json.dumps(preferences(PROFILE_ID)), encoding="utf-8")
        second = launch()
        report["initial"] = wait_until(lambda: capture("A"))
        report["bundled_core_version"] = api("/version")
        ui("profiles", second)
        revision["value"] = "B"
        before = revision["requests"]
        db_before = read_fixture_profile(data_dir / "database.sqlite")
        wait_until(lambda: int(time.time()) > db_before["last_update_date"], timeout=3)
        ui("update", second)
        wait_until(lambda: revision["requests"] > before)
        report["refreshed"] = wait_until(lambda: capture("B"))
        db_after = read_fixture_profile(data_dir / "database.sqlite")
        validate_refresh_result(db_before, db_after, before, revision["requests"])
        report["refreshed"]["database"] = db_after
        before = revision["requests"]
        db_before = db_after
        wait_until(lambda: int(time.time()) > db_before["last_update_date"], timeout=3)
        ui("update", second)
        wait_until(lambda: revision["requests"] > before)
        report["repeated_refresh"] = wait_until(lambda: capture("B"))
        db_after = read_fixture_profile(data_dir / "database.sqlite")
        validate_refresh_result(db_before, db_after, before, revision["requests"])
        report["repeated_refresh"]["database"] = db_after
        network_unchanged()
        validate_preferences(json.loads((data_dir / "shared_preferences.json").read_text(encoding="utf-8")))
        close(second)
        third = launch()
        report["restarted"] = wait_until(lambda: capture("B"))
        close(third)
        report.update({"passed": True, "app_tested": True, "graceful_exit": True,
                       "scope": "actual official app config, GUI refresh, startup/exit; seeded synthetic fixtures; no learned traffic"})
    except Exception as exc:
        report["error"] = str(exc)
        raise
    finally:
        if core:
            for pid, executable in list(owned):
                if executable == app:
                    try:
                        remember_children(pid)
                    except Exception as exc:
                        report["cleanup_error"] = str(exc)
                        report["passed"] = False
        # Stop recorded app parents first so they cannot restart a core during cleanup.
        for pid, executable in list(owned):
            if executable != app:
                continue
            try:
                if ps("alive", executable, pid):
                    ps("terminate", executable, pid)
                    wait_until(lambda: not ps("alive", executable, pid), timeout=10)
                    report["forced_cleanup"] = True
                    report["passed"] = False
                remember_children(pid)
            except Exception as exc:
                report["cleanup_error"] = str(exc)
                report["passed"] = False
        for pid, executable in reversed(owned):
            try:
                if ps("alive", executable, pid):
                    ps("terminate", executable, pid)
                    wait_until(lambda: not ps("alive", executable, pid), timeout=10)
                    report["forced_cleanup"] = True
                    report["passed"] = False
            except Exception as exc:
                report["cleanup_error"] = str(exc)
                report["passed"] = False
        for server in servers:
            server.shutdown()
            server.server_close()
        if app:
            try:
                checks = [{"pid": pid, "executable": str(executable), "alive": ps("alive", executable, pid)} for pid, executable in owned]
                inventory = ps("inventory", app)
                report["final_pid_checks"] = checks
                report["final_inventory"] = inventory
                validate_final_inventory(inventory, checks)
                report["owned_processes_and_ports_clear"] = True
            except Exception as exc:
                report["owned_processes_and_ports_clear"] = False
                report["cleanup_error"] = str(exc)
                report["passed"] = False
        try:
            network_unchanged()
            report["guest_network_restored"] = True
        except Exception as exc:
            report["guest_network_restored"] = False
            report["cleanup_error"] = str(exc)
            report["passed"] = False
        (work / "result.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
        print(json.dumps(report, indent=2))
    if not report["passed"]:
        raise RuntimeError("App acceptance did not pass cleanup")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--allow-isolated-app", action="store_true")
    parser.add_argument("--allow-window-input", action="store_true")
    parser.add_argument("--workdir")
    args = parser.parse_args()
    if not args.execute:
        result = plan()
        result.update({"execution_implemented": True, "release_execution_verified": False,
                       "command": "python -B scripts/flclash_app_acceptance.py --execute --allow-isolated-app --allow-window-input --workdir NEW_GUEST_DIR"})
        print(json.dumps(result, indent=2))
        return
    require_guest(args.allow_isolated_app)
    if not args.workdir:
        parser.error("a new guest workdir is required")
    run(args)


if __name__ == "__main__":
    main()
