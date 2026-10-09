import base64
import contextlib
import copy
import io
import json
import unittest
import tempfile
import shutil
import uuid
from pathlib import Path
from unittest.mock import patch, Mock
import real_acceptance as a


def node():
    # Synthetic noncredential fixture; never passed to an actual core.
    return dict(server="8.8.8.8", port=443, uuid="00000000-0000-0000-0000-000000000000", servername="example.com", reality_public_key=base64.urlsafe_b64encode(bytes(32)).decode().rstrip("="), short_id="")


def env():
    sha = "a" * 40
    return dict(REVIEWED_SHA=sha, REVIEWED_SHA_ALLOWLIST=sha, GITHUB_ACTIONS="true", RUNNER_ENVIRONMENT="github-hosted", GITHUB_REPOSITORY=a.REPO, GITHUB_EVENT_NAME="workflow_dispatch", GITHUB_REF="refs/heads/main", GITHUB_RUN_ATTEMPT="1", GITHUB_WORKFLOW_REF=a.REPO+"/.github/workflows/real-acceptance.yml@refs/heads/main", GITHUB_RUN_ID="123", APPROVAL_RECORD="test only", APPROVED_USD="0.10", PROVIDER_CAP_USD="0.05", PROVIDER_CAP_CONFIRMED="true", PROXY_COST_APPROVED="true")


CANARY = "SYNTHETIC_SECRET_CANARY_7dc161af"


def worker_result(failed=False):
    row = dict(stage="complete", reason="none", index=0, probe=dict(evidence=dict(direct_tls="verified_success", proxy_tls="not_tested"), dns="resolved", direct_attempts=["verified_success"], proxy_attempt="not_tested"), vless_tls="verified_success", model="answered", choice="UNCERTAIN", accepted="UNCERTAIN", acceptance_reason="choice_evidence_mismatch")
    rows = [row, copy.deepcopy(row)]
    rows[1]["index"] = 1
    return dict(stage="controller_contract" if failed else "complete", reason="guard_rejected" if failed else "none", route_inventory=None, hosts=rows, model_attempts=2, api_requests=[dict(attempt=i+1, outcome="http_response", http_status=200, request_id_presence="present") for i in range(2)], routing_updated=False, tun_tested=False)


class OwnedProcess:
    def __init__(self, result=None, failure=None):
        self.pid = 1234
        self.returncode = None
        self.result = result
        self.failure = failure
        self.terminated = False
    def poll(self):
        return self.returncode
    def communicate(self, timeout):
        if self.failure:
            raise self.failure
        self.returncode = 0 if self.result["reason"] == "none" else 3
        return json.dumps(self.result).encode(), None
    def terminate(self):
        self.terminated = True
        self.returncode = -15
    def kill(self):
        self.returncode = -9
    def wait(self, timeout):
        return self.returncode




@contextlib.contextmanager
def test_directory():
    # Python 3.14's Windows 0700 ACL excludes the executor sandbox token.
    # Test fixtures contain no credentials; ordinary inherited ACLs suffice.
    root = Path.cwd() / (".offline-test-" + uuid.uuid4().hex)
    root.mkdir(mode=0o755)
    try:
        yield str(root)
    finally:
        assert root.parent.resolve() == Path.cwd().resolve()
        shutil.rmtree(root)


def private_fixture(prefix, dir):
    root = Path(dir) / (prefix + uuid.uuid4().hex)
    root.mkdir(mode=0o755)
    return str(root)


