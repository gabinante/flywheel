#!/usr/bin/env python3
"""Fetch the browser test PR from its local fixture while keeping a GitHub identity."""
import os, sys
from pathlib import Path
args = sys.argv[1:]
root = Path(os.environ['FLYWHEEL_E2E_HARNESS_HOLD_DIR']).parent
assert root.name.startswith('flywheel-hardening.')
fixture = (root / 'work' / 'review-fixture').resolve()
if args and args[0] == 'fetch' and Path.cwd() == fixture and 'origin' in args:
    args[args.index('origin')] = str(fixture)
os.execv(os.environ['FLYWHEEL_E2E_REAL_GIT'], ['git', *args])
