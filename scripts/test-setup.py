#!/usr/bin/env python3
"""Exercise first-run configuration without touching the operator's services or .env."""
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parent.parent

class SetupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='flywheel-setup-test-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'scripts').mkdir()
        for name in ['.env.example', '.env.schema', 'scripts/dev-preflight.sh', 'scripts/varlock']:
            shutil.copy2(SOURCE / name, self.root / name)
        tools = self.root / 'tools'
        tools.mkdir()
        for name in ['go', 'node', 'npm', 'docker', 'migrate']:
            text = '#!/bin/sh\nexit 0\n'
            if name == 'go': text = '#!/bin/sh\necho "go version go1.26.1 test/test"\n'
            if name == 'node': text = '#!/bin/sh\nif [ "$1" = "--version" ]; then echo v22.12.0; fi\n'
            (tools / name).write_text(text)
            (tools / name).chmod(0o700)
        self.env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ['PATH'])
        for key in ['JWT_SECRET', 'DATABASE_URL', 'REDIS_URL', 'VARLOCK_ENV_FILE', 'VARLOCK_SCHEMA_FILE']:
            self.env.pop(key, None)

    def run_setup(self):
        return subprocess.run(['bash', 'scripts/dev-preflight.sh'], cwd=self.root, env=self.env, capture_output=True, text=True)

    def test_private_configuration_and_no_secret_output(self):
        result = self.run_setup()
        self.assertEqual(result.returncode, 0, result.stderr)
        env_file = self.root / '.env'
        self.assertEqual(stat.S_IMODE(env_file.stat().st_mode), 0o600)
        values = dict(line.split('=', 1) for line in env_file.read_text().splitlines() if line and not line.startswith('#') and '=' in line)
        self.assertEqual(len(values['JWT_SECRET']), 64)
        self.assertNotIn(values['JWT_SECRET'], result.stdout + result.stderr)
        self.assertEqual(values['REVIEW_ENABLED'], 'false')
        self.assertEqual(values['DISPATCH_ENABLED'], 'false')
        loaded = subprocess.run(['bash', 'scripts/varlock', 'load'], cwd=self.root, env=self.env, capture_output=True, text=True)
        self.assertEqual(loaded.returncode, 0, loaded.stderr)
        self.assertNotIn(values['JWT_SECRET'], loaded.stdout + loaded.stderr)
        self.assertIn('JWT_SECRET=<REDACTED>', loaded.stdout)

    def test_existing_configuration_is_untouched(self):
        self.assertEqual(self.run_setup().returncode, 0)
        before = (self.root / '.env').read_bytes()
        self.assertEqual(self.run_setup().returncode, 0)
        self.assertEqual((self.root / '.env').read_bytes(), before)

    def test_old_node_fails_before_creating_configuration(self):
        (self.root / 'tools/node').write_text('#!/bin/sh\nexit 1\n')
        result = self.run_setup()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / '.env').exists())

if __name__ == '__main__':
    unittest.main()
