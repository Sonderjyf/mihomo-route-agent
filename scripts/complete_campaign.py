"""Frozen hosted-only campaign driver. Default is an inert plan, never execution."""
import datetime
import hashlib
import http.server
import ipaddress
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import ssl
import subprocess
import sys
import threading
import time
import urllib.request
import zipfile

import real_acceptance as a
import learning_acceptance as learning
import flclash_app_acceptance as app

CAMPAIGN = "PR5-COMPLETE-20261016-01"
TARGETS = ("example.com", "www.cloudflare.com")
DEVICE = "RouteAgentCampaign"
PREFIX = "198.18.0.0/16"
APP_HASH = "9f72ed125300750e4170f1a5424118f7c4694a9e1c035977817ce8d52e4c10c9"
BUNDLED_HASH = "f291a9695368586ec19390c437b95e33479fe3f365d5b3f837acdc391e099c3a"
MAX_ATTEMPTS = 6
MANAGED = ("route-agent-tail-direct", "route-agent-tail-proxy")
SCENES = tuple("C%02d" % i for i in range(13))
STATE_LIMIT = 2 << 20


class OfficialAssetRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        from urllib.parse import urlsplit
        target = urlsplit(newurl)
        require(target.scheme == "https" and target.hostname in
                {"github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"}
                and target.username is None and target.password is None)
        return super().redirect_request(req, fp, code, msg, headers, newurl)


class StopCampaign(RuntimeError):
    pass


def require(value):
    if not value:
        raise StopCampaign()


def plan():
    return dict(campaign_id=CAMPAIGN, mode="inert_plan", execution_enabled=False,
                runner="github-hosted/windows-2025", max_model_transport_attempts=6,
                job_minutes=40, cleanup_reserved_minutes=5,
                observation_minutes=12, publish_interval_seconds=30,
                targets=list(TARGETS), scenes=list(SCENES),
                model="typesafe/jev-1.13", resolver="1.1.1.1:53",
                app="FlClash 0.8.99", standalone_core_substitution=False,
                requires_final_sha_and_budget_authorization=True)


def gate(env, head):
    # New workflow identity is checked directly, never forged to reuse another launcher.
    import re
    require(sys.platform == "win32" and env.get("RUNNER_OS") == "Windows")
    require(env.get("GITHUB_ACTIONS") == "true" and env.get("RUNNER_ENVIRONMENT") == "github-hosted")
    require(env.get("GITHUB_REPOSITORY") == a.REPO and env.get("GITHUB_EVENT_NAME") == "workflow_dispatch")
    require(env.get("GITHUB_REF") == "refs/heads/main" and env.get("GITHUB_RUN_ATTEMPT") == "1")
    require(env.get("GITHUB_WORKFLOW_REF") == a.REPO+"/.github/workflows/complete-campaign.yml@refs/heads/main")
    require(re.fullmatch(r"[0-9a-f]{40}", head or "") is not None
            and env.get("REVIEWED_SHA") == env.get("REVIEWED_SHA_ALLOWLIST") == head)
    require(env.get("EXECUTE_COMPLETE_CAMPAIGN") == "true" and env.get("CAMPAIGN_ID") == CAMPAIGN)
    require(env.get("APPROVED_MODEL_ATTEMPTS") == "6" and env.get("APPROVED_TUN") == "true")
    require(env.get("APPROVAL_RECORD", "").strip() and env.get("PROXY_COST_APPROVED") == "true")
    require(env.get("APPROVED_BUDGET_METADATA_GETS") == "2")
    a.budget(env)
    require(env.get("REMAINING_BUDGET_CONFIRMED") == "true")
    from decimal import Decimal
    require(re.fullmatch(r"(?:0|[1-9][0-9]{0,3})(?:\.[0-9]{1,4})?", env.get("REMAINING_USD","")) is not None)
    remaining = Decimal(env["REMAINING_USD"])
    require(0 < remaining <= Decimal(env["PROVIDER_CAP_USD"]))
    require(env.get("GITHUB_RUN_ID", "").isdigit())
    require(env.get("DIRECT_INTERFACE_INDEX", "").isdigit() and int(env["DIRECT_INTERFACE_INDEX"]) > 0)


def child_env(env, model=False):
    out = a.child_env(env, model=model)
    for name in ("APPDATA", "LOCALAPPDATA", "USERPROFILE", "GITHUB_ACTIONS",
                 "RUNNER_ENVIRONMENT", "RUNNER_OS", "GITHUB_REPOSITORY", "GITHUB_RUN_ID",
                 "GOPROXY", "GOSUMDB", "GOTOOLCHAIN"):
        if name in env:
            out[name] = env[name]
    return out


def command(argv, env, seconds=30, check=True):
    result = subprocess.run(argv, env=child_env(env), stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=seconds)
    if check:
        require(result.returncode == 0)
    return result


def powershell(script, env, seconds=20):
    result = command(["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script], env, seconds)
    require(len(result.stdout) <= STATE_LIMIT)
    return json.loads(result.stdout.decode("utf-8-sig"))


def guest(action, env, executable=None, pid=0, diagnostic=None):
    args = ["powershell.exe", "-NoProfile", "-NonInteractive", "-File",
            str(Path(__file__).with_name("flclash_guest.ps1")),
            "-Action", action, "-AllowIsolatedApp"]
    if executable:
        args += ["-Executable", str(executable)]
    if pid:
        args += ["-AppPid", str(pid)]
    if diagnostic:
        args += ["-DiagnosticPath", str(diagnostic), "-AllowWindowInput"]
    result = command(args, env, 60)
    require(len(result.stdout) <= STATE_LIMIT)
    return json.loads(result.stdout.decode("utf-8-sig"))


def wait(read, predicate=lambda x: bool(x), seconds=30, process=None):
    end = time.monotonic()+seconds
    while time.monotonic() < end:
        if process is not None:
            require(process.poll() is None)
        try:
            value = read()
            if predicate(value):
                return value
        except (OSError, ValueError, KeyError, StopCampaign):
            pass
        time.sleep(.15)
    raise StopCampaign()


def snapshot(env):
    extra = powershell(r"""
$ErrorActionPreference='Stop'
$rules=@(Get-NetFirewallRule | Sort-Object Name | Select-Object Name,Enabled,Direction,Action)
$fault=@(Get-NetFirewallRule -Group 'RouteAgentCampaign' -ErrorAction SilentlyContinue | Select-Object Name)
$owned=@(Get-NetRoute | Where-Object {$_.DestinationPrefix -eq '198.18.0.0/16'} | Sort-Object InterfaceIndex | Select-Object InterfaceIndex,DestinationPrefix,NextHop,RouteMetric)
$devices=@(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -eq 'RouteAgentCampaign'} | Select-Object InterfaceIndex,Name,Status)
@{firewall=$rules;fault_rules=$fault;owned_routes=$owned;owned_devices=$devices}|ConvertTo-Json -Depth 7 -Compress
""", env, 30)
    return dict(app=guest("snapshot", env), extra=extra)


def tun_route(env, install):
    if install:
        return powershell(r"""
$ErrorActionPreference='Stop'
$a=@(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -eq 'RouteAgentCampaign' -and $_.Status -eq 'Up'})
if($a.Count -ne 1){throw 'Owned TUN unavailable'}
if(@(Get-NetRoute | Where-Object {$_.DestinationPrefix -eq '198.18.0.0/16'}).Count -ne 0){throw 'Prefix already owned'}
New-NetRoute -DestinationPrefix '198.18.0.0/16' -InterfaceIndex $a[0].InterfaceIndex -NextHop '0.0.0.0' -RouteMetric 5 -PolicyStore ActiveStore | Out-Null
@{index=$a[0].InterfaceIndex;prefix='198.18.0.0/16'}|ConvertTo-Json -Compress
""", env)
    return powershell(r"""
$ErrorActionPreference='Stop'
$a=@(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -eq 'RouteAgentCampaign'})
foreach($d in $a){
 Get-NetRoute -InterfaceIndex $d.InterfaceIndex -DestinationPrefix '198.18.0.0/16' -ErrorAction SilentlyContinue |
 Remove-NetRoute -Confirm:$false -ErrorAction Stop
}
'true'
""", env)


def clean_budget_scope(env):
    require(time.time()-float(env["JOB_STARTED_AT"]) < 35*60)


def read_api(port, path, secret=""):
    request = urllib.request.Request(f"http://127.0.0.1:{port}"+path)
    if secret:
        request.add_header("Authorization", "Bearer "+secret)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), learning.NoRedirect())
    with opener.open(request, timeout=3) as response:
        raw = response.read(STATE_LIMIT+1)
    require(len(raw) <= STATE_LIMIT)
    return json.loads(raw)


