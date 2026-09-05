import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

// Exercise the real REST API, queue, checkout preparation, and harness lifecycle.
// Only GitHub and the harness executable are fixtures; the operator's data is untouched.
test('review buttons acknowledge the queue immediately and lead to a live session', async ({ page, request, baseURL }) => {
  test.setTimeout(90_000)
  expect(new URL(baseURL!).port).toBe('8091')
  const hold = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR!
  const root = dirname(hold)
  expect(root).toContain('flywheel-hardening.')
  const fixture = join(root, 'work', 'review-fixture')
  mkdirSync(fixture, { recursive: true })
  const git = (...args: string[]) => execFileSync(process.env.FLYWHEEL_E2E_REAL_GIT!, args, { cwd: fixture }).toString().trim()
  git('init', '-b', 'main')
  git('config', 'user.name', 'Flywheel Test')
  git('config', 'user.email', 'flywheel-test@localhost')
  git('remote', 'add', 'origin', 'git@github.com:flywheel-tests/review-fixture.git')
  writeFileSync(join(fixture, 'example.txt'), 'example\n')
  git('add', 'example.txt'); git('commit', '-m', 'Local review fixture')
  git('update-ref', 'refs/pull/7/head', 'HEAD')
  const url = 'https://github.com/flywheel-tests/review-fixture/pull/7'
  writeFileSync(join(root, 'review-fixture.json'), JSON.stringify({ number: 7, title: 'Explicit review request fixture', repository: { nameWithOwner: 'flywheel-tests/review-fixture' }, author: { login: 'author' }, state: 'OPEN', url, headRefName: 'review-fixture', headRefOid: git('rev-parse', 'HEAD'), baseRefName: 'main', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), reviewRequests: { nodes: [{ requestedReviewer: { login: 'flywheel-test' } }] } }))
  const settings = await (await request.get('/settings')).json()
  const saved = structuredClone(settings)
  settings.review.enabled = false
  settings.review.watch_requested = false
  settings.review.watch_authored = false
  settings.review.publish = false
  settings.review.harness = 'codex'
  settings.harnesses.codex.bin = fileURLToPath(new URL('./fake-reviewer.py', import.meta.url))
  settings.harnesses.codex.model = 'browser-selected-model'
  expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
  try {
    expect((await request.get('/me/reviews?refresh=true')).ok()).toBeTruthy()
    await page.goto('/#/my/reviews')
    const row = page.locator('[data-pr-ref="flywheel-tests/review-fixture#7"]')
    await expect(row).toBeVisible()
    // Keep the mutation pending long enough to check the immediate acknowledgement.
    let releaseRequest!: () => void
    const blocked = new Promise<void>(resolve => { releaseRequest = resolve })
    await page.route('**/code-reviews', async route => {
      if (route.request().method() === 'POST') { await blocked; await route.continue() }
      else await route.continue()
    })
    await row.getByRole('button', { name: 'Review with harness' }).click()
    await expect(row.getByRole('button', { name: 'Queueing…' })).toBeDisabled()
    releaseRequest()
    await expect(row.getByRole('button', { name: 'Queued', exact: true })).toBeDisabled()
    await expect(row.getByRole('status')).toContainText('review service is paused')
    await expect(row.getByRole('link', { name: 'View review', exact: true })).toBeVisible()
    const detail = await row.getByRole('link', { name: 'View review', exact: true }).getAttribute('href')
    const id = detail!.split('/').at(-1)!
    // Older queue entries have no model; they must inherit the selected default.
    const container = process.env.FLYWHEEL_E2E_PG_CONTAINER!
    expect(container).toMatch(/^flywheel-hardening-/)
    expect(id).toMatch(/^[a-f0-9-]+$/)
    execFileSync('docker', ['exec', container, 'psql', '-U', 'flywheel_test', '-d', 'flywheel_test', '-v', 'ON_ERROR_STOP=1', '-c', `UPDATE code_review_requests SET model='' WHERE id='${id}'`])
    await page.unroute('**/code-reviews')
    settings.review.enabled = true
    expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
    await expect(row.getByRole('button', { name: 'Agent reviewing', exact: true })).toBeDisabled({ timeout: 20_000 })
    await expect(row).toContainText('Worker connected')
    await expect(row).toContainText('1 reasoning update')
    await expect(row.getByRole('link', { name: 'Open session', exact: true })).toBeVisible()
    await page.screenshot({ path: 'test-results/my-reviews-live.png', fullPage: true })
    await row.getByRole('link', { name: 'Open session', exact: true }).click()
    await expect(page.locator('main')).toContainText('This session is running.')
    writeFileSync(join(hold, 'review.release'), '')
    await expect.poll(async () => (await (await request.get(`/code-reviews/${id}`)).json()).state).toBe('watching')
    await page.goto('/#/my/reviews')
    await expect(row).toContainText('Dry run complete')
    await expect(row).toContainText('Nothing was posted to GitHub.')
    await expect(row.getByRole('button', { name: 'Re-review', exact: true })).toBeEnabled()
    // A failed request stays local to its card and permits a retry.
    await page.route('**/code-reviews', route => route.fulfill({ status: 503, json: { message: 'Queue temporarily unavailable' } }))
    await row.getByRole('button', { name: 'Re-review', exact: true }).click()
    await expect(row.getByRole('alert')).toBeVisible()
    await expect(row.getByRole('button', { name: 'Re-review', exact: true })).toBeEnabled()
    await page.unroute('**/code-reviews')
    await row.getByRole('link', { name: 'View review', exact: true }).click()
    await page.getByRole('button', { name: 'Stop watching', exact: true }).click()
    await expect(page.locator('main')).toContainText('Review stopped')
    await page.goto('/#/my/reviews')
    await expect(row).toContainText('Review stopped')
    // The GitHub snapshot is cached, but local state must keep changing beneath it.
    expect((await request.post(`/code-reviews/${id}/rerun`)).ok()).toBeTruthy()
    await expect.poll(async () => (await (await request.get(`/code-reviews/${id}`)).json()).state).toBe('commented')
    await expect(row).toContainText('Dry run complete')
  } finally {
    writeFileSync(join(hold, 'review.release'), '')
    await request.put('/settings', { data: saved })
  }
})