class RealAcceptanceGuards(unittest.TestCase):
    def test_acceptance_reason_is_fixed_enum_only(self):
        value = worker_result()
        value["hosts"][0]["acceptance_reason"] = "probability_below_threshold"
        self.assertEqual(a.output_contract(json.dumps(value))["hosts"][0]["acceptance_reason"], "probability_below_threshold")
        value["hosts"][0]["acceptance_reason"] = CANARY
        with self.assertRaises(a.Refused):
            a.output_contract(json.dumps(value))
        value = worker_result()
        value["hosts"][0]["probabilities"] = {CANARY: 1}
        with self.assertRaises(a.Refused):
            a.output_contract(json.dumps(value))

    def test_child_preserves_only_module_cache_runtime_setting(self):
        for spelling in ("PSMODULEANALYSISCACHEPATH", "PSModuleAnalysisCachePath"):
            original = dict(PATH="synthetic-path", OPENROUTER_API_KEY=CANARY,
                            VLESS_NODE_JSON=CANARY, GITHUB_TOKEN=CANARY,
                            PSEXECUTIONPOLICYPREFERENCE="Bypass",
                            __PSLOCKDOWNPOLICY="0", PSModulePath=CANARY,
                            HTTP_PROXY=CANARY, REAL_CONTROLLER_SECRET=CANARY)
            original[spelling] = "C:\\synthetic path\\ModuleAnalysisCache"
            expected = {"PATH": "synthetic-path", spelling: original[spelling]}
            self.assertEqual(a.child_env(original), expected)
            self.assertEqual(a.child_env(original, model=True), dict(expected, OPENROUTER_API_KEY=CANARY))
            self.assertEqual(original["PSModulePath"], CANARY)
        self.assertEqual(a.child_env({}), {})

    def test_inventory_diagnostics_contract_and_canary(self):
        value = worker_result(True)
        inventory = dict(reason="query_timeout", elapsed_ms=15001, budget_ms=15000, query_exit_code=None, stderr_present=True)
        value["route_inventory"] = inventory
        self.assertEqual(a.output_contract(json.dumps(value))["route_inventory"], inventory)
        for key, bad in [("reason", CANARY), ("elapsed_ms", True), ("budget_ms", 30000), ("query_exit_code", CANARY), ("stderr_present", CANARY), (CANARY, CANARY)]:
            changed = copy.deepcopy(value)
            changed["route_inventory"][key] = bad
            with self.assertRaises(a.Refused):
                a.output_contract(json.dumps(changed))

    def test_schema_diagnostics_canary(self):
        cases = []
        missing = node(); del missing["uuid"]
        cases.append((json.dumps(missing), "missing_fields", ["uuid"], "", ""))
        cases.append((json.dumps(dict(node(), port=CANARY)), "type_mismatch", ["port"], "integer", "string"))
        cases.append((json.dumps(dict(node(), uuid=CANARY)), "invalid_value", ["uuid"], "", ""))
        unknown = node(); unknown[CANARY] = CANARY
        cases.append((json.dumps(unknown), "unknown_fields", [], "", ""))
        for raw, reason, fields, expected, actual in cases:
            report = a.Report(); report.stage = "node_schema"
            try:
                a.node_input(raw)
            except Exception as error:
                report.failure(error)
            wire = json.dumps(report.payload())
            self.assertNotIn(CANARY, wire)
            self.assertEqual(report.payload()["diagnostic"], dict(stage="node_schema", reason=reason, fields=fields, expected_type=expected, actual_type=actual))

    def test_execute_mock_success_and_failures_are_diagnostic(self):
        cases = ["success", "node", "core_hash", "once", "deadline", "config", "core_spawn", "core_interrupt", "core_exit", "core_timeout", "worker_spawn", "worker_interrupt", "worker_timeout", "worker_error", "invalid_output", "cleanup_stop", "cleanup_files", "cleanup_ports"]
        for case in cases:
            with self.subTest(case=case), test_directory() as directory:
                root = Path(directory)
                binary = b"not executable; offline mock only"
                for name in ("mihomo.exe", "worker.exe"):
                    (root / name).write_bytes(binary)
                digest = a.hashlib.sha256(binary).hexdigest()
                (root / "worker.sha256").write_text(digest, encoding="ascii")
                values = dict(env(), JOB_STARTED_AT=str(a.time.time()), VLESS_NODE_JSON=json.dumps(node()), OPENROUTER_API_KEY=CANARY)
                core = OwnedProcess()
                worker = OwnedProcess(worker_result(case == "worker_error"))
                report = a.Report()
                if case == "node": values["VLESS_NODE_JSON"] = json.dumps(dict(node(), uuid=CANARY))
                if case == "once": (root / "attempt-123").write_text("used", encoding="ascii")
                if case == "deadline": values["JOB_STARTED_AT"] = "0"
                if case == "core_exit": core.returncode = 7
                if case == "worker_timeout": worker.failure = a.subprocess.TimeoutExpired(CANARY, 270, output=CANARY)
                if case == "invalid_output": worker.communicate = Mock(return_value=(CANARY.encode(), None))
                spawn_effects = [core, worker]
                if case == "core_spawn": spawn_effects = [OSError(CANARY)]
                if case == "core_interrupt": spawn_effects = [KeyboardInterrupt(CANARY)]
                if case == "worker_spawn": spawn_effects = [core, OSError(CANARY)]
                if case == "worker_interrupt": spawn_effects = [core, KeyboardInterrupt(CANARY)]
                with contextlib.ExitStack() as stack:
                    stack.enter_context(patch.object(a.tempfile, "mkdtemp", side_effect=private_fixture))
                    stack.enter_context(patch.object(a.sys, "platform", "win32"))
                    stack.enter_context(patch.object(a, "CORE_HASH", "bad" if case == "core_hash" else digest))
                    ports = stack.enter_context(patch.object(a, "ports_free"))
                    if case == "cleanup_ports": ports.side_effect = [None, OSError(CANARY)]
                    connect = stack.enter_context(patch.object(a.socket, "create_connection"))
                    connect.return_value.__enter__ = Mock(); connect.return_value.__exit__ = Mock()
                    if case == "core_timeout": connect.side_effect = OSError(CANARY)
                    stack.enter_context(patch.object(a.time, "sleep"))
                    spawn = stack.enter_context(patch.object(a.subprocess, "Popen", side_effect=spawn_effects))
                    if case == "config":
                        original = Path.write_text
                        def write(p, *args, **kwargs):
                            if p.name == "config.json": raise OSError(CANARY)
                            return original(p, *args, **kwargs)
                        stack.enter_context(patch.object(Path, "write_text", write))
                    if case == "cleanup_stop":
                        original_stop = a.stop_owned
                        def stop(process):
                            original_stop(process)
                            if process is core: raise OSError(CANARY)
                        stack.enter_context(patch.object(a, "stop_owned", side_effect=stop))
                    if case == "cleanup_files": stack.enter_context(patch.object(a.shutil, "rmtree", side_effect=OSError(CANARY)))
                    capture = io.StringIO()
                    with contextlib.redirect_stdout(capture), contextlib.redirect_stderr(capture):
                        a.execute(root, values, report)
                    self.assertEqual(capture.getvalue(), "")
                    result = report.payload()
                    self.assertNotIn(CANARY, json.dumps(result))
                    if case == "success":
                        self.assertEqual(result["status"], "completed")
                        self.assertEqual(result["model_attempts"], 2)
                        self.assertTrue(all(v == "verified" for v in result["cleanup"].values()))
                    else:
                        self.assertEqual(result["status"], "failed")
                    if case in {"worker_spawn", "worker_interrupt", "worker_timeout", "invalid_output"}:
                        self.assertIsNone(result["model_attempts"])
                        self.assertEqual(result["model_attempts_state"], "unknown")
                    if case in {"worker_spawn", "worker_interrupt"}:
                        self.assertEqual(result["cleanup"]["worker"], "unknown")
                    if case in {"core_spawn", "core_interrupt"}:
                        self.assertEqual(result["cleanup"]["core"], "unknown")
                    if case == "worker_error":
                        self.assertEqual(result["worker_exit_code"], 3)
                        self.assertEqual(result["model_attempts"], 2)
                        self.assertEqual(result["worker_result"]["stage"], "controller_contract")
                    if case == "core_exit": self.assertEqual(result["core_start_exit_code"], 7)
                    if case == "node": self.assertEqual(result["diagnostic"]["fields"], ["uuid"])
                    if case.startswith("cleanup_"):
                        self.assertEqual(result["diagnostic"]["reason"], "cleanup_failed")
                        self.assertIn("failed", result["cleanup"].values())
                    if spawn.call_count:
                        self.assertNotIn("OPENROUTER_API_KEY", spawn.call_args_list[0].kwargs["env"])
                    if spawn.call_count == 2:
                        self.assertNotIn("VLESS_NODE_JSON", spawn.call_args_list[1].kwargs["env"])
                if case != "cleanup_files": self.assertFalse(list(root.glob("private-*")))

    def test_main_finally_emits_only_safe_failure_and_cleanup(self):
        values = dict(env(), RUNNER_TEMP="unused")
        capture = io.StringIO()
        def failure(root, values, report):
            report.stage = "worker_wait"
            report.model_attempts = None
            report.attempts_state = "unknown"
            report.cleanup["core"] = "failed"
            raise RuntimeError(CANARY)
        with patch.dict(a.os.environ, values, clear=True), patch.object(a.subprocess, "check_output", return_value=b"a"*40), patch.object(a, "execute", side_effect=failure), contextlib.redirect_stdout(capture), contextlib.redirect_stderr(capture):
            self.assertEqual(a.main(["execute"]), 1)
        output = capture.getvalue()
        self.assertNotIn(CANARY, output)
        report = json.loads(output)
        self.assertEqual(report["diagnostic"]["stage"], "worker_wait")
        self.assertEqual(report["cleanup"]["core"], "failed")
        self.assertIsNone(report["model_attempts"])

    def test_prepare_download_exception_canary(self):
        report = a.Report()
        with patch.object(a.urllib.request, "urlopen", side_effect=OSError(CANARY)):
            try:
                a.prepare(Mock(), env(), report)
            except Exception as error:
                report.failure(error)
        self.assertEqual(report.stage, "download")
        self.assertEqual(report.reason, "io_error")
        self.assertNotIn(CANARY, json.dumps(report.payload()))

    def test_schema(self):
        self.assertEqual(a.node_input(json.dumps(node())), node())
        bad = [dict(node(), port=True), dict(node(), port=0), dict(node(), server="127.0.0.1"), dict(node(), server="https://example.com"), dict(node(), servername="example.com\nsecret"), dict(node(), uuid="secret"), dict(node(), extra="secret"), dict(node(), short_id="a"), dict(node(), reality_public_key="secret")]
        for value in bad:
            with self.assertRaises(Exception):
                a.node_input(json.dumps(value))
        for raw in ['{"port":1,"port":2}', '{"port":NaN}', '{"port":Infinity}', "x" * 17000]:
            with self.assertRaises(Exception):
                a.node_input(raw)

    def test_gate_and_missing_budget(self):
        good = env()
        a.gate(good, "a" * 40)
        for key in good:
            bad = dict(good)
            del bad[key]
            with self.assertRaises(a.Refused, msg=key):
                a.gate(bad, "a" * 40)
        for value in ("", "0", "NaN", "1e-2", "-1", "10.01"):
            with self.assertRaises(a.Refused):
                a.budget(dict(good, APPROVED_USD=value))
        with self.assertRaises(a.Refused):
            a.gate(dict(good, GITHUB_RUN_ATTEMPT="2"), "a" * 40)
        with self.assertRaises(a.Refused):
            a.gate(good, "b" * 40)

    def test_secrets_are_scoped_and_config_has_no_background_sources(self):
        original = dict(PATH="test", OPENROUTER_API_KEY="secret-key", VLESS_NODE_JSON="secret-node", GITHUB_TOKEN="secret-token", HTTP_PROXY="secret-proxy")
        self.assertEqual(a.child_env(original), {"PATH": "test"})
        self.assertEqual(a.child_env(original, True), {"PATH": "test", "OPENROUTER_API_KEY": "secret-key"})
        c = a.core_config(node(), "synthetic")
        self.assertFalse(c["tun"]["enable"])
        self.assertFalse(c["dns"]["enable"])
        self.assertFalse(c["sniffer"]["enable"])
        self.assertFalse(c["proxies"][0]["skip-cert-verify"])
        self.assertEqual(c["rules"], ["MATCH,acceptance-vless"])
        self.assertEqual(c["proxy-groups"], [])

    def test_safe_exception_output(self):
        capture = io.StringIO()
        with patch.object(a.subprocess, "check_output", side_effect=RuntimeError("SENSITIVE UUID TOKEN CONFIG")), contextlib.redirect_stdout(capture):
            self.assertEqual(a.main(["execute"]), 1)
        self.assertNotIn("SENSITIVE", capture.getvalue())
        self.assertEqual(json.loads(capture.getvalue())["diagnostic"], {"stage":"source","reason":"unexpected_error","fields":[],"expected_type":"","actual_type":""})

    def test_owned_process_cleanup_timeout(self):
        p = Mock()
        p.poll.side_effect = [None, 0]
        p.wait.side_effect = [a.subprocess.TimeoutExpired("private-command", 10), 0]
        a.stop_owned(p)
        p.terminate.assert_called_once()
        p.kill.assert_called_once()
        self.assertEqual(p.wait.call_count, 2)

    def test_output_contract_rejects_unreviewed_fields_and_secret_values(self):
        row = dict(stage="complete",reason="none",index=0, probe=dict(evidence=dict(direct_tls="verified_success", proxy_tls="not_tested"), dns="resolved", direct_attempts=["verified_success"], proxy_attempt="not_tested"), vless_tls="verified_success", model="answered", choice="UNCERTAIN", accepted="UNCERTAIN", acceptance_reason="choice_evidence_mismatch")
        second = copy.deepcopy(row)
        second["index"] = 1
        result = worker_result()
        result["hosts"] = [row, second]
        self.assertEqual(a.output_contract(json.dumps(result)), result)
        for change in (dict(result, token="sensitive"), dict(result, model_attempts=True), dict(result, routing_updated=True)):
            with self.assertRaises(a.Refused):
                a.output_contract(json.dumps(change))
        row["model"] = "sensitive-body"
        with self.assertRaises(a.Refused):
            a.output_contract(json.dumps(result))


if __name__ == "__main__":
    unittest.main()