def public_address(host, env):
    require(host in TARGETS)
    script = ("$ErrorActionPreference='Stop'; $r=@(Resolve-DnsName -Name '"+host+
              "' -Server '1.1.1.1' -Type A -DnsOnly | Where-Object {$_.Type -eq 'A'} | "
              "Select-Object -ExpandProperty IPAddress); if($r.Count -lt 1){exit 7}; $r[0]|ConvertTo-Json -Compress")
    value = powershell(script, env, 15)
    addr = ipaddress.ip_address(value)
    require(addr.version == 4 and addr.is_global and not addr.is_reserved and not addr.is_multicast)
    return str(addr)


def fake_address(host):
    import dns.message
    import dns.query
    import dns.rdatatype
    answer = dns.query.udp(dns.message.make_query(host, "A"), "127.0.0.1", port=15354, timeout=3)
    addresses = [r.address for rr in answer.answer if rr.rdtype == dns.rdatatype.A for r in rr]
    require(len(addresses) == 1 and ipaddress.ip_address(addresses[0]) in ipaddress.ip_network(PREFIX))
    return addresses[0]


def connect(host, tls=True):
    sock = socket.create_connection((fake_address(host), 443), timeout=5)
    try:
        if tls:
            sock = ssl.create_default_context().wrap_socket(sock, server_hostname=host)
            require(bool(sock.getpeercert()))
        return sock
    except BaseException:
        sock.close()
        raise


def connection(snapshot_value, sock, host):
    port = str(sock.getsockname()[1])
    rows = [r for r in snapshot_value["connections"] if str(r["metadata"].get("sourcePort")) == port
            and r["metadata"].get("host") == host and r["metadata"].get("destinationPort") == "443"]
    require(len(rows) == 1 and rows[0].get("id"))
    return rows[0]


def receipt_header(status, first_id):
    import re
    rows = status.get("decisions")
    require(type(rows) is list and len(rows) == 1)
    row = rows[0]
    require(row.get("connection_id") == first_id
            and re.fullmatch("[0-9a-f]{32}",row.get("id","")) is not None
            and re.fullmatch("[0-9a-f]{64}",row.get("scope","")) is not None
            and row.get("policy_version") == "route-policy-v1"
            and type(row.get("policy_adopted")) is bool
            and row.get("status") in {"pending","rejected","superseded","applied","expired","invalidated","failed","withdrawn"})
    return dict(decision_id=row["id"],scope=row["scope"],status=row["status"],
                policy_adopted=row["policy_adopted"])


def same_target(first, later, ip):
    require(first["id"] != later["id"])
    for record in (first, later):
        require(record["metadata"].get("destinationIP") == ip
                and record["metadata"].get("network") == "tcp"
                and record["metadata"].get("type") == "Tun")
    require(later.get("upload", 0) > 0 and later.get("download", 0) > 0)


def original_profile(revision, secret, pins, node=None):
    dns = app.fixture_dns()
    dns.update(listen="127.0.0.1:15354", **{"fake-ip-range":"198.18.0.1/16"})
    proxies = [] if node is None else a.core_config(node, secret)["proxies"]
    rules = ["DOMAIN,known.route-lab.test,DIRECT"]
    if revision != "A":
        rules.insert(0, "DOMAIN,revision-b.route-lab.test,DIRECT")
    if revision == "ALT":
        rules.insert(0, "DOMAIN,alternate.route-lab.test,DIRECT")
    return dict(mode="rule", secret=secret, **{
        "allow-lan":False, "hosts":dict(pins), "log-level":"silent", "sniffer":{"enable":False},
        "tun":{"enable":True, "device":DEVICE, "stack":"gvisor", "auto-route":False,
               "auto-detect-interface":True, "strict-route":False, "dns-hijack":[]},
        "dns":dns, "proxies":proxies,
        "proxy-groups":[{"name":"LAB","type":"select","proxies":["DIRECT" if node is None else "acceptance-vless"]}],
        "rules":rules+["MATCH,LAB"]})


def script():
    # Pure transform: it never touches DNS, proxies or original rule order.
    return app.overwrite_script().replace("MATCH,DIRECT", "MATCH,LAB")


def prefs(selected=None):
    value = app.preferences(selected)
    config = json.loads(value["flutter.config"])
    config["patchClashConfig"]["tun"] = dict(enable=True, device=DEVICE, stack="gvisor",
                                              **{"auto-route":False,"auto-detect-interface":True,"dns-hijack":[]})
    value["flutter.config"] = json.dumps(config)
    return value


def verify_settings(path, selected):
    value = json.loads(path.read_text(encoding="utf-8"))
    expected = json.loads(prefs(selected)["flutter.config"])
    actual = json.loads(value["flutter.config"])
    require(value.get("flutter.version") == 1 and actual.get("currentProfileId") == selected)
    for group in ("appSettingProps", "networkProps", "vpnProps", "patchClashConfig"):
        for key, item in expected[group].items():
            if isinstance(item, dict):
                require(isinstance(actual[group].get(key), dict)
                        and all(actual[group][key].get(k) == v for k,v in item.items()))
            else:
                require(actual[group].get(key) == item)
    require(actual.get("overrideDns") is False and actual.get("overrideNtp") is False)
    return True


def seed(path, alternate=False):
    with sqlite3.connect(path) as db:
        require(db.execute("PRAGMA user_version").fetchone()[0] == 10)
        if not alternate:
            app.seed_tables(db)
            db.execute("UPDATE profiles SET label='CampaignPrimary' WHERE id=101")
        else:
            require(db.execute("SELECT count(*) FROM profiles").fetchone()[0] == 1)
            db.execute('''INSERT INTO profiles(id,label,url,last_update_date,overwrite_type,script_id,
                       auto_update_duration_millis,auto_update,selected_map,unfold_set,"order")
                       VALUES(?,?,?,?,?,?,?,?,?,?,?)''',
                       (102,"CampaignAlternate","http://127.0.0.1:18766/alternate.yaml",
                        int(time.time()),"script",201,86400000,0,"{}","[]",1))


def db_record(path, selected=101):
    uri = Path(path).resolve().as_uri()+"?mode=ro"
    with sqlite3.connect(uri, uri=True) as db:
        rows = db.execute("SELECT id,url,script_id,auto_update,last_update_date FROM profiles ORDER BY id").fetchall()
    require(len(rows) in (1,2) and all(r[2] == 201 and r[3] == 0 for r in rows))
    return dict(selected=selected, rows=rows)


class FixtureServer:
    def __init__(self, owner, providers=True):
        self.owner = owner
        self.instances = []
        root = self
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_GET(self):
                if self.path == "/health":
                    body = b"ok"
                elif self.path in ("/profile.yaml", "/alternate.yaml"):
                    import yaml
                    owner.requests[self.path] += 1
                    revision = owner.revision if self.path == "/profile.yaml" else "ALT"
                    body = yaml.safe_dump(owner.profile(revision)).encode()
                elif self.path in {"/rules/"+n+".yaml" for n in MANAGED}:
                    body = b"payload: []\n"
                else:
                    self.send_error(404); return
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers(); self.wfile.write(body)
        for port in ((18765,18766) if providers else (18766,)):
            srv = http.server.ThreadingHTTPServer(("127.0.0.1", port), Handler)
            self.instances.append(srv)
            threading.Thread(target=srv.serve_forever, daemon=True).start()
    def stop(self):
        for srv in self.instances:
            srv.shutdown(); srv.server_close()
        self.instances.clear()

