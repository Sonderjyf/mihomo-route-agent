import copy
import json
from pathlib import Path
import subprocess
import sys
import unittest

from tun_acceptance import (DEVICE, PREFIX, plan, require_hosted_guest_permission,
                            require_clean_guest, require_active_tun, require_clean_exit, require_tun_records, tun_config)


class AcceptanceGuards(unittest.TestCase):
    def test_pinned_core_tun_serialization_and_non_tun_rejection(self):
        require_tun_records([{"inbound_type": "Tun"}, {"inbound_type": "Tun"}])
        for records in [[], [{}], [{"inbound_type": "TUN"}], [{"inbound_type": "Socks5"}],
                        [{"inbound_type": "Tun"}, {"inbound_type": None}]]:
            with self.assertRaises(RuntimeError):
                require_tun_records(records)

    def test_default_command_is_an_inert_plan(self):
        output = subprocess.check_output([sys.executable, "-B", str(Path(__file__).with_name("tun_acceptance.py"))], text=True)
        result = json.loads(output)
        self.assertEqual(result["status"], "not_run")
        self.assertEqual(result["external_target_requests"], 0)
        self.assertFalse(result["host_flclash_changes"])
        self.assertTrue(result["requires_explicit_permission"])

    def test_host_self_hosted_linux_and_missing_approval_are_rejected(self):
        env = {"GITHUB_ACTIONS": "true", "RUNNER_ENVIRONMENT": "github-hosted", "RUNNER_OS": "Windows",
               "GITHUB_REPOSITORY": "Sonderjyf/mihomo-route-agent", "GITHUB_RUN_ID": "12345"}
        require_hosted_guest_permission(True, env, "nt", True)
        for field in env:
            altered = {**env, field: "wrong"}
            with self.assertRaises(RuntimeError):
                require_hosted_guest_permission(True, altered, "nt", True)
        for allowed, platform, admin in [(False, "nt", True), (True, "posix", True), (True, "nt", False)]:
            with self.assertRaises(RuntimeError):
                require_hosted_guest_permission(allowed, env, platform, admin)
        with self.assertRaises(RuntimeError):
            require_hosted_guest_permission(True, {}, "nt", True)

    def test_scope_and_cleanup_checks_reject_leaks_or_stale_state(self):
        before = {"unrelated_flclash_count": 0, "default_routes": [{"NextHop": "192.0.2.1"}],
                  "dns": [{"ServerAddresses": ["192.0.2.53"]}], "owned_routes": [], "owned_interfaces": [], "prefix_routes": []}
        require_clean_guest(before)
        active = copy.deepcopy(before)
        active["owned_routes"] = [{"DestinationPrefix": PREFIX}]
        active["prefix_routes"] = [{"InterfaceAlias": DEVICE, "DestinationPrefix": PREFIX}]
        require_active_tun(before, active)
        require_clean_exit(before, copy.deepcopy(before))
        with self.assertRaises(RuntimeError):
            require_clean_guest(active)
        with self.assertRaises(RuntimeError):
            require_active_tun(before, before)
        with self.assertRaises(RuntimeError):
            require_clean_exit(before, active)
        for key in ["default_routes", "dns"]:
            bad = copy.deepcopy(active)
            bad[key] = []
            with self.assertRaises(RuntimeError):
                require_active_tun(before, bad)
        bad = {**before, "unrelated_flclash_count": 1}
        with self.assertRaises(RuntimeError):
            require_clean_guest(bad)
        config = tun_config()
        self.assertEqual(config["route-address"], [PREFIX])
        self.assertEqual(config["dns-hijack"], [])
        self.assertFalse(config["strict-route"])
        self.assertEqual(plan()["route_prefix"], PREFIX)


if __name__ == "__main__":
    unittest.main()
