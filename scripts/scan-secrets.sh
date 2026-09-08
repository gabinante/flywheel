#!/usr/bin/env bash
# Scan only tracked working-tree files, then all available Git refs. Never print secrets.
set -euo pipefail
cd "$(dirname "$0")/.."
scanner="${GITLEAKS_BIN:-gitleaks}"
if ! command -v "$scanner" >/dev/null 2>&1; then
  echo 'Install Gitleaks v8.24.3 or set GITLEAKS_BIN to its path.' >&2
  exit 2
fi
if [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
  echo 'Full history is required: run git fetch --unshallow before scanning.' >&2
  exit 2
fi
git rev-parse --verify HEAD >/dev/null
repo=$(pwd)
snapshot=$(mktemp -d "${TMPDIR:-/tmp}/flywheel-secret-scan.XXXXXX")
trap 'rm -rf "$snapshot"' EXIT
python3 - "$snapshot" <<'PY'
import os, shutil, subprocess, sys
from pathlib import Path
root = Path(sys.argv[1])
for raw in subprocess.check_output(['git', 'ls-files', '-z']).split(b'\0'):
    if not raw:
        continue
    source = Path(os.fsdecode(raw))
    if not source.exists() and not source.is_symlink():
        continue  # A tracked deletion in the working tree.
    target = root / source
    target.parent.mkdir(parents=True, exist_ok=True)
    if source.is_symlink():
        target.write_text(os.readlink(source))  # Never follow links outside the checkout.
    else:
        shutil.copyfile(source, target)
PY
"$scanner" dir "$snapshot" --config "$repo/.gitleaks.toml" --gitleaks-ignore-path "$repo/.gitleaksignore" --redact=100 --ignore-gitleaks-allow
"$scanner" git "$repo" --config "$repo/.gitleaks.toml" --gitleaks-ignore-path "$repo/.gitleaksignore" --log-opts='--all --full-history' --redact=100 --ignore-gitleaks-allow