class AppRuntime:
    def __init__(self, root, env, state, node=None):
        self.root, self.env, self.state, self.node = root, env, state, node
        self.binary = root/"route-agent.exe"
        self.executable = Path(state["app"])
        self.core = self.executable.with_name("FlClashCore.exe")
        self.data = Path(state["data"])
        self.secret = state["controller_token"]
        self.pins = state.get("pins", {"capability.route-lab.test":"127.0.0.1"})
        self.revision = "A"
        self.requests = {"/profile.yaml":0, "/alternate.yaml":0}
        self.app_pid = None
        self.owned = [tuple(x) for x in state.get('owned',[])]
        self.fixtures = None
        self.dns = None
        self.connections = []
        self.children = []
        self.output_files = []
        self.observed = 0.0
        self.reservations = 0
        self.attempts = []
        self.cleanup_errors = []
        self.report = dict(schema_version=1, campaign_id=CAMPAIGN,
                           run_id=env["GITHUB_RUN_ID"], attempt=1,
                           source_sha=env["REVIEWED_SHA"],
                           launcher_sha=env.get("LAUNCHER_SHA"),
                           binary_sha256=hashlib.sha256(self.binary.read_bytes()).hexdigest(),
                           app_sha256=APP_HASH, core_sha256=BUNDLED_HASH,
                           scenes={n:dict(status="INCOMPLETE", evidence=[]) for n in SCENES},
                           model_transport_attempts=None, model_transport_attempts_known=False,
                           budget=dict(approved_usd=env["APPROVED_USD"],
                                       provider_cap_usd=env["PROVIDER_CAP_USD"],
                                       verified_remaining_before_usd=env["REMAINING_USD"],
                                       actual_cost_usd=None, remaining_after_usd=None),
                           cleanup={}, result="INCOMPLETE", long_term_stability_tested=False)
        self.report["scenes"]["C00"]["status"] = "PASS"
        self.state["driver_pid"] = os.getpid()
        self.state["driver_executable"] = sys.executable
        self.state["mode"] = "execute" if node is not None else "prepare"
        self.persist()

    def persist(self):
        self.state["owned"] = self.owned
        self.state["publisher_records"] = [
            dict(pid=p.pid,ownership=str(o),journal=str(j),config=str(c),output=str(out))
            for p,o,j,c,out in self.children]
        self.state["attempts"] = self.attempts
        self.state["reservations"] = self.reservations
        (self.root/"private-state.json").write_text(json.dumps(self.state),encoding="utf-8")

    def profile(self, revision):
        return original_profile(revision, self.secret, self.pins, self.node)

    def api(self, path):
        return read_api(9090, path, self.secret)

    def status(self):
        return read_api(18765, "/status")

    def empty(self):
        providers = self.api("/providers/rules")["providers"]
        return all(providers.get(n, {}).get("ruleCount") == 0 for n in MANAGED)

    def start_fixtures(self):
        import dns.message
        import dns.rrset
        import dns.rdatatype
        import dns.rcode
        import socketserver
        owner = self
        class DNSHandler(socketserver.BaseRequestHandler):
            def handle(self):
                data, server = self.request
                request = dns.message.from_wire(data)
                reply = dns.message.make_response(request)
                question = request.question[0]
                host = str(question.name).rstrip(".").lower()
                if host in owner.pins and question.rdtype == dns.rdatatype.A:
                    reply.answer.append(dns.rrset.from_text(question.name,30,"IN","A",owner.pins[host]))
                else:
                    reply.set_rcode(dns.rcode.NXDOMAIN)
                server.sendto(reply.to_wire(), self.client_address)
        self.dns = socketserver.ThreadingUDPServer(("127.0.0.1",15356), DNSHandler)
        threading.Thread(target=self.dns.serve_forever, daemon=True).start()
        self.fixtures = FixtureServer(self)

    def bootstrap(self):
        if self.fixtures:
            self.fixtures.stop()
        self.fixtures = FixtureServer(self)

    def stop_bootstrap(self):
        if self.fixtures:
            self.fixtures.stop()
        self.fixtures = FixtureServer(self, providers=False)

    def remember(self):
        if not self.app_pid:
            return
        for item in guest("children", self.env, self.core, self.app_pid):
            pair = (item["ProcessId"], str(self.core))
            if pair not in self.owned:
                self.owned.append(pair)
        self.persist()

    def launch(self, selected=101):
        verify_settings(self.data/"shared_preferences.json", selected)
        pid = guest("start", self.env, self.executable)["pid"]
        self.app_pid = pid
        self.owned.append((pid,str(self.executable)))
        self.persist()
        wait(lambda: guest("children",self.env,self.core,pid))
        self.remember()
        wait(lambda:self.api("/version"))
        wait(self.empty)
        wait(lambda: powershell(
            "@(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -eq 'RouteAgentCampaign' -and $_.Status -eq 'Up'}).Count | ConvertTo-Json",
            self.env), lambda v:v == 1)
        tun_route(self.env, True)
        self.network_current()
        self.capture("ALT" if selected == 102 else self.revision)
        return pid

    def network_current(self):
        active = snapshot(self.env)
        base = self.state["baseline"]
        require(active["app"]["proxy"] == base["app"]["proxy"]
                and active["app"]["winhttp"] == base["app"]["winhttp"]
                and active["app"]["routes"] == base["app"]["routes"])
        owned = {r["InterfaceIndex"] for r in active["extra"]["owned_devices"]}
        require(len(owned) == 1)
        require([r for r in active["app"]["dns"] if r["InterfaceIndex"] not in owned] == base["app"]["dns"])
        require([r for r in active["app"]["adapters"] if r["Name"] != DEVICE] == base["app"]["adapters"])

    def close_app(self):
        if self.app_pid is None:
            return
        self.remember()
        tun_route(self.env, False)
        pid = self.app_pid
        guest("close",self.env,self.executable,pid)
        wait(lambda:guest("alive",self.env,self.executable,pid),lambda v:not v,20)
        for child, exe in self.owned:
            if exe == str(self.core):
                wait(lambda:guest("alive",self.env,self.core,child),lambda v:not v,20)
        self.app_pid = None
        require(snapshot(self.env) == self.state["baseline"])

    def ui(self, action):
        result = guest(action,self.env,self.executable,self.app_pid,self.root/(action+"-private.json"))
        require(result.get("action") == action and result.get("page_title_verified") == "Profiles")
        # Never publish helper screenshots/tree/private diagnostics.
        return dict(action=action, method=result.get("method"), owned_window_verified=True)

    def capture(self, revision):
        import yaml
        source = self.profile(revision)
        path = self.data/"config.yaml"
        actual = yaml.safe_load(path.read_text(encoding="utf-8"))
        require(isinstance(actual,dict))
        for key in ("dns","proxies","proxy-groups","hosts"):
            require(actual.get(key) == source.get(key))
        for key,value in source["tun"].items():
            require(actual.get("tun",{}).get(key) == value)
        expected = source["rules"][:-1] + [
            "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-direct)),DIRECT",
            "AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,route-agent-tail-proxy)),LAB",
            source["rules"][-1]]
        require(actual["rules"] == expected)
        for name in MANAGED:
            p = actual["rule-providers"][name]
            require(p["type"] == "http" and p["behavior"] == "classical" and p["format"] == "yaml"
                    and p["proxy"] == "DIRECT" and p["url"] == f"http://127.0.0.1:18765/rules/{name}.yaml")
        require(self.api("/configs")["mode"] == "rule" and self.api("/configs")["tun"]["enable"] is True)
        rules = self.api("/rules")["rules"]
        original = [("Domain", r.split(",")[1],"DIRECT") for r in source["rules"][:-1]]
        managed = [("AND","((Network,tcp) && (DstPort,443) && (RuleSet,"+n+"))",v)
                   for n,v in zip(MANAGED,("DIRECT","LAB"))] + [("Match","","LAB")]
        require([(r["type"],r["payload"],r["proxy"]) for r in rules] == original+managed)
        return dict(config_sha256=hashlib.sha256(path.read_bytes()).hexdigest(),
                    rules_sha256=hashlib.sha256(json.dumps(rules,sort_keys=True).encode()).hexdigest(),
                    revision=revision)

    def config(self, slot, host_index, maintenance=None, stub=False):
        path = self.root/f"slot-{slot}-agent.json"
        config = dict(mode="async", judge="stub" if stub else "jev",
                      controller="http://127.0.0.1:9090", http_listen="127.0.0.1:18765",
                      proxy_group="LAB", api_proxy="http://127.0.0.1:17891",
                      preflight_ms=10000, dns_deadline_ms=11000, learned_ttl_seconds=60,
                      publish_interval_seconds=30, capacity=2, max_api_requests=1,
                      observation=dict(dns="1.1.1.1:53",proxy="http://127.0.0.1:17891",
                                       hosts=[TARGETS[host_index]],attempt_timeout_ms=1000))
        if maintenance:
            config["maintenance"] = maintenance
        serialized = json.dumps(config)
        if path.exists():require(path.read_text(encoding="utf-8") == serialized)
        else:path.write_text(serialized,encoding="utf-8")
        return path

    def prepare_owner(self, config, name):
        ownership, state = self.root/(name+"-ownership.json"), self.root/(name+"-journal.json")
        env = child_env(self.env)
        env["MIHOMO_SECRET"] = self.secret
        args = [str(self.binary),"prepare-apply","--config",str(config),"--exclusive-controller",
                "--profile",str(self.data/"config.yaml"),"--output",str(ownership)]
        result = subprocess.run(args,env=env,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL,timeout=20)
        require(result.returncode == 0)
        return ownership, state

    def pause(self, ownership, process):
        token = json.loads(ownership.read_text())["pause_token"]
        req = urllib.request.Request("http://127.0.0.1:18765/control/pause",data=b"",
                                     headers={"Authorization":"Bearer "+token},method="POST")
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), learning.NoRedirect())
        with opener.open(req,timeout=25) as response:
            value = json.load(response)
        require(value.get("owned_providers_empty") is True and self.empty())

    def stop_session(self, process, ownership, state, output, phase):
        require(process.wait(timeout=90) == 0)
        summary = learning.final_summary(output)
        require(summary["observer_stopped"] and json.loads(state.read_text())["phase"] == "stopped" and self.empty())
        self.report["scenes"]["C09"].update(status="PASS")
        self.report["scenes"]["C09"]["evidence"].append(dict(slot=phase,normal_exit=True,empty_ack_readback=True))
        self.bootstrap()

    def receipt(self, status, first_id, decision):
        import re
        rows = status.get("decisions")
        require(type(rows) is list and len(rows) == 1)
        r = rows[0]
        require(r.get("connection_id") == first_id and r.get("decision") == decision
                and r.get("policy_adopted") is True and r.get("policy_reason") == "accepted"
                and r.get("status") == "applied" and r.get("policy_version") == "route-policy-v1")
        require(re.fullmatch("[0-9a-f]{32}",r.get("id","")) is not None
                and re.fullmatch("[0-9a-f]{64}",r.get("scope","")) is not None
                and re.fullmatch("[0-9a-f]{32}-[1-9][0-9]*",r.get("publication_id","")) is not None)
        require(datetime.datetime.fromisoformat(r["expires_at"].replace("Z","+00:00")).timestamp() > time.time())
        return dict(decision_id=r["id"], publication_id=r["publication_id"], scope=r["scope"])

    def learning_session(self, slot, index, decision, scene, maintenance=None, crash=False):
        clean_budget_scope(self.env)
        require(self.reservations < MAX_ATTEMPTS and self.observed < 12*60)
        config = self.config(slot,index,maintenance)
        ownership, state = self.prepare_owner(config,"slot-"+str(slot))
        self.stop_bootstrap()
        env = child_env(self.env,model=True); env["MIHOMO_SECRET"] = self.secret
        output = self.root/f"slot-{slot}-final.json"
        error = self.root/f"slot-{slot}-private.stderr"
        out_file, err_file = output.open("xb"), error.open("xb")
        self.output_files += [out_file,err_file]
        duration = 130 if maintenance else 70
        args = [str(self.binary),"observe-apply","--config",str(config),"--allow-external-probes",
                "--allow-model-api","--allow-controlled-apply","--exclusive-controller",
                "--ownership",str(ownership),"--state-file",str(state),
                "--direct-interface-index",self.env["DIRECT_INTERFACE_INDEX"],"--run-for",str(duration)+"s"]
        self.reservations += 1
        self.persist()
        process = subprocess.Popen(args,env=env,stdin=subprocess.DEVNULL,stdout=out_file,stderr=err_file,
                                   creationflags=getattr(subprocess,"CREATE_NO_WINDOW",0))
        self.children.append((process,ownership,state,config,output))
        self.persist()
        started = time.monotonic()
        active_scene = scene
        try:
            wait(self.status,lambda v:v["observer_ready"],process=process,seconds=25)
            if maintenance:
                wait(self.status,lambda v:v["maintenance"] == "paused_empty",seconds=70,process=process)
                require(self.empty())
                wait(self.status,lambda v:v["observer_ready"] and v["maintenance"] == "scheduled",seconds=30,process=process)
            first = connect(TARGETS[index]); self.connections.append(first)
            first_record = connection(self.api("/connections"),first,TARGETS[index])
            require(first_record["rule"] == "Match" and first_record.get("rulePayload","") == ""
                    and first_record["chains"][-1:] == ["LAB"])
            require(all(n in {"DIRECT","LAB","acceptance-vless"} for n in first_record["chains"]))
            status = wait(self.status,lambda v:v.get("evaluation") is not None,seconds=15,process=process)
            diagnostic = learning.diagnostic(status["evaluation"])
            self.attempts.append(dict(slot=slot,known=diagnostic["transport_attempts_known"],
                                      count=diagnostic["transport_attempts"] if diagnostic["transport_attempts_known"] else None))
            self.persist()
            # Retain the strictly allowlisted diagnostic even on refusal/failure.
            self.report["scenes"][scene]["evidence"].append(dict(
                slot=slot,target_index=index,first_connection_id=first_record["id"],
                first_fallback_rule="Match",first_fallback_chain=first_record["chains"],
                evaluation=diagnostic,model_http_status=diagnostic.get("model_http_status"),
                policy_accepted=diagnostic["policy_reason"] == "accepted",
                publication_attempted=status.get("commits",0) > 0))
            header = receipt_header(status,first_record["id"])
            self.report["scenes"][scene]["evidence"][-1]["judgment"] = header
            require(diagnostic.get("model_http_status") == 200)
            require(diagnostic["transport_attempts_known"] and diagnostic["transport_attempts"] == 1)
            require(diagnostic["connection_target_match"] and diagnostic["stage_error"] == "none"
                    and diagnostic["policy_reason"] == "accepted" and diagnostic["decision"] == decision)
            if decision == "PROXY":
                require(diagnostic["evidence"] == dict(direct_tls="repeated_failure",proxy_tls="verified_success"))
            else:
                require(diagnostic["evidence"]["direct_tls"] == "verified_success")
            if slot == 1:
                self.report["scenes"]["C01"].update(status="PASS",evidence=[dict(evaluation=diagnostic)])
                active_scene = "C02"
            before = learning.publication_schedule(status)
            require(before["nonempty_submissions"] == 0 and status["commits"] == 0 and self.empty())
            if not maintenance:
                require(time.time()*1000 < before["first_due_unix_ms"])
            status = wait(self.status,lambda v:v["commits"] == 1,seconds=45,process=process)
            after = learning.publication_schedule(status)
            require(after["nonempty_submissions"] == 1
                    and after["first_nonempty_submission_unix_ms"] >= after["first_due_unix_ms"])
            correlation = self.receipt(status,first_record["id"],decision)
            if slot == 1:
                self.report["scenes"]["C02"].update(status="PASS",evidence=[dict(publication=correlation,schedule=after)])
                active_scene = "C03"
            later = connect(TARGETS[index]); self.connections.append(later)
            snapshot_value = self.api("/connections")
            later_record = connection(snapshot_value,later,TARGETS[index])
            unchanged = connection(snapshot_value,first,TARGETS[index])
            same_target(first_record,later_record,self.pins[TARGETS[index]])
            provider = MANAGED[0 if decision == "DIRECT" else 1]
            expected = "((Network,tcp) && (DstPort,443) && (RuleSet,"+provider+"))"
            require(later_record["rule"] == "AND" and later_record["rulePayload"] == expected
                    and later_record["chains"][-1:] == (["DIRECT"] if decision == "DIRECT" else ["LAB"]))
            require(all(n in {"DIRECT","LAB","acceptance-vless"} for n in later_record["chains"]+first_record["chains"]))
            require(unchanged["id"] == first_record["id"] and unchanged["rule"] == "Match"
                    and unchanged["chains"] == first_record["chains"])
            counts = self.api("/providers/rules")["providers"]
            require(counts[provider]["ruleCount"] == 1 and counts[MANAGED[1 if decision == "DIRECT" else 0]]["ruleCount"] == 0)
            evidence = dict(slot=slot,target_index=index,target_identity_sha256=hashlib.sha256(self.pins[TARGETS[index]].encode()).hexdigest(),
                            first_connection_id=first_record["id"],later_connection_id=later_record["id"],
                            payload=expected,chain=later_record["chains"],inbound="Tun",
                            transferred=dict(upload=later_record["upload"],download=later_record["download"]),
                            evaluation=diagnostic,publication=correlation,
                            provider_nonempty_ack_readback=True,preperiod_nonempty_submissions=0,
                            publication_schedule=after,config=self.capture("ALT" if slot >= 4 else self.revision))
            self.report["scenes"][scene]["evidence"].append(evidence)
            if slot == 1:
                for name in ("C01","C02","C03"):
                    self.report["scenes"][name].update(status="PASS")
                    if name != scene:self.report["scenes"][name]["evidence"].append(evidence)
            self.report["scenes"]["C07"]["evidence"].append(dict(decision=decision,**correlation,
                inbound="Tun",connection_id=later_record["id"],transferred=evidence["transferred"]))
            if crash:
                active_scene = "C11"
                require(not self.empty())
                process.kill(); require(process.wait(timeout=10) is not None)
                require(not self.empty()) # Cache residue is a distinct fault observation.
                recovery_env = child_env(self.env); recovery_env["MIHOMO_SECRET"] = self.secret
                result = subprocess.run([str(self.binary),"recover-apply","--config",str(config),
                    "--allow-owned-recovery","--exclusive-controller","--ownership",str(ownership),
                    "--state-file",str(state)],env=recovery_env,stdin=subprocess.DEVNULL,
                    stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30)
                require(result.returncode == 0 and self.empty()
                        and json.loads(state.read_text())["phase"] == "recovered_stopped")
                self.report["scenes"]["C11"].update(status="PASS",evidence=[dict(
                    publisher_exit_confirmed=True,cache_residue_observed=True,
                    recovery_empty_ack_readback=True,learning_restarted=False)])
                self.bootstrap()
            else:
                if slot == 1:
                    # Two periods and real expiry; read-only polling, no new external request.
                    wait(self.status,lambda v:time.time()*1000 >= after["first_due_unix_ms"]+32000,
                         seconds=40,process=process)
                    stable = self.status()
                    require(stable["commits"] == 1 and learning.publication_schedule(stable)["nonempty_submissions"] == 1)
                    wait(self.empty,seconds=10,process=process)
                    self.report["scenes"]["C08"]["evidence"].append(dict(noop_tick_no_resubmit=True,real_expiry_empty=True))
                active_scene = "C09"
                self.stop_session(process,ownership,state,output,slot)
            if maintenance:
                self.report["scenes"]["C10"].update(status="PASS",evidence=[dict(
                    window=maintenance,paused_empty=True,fresh_after_resume=True,**correlation)])
            self.report["scenes"][scene]["status"] = "PASS"
            return ownership,config
        except Exception:
            self.report["scenes"][active_scene]["status"] = "NO-GO"
            if process.poll() is None:
                try:self.pause(ownership,process)
                except Exception:pass
            raise
        finally:
            self.observed += time.monotonic()-started
            out_file.flush(); err_file.flush()
            for sock in self.connections:
                try:sock.close()
                except Exception:pass
            self.connections.clear()

