#!/usr/bin/env python3
"""Local planner protocol fixture; never launches tools or contacts external services."""
import json, os, sys, time, uuid
from pathlib import Path
root = Path(os.environ['FLYWHEEL_E2E_HARNESS_HOLD_DIR'])
assert root.name == 'holds' and root.parent.name.startswith('flywheel-hardening.')
if '--version' in sys.argv:
    print('isolated-planner 1.0')
    sys.exit(0)
session = sys.argv[sys.argv.index('--resume') + 1] if '--resume' in sys.argv else str(uuid.uuid4())
print(json.dumps({'type': 'system', 'subtype': 'init', 'session_id': session}), flush=True)
print(json.dumps({'type': 'assistant', 'message': {'content': [{'type': 'text', 'text': 'Inspecting the proposed change.'}]}}), flush=True)
deadline = time.monotonic() + 30
while not (root / 'planner.release').exists() and time.monotonic() < deadline:
    time.sleep(.1)
assert (root / 'planner.release').exists(), 'Test did not release planner'
print(json.dumps({'type': 'result', 'subtype': 'success', 'is_error': False, 'session_id': session, 'num_turns': 1, 'result': 'Start with a small ticket to validate the proposed change. Keep the existing behavior covered by tests.'}), flush=True)
