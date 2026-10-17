from contextlib import closing
import json
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import unittest
from unittest.mock import Mock, patch

from flclash_acceptance import dns_diagnostic
from flclash_app_acceptance import preferences, validate_preferences, profile, overwrite_script, require_guest, seed_tables, ps, validate_refresh_result, validate_final_inventory, read_fixture_profile, check_synthetic_config, wait_for_refresh, validate_restart_result


class AppPreparationTests(unittest.TestCase):
    def test_complete_fixture_matches_pinned_source_serialization_evidence(self):
        path = Path(__file__).resolve().parents[1] / "evidence/windows-flclash-dns-offline-2026-10-08.json"
        evidence = json.loads(path.read_text(encoding="utf-8"))
        materialized = evidence["source_derived_materialized_dns"]
        sparse = evidence["old_sparse_input"]
        self.assertEqual(len(materialized), 27)
        self.assertEqual({key: materialized[key] for key in sparse}, sparse)
        self.assertEqual(len(dns_diagnostic({"dns": sparse}, {"dns": materialized})["differences"]), 23)
        self.assertEqual(profile("A")["dns"], evidence["explicit_fixture_serialized_dns"])
        self.assertEqual(profile("A")["dns"], profile("B")["dns"])
        self.assertEqual(profile("A")["proxies"], [])

    def test_dns_snapshot_survives_contract_failure_without_poll_growth(self):
        report = {}
        actual = profile("A")
        actual["dns"]["nameserver"].append("system://")
        for _ in range(3):
            with self.assertRaises(ValueError):
                check_synthetic_config("A", actual, report)
        diagnostic = report["dns_diagnostics"]["A"]
        self.assertEqual(list(report["dns_diagnostics"]), ["A"])
        self.assertEqual(diagnostic["differences"][0]["path"], "/dns/nameserver/1")
        self.assertEqual(diagnostic["actual"]["value"], actual["dns"])
        self.assertEqual(diagnostic["expected"]["value"], profile("A")["dns"])
        actual["dns"]["nameserver"].clear()
        self.assertEqual(len(diagnostic["actual"]["value"]["nameserver"]), 2)
        self.assertEqual(json.loads(json.dumps(report)), report)

    def test_gui_helper_passes_diagnostic_path_and_is_bounded(self):
        with patch("flclash_app_acceptance.subprocess.run") as run:
            run.return_value = subprocess.CompletedProcess([], 0, "true", "")
            self.assertTrue(ps("profiles", "synthetic.exe", 7, "synthetic.json", allow_window_input=True))
            self.assertIn("-DiagnosticPath", run.call_args.args[0])
            self.assertIn("-AllowWindowInput", run.call_args.args[0])
            self.assertEqual(run.call_args.kwargs["timeout"], 60)

    def test_click_and_unchanged_config_are_not_refresh_evidence(self):
        before = {"last_update_date": 10}
        after = {"last_update_date": 11}
        validate_refresh_result(before, after, 2, 3)
        for changed, count in [(before, 3), (after, 2)]:
            with self.assertRaises(RuntimeError):
                validate_refresh_result(before, changed, 2, count)

    def test_refresh_waits_for_database_and_fresh_runtime_evidence(self):
        before, after = {"last_update_date": 10}, {"last_update_date": 11}
        database = Mock(side_effect=[before, after, after])
        capture = Mock(side_effect=[RuntimeError("runtime still A"), {"revision": "B"}])
        with patch("flclash_app_acceptance.time.sleep"):
            result = wait_for_refresh(before, 2, database, lambda: 3, capture)
        self.assertEqual(database.call_count, 3)
        self.assertEqual(capture.call_count, 2)
        self.assertEqual(result["database"], after)
        self.assertEqual(result["subscription_requests"], {"before": 2, "after": 3})

    def test_repeated_B_cannot_pass_without_new_request(self):
        capture = Mock(return_value={"revision": "B"})
        with patch("flclash_app_acceptance.time.monotonic", side_effect=[0, 0, 31]), patch("flclash_app_acceptance.time.sleep"):
            with self.assertRaisesRegex(RuntimeError, "new subscription request"):
                wait_for_refresh({"last_update_date": 10}, 2, lambda: {"last_update_date": 11}, lambda: 2, capture)
        capture.assert_not_called()

    def test_update_all_rejects_an_additional_profile(self):
        db = sqlite3.connect(":memory:")
        db.execute("CREATE TABLE profiles(id INTEGER)")
        db.executemany("INSERT INTO profiles VALUES(?)", [(101,), (102,)])
        with patch("flclash_app_acceptance.sqlite3.connect", return_value=db):
            with self.assertRaisesRegex(RuntimeError, "exactly one synthetic"):
                read_fixture_profile("synthetic.sqlite")

    def test_restart_requires_persisted_identity_without_refetch(self):
        saved = {"id": 101, "last_update_date": 11, "script_id": 201}
        validate_restart_result(saved, dict(saved), 3, 3)
        for after, count in [(dict(saved, script_id=999), 3), (dict(saved, last_update_date=10), 3), (saved, 4)]:
            with self.assertRaisesRegex(RuntimeError, "without subscription refetch"):
                validate_restart_result(saved, after, 3, count)

    def test_database_readback_is_readonly_and_identity_bound(self):
        for script_id in (201, 999):
            db = sqlite3.connect(":memory:")
            db.execute("CREATE TABLE profiles(id INTEGER,url TEXT,overwrite_type TEXT,script_id INTEGER,auto_update INTEGER,last_update_date INTEGER)")
            db.execute("INSERT INTO profiles VALUES(101, 'http://127.0.0.1:18766/profile.yaml', 'script', ?, 0, 100)", (script_id,))
            with patch("flclash_app_acceptance.sqlite3.connect", return_value=db) as connect:
                if script_id == 201:
                    self.assertEqual(read_fixture_profile("synthetic.sqlite")["last_update_date"], 100)
                else:
                    with self.assertRaises(RuntimeError):
                        read_fixture_profile("synthetic.sqlite")
                self.assertTrue(connect.call_args.args[0].endswith("?mode=ro"))
                self.assertTrue(connect.call_args.kwargs["uri"])

    def test_final_cleanup_rejects_orphan_and_remaining_listener(self):
        clean = {"processes": [], "tcp_listeners": [], "udp_endpoints": []}
        validate_final_inventory(clean, [{"pid": 7, "alive": False}])
        for key in clean:
            with self.subTest(key=key), self.assertRaises(RuntimeError):
                validate_final_inventory(dict(clean, **{key: [{"pid": 8}]}), [{"pid": 7, "alive": False}])
        with self.assertRaises(RuntimeError):
            validate_final_inventory(clean, [{"pid": 7, "alive": True}])
        with self.assertRaises(RuntimeError):
            validate_final_inventory({}, [])

    @unittest.skipUnless(shutil.which("powershell.exe"), "Windows PowerShell unavailable")
    def test_selector_with_synthetic_controls_only(self):
        script = Path(__file__).with_name("test_flclash_ui_policy.ps1")
        subprocess.run(["powershell.exe", "-NoProfile", "-NonInteractive", "-File", str(script)],
                       capture_output=True, text=True, timeout=20, check=True)

    def test_default_is_inert(self):
        script = Path(__file__).with_name("flclash_app_acceptance.py")
        result = subprocess.run([sys.executable, "-B", str(script)], capture_output=True, text=True, check=True)
        plan = json.loads(result.stdout)
        self.assertFalse(plan["app_tested"])
        self.assertFalse(plan["release_execution_verified"])

    def test_host_and_missing_permission_block_before_app_access(self):
        guest = {"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted", "RUNNER_OS": "Windows",
                 "GITHUB_REPOSITORY": "Sonderjyf/mihomo-route-agent", "GITHUB_RUN_ID": "123"}
        require_guest(True, guest, "nt")
        for allowed, env, platform in [(False, guest, "nt"), (True, {}, "nt"), (True, guest, "posix"),
                                       (True, dict(guest, RUNNER_ENVIRONMENT="self-hosted"), "nt")]:
            with self.subTest(env=env), self.assertRaises(RuntimeError):
                require_guest(allowed, env, platform)

    def test_preseed_uses_pinned_store_prefix_and_rejects_unsafe_defaults(self):
        raw = preferences()
        self.assertIsNone(validate_preferences(raw)["currentProfileId"])
        self.assertEqual(validate_preferences(preferences(101), 101)["currentProfileId"], 101)
        with self.assertRaisesRegex(RuntimeError, "selected profile"):
            validate_preferences(preferences(102), 101)
        for group, key in [("networkProps", "systemProxy"), ("networkProps", "autoSetSystemDns"),
                           ("appSettingProps", "autoRun"), ("appSettingProps", "autoCheckUpdate")]:
            candidate = json.loads(raw["flutter.config"])
            candidate[group][key] = True
            with self.subTest(key=key), self.assertRaises(RuntimeError):
                validate_preferences(dict(raw, **{"flutter.config": json.dumps(candidate)}))
        candidate = json.loads(raw["flutter.config"])
        candidate["patchClashConfig"]["tun"]["enable"] = True
        with self.assertRaises(RuntimeError):
            validate_preferences(dict(raw, **{"flutter.config": json.dumps(candidate)}))

    def test_database_seed_refuses_unknown_or_existing_app_state(self):
        with closing(sqlite3.connect(":memory:")) as db:
            db.execute("PRAGMA user_version=9")
            with self.assertRaises(RuntimeError):
                seed_tables(db)
            db.execute("PRAGMA user_version=10")
            db.execute("CREATE TABLE profiles(id INTEGER)")
            db.execute("CREATE TABLE scripts(id INTEGER)")
            db.execute("INSERT INTO profiles VALUES(99)")
            with self.assertRaises(RuntimeError):
                seed_tables(db)

    @unittest.skipUnless(shutil.which("node"), "Node unavailable for offline JS fixture validation")
    def test_actual_overwrite_source_handles_fresh_A_B_and_rejects_duplicates(self):
        script = overwrite_script() + '''
const input=JSON.parse(process.argv[1]);
const output=main(input);
let rejected=false;
try { main(output); } catch(e) { rejected=true; }
console.log(JSON.stringify({output,rejected}));
'''
        for revision in ("A", "B"):
            original = profile(revision)
            result = subprocess.run([shutil.which("node"), "-e", script, json.dumps(original)], capture_output=True, text=True, check=True)
            data = json.loads(result.stdout)
            self.assertTrue(data["rejected"])
            effective = data["output"]
            effective["external-controller"] = "127.0.0.1:9090"
            report = {}
            self.assertTrue(check_synthetic_config(revision, effective, report)["config_contract_passed"])
            self.assertEqual(report["dns_diagnostics"][revision]["differences"], [])
            for key, value in [("enhanced-mode", "redir-host"), ("fake-ip-range", "198.19.0.1/16"),
                               ("fake-ip-filter", []), ("nameserver-policy", {}),
                               ("nameserver", ["system://"]), ("unknown-field", False)]:
                candidate = json.loads(json.dumps(effective))
                candidate["dns"][key] = value
                with self.subTest(revision=revision, key=key), self.assertRaisesRegex(ValueError, "DNS changed"):
                    check_synthetic_config(revision, candidate, report)
                self.assertTrue(report["dns_diagnostics"][revision]["differences"])


if __name__ == "__main__":
    unittest.main()