def prepare(root, env):
    """All capability work is guest-only, gated, and receives no application Secrets."""
    import ctypes
    require(ctypes.windll.shell32.IsUserAnAdmin() != 0)
    require(not root.exists())
    root.mkdir()
    baseline = snapshot(env)
    require(not baseline["extra"]["owned_routes"] and not baseline["extra"]["owned_devices"] and not baseline["extra"]["fault_rules"])
    (root/"owned-marker.json").write_text(json.dumps(dict(campaign_id=CAMPAIGN,run_id=env["GITHUB_RUN_ID"],source_sha=env["REVIEWED_SHA"],baseline=baseline)),encoding="utf-8")
    guest("clean",env)
    require(guest("preflight",env).get("ready") is True)
    archive = root/"official-app.zip"
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}),OfficialAssetRedirect())
    with opener.open(app.plan()["archive_url"],timeout=60) as response, archive.open("xb") as out:
        total = 0
        while True:
            block = response.read(1<<20)
            if not block:break
            total += len(block); require(total <= 70_000_000)
            out.write(block)
    require(hashlib.sha256(archive.read_bytes()).hexdigest() == app.ARCHIVE_SHA256)
    unpack = root/"app"
    with zipfile.ZipFile(archive) as bundle:
        require(all((unpack/n).resolve().is_relative_to(unpack.resolve()) for n in bundle.namelist()))
        bundle.extractall(unpack)
    matches = list(unpack.rglob("FlClash.exe"))
    require(len(matches) == 1)
    executable = matches[0]
    require(hashlib.sha256(executable.read_bytes()).hexdigest() == APP_HASH)
    require(hashlib.sha256(executable.with_name("FlClashCore.exe").read_bytes()).hexdigest() == BUNDLED_HASH)
    result = command(["go","build","-trimpath","-o",str(root/"route-agent.exe"),"./cmd/route-agent"],env,120)
    metadata = guest("metadata",env,executable)
    data = Path(metadata["roaming"])/metadata["company"]/metadata["product"]
    require(metadata["company"] == "com.follow" and metadata["product"] == "clash" and not data.exists())
    data.mkdir(parents=True)
    state = dict(version=1,campaign_id=CAMPAIGN,run_id=env["GITHUB_RUN_ID"],source_sha=env["REVIEWED_SHA"],app=str(executable),data=str(data),
                 controller_token=secrets.token_urlsafe(32),baseline=baseline,capability_passed=False,
                 product_sha256=hashlib.sha256((root/"route-agent.exe").read_bytes()).hexdigest())
    (root/"private-state.json").write_text(json.dumps(state),encoding="utf-8")
    # Initial empty app bootstrap keeps TUN off. It only creates the actual DB schema.
    (data/"shared_preferences.json").write_text(json.dumps(app.preferences()),encoding="utf-8")
    owned = []
    runtime = None
    echo = None
    try:
        pid = guest("start",env,executable)["pid"]; owned.append((pid,str(executable)))
        state["owned"] = owned
        (root/"private-state.json").write_text(json.dumps(state),encoding="utf-8")
        wait(lambda:(data/"database.sqlite").exists())
        wait(lambda:guest("children",env,executable.with_name("FlClashCore.exe"),pid))
        children = guest("children",env,executable.with_name("FlClashCore.exe"),pid)
        owned += [(p["ProcessId"],str(executable.with_name("FlClashCore.exe"))) for p in children]
        state["owned"] = owned
        (root/"private-state.json").write_text(json.dumps(state),encoding="utf-8")
        ui = guest("profiles",env,executable,pid,root/"blank-ui-private.json")
        require(ui.get("page_title_verified") == "Profiles")
        guest("close",env,executable,pid)
        for p,exe in owned:wait(lambda p=p,exe=exe:guest("alive",env,exe,p),lambda v:not v,20)
        require(snapshot(env) == baseline)
        seed(data/"database.sqlite")
        (data/"profiles").mkdir(exist_ok=True); (data/"scripts").mkdir(exist_ok=True)
        import yaml
        capability = original_profile("A",state["controller_token"],
                                      {"capability.route-lab.test":"127.0.0.1"},None)
        (data/"profiles"/"101.yaml").write_text(yaml.safe_dump(capability),encoding="utf-8")
        (data/"scripts"/"201.js").write_text(script(),encoding="utf-8")
        (data/"shared_preferences.json").write_text(json.dumps(prefs(101)),encoding="utf-8")
        import socketserver
        class Echo(socketserver.BaseRequestHandler):
            def handle(self):
                self.request.settimeout(15)
                try:
                    while True:
                        value = self.request.recv(512)
                        if not value:break
                        self.request.sendall(value)
                except OSError:pass
        echo = socketserver.ThreadingTCPServer(("127.0.0.1",443),Echo)
        threading.Thread(target=echo.serve_forever,daemon=True).start()
        runtime = AppRuntime(root,env,state)
        runtime.start_fixtures()
        runtime.launch(101)
        probe = connect("capability.route-lab.test",False)
        try:
            probe.sendall(b"owned-campaign-tun-capability")
            require(probe.recv(64) == b"owned-campaign-tun-capability")
            record = connection(runtime.api("/connections"),probe,"capability.route-lab.test")
            require(record["metadata"].get("type") == "Tun")
        finally:probe.close()
        runtime.close_app()
        state["capability_passed"] = True
        state["capability_evidence"] = dict(actual_native_tun_inbound=True,echo_bytes=29,
                                          synthetic_only=True,real_learning=False)
        (root/"private-state.json").write_text(json.dumps(state),encoding="utf-8")
    finally:
        if runtime:
            if runtime.fixtures:runtime.fixtures.stop()
            if runtime.dns:runtime.dns.shutdown();runtime.dns.server_close()
            if runtime.app_pid:
                try:runtime.close_app()
                except Exception:
                    for p,exe in runtime.owned:
                        try:
                            if guest("alive",env,exe,p):guest("terminate",env,exe,p)
                        except Exception:pass
        if echo:echo.shutdown();echo.server_close()
        # Independently clean partial blank launches as well as capability launches.
        for p,exe in owned:
            try:
                if guest("alive",env,exe,p):guest("terminate",env,exe,p)
            except Exception:pass
        tun_route(env,False)
    require(state["capability_passed"] and snapshot(env) == baseline)
    print(json.dumps(dict(campaign_id=CAMPAIGN,stage="capability_prepared",
                          source_sha=env["REVIEWED_SHA"],binary_sha256=state["product_sha256"],
                          app_sha256=APP_HASH,core_sha256=BUNDLED_HASH,
                          real_learning_tested=False,model_transport_attempts=0,
                          actual_synthetic_tun_capability=True),separators=(",",":")))


