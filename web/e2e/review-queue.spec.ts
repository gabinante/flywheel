import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
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
  git('update-ref', 'refs/pull/8/head', 'HEAD')
  const url = 'https://github.com/flywheel-tests/review-fixture/pull/7'
  const secondURL = 'https://github.com/flywheel-tests/review-fixture/pull/8'
  const pr = { number: 7, title: 'Explicit review request fixture', repository: { nameWithOwner: 'flywheel-tests/review-fixture' }, author: { login: 'author' }, state: 'OPEN', url, headRefName: 'review-fixture', headRefOid: git('rev-parse', 'HEAD'), baseRefName: 'main', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), reviewRequests: { nodes: [{ requestedReviewer: { login: 'flywheel-test' } }] } }
  writeFileSync(join(root, 'review-fixture.json'), JSON.stringify([pr, { ...pr, number: 8, url: secondURL, title: 'Second concurrent review fixture' }]))
  const settings = await (await request.get('/settings')).json()
  const saved = structuredClone(settings)
  settings.review.enabled = false
  settings.review.watch_requested = false
  settings.review.watch_authored = false
  settings.review.publish = false
  settings.review.harness = 'codex'
  settings.review.max_concurrent = 2
  settings.dispatch.enabled = false
  settings.dispatch.max_workers = 3
  settings.harnesses.codex.bin = fileURLToPath(new URL('./fake-reviewer.py', import.meta.url))
  settings.harnesses.codex.model = 'browser-selected-model'
  expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
  const alreadyQueued = (await (await request.get('/code-reviews/status')).json()).queued
  try {
    expect((await request.get('/me/reviews?refresh=true')).ok()).toBeTruthy()
    await page.goto('/#/my/reviews')
    const row = page.locator('[data-pr-ref="flywheel-tests/review-fixture#7"]')
    const tray = page.getByRole('complementary', { name: 'Global work' })
    const reviewers = tray.getByRole('group', { name: 'Reviewer status' })
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
    const second = await request.post('/code-reviews', { data: { text: secondURL, watch: true } })
    expect(second.ok()).toBeTruthy()
    const secondID = (await second.json()).requests[0].id
    await expect(reviewers).toContainText('Reviewers paused')
    await expect(reviewers).toContainText('0/2 workers')
    await expect(reviewers).toContainText(`${alreadyQueued + 2} queued · waiting for service`)
    settings.review.enabled = true
    expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
    await expect(row.getByRole('button', { name: 'Agent reviewing', exact: true })).toBeDisabled({ timeout: 20_000 })
    await expect(row).toContainText('Worker connected')
    await expect(row).toContainText('1 reasoning update')
    await expect(row.getByRole('link', { name: 'Open session', exact: true })).toBeVisible()
    await expect(reviewers).toContainText('Reviewers on')
    await expect(reviewers).toContainText('2/2 workers')
    await expect(reviewers).toContainText('0 queued')
    await expect(tray.getByRole('link', { name: /Dispatch off/ })).toContainText('0/3 workers')
    const firstCard = tray.locator('[data-work-id]').filter({ has: page.getByRole('link', { name: 'Open PR flywheel-tests/review-fixture#7 on GitHub' }) })
    const secondCard = tray.locator('[data-work-id]').filter({ has: page.getByRole('link', { name: 'Open PR flywheel-tests/review-fixture#8 on GitHub' }) })
    for (const [card, prURL] of [[firstCard, url], [secondCard, secondURL]] as const) {
      await expect(card).toHaveCount(1)
      await expect(card).toContainText('Codex · browser-selected-model')
      await expect(card).toContainText('Worker connected')
      await expect(card.getByRole('link', { name: 'Open reviewer thread' })).toBeVisible()
      await expect(card.getByRole('link', { name: /^Open PR / })).toHaveAttribute('href', prURL)
    }
    const firstThread = await firstCard.getByRole('link', { name: 'Open reviewer thread' }).getAttribute('href')
    const secondThread = await secondCard.getByRole('link', { name: 'Open reviewer thread' }).getAttribute('href')
    expect(firstThread).not.toBe(secondThread)
    expect(firstThread).toBe(await row.getByRole('link', { name: 'Open session', exact: true }).getAttribute('href'))
    await page.screenshot({ path: 'test-results/my-reviews-live.png', fullPage: true })
    await firstCard.getByRole('link', { name: 'Open reviewer thread' }).click()
    await expect(page.locator('main')).toContainText('This session is running.')
    await expect(reviewers).toContainText('2/2 workers')
    await secondCard.getByRole('link', { name: 'Open reviewer thread' }).click()
    await expect(page).toHaveURL(new RegExp(secondThread!.replace(/^#/, '') + '$'))
    await expect(page.locator('main')).toContainText('This session is running.')
    // Pausing intake keeps the two already-running reviewers visible.
    settings.review.enabled = false
    expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
    await expect(reviewers).toContainText('Reviewers paused')
    await expect(reviewers).toContainText('2/2 workers')
    await expect(reviewers).toContainText('active reviews will finish')
    settings.review.enabled = true
    expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
    writeFileSync(join(hold, 'review.release'), '')
    await expect.poll(async () => (await (await request.get(`/code-reviews/${id}`)).json()).state).toBe('watching')
    await expect.poll(async () => (await (await request.get(`/code-reviews/${secondID}`)).json()).state).toBe('watching')
    await expect(reviewers).toContainText('0/2 workers')
    await expect(firstCard).toHaveCount(0)
    await expect(secondCard).toHaveCount(0)
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
    await expect.poll(async () => (await (await request.get(`/code-reviews/${id}`)).json()).state, { timeout: 20_000 }).toBe('commented')
    await expect(row).toContainText('Dry run complete')
  } finally {
    writeFileSync(join(hold, 'review.release'), '')
    await request.put('/settings', { data: saved })
  }
})

test('transient failures and new commits retry automatically with visible persisted status', async ({ page, request, baseURL }) => {
  test.setTimeout(90_000)
  expect(new URL(baseURL!).port).toBe('8091')
  const hold = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR!
  const root = dirname(hold)
  expect(root).toContain('flywheel-hardening.')
  const fixture = join(root, 'work', 'review-fixture')
  const git = (...args: string[]) => execFileSync(process.env.FLYWHEEL_E2E_REAL_GIT!, args, { cwd: fixture }).toString().trim()
  const prs = JSON.parse(readFileSync(join(root, 'review-fixture.json'), 'utf8'))
  const pr = { ...prs[0], number: 9, title: 'Automatic recovery fixture', url: 'https://github.com/flywheel-tests/review-fixture/pull/9', headRefOid: git('rev-parse', 'HEAD') }
  prs.push(pr)
  git('update-ref', 'refs/pull/9/head', 'HEAD')
  writeFileSync(join(root, 'review-fixture.json'), JSON.stringify(prs))
  writeFileSync(join(root, 'review-view-failure-9'), '')
  rmSync(join(hold, 'review.release'), { force: true })
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
    const queued = await request.post('/code-reviews', { data: { text: pr.url, watch: true } })
    expect(queued.ok()).toBeTruthy()
    const id = (await queued.json()).requests[0].id
    const get = async () => (await (await request.get(`/code-reviews/${id}`)).json())
    const container = process.env.FLYWHEEL_E2E_PG_CONTAINER!
    expect(container).toMatch(/^flywheel-hardening-/)
    expect(id).toMatch(/^[a-f0-9-]+$/)
    const makeDue = () => execFileSync('docker', ['exec', container, 'psql', '-U', 'flywheel_test', '-d', 'flywheel_test', '-v', 'ON_ERROR_STOP=1', '-c', `UPDATE code_review_requests SET retry_at=now()-interval '1 second' WHERE id='${id}'`])
    await page.goto(`/#/code-reviews/${id}`)
    settings.review.enabled = true
    expect((await request.put('/settings', { data: settings })).ok()).toBeTruthy()
    await expect(page.locator('main')).toContainText('Retry scheduled')
    await expect(page.locator('main')).toContainText('Retry 1 of 3')
    let retry = await get()
    expect(retry.state).toBe('queued')
    expect(retry.attempt).toBe(2)
    expect(Date.parse(retry.retry_at)).toBeGreaterThan(Date.now())
    expect(retry.session_id).toBeUndefined()
    // Expire the durable delay without sleeping for it; the real queue still claims it.
    makeDue()
    await expect.poll(async () => (await get()).state, { timeout: 20_000 }).toBe('reviewing')
    const tray = page.getByRole('complementary', { name: 'Global work' })
    const card = tray.locator('[data-work-id]').filter({ has: page.getByRole('link', { name: 'Open PR flywheel-tests/review-fixture#9 on GitHub' }) })
    await expect(card.getByRole('link', { name: 'Open reviewer thread' })).toBeVisible()
    const oldSession = await card.getByRole('link', { name: 'Open reviewer thread' }).getAttribute('href')
    // A push while the agent is reviewing must invalidate its result, not fail the request.
    writeFileSync(join(fixture, 'example.txt'), 'example\nnew head\n')
    git('add', 'example.txt'); git('commit', '-m', 'Change PR while review is running')
    git('update-ref', 'refs/pull/9/head', 'HEAD')
    pr.headRefOid = git('rev-parse', 'HEAD')
    writeFileSync(join(root, 'review-fixture.json'), JSON.stringify(prs))
    writeFileSync(join(hold, 'review.release'), '')
    await expect(page.locator('main')).toContainText('PR head changed before publication')
    await expect(page.locator('main')).toContainText('Retry scheduled')
    retry = await get()
    expect(retry.attempt).toBe(3)
    expect(retry.retry_count).toBe(1)
    expect(retry.verdict).toBe('')
    expect(retry.review_url).toBeUndefined()
    expect(retry.session_id).toBeUndefined()
    await page.screenshot({ path: 'test-results/review-retry-scheduled.png', fullPage: true })
    makeDue()
    await expect.poll(async () => (await get()).state, { timeout: 20_000 }).toBe('watching')
    await expect(page.locator('main')).toContainText('Dry run complete')
    const result = await get()
    expect(result.head_sha).toBe(pr.headRefOid)
    expect(result.retry_at).toBeUndefined()
    expect(result.error).toBeUndefined()
    expect(`#/sessions/${result.session_id}`).not.toBe(oldSession)
  } finally {
    writeFileSync(join(hold, 'review.release'), '')
    await request.put('/settings', { data: saved })
  }
})
