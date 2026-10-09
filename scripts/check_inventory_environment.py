"""Hosted secret-free comparison; read-only route tables, never target packets."""
import json
import os
import subprocess
import sys
import real_acceptance as acceptance


def main():
    acceptance.require(sys.platform == 'win32' and os.environ.get('GITHUB_ACTIONS') == 'true'
                       and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
                       and os.environ.get('GITHUB_REPOSITORY') == acceptance.REPO)
    acceptance.require(len(sys.argv) == 2)
    acceptance.require(not any(os.environ.get(k) for k in ('OPENROUTER_API_KEY', 'VLESS_NODE_JSON', 'REAL_CONTROLLER_SECRET')))
    minimal = acceptance.child_env(os.environ)
    runtime_names = {'USERPROFILE', 'HOMEDRIVE', 'HOMEPATH', 'APPDATA', 'LOCALAPPDATA', 'PROGRAMDATA', 'PROGRAMFILES', 'PROGRAMFILES(X86)', 'PROGRAMW6432', 'COMMONPROGRAMFILES', 'COMMONPROGRAMFILES(X86)', 'COMMONPROGRAMW6432', 'COMSPEC', 'PSMODULEPATH'}
    runtime_env = dict(minimal)
    runtime_env.update({k: v for k, v in os.environ.items() if k.upper() in runtime_names})
    identity_names = {'COMPUTERNAME', 'USERNAME', 'USERDOMAIN', 'USERDNSDOMAIN', 'USERDOMAIN_ROAMINGPROFILE', 'LOGONSERVER', 'OS', 'PROCESSOR_ARCHITECTURE', 'PROCESSOR_IDENTIFIER', 'NUMBER_OF_PROCESSORS'}
    computer_env = dict(minimal)
    computer_env.update({k: v for k, v in os.environ.items() if k.upper() == 'COMPUTERNAME'})
    identity_env = dict(minimal)
    identity_env.update({k: v for k, v in os.environ.items() if k.upper() in identity_names})
    combined_env = dict(runtime_env)
    combined_env.update({k: v for k, v in os.environ.items() if k.upper() in identity_names})
    without_computer = {k: v for k, v in os.environ.items() if k.upper() != 'COMPUTERNAME'}
    cases = [('runner_baseline', dict(os.environ)), ('worker_minimal', minimal),
             ('minimal_plus_computername', computer_env), ('minimal_plus_identity', identity_env),
             ('runtime_plus_identity', combined_env), ('baseline_without_computername', without_computer)]
    passed = True
    for label, env in cases:
        env['ROUTE_AGENT_INVENTORY_CHILD'] = '1'
        try:
            child = subprocess.run([sys.argv[1], '-test.run=^TestAcceptanceInventoryEnvironmentChild$'], env=env, capture_output=True, timeout=25)
            acceptance.require(child.returncode == 0)
            row = json.loads(child.stdout)
            acceptance.require(set(row) == {'inventory', 'physical_guard', 'physical_guard_elapsed_ms', 'physical_guard_budget_ms', 'target_packets_sent'})
            # Reuse the production output contract for the nested safe diagnostic.
            acceptance.output_contract(json.dumps(dict(stage='route_inventory', reason='guard_rejected', route_inventory=row['inventory'], hosts=[], model_attempts=0, api_requests=[], routing_updated=False, tun_tested=False)))
            acceptance.require(row['physical_guard'] in {'passed', 'rejected', 'not_run'})
            acceptance.require(type(row['physical_guard_elapsed_ms']) is int and 0 <= row['physical_guard_elapsed_ms'] <= 25000)
            acceptance.require(row['physical_guard_budget_ms'] == 3000 and row['target_packets_sent'] is False)
            print(json.dumps(dict(case=label, **row)), flush=True)
            passed = passed and row['inventory']['reason'] == 'none' and row['physical_guard'] == 'passed'
        except Exception:
            print(json.dumps({'case': label, 'diagnostic': 'child_result_unavailable'}), flush=True)
            passed = False
    return 0 if passed else 1


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception:
        print(json.dumps({'diagnostic': 'comparison_refused'}))
        raise SystemExit(1)
