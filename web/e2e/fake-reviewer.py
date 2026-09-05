#!/usr/bin/env python3
"""Headless reviewer protocol with a controllable end, for the isolated test server."""
import json, os, sys, time, uuid
from pathlib import Path
root = Path(os.environ['FLYWHEEL_E2E_HARNESS_HOLD_DIR'])
assert root.parent.name.startswith('flywheel-hardening.')
if 'exec' not in sys.argv:
    print('fake-reviewer 1.0')
    sys.exit(0)
sys.stdin.read()
assert '-m' in sys.argv and sys.argv[sys.argv.index('-m')+1] == 'browser-selected-model', 'Selected review model was lost'
print(json.dumps({'type':'thread.started','thread_id':str(uuid.uuid4())}), flush=True)
print(json.dumps({'type':'item.completed','item':{'type':'reasoning','text':'Private test reasoning'}}), flush=True)
print(json.dumps({'type':'item.started','item':{'type':'command_execution','command':'Read example.txt'}}), flush=True)
deadline = time.monotonic() + 45
while not (root / 'review.release').exists() and time.monotonic() < deadline:
    time.sleep(0.1)
assert (root / 'review.release').exists(), 'Browser did not release the reviewer'
result = json.dumps({'summary':'No blocking findings. Recommend approval.','findings':[]})
Path(sys.argv[sys.argv.index('-o')+1]).write_text(result)
print(json.dumps({'type':'turn.completed','usage':{'input_tokens':30,'output_tokens':10}}), flush=True)
