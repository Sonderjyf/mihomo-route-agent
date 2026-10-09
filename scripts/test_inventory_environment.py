"""Mock-only checks of the secret-free hosted regression driver."""
import contextlib
import io
import json
import subprocess
import unittest
from types import SimpleNamespace
from unittest.mock import patch
import check_inventory_environment as check


CANARY = 'SYNTHETIC_PRIVATE_OUTPUT_CANARY'


def result(timeout=False):
    return SimpleNamespace(returncode=0, stdout=json.dumps(dict(
        inventory=dict(reason='query_timeout' if timeout else 'none',
                       elapsed_ms=15001 if timeout else 500, budget_ms=15000,
                       query_exit_code=None if timeout else 0, stderr_present=None),
        physical_guard='not_run' if timeout else 'passed',
        physical_guard_elapsed_ms=0 if timeout else 500,
        physical_guard_budget_ms=3000, target_packets_sent=False)).encode())


class InventoryEnvironmentChecks(unittest.TestCase):
    def run_driver(self, outcomes, extra=None):
        env = dict(GITHUB_ACTIONS='true', RUNNER_ENVIRONMENT='github-hosted',
                   GITHUB_REPOSITORY=check.acceptance.REPO, PATH='synthetic',
                   PSModuleAnalysisCachePath='C:\\synthetic cache\\file')
        env.update(extra or {})
        capture = io.StringIO()
        with patch.dict(check.os.environ, env, clear=True), \
             patch.object(check.sys, 'platform', 'win32'), \
             patch.object(check.sys, 'argv', ['check', 'synthetic-test.exe']), \
             patch.object(check.subprocess, 'run', side_effect=outcomes) as run, \
             contextlib.redirect_stdout(capture):
            code = check.main()
        self.assertNotIn(CANARY, capture.getvalue())
        return code, run, [json.loads(line) for line in capture.getvalue().splitlines()]

    def test_fixed_worker_required_and_legacy_observation_only(self):
        for legacy_timeout in (True, False):
            code, run, rows = self.run_driver([result(), result(), result(legacy_timeout), result()])
            self.assertEqual(code, 0)
            self.assertEqual([row['success_required'] for row in rows], [True, True, False, True])
            self.assertEqual(rows[0]['case'], 'worker_fixed_first')
            environments = [call.kwargs['env'] for call in run.call_args_list]
            self.assertIn('PSMODULEANALYSISCACHEPATH', {k.upper() for k in environments[0]})
            self.assertNotIn('PSMODULEANALYSISCACHEPATH', {k.upper() for k in environments[2]})
            self.assertNotIn('GITHUB_ACTIONS', environments[0])
            self.assertTrue(all(call.kwargs['timeout'] == 25 for call in run.call_args_list))

    def test_positive_failure_and_unavailable_child_fail_closed(self):
        for position in (0, 1, 3):
            outcomes = [result(), result(), result(True), result()]
            outcomes[position] = result(True)
            self.assertEqual(self.run_driver(outcomes)[0], 1)
        for bad in (SimpleNamespace(returncode=0, stdout=CANARY.encode()),
                    subprocess.TimeoutExpired(CANARY, 25, output=CANARY, stderr=CANARY)):
            code, _, rows = self.run_driver([result(), result(), bad, result()])
            self.assertEqual(code, 1)
            self.assertEqual(rows[2]['diagnostic'], 'child_result_unavailable')

    def test_secret_bearing_environment_refused_before_spawn(self):
        for key in ('OPENROUTER_API_KEY', 'VLESS_NODE_JSON', 'REAL_CONTROLLER_SECRET'):
            with self.assertRaises(check.acceptance.Refused):
                self.run_driver([AssertionError('must not spawn')], {key: CANARY})


if __name__ == '__main__':
    unittest.main()
