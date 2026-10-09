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
    worker = acceptance.child_env(os.environ)
    acceptance.require(any(k.upper() == 'PSMODULEANALYSISCACHEPATH' for k in worker))
    legacy = {k: v for k, v in worker.items() if k.upper() != 'PSMODULEANALYSISCACHEPATH'}
    # Fixed worker runs first, before this comparison can warm a full-env query.
    # The removed-variable case is evidence, not a requirement that future
    # runner images must retain today's bug. All positive cases must pass.
    cases = [('worker_fixed_first', worker, True), ('runner_baseline', dict(os.environ), True),
             ('legacy_without_module_cache', legacy, False), ('worker_fixed_after', worker, True)]
    passed = True
    for label, source, required in cases:
        env = dict(source)
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
            print(json.dumps(dict(case=label, success_required=required, **row)), flush=True)
            if required:
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
