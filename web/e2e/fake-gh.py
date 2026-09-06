#!/usr/bin/env python3
"""Only local fixtures; no GitHub requests or publications in browser tests."""
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
root = Path(os.environ['FLYWHEEL_E2E_HARNESS_HOLD_DIR']).parent
assert root.name.startswith('flywheel-hardening.')
fixture = root / 'review-fixture.json'
prs = json.loads(fixture.read_text()) if fixture.exists() else []
if isinstance(prs, dict):
    prs = [prs]
pr = next((pr for pr in prs if len(args) > 2 and args[2] == str(pr['number'])), None)
if args[:2] == ['api', 'user']:
    print('flywheel-test' if '--jq' in args else '{"login":"flywheel-test","id":1}')
elif args[:2] == ['search', 'prs']:
    print('[]')  # Automatic external intake stays disabled.
elif args[:2] == ['api', 'graphql']:
    query = next((x for x in args if x.startswith('q=')), '')
    print(json.dumps({'data': {'search': {'nodes': prs if 'review-requested:' in query else []}}}))
elif pr and args[:3] == ['pr', 'view', str(pr['number'])]:
    fail_once = root / ('review-view-failure-' + str(pr['number']))
    if fail_once.exists():
        fail_once.unlink()
        print('HTTP 502: temporary GitHub failure', file=sys.stderr)
        sys.exit(1)
    print(json.dumps({**pr, 'latestReviews': [], 'commits': []}))
elif pr and args[:3] == ['pr', 'diff', str(pr['number'])]:
    print('diff --git a/example.txt b/example.txt\nnew file mode 100644\n--- /dev/null\n+++ b/example.txt\n@@ -0,0 +1 @@\n+example')
else:
    print('GitHub is disabled in the isolated browser test server', file=sys.stderr)
    sys.exit(1)
