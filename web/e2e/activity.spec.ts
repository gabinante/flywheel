import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { randomUUID } from 'node:crypto'

const sql = (value: string) => `'${value.replaceAll("'", "''")}'`
function seed(statement: string) {
  const container = process.env.FLYWHEEL_E2E_PG_CONTAINER ?? ''
  if (!container.startsWith('flywheel-hardening-')) throw new Error('Activity tests require a disposable database')
  execFileSync('docker', ['exec', '-i', container, 'psql', '-U', 'flywheel_test', '-d', 'flywheel_test', '-v', 'ON_ERROR_STOP=1'], { input: statement })
}

test.beforeEach(async ({ baseURL }) => {
  expect(new URL(baseURL!).port).toBe('8091')
})

test('committed background changes update the tray and session pages with polling disabled', async ({ page }) => {
  // Disable repeating network/fallback timers. These assertions can only pass
  // from SSE notices, not from the old three/ten-second page pollers.
  await page.addInitScript(() => {
    const interval = window.setInterval.bind(window)
    window.setInterval = ((handler: TimerHandler, delay?: number, ...args: unknown[]) => interval((delay ?? 0) >= 1000 ? () => {} : handler, delay, ...args)) as typeof window.setInterval
  })
  let streams = 0
  page.on('request', r => { if (new URL(r.url()).pathname === '/api/activity/events') streams++ })
  await page.goto('/#/sessions')
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected')
  const id = randomUUID()
  const title = `SSE session ${id}`
  seed(`INSERT INTO agent_sessions(id,harness,external_id,title,started_at,last_activity_at) VALUES(${sql(id)},'codex',${sql(id)},${sql(title)},now(),now());`)
  await expect(page.locator('main').getByText(title, { exact: true })).toBeVisible()
  await page.locator('main').getByText(title, { exact: true }).click()
  await expect(page.locator('main').getByRole('heading', { name: title })).toBeVisible()
  seed(`UPDATE agent_sessions SET title=${sql(title+' updated')} WHERE id=${sql(id)};`)
  await expect(page.locator('main').getByRole('heading', { name: title+' updated' })).toBeVisible()
  const review = randomUUID()
  const failure = `SSE publication failure ${review}`
  seed(`INSERT INTO code_review_requests(id,repo,number,title,state,error) VALUES(${sql(review)},'flywheel-tests/sse',1,${sql(failure)},'failed','Fixture failure');`)
  await expect(page.getByRole('complementary', { name: 'Global work' }).getByText(failure, { exact: true })).toBeVisible()
  seed(`DELETE FROM code_review_requests WHERE id=${sql(review)};`)
  await expect(page.getByRole('complementary', { name: 'Global work' }).getByText(failure, { exact: true })).toHaveCount(0)
  expect(streams).toBe(1) // SPA navigation reuses the global connection.
  await page.screenshot({ path: 'test-results/activity-live.png', fullPage: true })
})

test('browser reconnect catches changes made while disconnected', async ({ page, context }) => {
  const id = randomUUID()
  const title = `Offline session ${id}`
  seed(`INSERT INTO agent_sessions(id,harness,external_id,title,started_at,last_activity_at) VALUES(${sql(id)},'codex',${sql(id)},${sql(title)},now(),now());`)
  await page.goto(`/#/sessions/${id}`)
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected')
  await expect(page.locator('main').getByRole('heading', { name: title })).toBeVisible()
  await context.setOffline(true)
  await expect(page.getByRole('status', { name: 'Activity connection' })).toContainText('Reconnecting')
  seed(`UPDATE agent_sessions SET title=${sql(title+' recovered')} WHERE id=${sql(id)};`)
  await context.setOffline(false)
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected', { timeout: 10_000 })
  await expect(page.locator('main').getByRole('heading', { name: title+' recovered' })).toBeVisible()
})

test('database listener reconnect resyncs an already-open browser', async ({ page }) => {
  const id = randomUUID()
  const title = `Listener recovery ${id}`
  seed(`INSERT INTO agent_sessions(id,harness,external_id,title,started_at,last_activity_at) VALUES(${sql(id)},'codex',${sql(id)},${sql(title)},now(),now());`)
  await page.goto(`/#/sessions/${id}`)
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected')
  await expect(page.locator('main').getByRole('heading', { name: title })).toBeVisible()
  seed(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name='flywheel-activity' AND datname=current_database();
    UPDATE agent_sessions SET title=${sql(title+' recovered')} WHERE id=${sql(id)};`)
  await expect(page.locator('main').getByRole('heading', { name: title+' recovered' })).toBeVisible({ timeout: 8_000 })
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected')
})

test('an activity notice during a slow fetch cannot leave the tray stale', async ({ page }) => {
  await page.goto('/#/sessions')
  await expect(page.getByRole('status', { name: 'Activity connection' })).toHaveText('Live updates connected')
  let release!: () => void
  const blocked = new Promise<void>(resolve => { release = resolve })
  let captured!: () => void
  const capturedResponse = new Promise<void>(resolve => { captured = resolve })
  await page.route('**/api/overview', async route => {
    const response = await route.fetch()
    const body = await response.text()
    captured()
    await blocked
    await route.fulfill({ response, body })
  }, { times: 1 })
  await page.getByRole('button', { name: 'Refresh current work' }).click()
  await capturedResponse
  const review = randomUUID()
  const title = `Concurrent update ${review}`
  seed(`INSERT INTO code_review_requests(id,repo,number,title,state,error) VALUES(${sql(review)},'flywheel-tests/sse-race',1,${sql(title)},'failed','Fixture failure');`)
  // Wait for the actual SSE notice, then deliver the stale REST response.
  await page.waitForTimeout(1200)
  release()
  await expect(page.getByRole('complementary', { name: 'Global work' }).getByText(title, { exact: true })).toBeVisible()
  seed(`DELETE FROM code_review_requests WHERE id=${sql(review)};`)
})
