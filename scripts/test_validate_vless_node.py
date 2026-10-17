import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import mock_open, patch

import validate_vless_node as validator


CANARY = "SYNTHETIC_SECRET_CANARY_84a2c0"


def fixture():
    return dict(server="8.8.8.8", port=443,
                uuid="00000000-0000-0000-0000-000000000000",
                servername="example.com", reality_public_key="A" * 43,
                short_id="")


class OfflineNodeValidator(unittest.TestCase):
    def invoke(self, raw, args=()):
        stdout, stderr = io.StringIO(), io.StringIO()
        with patch.object(sys, "stdin", io.StringIO(raw)), \
                contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            code = validator.main(list(args))
        self.assertEqual(stderr.getvalue(), "")
        self.assertNotIn(CANARY, stdout.getvalue())
        report = json.loads(stdout.getvalue())
        self.assertEqual(set(report), {"valid", "error", "line", "column", "fields", "expected_type"})
        self.assertEqual(code, 0 if report["valid"] else 1)
        return report

    def test_success_reuses_runtime_without_network_processes_or_writes(self):
        raw = json.dumps(fixture())
        with patch.object(validator.acceptance, "node_input", wraps=validator.acceptance.node_input) as runtime, \
                patch("builtins.open", side_effect=AssertionError("unexpected file access")), \
                patch("socket.socket", side_effect=AssertionError("unexpected socket")), \
                patch("subprocess.Popen", side_effect=AssertionError("unexpected process")):
            self.assertTrue(self.invoke(raw)["valid"])
            runtime.assert_called_once_with(raw)

    def test_bad_json_quotes_fences_bom_and_literal_escapes_have_safe_coordinates(self):
        cases = [('{\n"uuid" "' + CANARY + '"\n}', 2, 8),
                 ('{\u201cuuid\u201d: \u201c' + CANARY + '\u201d}', 1, 2),
                 ('```json\n' + CANARY + '\n```', 1, 1),
                 ('\ufeff{"uuid":"' + CANARY + '"}', 1, 1),
                 ('{\\n"uuid":"' + CANARY + '"}', 1, 2)]
        for raw, line, column in cases:
            with self.subTest(line=line, column=column):
                report = self.invoke(raw)
                self.assertEqual((report["error"], report["line"], report["column"]), ("invalid_json", line, column))

    def test_duplicate_unknown_keys_and_types_never_echo_secrets(self):
        cases = [('{"' + CANARY + '":1,"' + CANARY + '":2}', "duplicate_key", [], ""),
                 (json.dumps(dict(fixture(), **{CANARY: CANARY})), "unknown_fields", [], ""),
                 (json.dumps(dict(fixture(), port=CANARY)), "type_mismatch", ["port"], "integer"),
                 (json.dumps(dict(fixture(), port=True)), "type_mismatch", ["port"], "integer"),
                 (json.dumps(dict(fixture(), uuid=CANARY)), "invalid_value", ["uuid"], ""),
                 (json.dumps([CANARY]), "type_mismatch", [], "object"),
                 ('{"port":NaN}', "nonfinite_json", [], ""),
                 (' ' * 16385 + CANARY, "input_size", [], "")]
        for raw, reason, fields, expected in cases:
            with self.subTest(reason=reason):
                report = self.invoke(raw)
                self.assertEqual((report["error"], report["fields"], report["expected_type"]), (reason, fields, expected))
                self.assertIsNone(report["line"])
                self.assertIsNone(report["column"])

    def test_explicit_file_is_read_only_and_path_not_echoed(self):
        source = mock_open(read_data=json.dumps(fixture()))
        with patch("builtins.open", source):
            self.assertTrue(self.invoke("", ["--file", CANARY])["valid"])
        source.assert_called_once_with(CANARY, "r", encoding="utf-8", newline="")
        source().read.assert_called_once_with(16385)
        source().write.assert_not_called()

    def test_argument_io_encoding_interrupt_and_unexpected_errors_are_fixed(self):
        self.assertEqual(self.invoke("", [CANARY])["error"], "invalid_arguments")
        for error, reason in [(OSError(CANARY), "io_error"),
                              (UnicodeError(CANARY), "invalid_encoding"),
                              (KeyboardInterrupt(CANARY), "interrupted"),
                              (RuntimeError(CANARY), "unexpected_error")]:
            with patch("builtins.open", side_effect=error):
                self.assertEqual(self.invoke("", ["--file", CANARY])["error"], reason)

    def test_cli_stdin_utf8_and_env_transport_with_synthetic_input(self):
        # Exercise the actual CLI boundary, including CRLF/Unicode, no file input.
        script = Path(__file__).with_name("validate_vless_node.py")
        for raw, reason in [(json.dumps(fixture(), indent=2).replace("\n", "\r\n"), "none"),
                            ('{\u201cuuid\u201d:"' + CANARY + '"}', "invalid_json")]:
            child = subprocess.run([sys.executable, "-B", "-X", "utf8", str(script)],
                                   input=raw.encode("utf-8"), capture_output=True, timeout=10)
            self.assertEqual(child.stderr, b"")
            self.assertNotIn(CANARY.encode(), child.stdout)
            self.assertEqual(json.loads(child.stdout)["error"], reason)
        # Mirrors runner env -> os.environ -> node_input; no shell interpolation.
        import os
        raw = json.dumps(fixture(), indent=2).replace("\n", "\r\n")
        child_env = {k: v for k, v in os.environ.items()
                     if k.upper() in {"SYSTEMROOT", "WINDIR", "SYSTEMDRIVE", "TEMP", "TMP", "PATH"}}
        child_env["VLESS_NODE_JSON"] = raw
        code = "import os,sys;sys.path.insert(0,'scripts');from real_acceptance import node_input;node_input(os.environ['VLESS_NODE_JSON']);print('valid')"
        child = subprocess.run([sys.executable, "-B", "-X", "utf8", "-c", code],
                               cwd=script.resolve().parents[1], env=child_env,
                               capture_output=True, timeout=10)
        self.assertEqual(child.returncode, 0)
        self.assertEqual(child.stdout.strip(), b"valid")
        self.assertEqual(child.stderr, b"")


if __name__ == "__main__":
    unittest.main()
