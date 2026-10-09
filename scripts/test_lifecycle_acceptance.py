import json
from pathlib import Path
import subprocess
import sys
import unittest

import lifecycle_acceptance as acceptance


class LifecycleGuards(unittest.TestCase):
    def test_default_is_inert_and_does_not_claim_real_paths(self):
        output=subprocess.check_output([sys.executable,"-B",str(Path(__file__).with_name("lifecycle_acceptance.py"))],text=True)
        plan=json.loads(output)
        self.assertEqual(plan["status"],"not_run")
        for field in ("app_tested","tun_tested","physical_path_tested","host_changes","system_tasks","services"):
            self.assertFalse(plan[field])
        self.assertEqual(plan["model_requests"],0)

    def test_permission_does_not_accept_host_or_self_hosted(self):
        env={"GITHUB_ACTIONS":"true","RUNNER_ENVIRONMENT":"github-hosted","RUNNER_OS":"Windows",
             "GITHUB_REPOSITORY":"Sonderjyf/mihomo-route-agent","GITHUB_RUN_ID":"123"}
        acceptance.require_permission(True,env,"nt")
        for allowed,environment,platform in ((False,env,"nt"),(True,{},"nt"),(True,env,"posix"),(True,{**env,"RUNNER_ENVIRONMENT":"self-hosted"},"nt")):
            with self.assertRaises(RuntimeError): acceptance.require_permission(allowed,environment,platform)

    def test_real_execution_requires_explicit_binary_and_sha_arguments(self):
        result=subprocess.run([sys.executable,"-B",str(Path(__file__).with_name("lifecycle_acceptance.py")),"--execute"],capture_output=True,text=True)
        self.assertNotEqual(result.returncode,0)
        self.assertIn("approved SHA required",result.stderr)

    def test_cleanup_failure_is_retained_without_skipping_other_cleanup(self):
        errors=[]
        def fail(): raise RuntimeError("owned PID identity changed")
        self.assertIsNone(acceptance.cleanup_attempt(errors,fail))
        self.assertEqual(acceptance.cleanup_attempt(errors,lambda: "next cleanup completed"),"next cleanup completed")
        self.assertEqual(errors,["owned PID identity changed"])
