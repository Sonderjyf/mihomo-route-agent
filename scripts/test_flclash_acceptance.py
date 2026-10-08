import copy
import unittest

from flclash_acceptance import MANAGED, check_config, plan


class ConfigContractTests(unittest.TestCase):
    def setUp(self):
        self.original = {
            "rules": ["DOMAIN,known.route-lab.test,DIRECT", "MATCH,LAB"],
            "dns": {"enable": True, "enhanced-mode": "fake-ip", "nameserver": ["127.0.0.1:15356"]},
            "proxy-groups": [{"name": "LAB", "type": "select", "proxies": ["DIRECT"]}],
            "rule-providers": {"original": {"type": "file", "behavior": "domain", "path": "old.yaml"}},
        }
        self.effective = copy.deepcopy(self.original)
        self.effective.update({"mode": "rule", "tun": {"enable": False}, "allow-lan": False,
                               "external-controller": "127.0.0.1:9090"})
        self.effective["rules"][1:1] = [
            f"AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,{name})),{target}"
            for name, target in zip(MANAGED, ("DIRECT", "LAB"))]
        self.effective["rule-providers"]["original"]["path"] = "guest/profiles/providers/original"
        for name in MANAGED:
            self.effective["rule-providers"][name] = {
                "type": "http", "behavior": "classical", "format": "yaml", "proxy": "DIRECT",
                "url": f"http://127.0.0.1:18765/rules/{name}.yaml", "interval": 3600,
                "path": f"guest/profiles/providers/{name}",
            }

    def check(self):
        return check_config(self.original, self.effective, "LAB")

    def test_valid_contract_is_not_app_acceptance(self):
        result = self.check()
        self.assertTrue(result["config_contract_passed"])
        self.assertFalse(result["app_tested"])
        self.assertEqual(result["original_provider_paths_rewritten"], ["original"])
        self.assertFalse(plan()["execution_implemented"])

    def test_rule_loss_duplication_and_scope_expansion(self):
        expected = self.effective["rules"][:]
        for bad in [expected[1:], expected + [expected[-2]], expected[:1] + expected[3:],
                    [r.replace("DST-PORT,443", "DST-PORT,8443") for r in expected]]:
            with self.subTest(rules=bad), self.assertRaises(ValueError):
                self.effective["rules"] = bad
                self.check()

    def test_refresh_requires_new_original_rule(self):
        self.original["rules"].insert(0, "DOMAIN,new.route-lab.test,DIRECT")
        with self.assertRaises(ValueError):
            self.check()
        self.effective["rules"].insert(0, "DOMAIN,new.route-lab.test,DIRECT")
        self.assertTrue(self.check()["config_contract_passed"])

    def test_dns_override_is_detected(self):
        self.effective["dns"]["nameserver"].append("system://")
        with self.assertRaises(ValueError):
            self.check()

    def test_provider_redirection_and_loss_rejected(self):
        for key, value in [("proxy", "LAB"), ("url", "https://external.invalid/payload"), ("path", "")]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                candidate = copy.deepcopy(self.effective)
                candidate["rule-providers"][MANAGED[0]][key] = value
                check_config(self.original, candidate, "LAB")
        del self.effective["rule-providers"]["original"]
        with self.assertRaises(ValueError):
            self.check()

    def test_tun_or_controller_drift_rejected(self):
        for field, value in [("tun", {"enable": True}), ("external-controller", "127.0.0.1:19091"),
                             ("allow-lan", True), ("mode", "global")]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                candidate = copy.deepcopy(self.effective)
                candidate[field] = value
                check_config(self.original, candidate, "LAB")


if __name__ == "__main__":
    unittest.main()
