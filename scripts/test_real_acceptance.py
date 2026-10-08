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
    def test_execute_cleanup_at_every_failure_boundary(self):
        for failure in ("spawn", "worker_timeout", "invalid_output", "stop"):
            with self.subTest(failure=failure), test_directory() as directory:
                root = Path(directory)
                core_bytes = b"not an executable"
                (root / "mihomo.exe").write_bytes(core_bytes)
                (root / "worker.exe").write_bytes(core_bytes)
                digest = a.hashlib.sha256(core_bytes).hexdigest()
                (root / "worker.sha256").write_text(digest, encoding="ascii")
                values = dict(env(), JOB_STARTED_AT=str(a.time.time()), VLESS_NODE_JSON=json.dumps(node()), OPENROUTER_API_KEY="synthetic-not-a-key")
                core = Mock(pid=1234)
                core.poll.return_value = None
                with patch.object(a.tempfile, "mkdtemp", side_effect=private_fixture), patch.object(a.sys, "platform", "win32"), patch.object(a, "CORE_HASH", digest), patch.object(a, "ports_free"), patch.object(a.socket, "create_connection") as connect, patch.object(a.subprocess, "Popen", return_value=core) as spawn, patch.object(a.subprocess, "run") as run, patch.object(a, "stop_owned") as stop:
                    connect.return_value.__enter__ = Mock()
                    connect.return_value.__exit__ = Mock()
                    if failure == "spawn":
                        spawn.side_effect = RuntimeError("secret-spawn-details")
                    elif failure == "worker_timeout":
                        run.side_effect = a.subprocess.TimeoutExpired("secret", 270)
                    else:
                        run.return_value.stdout = b'{"secret":"unapproved"}'
                    if failure == "stop":
                        stop.side_effect = RuntimeError("secret-cleanup-details")
                    with self.assertRaises(Exception):
                        a.execute(root, values)
                    self.assertFalse(list(root.glob("private-*")))
                    stop.assert_called_once()
                    if failure != "spawn":
                        self.assertNotIn("OPENROUTER_API_KEY", spawn.call_args.kwargs["env"])
                        self.assertNotIn("VLESS_NODE_JSON", run.call_args.kwargs["env"])

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
        self.assertEqual(json.loads(capture.getvalue())["details"], "redacted")

    def test_owned_process_cleanup_timeout(self):
        p = Mock()
        p.poll.return_value = None
        p.wait.side_effect = [a.subprocess.TimeoutExpired("private-command", 10), 0]
        a.stop_owned(p)
        p.terminate.assert_called_once()
        p.kill.assert_called_once()
        self.assertEqual(p.wait.call_count, 2)

    def test_output_contract_rejects_unreviewed_fields_and_secret_values(self):
        row = dict(index=0, probe=dict(evidence=dict(direct_tls="verified_success", proxy_tls="not_tested"), dns="resolved", direct_attempts=["verified_success"], proxy_attempt="not_tested"), vless_tls="verified_success", model="answered", choice="UNCERTAIN", accepted="UNCERTAIN")
        second = copy.deepcopy(row)
        second["index"] = 1
        result = dict(hosts=[row, second], model_attempts=2, routing_updated=False, tun_tested=False)
        self.assertEqual(a.output_contract(json.dumps(result)), result)
        for change in (dict(result, token="sensitive"), dict(result, model_attempts=True), dict(result, routing_updated=True)):
            with self.assertRaises(a.Refused):
                a.output_contract(json.dumps(change))
        row["model"] = "sensitive-body"
        with self.assertRaises(a.Refused):
            a.output_contract(json.dumps(result))


if __name__ == "__main__":
    unittest.main()
