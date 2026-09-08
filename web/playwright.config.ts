import { defineConfig } from '@playwright/test'
export default defineConfig({
  testDir: './e2e', workers: 1, fullyParallel: false, timeout: 30_000,
  use: { baseURL: process.env.FLYWHEEL_E2E_BASE_URL ?? 'http://127.0.0.1:8091', viewport: { width: 1440, height: 1000 }, trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  reporter: [['list'], ['json', { outputFile: 'test-results/results.json' }], ['html', { open: 'never' }]],
})