def firewall(env, binary, core, address, enable):
    require(ipaddress.ip_address(address).is_global)
    # These paths are generated from the fixed owned extraction/build roots.
    for item in (binary,core):
        require("'" not in str(item))
    if enable:
        script_value = ("$ErrorActionPreference='Stop'; "
            "if(@(Get-NetFirewallRule -Group 'RouteAgentCampaign' -ErrorAction SilentlyContinue).Count -ne 0){throw 'Fault group exists'}; ")
        for i,item in enumerate((binary,core)):
            script_value += ("New-NetFirewallRule -DisplayName 'RouteAgentCampaign-"+str(i)+
                "' -Group 'RouteAgentCampaign' -Direction Outbound -Action Block -Protocol TCP "
                "-RemotePort 443 -RemoteAddress '"+address+"' -Program '"+str(item)+"' | Out-Null; ")
        return powershell(script_value+"'true'",env)
    return powershell("$ErrorActionPreference='Stop'; Get-NetFirewallRule -Group 'RouteAgentCampaign' "
                      "-ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction Stop; 'true'",env)


def budget_metadata(runtime):
    # Exactly one GET before paid calls and one only after all scenes succeed.
    # No list/create/update/reset endpoint is used; no raw response is retained.
    from decimal import Decimal
    request = urllib.request.Request("https://openrouter.ai/api/v1/key",
            headers={"Authorization":"Bearer "+runtime.env["OPENROUTER_API_KEY"]})
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({"https":"http://127.0.0.1:17891"}),learning.NoRedirect())
    runtime.report["budget"].setdefault("metadata_get_attempts",0)
    runtime.report["budget"]["metadata_get_attempts"] += 1
    with opener.open(request,timeout=15) as response:
        raw = response.read(16385)
    require(len(raw) <= 16384)
    data = a.strict_json(raw.decode())["data"]
    # A numeric dedicated account/key cap is mandatory; never replace/reset it.
    require(data.get("limit_reset") is None)
    result = {k:Decimal(str(data[k])) for k in ("limit","limit_remaining","usage")}
    require(all(v.is_finite() and v >= 0 for v in result.values()))
    require(result["limit"] == Decimal(runtime.env["PROVIDER_CAP_USD"]))
    return dict(limit=str(result["limit"]),remaining=str(result["limit_remaining"]),
                usage=str(result["usage"]),observed_at=datetime.datetime.now(datetime.timezone.utc).isoformat())


