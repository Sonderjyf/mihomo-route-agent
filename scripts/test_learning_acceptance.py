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

    def test_scheduled_receipt_requires_current_adopted_applied_correlation(self):
        row = dict(id="c"*32, connection_id="a", choice="DIRECT", decision="DIRECT",
                   policy_reason="accepted", policy_adopted=True, status="applied",
                   expires_at=(l.datetime.datetime.now(l.datetime.timezone.utc)+l.datetime.timedelta(seconds=55)).isoformat(),
                   policy_version="route-policy-v1", scope="d"*64, publication_id="e"*32+"-2")
        self.assertEqual(l.applied_receipt(dict(decisions=[row]), "a")["decision_id"], "c"*32)
        for key, value in [("status", "pending"), ("policy_adopted", False),
                           ("connection_id", "other"), ("policy_version", "old"),
                           ("expires_at", "2000-01-01T00:00:00Z"), (CANARY, CANARY)]:
            bad = dict(row); bad[key] = value
            with self.subTest(field=key), self.assertRaises(l.a.Refused):
                l.applied_receipt(dict(decisions=[bad]), "a")
        with self.assertRaises(l.a.Refused):
            l.publication_schedule(dict(publication_schedule={"interval_seconds":30}))

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
                row.update(decision="DIRECT", policy_reason="accepted", publication="not_attempted")
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
                schedule = dict(interval_seconds=30, first_due_unix_ms=int(time.time()*1000)+30000,
                                first_nonempty_submission_unix_ms=0, nonempty_submissions=0)
                published_schedule = dict(schedule, first_nonempty_submission_unix_ms=schedule["first_due_unix_ms"], nonempty_submissions=1)
                receipt = dict(id="c"*32, connection_id="a", choice="DIRECT", decision="DIRECT",
                               policy_reason="accepted", policy_adopted=True, status="applied",
                               expires_at=(l.datetime.datetime.now(l.datetime.timezone.utc)+l.datetime.timedelta(seconds=55)).isoformat(), policy_version="route-policy-v1",
                               scope="d"*64, publication_id="e"*32+"-2")
                pending = dict(evaluation=row, attempts=1, judge_calls=1, commits=0, publication_schedule=schedule)
                applied = dict(evaluation=row, attempts=1, judge_calls=1, commits=1,
                               publication_schedule=published_schedule, decisions=[receipt])
                report = l.result()
                with contextlib.ExitStack() as stack:
                    replacements = [(l.a, "CORE_HASH", dict(new=digest)),
                        (l, "ports_free", {}), (l, "pin_target", dict(return_value="8.8.8.8")),
                        (l.tempfile, "mkdtemp", dict(return_value=str(private))),
                        (l.subprocess, "check_output", dict(return_value=json.dumps({"candidate":core_config}).encode())),
                        (l.subprocess, "Popen", dict(side_effect=spawn)), (l.subprocess, "run", {}),
                        (l, "ThreadingHTTPServer", {}), (l.threading, "Thread", {}),
                        (l, "wait_for", dict(side_effect=[{}, True, {}, l.a.Refused() if case == "pipeline_failure" else pending, applied, applied])),
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


class CompleteCampaignGuards(unittest.TestCase):
    def test_complete_campaign_inert_and_rejected_admission_has_no_side_effects(self):
        import complete_campaign as c
        good = environment()
        good.update(GITHUB_WORKFLOW_REF=l.a.REPO+"/.github/workflows/complete-campaign.yml@refs/heads/main",
                    EXECUTE_COMPLETE_CAMPAIGN="true", CAMPAIGN_ID=c.CAMPAIGN, APPROVED_MODEL_ATTEMPTS="6",
                    APPROVED_TUN="true", APPROVED_BUDGET_METADATA_GETS="2")
        with patch.object(c.subprocess,"check_output") as git, patch.object(c.subprocess,"Popen") as spawn, \
             patch.object(c,"prepare") as prepare, patch.object(c,"execute") as execute, patch.object(c,"cleanup") as cleanup:
            output = io.StringIO()
            with contextlib.redirect_stdout(output): self.assertEqual(c.main([]),0)
            self.assertFalse(json.loads(output.getvalue())["execution_enabled"])
            cases = [{}, {**good,"RUNNER_ENVIRONMENT":"self-hosted"}, {**good,"GITHUB_RUN_ATTEMPT":"2"},
                     {**good,"REVIEWED_SHA_ALLOWLIST":"b"*40}, {**good,"APPROVED_TUN":"false"},
                     {**good,"APPROVED_MODEL_ATTEMPTS":"7"}, {**good,"APPROVED_BUDGET_METADATA_GETS":"0"},
                     {**good,"REMAINING_USD":"NaN"}, {**good,"PROVIDER_CAP_CONFIRMED":"false"}]
            for e in cases:
                with patch.dict(c.os.environ,e,clear=True), patch.object(c.sys,"platform","win32"):
                    output = io.StringIO()
                    with contextlib.redirect_stdout(output): self.assertEqual(c.main(["execute"]),1)
                    self.assertNotIn(CANARY,output.getvalue())
                    self.assertEqual(json.loads(output.getvalue())["result"],"INCOMPLETE")
            for operation in (git,spawn,prepare,execute,cleanup):operation.assert_not_called()
        with patch.object(c.sys,"platform","win32"):c.gate(good,"a"*40)
        safe = c.child_env({**good,"GH_TOKEN":CANARY,"GOPROXY":"off","GOSUMDB":"off"})
        for name in ("OPENROUTER_API_KEY","VLESS_NODE_JSON","GH_TOKEN"):self.assertNotIn(name,safe)
        self.assertEqual(safe["GOPROXY"],"off")

    def test_complete_campaign_correlates_native_transport_and_refuses_weak_receipts(self):
        import complete_campaign as c
        runtime = object.__new__(c.AppRuntime)
        row = dict(id="c"*32,connection_id="first",decision="PROXY",choice="PROXY",
                   policy_reason="accepted",policy_adopted=True,status="applied",
                   policy_version="route-policy-v1",scope="d"*64,publication_id="e"*32+"-1",
                   expires_at=(l.datetime.datetime.now(l.datetime.timezone.utc)+l.datetime.timedelta(seconds=50)).isoformat())
        self.assertEqual(runtime.receipt(dict(decisions=[row]),"first","PROXY")["decision_id"],"c"*32)
        rejected = dict(row,status="rejected",policy_adopted=False)
        self.assertFalse(c.receipt_header(dict(decisions=[rejected]),"first")["policy_adopted"])
        for key,value in (("status","pending"),("policy_adopted",False),("connection_id","other"),
                          ("policy_version","old"),("expires_at","2000-01-01T00:00:00Z")):
            bad = dict(row);bad[key] = value
            with self.assertRaises(c.StopCampaign):runtime.receipt(dict(decisions=[bad]),"first","PROXY")
        first = dict(id="first",metadata=dict(destinationIP="8.8.8.8",network="tcp",type="Tun"))
        later = copy.deepcopy(first);later.update(id="later",upload=100,download=200)
        c.same_target(first,later,"8.8.8.8")
        for bad in (dict(later,id="first"),dict(later,download=0),
                    dict(later,metadata=dict(later["metadata"],type="Mixed")),
                    dict(later,metadata=dict(later["metadata"],destinationIP="1.1.1.1"))):
            with self.assertRaises(c.StopCampaign):c.same_target(first,bad,"8.8.8.8")

    def test_complete_campaign_normal_exit_cannot_be_repaired_by_summary_or_recovery(self):
        import complete_campaign as c
        for code,stopped,journal in ((1,True,"stopped"),(0,False,"stopped"),(0,True,"recovered_stopped")):
            with test_directory() as folder:
                root = Path(folder)
                state = root/"journal.json";state.write_text(json.dumps(dict(phase=journal)))
                runtime = object.__new__(c.AppRuntime)
                runtime.report = dict(scenes={"C09":dict(status="INCOMPLETE",evidence=[])})
                runtime.empty = Mock(return_value=True);runtime.bootstrap = Mock()
                process = Mock();process.wait.return_value=code
                with patch.object(c.learning,"final_summary",return_value=dict(observer_stopped=stopped)):
                    with self.assertRaises(c.StopCampaign):
                        runtime.stop_session(process,root/"owner",state,root/"out",1)
                self.assertEqual(runtime.report["scenes"]["C09"]["status"],"INCOMPLETE")
                runtime.bootstrap.assert_not_called()

    def test_complete_campaign_independent_cleanup_continues_after_ownership_and_network_failure(self):
        import complete_campaign as c
        with test_directory() as folder:
            base = Path(folder);root = base/"complete-campaign-123";root.mkdir()
            e = dict(environment(),RUNNER_TEMP=str(base),GITHUB_RUN_ID="123",REVIEWED_SHA="a"*40)
            marker = dict(campaign_id=c.CAMPAIGN,run_id="123",source_sha="a"*40,baseline={"clean":True})
            (root/"owned-marker.json").write_text(json.dumps(marker))
            (root/"private-state.json").write_text(json.dumps(dict(
                publisher_records=[dict(pid=7,ownership=str(base/"outside"),journal=str(root/"j"),
                                        config=str(root/"c"),output=str(root/"o"))],
                attempts=[],reservations=1)))
            output = io.StringIO()
            with patch.object(c,"guest",return_value=False), patch.object(c,"powershell",side_effect=OSError(CANARY)), \
                 patch.object(c,"tun_route",side_effect=OSError(CANARY)), patch.object(c,"snapshot",return_value=marker["baseline"]), \
                 patch.object(c.socket,"socket"), patch.object(c.subprocess,"run") as run, contextlib.redirect_stdout(output):
                self.assertEqual(c.cleanup(root,e),1)
            run.assert_not_called()
            result = json.loads(output.getvalue())
            self.assertNotIn(CANARY,output.getvalue())
            self.assertEqual(result["independent_cleanup"]["publisher-7"],"failed")
            self.assertEqual(result["independent_cleanup"]["firewall"],"failed")
            self.assertEqual(result["independent_cleanup"]["private_files"],"verified")
            self.assertEqual(result["result"],"INCOMPLETE")
            self.assertIsNone(result["model_transport_attempts"])
            self.assertFalse(root.exists())


    def test_complete_campaign_guest_helpers_parse_without_running_app_or_network(self):
        import shutil
        import subprocess
        executable = shutil.which("powershell.exe")
        if not executable:self.skipTest("Windows PowerShell parser unavailable")
        source = "$e=$null; $t=$null; foreach($f in @('scripts/flclash_guest.ps1','scripts/flclash_pointer.ps1')) { [void][Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) $f),[ref]$t,[ref]$e); if($e.Count -gt 0){throw 'Guest helper syntax error'} }"
        subprocess.run([executable,"-NoProfile","-NonInteractive","-Command",source],
                       cwd=Path(__file__).resolve().parents[1],capture_output=True,check=True,timeout=20)


    def test_observed_http_status_stays_separate_from_policy_adoption(self):
        value = dict(diagnostic(),model_http_status=200)
        self.assertEqual(l.diagnostic(value)["model_http_status"],200)
        self.assertEqual(l.diagnostic(value)["policy_reason"],"probability_below_threshold")
        for status in (True,0,99,600,CANARY):
            with self.assertRaises(l.a.Refused):l.diagnostic(dict(value,model_http_status=status))


if __name__ == "__main__":
    unittest.main()
