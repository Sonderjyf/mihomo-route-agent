import contextlib
import copy
import hashlib
import io
import json
import time
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

import learning_acceptance as l
from test_real_acceptance import CANARY, env, node, test_directory


def environment():
    e = env()
    e.update(RUNNER_OS="Windows", EXECUTE_LEARNING_ACCEPTANCE="true", DIRECT_INTERFACE_INDEX="3",
             REMAINING_USD="0.03", REMAINING_BUDGET_CONFIRMED="true",
             GITHUB_WORKFLOW_REF=l.a.REPO+"/.github/workflows/learning-acceptance.yml@refs/heads/main",
             JOB_STARTED_AT=str(time.time()), OPENROUTER_API_KEY=CANARY,
             VLESS_NODE_JSON=json.dumps(node()))
    return e


def diagnostic():
    value = {k: 0 for k in l.TIMES}
    value.update(version=1, stage="policy", stage_error="none", policy_reason="probability_below_threshold",
                 choice="DIRECT", decision="UNCERTAIN", evidence=dict(direct_tls="verified_success", proxy_tls="not_tested"),
                 dns="resolved", model_calls=1, transport_attempts=1, transport_attempts_known=True,
                 publication="not_attempted", connection_target_match=True)
    return value


class LearningGuards(unittest.TestCase):
    def test_inert_and_all_pre_execution_gates_have_no_side_effects(self):
        with patch.object(l.subprocess, "check_output") as git, patch.object(l.subprocess, "Popen") as spawn, patch.object(l, "ports_free") as ports, patch.object(l, "execute") as execute:
            output = io.StringIO()
            with contextlib.redirect_stdout(output): self.assertEqual(l.main([]), 0)
            self.assertEqual(json.loads(output.getvalue()), l.plan())
            cases = [{}, {**environment(), "RUNNER_ENVIRONMENT": "self-hosted"},
                     {**environment(), "EXECUTE_LEARNING_ACCEPTANCE": "false"},
                     {**environment(), "GITHUB_RUN_ATTEMPT": "2"},
                     {**environment(), "REVIEWED_SHA_ALLOWLIST": "b"*40},
                     {**environment(), "GITHUB_REF": "refs/pull/5/merge"},
                     {**environment(), "PROVIDER_CAP_CONFIRMED": "false"},
                     {**environment(), "REMAINING_BUDGET_CONFIRMED": "false"},
                     {**environment(), "REMAINING_USD": "0"},
                     {**environment(), "DIRECT_INTERFACE_INDEX": "0"}]
            for e in cases:
                with self.subTest(gate=list(e)), patch.dict(l.os.environ, e, clear=True), patch.object(l.sys, "platform", "win32"):
                    output = io.StringIO()
                    with contextlib.redirect_stdout(output): self.assertEqual(l.main(["execute"]), 1)
                    self.assertNotIn(CANARY, output.getvalue())
                    self.assertEqual(json.loads(output.getvalue())["model_transport_attempts"], 0)
            for operation in (git, spawn, ports, execute): operation.assert_not_called()
        with patch.object(l.sys, "platform", "win32"):
            l.gate(environment(), "a"*40)
            with self.assertRaises(l.a.Refused): l.gate(environment(), "b"*40)

    def test_output_contract_and_http_runtime_template(self):
        self.assertEqual(l.diagnostic(diagnostic()), diagnostic())
        for key, value in [(CANARY, CANARY), ("policy_reason", CANARY), ("transport_attempts", 2), ("elapsed_ms", True), ("evidence", {"direct_tls": CANARY, "proxy_tls": "not_tested"})]:
            with self.subTest(field=key):
                invalid = copy.deepcopy(diagnostic()); invalid[key] = value
                with self.assertRaises(l.a.Refused): l.diagnostic(invalid)
        core, agent = l.template(node(), CANARY)
        self.assertFalse(core["tun"]["enable"])
        self.assertEqual(agent["max_api_requests"], 1)
        self.assertEqual(agent["observation"]["hosts"], [l.HOST])
        self.assertEqual(core["rules"][-1], "MATCH,acceptance-vless")
        for name in l.PROVIDERS:
            provider = core["rule-providers"][name]
            self.assertEqual((provider["type"], provider["proxy"]), ("http", "DIRECT"))
            self.assertEqual(provider["url"], f"http://127.0.0.1:18797/rules/{name}.yaml")
            self.assertEqual(provider["path"], f"./route-agent/{name}.yaml")
        pinned, _ = l.template(node(), CANARY, "8.8.8.8")
        self.assertEqual(pinned["hosts"], {l.HOST: "8.8.8.8"})
        for ip in ("127.0.0.1", "198.18.0.1", "192.168.1.1"):
            with self.assertRaises(l.a.Refused): l.template(node(), CANARY, ip)

    def test_port_conflict_refuses_before_processes_and_partial_cleanup_continues(self):
        for conflict in (True, False):
            with self.subTest(conflict=conflict), test_directory() as folder:
                root = Path(folder)
                wire = b"synthetic-binary-not-runnable"
                for name in ("mihomo.exe", "agent.exe"): (root/name).write_bytes(wire)
                digest = hashlib.sha256(wire).hexdigest()
                (root/"agent.sha256").write_text(digest, encoding="ascii")
                private = root/"private-fixture"; private.mkdir(mode=0o755)
                core, _ = l.template(node(), CANARY)
                fake_core = Mock(); fake_core.poll.return_value = None
                report = l.result()
                with patch.object(l.a, "CORE_HASH", digest), patch.object(l, "ports_free", side_effect=OSError(CANARY) if conflict else None) as ports, patch.object(l, "pin_target", return_value="8.8.8.8"), patch.object(l.tempfile, "mkdtemp", return_value=str(private)), patch.object(l.subprocess, "check_output", return_value=json.dumps({"candidate":core}).encode()), patch.object(l.subprocess, "Popen", return_value=fake_core) as spawn, patch.object(l, "ThreadingHTTPServer"), patch.object(l.threading, "Thread"), patch.object(l, "wait_for", side_effect=l.a.Refused()), patch.object(l.a, "stop_owned", side_effect=OSError(CANARY)):
                    l.execute(root, environment(), report)
                self.assertNotIn(CANARY, json.dumps(report))
                self.assertEqual(report["acceptance_status"], "incomplete")
                if conflict:
                    spawn.assert_not_called()
                    self.assertEqual(report["model_transport_attempts"], 0)
                else:
                    self.assertEqual(report["cleanup"]["core"], "failed")
                    self.assertEqual(report["cleanup"]["private_files"], "verified")
                    self.assertEqual(report["cleanup"]["ports"], "verified")
                    self.assertEqual(ports.call_count, 2)

    def test_learning_pass_requires_normal_stop_and_matching_final_summary(self):
        # Entire execution is mocked: synthetic binaries, no sockets/processes/API.
        for case in ("success", "nonzero", "timeout", "journal", "missing", "invalid", "not_stopped", "mismatch", "pipeline_failure"):
            with self.subTest(case=case), test_directory() as folder:
                root = Path(folder)
                wire = b"synthetic-binary-not-runnable"
                digest = hashlib.sha256(wire).hexdigest()
                for name in ("mihomo.exe", "agent.exe"): (root/name).write_bytes(wire)
                (root/"agent.sha256").write_text(digest, encoding="ascii")
                private = root/"private-fixture"; private.mkdir(mode=0o755)
                core_config, _ = l.template(node(), CANARY)
                row = diagnostic()
                row.update(decision="DIRECT", policy_reason="accepted", publication="committed")
                if case == "pipeline_failure":
                    row.update(stage="model", stage_error="model_transport_failed", decision="UNCERTAIN", policy_reason="not_evaluated", publication="not_attempted")
                summary = dict(evaluation=copy.deepcopy(row), observer_stopped=True)
                if case == "invalid": summary[CANARY] = CANARY
                if case in ("not_stopped", "pipeline_failure"): summary["observer_stopped"] = False
                if case == "mismatch": summary["evaluation"]["model_ms"] = 1
                core, agent = Mock(), Mock()
                core.poll.return_value = agent.poll.return_value = None
                agent.wait.return_value = 7 if case == "nonzero" else 0
                if case == "timeout": agent.wait.side_effect = l.subprocess.TimeoutExpired("synthetic", 90)
                def spawn(command, **kwargs):
                    if command[1] != "observe-apply": return core
                    if case != "missing": Path(kwargs["stdout"].name).write_text(json.dumps(summary), encoding="utf-8")
                    (private/"state.json").write_text(json.dumps(dict(phase="active" if case == "journal" else "stopped")), encoding="utf-8")
                    return agent
                record_a = dict(id="a", rule="Match", rulePayload="", chains=["acceptance-vless"], metadata=dict(destinationIP="8.8.8.8"))
                record_b = dict(id="b", rule="AND", rulePayload="((Network,tcp) && (DstPort,443) && (RuleSet,route-agent-tail-direct))", chains=["DIRECT"], metadata=dict(destinationIP="8.8.8.8"))
                def api(path, *args, **kwargs):
                    if path == "/configs": return dict(tun=dict(enable=False))
                    if path == "/providers/rules": return dict(providers={l.PROVIDERS[0]:dict(ruleCount=1), l.PROVIDERS[1]:dict(ruleCount=0)})
                    return {}
                report = l.result()
                with contextlib.ExitStack() as stack:
                    replacements = [(l.a, "CORE_HASH", dict(new=digest)),
                        (l, "ports_free", {}), (l, "pin_target", dict(return_value="8.8.8.8")),
                        (l.tempfile, "mkdtemp", dict(return_value=str(private))),
                        (l.subprocess, "check_output", dict(return_value=json.dumps({"candidate":core_config}).encode())),
                        (l.subprocess, "Popen", dict(side_effect=spawn)), (l.subprocess, "run", {}),
                        (l, "ThreadingHTTPServer", {}), (l.threading, "Thread", {}),
                        (l, "wait_for", dict(side_effect=[{}, True, {}, l.a.Refused() if case == "pipeline_failure" else dict(evaluation=row, attempts=1, judge_calls=1, commits=1)])),
                        (l, "api", dict(side_effect=api)), (l, "empty", dict(return_value=True)),
                        (l, "tls_connection", dict(side_effect=[Mock(), Mock()])),
                        (l, "connection_record", dict(side_effect=[record_a, record_b, record_a])),
                        (l.a, "stop_owned", {})]
                    for owner, name, kwargs in replacements: stack.enter_context(patch.object(owner, name, **kwargs))
                    l.execute(root, environment(), report)
                self.assertNotIn(CANARY, json.dumps(report))
                self.assertTrue(all(value == "verified" for value in report["cleanup"].values()))
                if case == "pipeline_failure":
                    self.assertFalse(report["new_connection_verified"])
                    self.assertEqual((report["phase_status"], report["acceptance_status"], report["reason"]), ("completed", "no_go", "pipeline_failed"))
                    self.assertEqual(report["model_transport_attempts"], 1)
                    self.assertTrue(report["model_transport_attempts_known"])
                    continue
                self.assertTrue(report["new_connection_verified"])
                self.assertEqual(report["acceptance_status"], "pass" if case == "success" else "incomplete")
                self.assertEqual(report["phase_status"], "completed" if case == "success" else "failed")


if __name__ == "__main__":
    unittest.main()
