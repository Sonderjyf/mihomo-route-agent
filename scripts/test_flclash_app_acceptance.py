from contextlib import closing
import json
from pathlib import Path
import shutil
import sqlite3
import subprocess
import sys
import unittest
from unittest.mock import patch

from flclash_acceptance import check_config
from flclash_app_acceptance import preferences, validate_preferences, profile, overwrite_script, require_guest, seed_tables, ps, validate_refresh_result, validate_final_inventory, read_fixture_profile


class AppPreparationTests(unittest.TestCase):
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
                       capture_output=True, text=True, timeout=10, check=True)

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
            self.assertTrue(check_config(original, effective, "LAB")["config_contract_passed"])


if __name__ == "__main__":
    unittest.main()