def execute(root, env):
    import yaml
    require((root/"private-state.json").is_file())
    state = json.loads((root/"private-state.json").read_text(encoding="utf-8"))
    require(state["source_sha"] == env["REVIEWED_SHA"] and state["capability_passed"])
    require(hashlib.sha256((root/"route-agent.exe").read_bytes()).hexdigest() == state["product_sha256"])
    require(snapshot(env) == state["baseline"])
    node = a.node_input(env.get("VLESS_NODE_JSON",""))
    require(env.get("APPROVED_NODE_ENDPOINT") == node["server"]+":"+str(node["port"]))
    key = env.get("OPENROUTER_API_KEY","")
    require(0 < len(key) <= 4096 and all(33 <= ord(c) <= 126 for c in key))
    state["pins"] = {host:public_address(host,env) for host in TARGETS}
    require(node["server"] not in state["pins"].values())
    runtime = AppRuntime(root,env,state,node)
    start = datetime.datetime.now(datetime.timezone.utc)
    maintenance_start = (start+datetime.timedelta(minutes=8)).replace(second=0,microsecond=0)
    maintenance = dict(start=maintenance_start.strftime("%H:%M"),timezone="UTC",
                       duration_seconds=12,resume="unchanged")
    fault_enabled = False
    scene = "C01"
    old_comparison = None
    successful = False
    try:
        # These profile revisions and agent configurations are all generated and
        # hashed before any model request. Later generation changes are declared.
        profiles = {r:runtime.profile(r) for r in ("A","B","ALT")}
        for revision,profile in profiles.items():
            (root/("profile-"+revision+"-private.json")).write_text(json.dumps(profile),encoding="utf-8")
        for slot,index in ((1,0),(2,1),(3,1),(4,0),(5,0),(6,0)):
            runtime.config(slot,index,maintenance if slot == 5 else None)
        runtime.config(99,0,stub=True)
        (runtime.data/"profiles"/"101.yaml").write_text(yaml.safe_dump(profiles["A"]),encoding="utf-8")
        (runtime.data/"shared_preferences.json").write_text(json.dumps(prefs(101)),encoding="utf-8")
        runtime.start_fixtures()
        runtime.launch(101)
        before = budget_metadata(runtime)
        from decimal import Decimal
        require(Decimal(before["remaining"]) == Decimal(env["REMAINING_USD"]))
        runtime.report["budget"]["before"] = before
        runtime.report["manifest"] = dict(
            profiles={r:hashlib.sha256(json.dumps(p,sort_keys=True).encode()).hexdigest() for r,p in profiles.items()},
            agent_configs={str(i):hashlib.sha256((root/f"slot-{i}-agent.json").read_bytes()).hexdigest() for i in (*range(1,7),99)},
            maintenance=maintenance,one_product_binary=True,one_official_app_core_pair=True)
        first_owner,first_config = runtime.learning_session(1,0,"DIRECT","C01")
        scene = "C04"
        runtime.learning_session(2,1,"PROXY","C04")
        if env.get("APPROVED_GUEST_FIREWALL") == "true":
            scene = "C05"
            firewall(env,runtime.binary,runtime.core,state["pins"][TARGETS[1]],True)
            fault_enabled = True
            try:
                runtime.learning_session(3,1,"PROXY","C05")
                runtime.report["scenes"]["C05"]["controlled_failure_only"] = True
                runtime.report["scenes"]["C05"]["natural_proxy_need_proved"] = False
            finally:
                firewall(env,runtime.binary,runtime.core,state["pins"][TARGETS[1]],False)
                fault_enabled = False
        scene = "C06"
        # Fresh comparison ownership is obtained while A is current and drained.
        old_comparison,_ = runtime.prepare_owner(first_config,"pre-refresh-comparison")
        runtime.ui("profiles")
        runtime.revision = "B"
        for round_index in range(2):
            prior = db_record(runtime.data/"database.sqlite")
            prior_requests = dict(runtime.requests)
            wait(lambda:int(time.time()) > max(row[4] for row in prior["rows"]),seconds=3)
            runtime.ui("update") # Exactly one input; only readback is polled.
            def refreshed():
                saved = db_record(runtime.data/"database.sqlite")
                require(saved["rows"][0][4] > prior["rows"][0][4]
                        and runtime.requests["/profile.yaml"] > prior_requests["/profile.yaml"])
                return runtime.capture("B")
            wait(refreshed)
        # Add the already declared alternate profile only with the app stopped.
        runtime.close_app()
        seed(runtime.data/"database.sqlite",True)
        (runtime.data/"profiles"/"102.yaml").write_text(yaml.safe_dump(profiles["ALT"]),encoding="utf-8")
        runtime.launch(101); runtime.ui("profiles")
        runtime.ui("alternate")
        switched = wait(lambda:runtime.capture("ALT"))
        saved_db = db_record(runtime.data/"database.sqlite",102)
        request_count = dict(runtime.requests)
        runtime.close_app()
        verify_settings(runtime.data/"shared_preferences.json",102)
        runtime.launch(102)
        require(db_record(runtime.data/"database.sqlite",102) == saved_db and runtime.requests == request_count)
        require(runtime.capture("ALT") == switched)
        # Test old-generation refusal with a stub that cannot emit valid rules.
        negative_config = runtime.config(99,0,stub=True)
        rejected_env = child_env(env); rejected_env["MIHOMO_SECRET"] = runtime.secret
        negative = subprocess.run([str(runtime.binary),"observe-apply","--config",str(negative_config),
                "--allow-lab-fixtures","--allow-external-probes","--allow-controlled-apply",
                "--exclusive-controller","--ownership",str(old_comparison),"--state-file",str(root/"old-owner-journal.json"),
                "--direct-interface-index",env["DIRECT_INTERFACE_INDEX"],"--run-for","1s"],
                env=rejected_env,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=25)
        require(negative.returncode != 0 and
                (b"effective_config_changed" in negative.stderr or b"core_process_restarted" in negative.stderr))
        runtime.learning_session(4,0,"DIRECT","C06")
        runtime.report["scenes"]["C06"]["evidence"].append(dict(
            actual_gui_refreshes=2,actual_gui_profile_switch=True,restart_persistence=True,
            old_generation_refused=True,refresh_after_verified_drain=True))
        scene = "C10"
        require(datetime.datetime.now(datetime.timezone.utc) < maintenance_start-datetime.timedelta(seconds=10))
        while datetime.datetime.now(datetime.timezone.utc) < maintenance_start-datetime.timedelta(seconds=45):
            clean_budget_scope(env);time.sleep(.25)
        runtime.learning_session(5,0,"DIRECT","C10",maintenance=maintenance)
        scene = "C11"
        runtime.learning_session(6,0,"DIRECT","C11",crash=True)
        runtime.report["scenes"]["C07"]["status"] = "PASS"
        # Existing same-source focused tests are deterministic fault evidence,
        # separately labeled; they do not stand in for real learning or TUN.
        guards = command(["go","test","./internal/route","-run",
                         "TestScheduledExpiryRejectionAndVersionChangesDoNotBecomeRules|TestControlledDiagnosticRejectsLateModelWithoutPublication",
                         "-count=1"],env,60)
        runtime.report["scenes"]["C08"].update(status="PASS")
        runtime.report["scenes"]["C08"]["evidence"].append(dict(
            policy_late_version_fault_guards="offline_deterministic_same_source",
            old_app_generation_refused_live=True))
        after = budget_metadata(runtime)
        require(Decimal(after["usage"]) >= Decimal(before["usage"]))
        delta = Decimal(after["usage"])-Decimal(before["usage"])
        require(delta <= Decimal(env["APPROVED_USD"]))
        runtime.report["budget"].update(after=after,observed_usage_delta_usd=str(delta),
                                       remaining_after_usd=after["remaining"])
        successful = True
    except Exception:
        if runtime.report["scenes"][scene]["status"] != "PASS":
            runtime.report["scenes"][scene]["status"] = "NO-GO"
        # No new external request, including a billing GET, after a failed scene.
    finally:
        def clean(name, action):
            try:
                action();runtime.report["cleanup"][name] = "verified"
            except Exception:
                runtime.report["cleanup"][name] = "failed"
        for sock in runtime.connections:
            clean("connections",sock.close)
        for process,ownership,journal,config,output in runtime.children:
            def child_stop(process=process,ownership=ownership,journal=journal,config=config):
                if process.poll() is None:
                    try:runtime.pause(ownership,process)
                    except Exception:pass
                    try:require(process.wait(timeout=90) == 0)
                    except Exception:
                        a.stop_owned(process)
                        runtime.report["forced_cleanup"] = True
                require(process.poll() is not None)
                if journal.exists() and json.loads(journal.read_text()).get("phase") in ("stopped","recovered_stopped"):
                    return
                # Only confirmed exit may enter the production owned-empty recovery.
                if runtime.fixtures:runtime.fixtures.stop()
                recovery_env = child_env(env);recovery_env["MIHOMO_SECRET"] = runtime.secret
                recovery = subprocess.run([str(runtime.binary),"recover-apply","--config",str(config),
                    "--allow-owned-recovery","--exclusive-controller","--ownership",str(ownership),
                    "--state-file",str(journal)],env=recovery_env,stdin=subprocess.DEVNULL,
                    stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30)
                require(recovery.returncode == 0 and runtime.empty())
            clean("publisher-"+str(process.pid),child_stop)
        clean("firewall",lambda:firewall(env,runtime.binary,runtime.core,state["pins"][TARGETS[1]],False))
        clean("routes",lambda:tun_route(env,False))
        clean("app_normal_exit",runtime.close_app)
        # Independently stop only recorded app/core paths if normal exit failed.
        for pid,exe in runtime.owned:
            def retained_stop(pid=pid,exe=exe):
                if guest("alive",env,exe,pid):
                    guest("terminate",env,exe,pid)
                    wait(lambda:guest("alive",env,exe,pid),lambda v:not v,15)
                    runtime.report["forced_cleanup"] = True
            clean("owned-process-"+str(pid),retained_stop)
        if runtime.fixtures:clean("http_servers",runtime.fixtures.stop)
        if runtime.dns:
            clean("dns_server",lambda:(runtime.dns.shutdown(),runtime.dns.server_close()))
        for output in runtime.output_files:clean("private_handles",output.close)
        clean("network",lambda:require(snapshot(env) == state["baseline"]))
        def ports_free():
            for port in (443,9090,17891,18765,18766,15354,15356):
                for kind in (socket.SOCK_STREAM,socket.SOCK_DGRAM):
                    with socket.socket(socket.AF_INET,kind) as sock:
                        if hasattr(socket,"SO_EXCLUSIVEADDRUSE"):sock.setsockopt(socket.SOL_SOCKET,socket.SO_EXCLUSIVEADDRUSE,1)
                        sock.bind(("127.0.0.1",port))
        clean("ports",ports_free)
        def private_remove():
            allowed_data = Path(os.environ["APPDATA"])/"com.follow"/"clash"
            require(runtime.data.resolve() == allowed_data.resolve() and root.parent.resolve() == Path(env["RUNNER_TEMP"]).resolve())
            if runtime.data.exists():shutil.rmtree(runtime.data)
            shutil.rmtree(root)
            require(not root.exists() and not runtime.data.exists())
        clean("private_files",private_remove)
        known = len(runtime.attempts) == runtime.reservations and all(x["known"] for x in runtime.attempts)
        runtime.report["model_transport_attempts_known"] = known
        runtime.report["model_transport_attempts"] = sum(x["count"] for x in runtime.attempts) if known else None
        runtime.report["known_attempt_observations"] = runtime.attempts
        runtime.report["model_slots_reserved"] = runtime.reservations
        runtime.report["observation_seconds"] = round(runtime.observed,3)
        cleanup_ok = all(v == "verified" for v in runtime.report["cleanup"].values())
        runtime.report["scenes"]["C12"]["status"] = "PASS" if cleanup_ok else "INCOMPLETE"
        required = [n for n in SCENES if n != "C05" or env.get("APPROVED_GUEST_FIREWALL") == "true"]
        passed = successful and known and cleanup_ok and not runtime.report.get("forced_cleanup") and all(
            runtime.report["scenes"][n]["status"] == "PASS" for n in required)
        runtime.report["result"] = "PASS" if passed else ("NO-GO" if any(
            runtime.report["scenes"][n]["status"] == "NO-GO" for n in required) else "INCOMPLETE")
        public = Path(env["RUNNER_TEMP"])/("campaign-public-"+env["GITHUB_RUN_ID"]+".json")
        public.write_text(json.dumps(runtime.report,indent=2),encoding="utf-8")
        print(json.dumps(runtime.report,separators=(",",":")),flush=True)
    return runtime.report["result"]


