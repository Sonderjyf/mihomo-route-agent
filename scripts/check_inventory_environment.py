"""Hosted secret-free comparison; read-only route tables, never target packets."""
import json
import os
import subprocess
import sys
import time
import real_acceptance as acceptance


def phase_probe(label, env):
    # Test-only marker script, one unchanged 15s total budget. No raw output.
    phases = ['started', 'import_started', 'import_done', 'query_started', 'query_done']
    script = "$ErrorActionPreference='Stop'; 'started'; 'import_started'; Import-Module NetTCPIP -ErrorAction Stop; 'import_done'; 'query_started'; $r=@(Find-NetRoute -RemoteIPAddress '1.1.1.1'); if($r.Count -ne 2){exit 7}; $index=[int]$r[0].InterfaceIndex; if($index -lt 1){exit 8}; 'query_done'"
    start = time.monotonic()
    try:
        child = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', script], env=env, capture_output=True, timeout=15)
        raw, reason, code = child.stdout, 'completed' if child.returncode == 0 else 'exit_failed', child.returncode
    except subprocess.TimeoutExpired as error:
        raw, reason, code = error.stdout or b'', 'timeout', None
    lines = raw.decode('utf-8', errors='replace').splitlines()
    reached = [p for p in phases if p in lines]
    print(json.dumps({'case': label, 'phase_probe': reason, 'last_phase': reached[-1] if reached else 'not_observed', 'elapsed_ms': int((time.monotonic()-start)*1000), 'budget_ms': 15000, 'exit_code': code, 'target_packets_sent': False}), flush=True)


def main():
    acceptance.require(sys.platform == 'win32' and os.environ.get('GITHUB_ACTIONS') == 'true'
                       and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
                       and os.environ.get('GITHUB_REPOSITORY') == acceptance.REPO)
    acceptance.require(len(sys.argv) == 2)
    acceptance.require(not any(os.environ.get(k) for k in ('OPENROUTER_API_KEY', 'VLESS_NODE_JSON', 'REAL_CONTROLLER_SECRET')))
    minimal = acceptance.child_env(os.environ)
    candidates = [('process_policy', 'PSEXECUTIONPOLICYPREFERENCE'), ('module_cache', 'PSMODULEANALYSISCACHEPATH'), ('lockdown_hook', '__PSLOCKDOWNPOLICY')]
    cases = [('runner_baseline', dict(os.environ)), ('worker_minimal', minimal)]
    for label, key in candidates:
        selected = {k: v for k, v in os.environ.items() if k.upper() == key}
        print(json.dumps({'candidate': label, 'present': bool(selected)}), flush=True)
        if selected:
            added = dict(minimal); added.update(selected)
            removed = {k: v for k, v in os.environ.items() if k.upper() != key}
            cases.extend([('minimal_plus_' + label, added), ('baseline_without_' + label, removed)])
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
    phase_probe('baseline_phases', dict(os.environ))
    phase_probe('minimal_phases', minimal)
    return 0 if passed else 1


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception:
        print(json.dumps({'diagnostic': 'comparison_refused'}))
        raise SystemExit(1)