def cleanup(root, env):
    """Independent final step: no model/node Secrets, no external service call."""
    public = Path(env["RUNNER_TEMP"])/("campaign-public-"+env["GITHUB_RUN_ID"]+".json")
    if not root.exists():
        require(public.exists())
        previous = json.loads(public.read_text(encoding="utf-8"))
        print(json.dumps(dict(campaign_id=CAMPAIGN,stage="independent_cleanup",
                              primary_result=previous.get("result"),private_root_absent=True)))
        return 0 if all(v == "verified" for v in previous.get("cleanup",{}).values()) else 1
    marker = json.loads((root/"owned-marker.json").read_text(encoding="utf-8"))
    require(marker["campaign_id"] == CAMPAIGN and marker["run_id"] == env["GITHUB_RUN_ID"]
            and marker["source_sha"] == env["REVIEWED_SHA"]
            and root.parent.resolve() == Path(env["RUNNER_TEMP"]).resolve())
    state_path = root/"private-state.json"
    state = json.loads(state_path.read_text(encoding="utf-8")) if state_path.exists() else {}
    checks = {}
    forced = False
    def attempt(name, callback):
        try:callback();checks[name] = "verified"
        except Exception:checks[name] = "failed"
    driver = state.get("driver_pid")
    driver_exe = state.get("driver_executable")
    if driver and driver != os.getpid() and driver_exe:
        def stop_driver():
            if guest("alive",env,driver_exe,int(driver)):
                require(str(driver_exe) == sys.executable)
                guest("terminate",env,driver_exe,int(driver))
                wait(lambda:guest("alive",env,driver_exe,int(driver)),lambda v:not v,10)
        attempt("driver_exit",stop_driver)
    binary = root/"route-agent.exe"
    for record in state.get("publisher_records",[]):
        def stop_and_recover(record=record):
            nonlocal forced
            pid = int(record["pid"])
            require(all(Path(record[k]).resolve().is_relative_to(root.resolve())
                        for k in ("ownership","journal","config","output")))
            if guest("alive",env,binary,pid):
                guest("terminate",env,binary,pid);forced=True
                wait(lambda:guest("alive",env,binary,pid),lambda v:not v,10)
            require(not guest("alive",env,binary,pid))
            journal = Path(record["journal"])
            if journal.exists() and json.loads(journal.read_text()).get("phase") in ("stopped","recovered_stopped"):
                return
            child = child_env(env);child["MIHOMO_SECRET"] = state["controller_token"]
            result = subprocess.run([str(binary),"recover-apply","--config",record["config"],
                "--allow-owned-recovery","--exclusive-controller","--ownership",record["ownership"],
                "--state-file",record["journal"]],env=child,stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=30)
            require(result.returncode == 0)
        attempt("publisher-"+str(record["pid"]),stop_and_recover)
    attempt("firewall",lambda:powershell(
        "$ErrorActionPreference='Stop'; Get-NetFirewallRule -Group 'RouteAgentCampaign' "
        "-ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction Stop; 'true'",env))
    attempt("routes",lambda:tun_route(env,False))
    for pid,exe in state.get("owned",[]):
        def stop_recorded(pid=int(pid),exe=str(exe)):
            nonlocal forced
            require(Path(exe).resolve().is_relative_to((root/"app").resolve()))
            if guest("alive",env,exe,pid):
                guest("terminate",env,exe,pid);forced=True
                wait(lambda:guest("alive",env,exe,pid),lambda v:not v,10)
        attempt("owned-"+str(pid),stop_recorded)
    attempt("network",lambda:require(snapshot(env) == marker["baseline"]))
    def free_ports():
        for port in (443,9090,17891,18765,18766,15354,15356):
            for kind in (socket.SOCK_STREAM,socket.SOCK_DGRAM):
                with socket.socket(socket.AF_INET,kind) as sock:
                    if hasattr(socket,"SO_EXCLUSIVEADDRUSE"):sock.setsockopt(socket.SOL_SOCKET,socket.SO_EXCLUSIVEADDRUSE,1)
                    sock.bind(("127.0.0.1",port))
    attempt("ports",free_ports)
    observations = state.get("attempts",[])
    reserved = state.get("reservations",0)
    known = len(observations) == reserved and all(r["known"] for r in observations)
    report = json.loads(public.read_text(encoding="utf-8")) if public.exists() else dict(
        schema_version=1,campaign_id=CAMPAIGN,run_id=env["GITHUB_RUN_ID"],attempt=1,
        source_sha=env["REVIEWED_SHA"],result="INCOMPLETE",
        scenes={n:dict(status="INCOMPLETE",evidence=[]) for n in SCENES},
        model_transport_attempts=sum(r["count"] for r in observations) if known else None,
        model_transport_attempts_known=known,model_slots_reserved=reserved,
        budget=dict(actual_cost_usd=None,remaining_after_usd=None))
    def private_remove():
        if state.get("data"):
            data = Path(state["data"])
            require(data.resolve() == (Path(os.environ["APPDATA"])/"com.follow"/"clash").resolve())
            if data.exists():shutil.rmtree(data)
        shutil.rmtree(root)
        require(not root.exists())
    attempt("private_files",private_remove)
    report["independent_cleanup"] = checks
    report["forced_independent_cleanup"] = forced
    # Independent recovery/forced cleanup can never repair normal-exit PASS.
    if forced or not all(v == "verified" for v in checks.values()):
        report["result"] = "INCOMPLETE"
    public.write_text(json.dumps(report,indent=2),encoding="utf-8")
    print(json.dumps(report,separators=(",",":")),flush=True)
    return 0 if all(v == "verified" for v in checks.values()) else 1


def main(args=None):
    args = sys.argv[1:] if args is None else args
    if not args or args == ["plan"]:
        print(json.dumps(plan(),separators=(",",":")));return 0
    try:
        require(args in (["prepare"],["execute"],["cleanup"]))
        env = dict(os.environ)
        gate(env,env.get("REVIEWED_SHA","")) # Before git, filesystem, process or Secret use.
        head = subprocess.check_output(["git","rev-parse","HEAD"],timeout=10).decode().strip()
        gate(env,head)
        require(not subprocess.check_output(["git","status","--porcelain","--untracked-files=normal"],timeout=10).strip())
        if args != ["cleanup"]:clean_budget_scope(env)
        root = Path(env["RUNNER_TEMP"]).resolve()/("complete-campaign-"+env["GITHUB_RUN_ID"])
        if args == ["cleanup"]:return cleanup(root,env)
        if args == ["prepare"]:
            prepare(root,env);return 0
        return 0 if execute(root,env) == "PASS" else 1
    except Exception:
        print(json.dumps(dict(campaign_id=CAMPAIGN,result="INCOMPLETE",
                              stage="admission_or_capability",reason="blocked",
                              model_transport_attempts=None,model_transport_attempts_known=False)),flush=True)
        return 1


if __name__ == "__main__":
    sys.exit(main())
