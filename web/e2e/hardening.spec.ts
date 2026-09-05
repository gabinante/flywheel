import { test, expect, type APIRequestContext } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import { fileURLToPath } from 'node:url'

test.describe.configure({ mode: 'serial' })
let api: APIRequestContext
let project: { id: string; slug: string }
let empty: { id: string; slug: string }
let org: { id: string; slug: string }
let gateTicket: string
let retryTicket: string
let continueTicket: string
let repo: string
const sql = (value: string) => `'${value.replaceAll("'", "''")}'`
function seed(statement: string) {
  // This suite may mutate only its disposable local test container.
  const container = process.env.FLYWHEEL_E2E_PG_CONTAINER ?? 'flywheel-hardening-pg'
  if (!container.startsWith('flywheel-hardening-')) throw new Error('Use a disposable flywheel-hardening-* database container')
  execFileSync('docker', ['exec', '-i', container, 'psql', '-U', 'flywheel_test', '-d', 'flywheel_test', '-v', 'ON_ERROR_STOP=1'], { input: statement })
}
async function json(method: string, path: string, data?: unknown) {
  const res = await api.fetch(path, { method, data })
  expect(res.ok(), `${method} ${path}: ${await res.text()}`).toBeTruthy()
  return res.json()
}
test.beforeAll(async ({ playwright, baseURL }) => {
  if (new URL(baseURL!).port !== '8091') throw new Error('The hardening suite requires the isolated server on port 8091')
  api = await playwright.request.newContext({ baseURL })
  const localOrgs = await json('GET', '/orgs')
  expect(localOrgs.length).toBeGreaterThan(0)
  const suffix = randomUUID().slice(0, 8)
  org = await json('POST', '/orgs', { name: 'Browser Tests', slug: `browser-${suffix}` })
  repo = `flywheel-tests/repo.with.dots-${suffix}`
  project = await json('POST', `/orgs/${org.id}/projects`, { name: 'Scoped Project', slug: `scoped-${suffix}`, repo_url: `https://github.com/${repo}.git` })
  empty = await json('POST', `/orgs/${org.id}/projects`, { name: 'Empty Project', slug: `empty-${suffix}` })
  await json('PUT', `/projects/${project.id}/workflow`, { name: 'Approval test', phases: [{ id: 'approve', name: 'Operator sign-off', type: 'gate', config: { conditions: [{ type: 'human_approval' }] } }, { id: 'implement', name: 'Implementation', type: 'agent', config: { role: 'executor', harness: 'codex', effort: 'high' } }] })
  const task = { title: 'Workflow test', type: 'task', created_by: 'browser-test', objective: { description: 'Exercise the current workflow', success_criteria: ['Operator decision is recorded'] } }
  gateTicket = (await json('POST', `/projects/${project.id}/tickets`, { ...task, title: 'Approval checkpoint' })).id
  retryTicket = (await json('POST', `/projects/${project.id}/tickets`, { ...task, title: 'Failed checkpoint' })).id
  await json('PUT', `/projects/${empty.id}/workflow`, { name: 'Continuation test', phases: [{ id: 'execute', name: 'Execution', type: 'agent', config: { role: 'operator', auto_advance: false } }] })
  continueTicket = (await json('POST', `/projects/${empty.id}/tickets`, { ...task, title: 'Awaiting continuation' })).id
  seed(`UPDATE tickets SET workflow_phase_status='blocked' WHERE id=${sql(gateTicket)};
    UPDATE tickets SET workflow_phase_status='failed' WHERE id=${sql(retryTicket)};
    UPDATE tickets SET workflow_phase_status='blocked',state='awaiting_validation' WHERE id=${sql(continueTicket)};
    INSERT INTO code_review_requests(id,repo,number,title,url,state,dry_run,attempt)
      SELECT gen_random_uuid()::text,${sql(repo)},n,'Scoped review '||n,${sql('https://github.com/'+repo+'/pull/')}||n,'watching',true,2 FROM generate_series(1,105) n;
    INSERT INTO code_review_requests(id,repo,number,title,state) VALUES(gen_random_uuid()::text,'unrelated/other-${suffix}',1,'Unrelated review','watching');
    INSERT INTO code_review_findings(id,request_id,attempt,severity,title,body) SELECT gen_random_uuid()::text,id,1,'P1','Stale finding','Previous attempt only' FROM code_review_requests WHERE repo=${sql(repo)} AND number=105;
    INSERT INTO agent_sessions(id,harness,external_id,repo,title,started_at,last_activity_at) VALUES
      (${sql(randomUUID())},'codex',${sql(randomUUID())},${sql(repo)},'Scoped session',now(),now()),
      (${sql(randomUUID())},'claude_code',${sql(randomUUID())},'unrelated/other-${suffix}','Unrelated session',now(),now());`)
})
test.afterAll(async () => { await api?.dispose() })
test('local workspace opens directly without login and survives a reload', async ({ page }) => {
  const requests: { path: string; authorization?: string }[] = []
  page.on('request', request => requests.push({ path: new URL(request.url()).pathname, authorization: request.headers().authorization }))
  await page.goto('/')
  await expect(page).toHaveURL(/#\/orgs\/[^/]+\/projects$/)
  await expect(page.getByRole('heading', { name: /^Projects\b/ })).toBeVisible()
  await expect(page.getByRole('link', { name: /sign in|sign out/i })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /sign in|sign out/i })).toHaveCount(0)
  expect(await page.evaluate(() => sessionStorage.getItem('flywheel_jwt'))).toBeNull()
  await page.reload()
  await expect(page.getByRole('heading', { name: /^Projects\b/ })).toBeVisible()
  expect(requests.some(request => request.path.startsWith('/auth/') || request.authorization)).toBe(false)
  await expect(page.locator('main').getByRole('listitem').last()).toHaveCSS('opacity', '1')
  await page.screenshot({ path: 'test-results/local-workspace.png', fullPage: true })
})
test('local controls enforce the browser boundary and MCP still needs credentials', async ({ request }) => {
  expect((await request.get('/settings')).status()).toBe(200)
  expect((await request.get('/auth/login', { maxRedirects: 0 })).status()).toBe(404)
  expect((await request.get('/settings', { headers: { Origin: 'https://attacker.test' } })).status()).toBe(403)
  expect((await request.get('/settings', { headers: { Host: 'attacker.test' } })).status()).toBe(403)
  expect((await request.get('/settings', { headers: { 'Sec-Fetch-Site': 'cross-site' } })).status()).toBe(403)
  const mcp = await request.post('/mcp', { data: {} })
  expect(mcp.status()).toBe(401)
  expect(mcp.headers()['www-authenticate']).toBeUndefined()
  expect(await mcp.text()).toContain('X-API-Key')
})
test('manually registered harness keys can access the local workspace', async ({ request }) => {
  const registered = await json('POST', '/agents', { name: 'Manual local harness', type: 'custom' })
  expect(registered.agent.user_id).toBeTruthy()
  expect((await json('GET', `/agents/${registered.agent.id}`)).id).toBe(registered.agent.id)
  const headers: Record<string, string> = { 'X-API-Key': registered.api_key, Accept: 'application/json, text/event-stream' }
  const init = await request.post('/mcp', { headers, data: { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'manual-worker-test', version: '1' } } } })
  expect(init.ok()).toBeTruthy()
  const session = init.headers()['mcp-session-id']
  if (session) headers['Mcp-Session-Id'] = session
  const response = await request.post('/mcp', { headers, data: { jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'list_orgs', arguments: {} } } })
  expect(response.ok()).toBeTruthy()
  const body = await response.text()
  const message = JSON.parse(body.startsWith('data:') || body.startsWith('event:') ? body.split('\n').find(line => line.startsWith('data:'))!.slice(5) : body)
  expect(message.error).toBeUndefined()
  expect(message.result.isError).not.toBe(true)
  expect(JSON.stringify(message.result)).toContain(org.id)
  if (session) await request.delete('/mcp', { headers })
})
test('the global tray keeps cross-project decisions and navigation on every page', async ({ page }) => {
  const tray = page.getByRole('complementary', { name: 'Global work' })
  for (const path of ['/settings', '/workflows', `/orgs/${org.slug}/projects/${empty.slug}/command`, `/orgs/${org.slug}/projects/${project.slug}/tickets/${gateTicket}`]) {
    await page.goto(`/#${path}`)
    await expect(tray.getByRole('heading', { name: 'In flight' })).toBeVisible()
    await expect(tray.locator(`[data-work-id="ticket:${gateTicket}"]`)).toContainText('Approve phase')
    await expect(tray.locator(`[data-work-id="ticket:${retryTicket}"]`)).toContainText('Retry phase')
    await expect(tray.locator(`[data-work-id="ticket:${continueTicket}"]`)).toContainText('Continue workflow')
    await expect(tray.getByText('Live Activity')).toHaveCount(0)
  }
  await tray.locator(`[data-work-id="ticket:${continueTicket}"]`).getByRole('link', { name: 'Continue workflow' }).click()
  await expect(page).toHaveURL(new RegExp(`/projects/${empty.slug}/tickets/${continueTicket}$`))
  await page.screenshot({ path: 'test-results/global-tray-attention.png', fullPage: true })
})
test('tray errors preserve the last known work and the tray opens on narrow screens', async ({ page }) => {
  await page.goto('/#/settings')
  const tray = page.getByRole('complementary', { name: 'Global work' })
  await expect(tray.locator(`[data-work-id="ticket:${gateTicket}"]`)).toBeVisible()
  await page.route('**/api/overview', route => route.fulfill({ status: 503, body: 'unavailable' }))
  await tray.getByRole('button', { name: 'Refresh current work' }).click()
  await expect(tray.getByRole('alert')).toContainText('Showing the last successful update')
  await expect(tray.locator(`[data-work-id="ticket:${gateTicket}"]`)).toBeVisible()
  await page.setViewportSize({ width: 800, height: 900 })
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect(tray.getByRole('heading', { name: 'Needs attention' })).toBeVisible()
  await page.getByRole('button', { name: 'Close work tray', exact: true }).click()
  await expect(tray).not.toBeVisible()
})
test('project review filtering precedes pagination and clean attempts hide stale findings', async ({ page }) => {
  const list = await json('GET', `/code-reviews?project_id=${project.id}&limit=100`)
  expect(list.total).toBe(105)
  expect(list.requests).toHaveLength(100)
  expect(list.requests.every((r: {repo: string; findings?: unknown[]}) => r.repo === repo && !r.findings?.length)).toBeTruthy()
  await page.goto(`/#/orgs/${org.slug}/projects/${project.slug}/code-reviews`)
  await expect(page.getByRole('heading', { name: 'Code Reviews', exact: true })).toBeVisible()
  await expect(page.getByText('Unrelated review', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('link').filter({ hasText: 'Scoped review' })).toHaveCount(100)
  await page.getByRole('button', { name: 'Next reviews' }).click()
  await expect(page.getByRole('link').filter({ hasText: 'Scoped review' })).toHaveCount(5)
  await page.reload()
  await expect(page.getByRole('link').filter({ hasText: 'Scoped review' })).toHaveCount(5)
  await page.goto(`/#/orgs/${org.slug}/projects/${empty.slug}/code-reviews`)
  await expect(page.getByText('No review requests yet.', { exact: false })).toBeVisible()
  await expect(page.getByRole('link').filter({ hasText: 'Scoped review' })).toHaveCount(0)
})
test('sessions are scoped and refresh without a page reload', async ({ page }) => {
  await page.goto(`/#/orgs/${org.slug}/projects/${project.slug}/sessions`)
  await expect(page.getByText('Scoped session', { exact: true })).toBeVisible()
  await expect(page.getByText('Unrelated session', { exact: true })).toHaveCount(0)
  seed(`INSERT INTO agent_sessions(id,harness,external_id,repo,title,started_at,last_activity_at) VALUES(${sql(randomUUID())},'codex',${sql(randomUUID())},${sql(repo)},'New session after poll',now(),now());`)
  await expect(page.getByText('New session after poll', { exact: true })).toBeVisible({ timeout: 15_000 })
  await page.goto(`/#/orgs/${org.slug}/projects/${empty.slug}/sessions`)
  await expect(page.getByText('Scoped session', { exact: true })).toHaveCount(0)
})
test('operator approval is tied to the current phase attempt', async ({ page }) => {
  await page.goto(`/#/orgs/${org.slug}/projects/${project.slug}/tickets/${gateTicket}`)
  await page.getByRole('button', { name: 'Approve phase', exact: true }).click()
  await expect.poll(async () => (await json('GET', `/tickets/${gateTicket}/workflow`)).status).toBe('ready')
  const ticket = await json('GET', `/tickets/${gateTicket}`)
  expect(ticket.outputs._human_approval_approve).toBeTruthy()
  const stale = await api.post(`/tickets/${gateTicket}/workflow/decision`, { data: { action: 'approve', phase_id: 'approve', entered_at: '2020-01-01T00:00:00Z' } })
  expect(stale.status()).toBe(409)
})
test('failed phases retry with a new attempt; manual continuation closes the workflow', async ({ page }) => {
  const before = await json('GET', `/tickets/${retryTicket}/workflow`)
  await page.goto(`/#/orgs/${org.slug}/projects/${project.slug}/tickets/${retryTicket}`)
  await expect(page.getByRole('button', { name: 'Retry phase' })).toBeVisible()
  await page.getByRole('button', { name: 'Retry phase' }).click()
  await expect.poll(async () => (await json('GET', `/tickets/${retryTicket}/workflow`)).status).toBe('ready')
  expect((await json('GET', `/tickets/${retryTicket}/workflow`)).entered_at).not.toBe(before.entered_at)
  await page.goto(`/#/orgs/${org.slug}/projects/${empty.slug}/tickets/${continueTicket}`)
  await page.getByRole('button', { name: 'Continue workflow' }).click()
  await expect.poll(async () => (await json('GET', `/tickets/${continueTicket}`)).state).toBe('closed')
})
test('saving operator settings preserves separately edited prompts', async ({ page }) => {
  await json('PUT', '/prompts/code_review', { text: 'Keep this review override after saving settings.' })
  await page.goto('/#/settings?section=models')
  await page.locator('#codex-model').fill(`local-browser-test-${randomUUID().slice(0, 8)}`)
  await page.getByRole('button', { name: 'Save changes' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  const prompts = await json('GET', '/prompts')
  expect(JSON.stringify(prompts.items.find((p: {id: string}) => p.id === 'code_review'))).toContain('Keep this review override')
})
test('production navigation renders without browser errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  for (const path of ['/settings', '/workflows', `/orgs/${org.slug}/projects/${project.slug}/command`, `/orgs/${org.slug}/projects/${project.slug}/settings`]) {
    await page.goto(`/#${path}`)
    await expect(page.locator('body')).not.toBeEmpty()
    await page.waitForTimeout(300)
  }
  expect(errors).toEqual([])
  await page.screenshot({ path: 'test-results/project-settings.png', fullPage: true })
})

test('real dispatcher phases drive both harness protocols, exact claims, session capture, and approval', async ({ page }) => {
  test.setTimeout(90_000)
  const workerProject = await json('POST', `/orgs/${org.id}/projects`, { name: 'Harness Project', slug: `harness-${randomUUID().slice(0, 8)}` })
  await json('PUT', `/projects/${workerProject.id}/workflow`, { name: 'Live local workflow', phases: [
    { id: 'execute', name: 'Execute', type: 'agent', config: { role: 'operator', harness: 'claude' } },
    { id: 'followup', name: 'Follow up', type: 'agent', config: { role: 'operator', harness: 'codex', effort: 'high' } },
    { id: 'approval', name: 'Operator approval', type: 'gate', config: { conditions: [{ type: 'human_approval' }] } },
  ] })
  seed(`UPDATE projects SET dispatch_enabled=(id=${sql(workerProject.id)});`)
  const settings = await json('GET', '/settings')
  const saved = structuredClone(settings)
  settings.dispatch.enabled = true
  settings.dispatch.driver = 'claude'
  settings.harnesses.claude.bin = fileURLToPath(new URL('./fake-claude.py', import.meta.url))
  settings.harnesses.codex.bin = settings.harnesses.claude.bin
  settings.workers = { workers: [], policies: {}, roles: [] }
  await json('PUT', '/settings', settings)
  try {
    const ticket = await json('POST', `/projects/${workerProject.id}/tickets`, { title: 'Real local worker run', type: 'task', created_by: 'browser-test', objective: { description: 'Exercise dispatch through MCP', success_criteria: ['Exact ticket and human approval'] } })
    const holdDir = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR
    if (!holdDir || !holdDir.includes('flywheel-hardening.')) throw new Error('Run with the disposable hardening script')
    const tray = page.getByRole('complementary', { name: 'Global work' })
    for (const harness of ['claude', 'codex']) {
      await expect.poll(async () => (await json('GET', '/api/overview')).in_flight.some((run: { ticket_id: string; harness: string; session_href?: string }) => run.ticket_id === ticket.id && run.harness.startsWith(harness) && !!run.session_href), { timeout: 30_000 }).toBe(true)
      await page.goto('/#/settings')
      const row = tray.locator('li').filter({ hasText: ticket.title })
      await expect(row).toHaveCount(1)
      await expect(row).toContainText(harness === 'codex' ? 'Codex' : 'Claude Code', { timeout: 10_000 })
      await expect(row).toContainText('Worker connected')
      await expect(row).toContainText('1 reasoning update')
      await expect(row).toContainText(harness === 'claude' ? '25 in / 7 out tokens' : 'Tokens not reported yet')
      await expect(row.getByRole('link', { name: /Open session/ })).toBeVisible()
      const sessionHref = await row.getByRole('link', { name: /Open session/ }).getAttribute('href')
      await row.getByRole('link', { name: /Open session/ }).click()
      await expect(page).toHaveURL(new RegExp(sessionHref!.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$'))
      await expect(page.getByText('This session is running.', { exact: false })).toBeVisible()
      await expect(page.locator('main').getByRole('heading', { name: 'Run progress' })).toBeVisible()
      await expect(page.locator('main')).toContainText('Last output')
      const liveSessionId = sessionHref!.split('/').at(-1)!
      expect((await json('GET', `/sessions/${liveSessionId}`)).session.status).toBe('active')
      if (harness === 'claude') await page.screenshot({ path: 'test-results/global-tray-running.png', fullPage: true })
      writeFileSync(join(holdDir, `${ticket.id}-${harness}.release`), '')
    }
    await expect.poll(async () => (await json('GET', `/tickets/${ticket.id}/workflow`)).position?.current_phase?.id, { timeout: 30_000 }).toBe('approval')
    expect((await json('GET', `/tickets/${ticket.id}`)).state).toBe('awaiting_validation')
    await expect.poll(async () => (await json('GET', '/api/overview')).in_flight.some((run: { ticket_id: string }) => run.ticket_id === ticket.id)).toBe(false)
    await expect.poll(async () => (await json('GET', '/api/overview')).attention.some((item: { ticket_id: string }) => item.ticket_id === ticket.id)).toBe(true)
    await expect.poll(async () => (await json('GET', `/sessions?project_id=${workerProject.id}`)).total, { timeout: 10_000 }).toBe(2)
    await page.goto(`/#/orgs/${org.slug}/projects/${workerProject.slug}/tickets/${ticket.id}`)
    await page.getByRole('button', { name: 'Approve phase', exact: true }).click()
    // Enable applies an immediate reconcile; no need to wait for the periodic scan.
    await json('PUT', '/settings', { ...settings, dispatch: { ...settings.dispatch, enabled: false } })
    await json('PUT', '/settings', settings)
    await expect.poll(async () => (await json('GET', `/tickets/${ticket.id}`)).state, { timeout: 15_000 }).toBe('closed')
  } finally { await json('PUT', '/settings', saved) }
})

test('review progress distinguishes an orphaned record from a quiet worker', async ({ page }) => {
  const id = randomUUID()
  seed(`INSERT INTO code_review_requests(id,repo,number,title,state,updated_at) VALUES(${sql(id)},'flywheel-tests/progress',999,'Progress review','reviewing',now()-interval '2 hours');`)
  await page.goto(`/#/code-reviews/${id}`)
  const main = page.locator('main')
  await expect(main.getByText('No live worker')).toBeVisible()
  const snapshot = await json('GET', '/api/overview')
  const item = snapshot.attention.find((item: { review_id?: string }) => item.review_id === id)
  expect(item.progress.health).toBe('untracked')
  snapshot.attention = snapshot.attention.filter((item: { review_id?: string }) => item.review_id !== id)
  snapshot.in_flight.push({ ...item, progress: { worker_state: 'running', health: 'quiet', pid: 123, last_output_at: new Date(Date.now()-300_000).toISOString(), last_activity_at: new Date(Date.now()-300_000).toISOString(), tool_calls: 4, assistant_messages: 2, reasoning_updates: 3, output_events: 9, recent: [{ at: new Date(Date.now()-300_000).toISOString(), kind: 'tool', summary: 'Command started' }] } })
  await page.route('**/api/overview', route => route.fulfill({ json: snapshot }))
  await page.getByRole('button', { name: 'Refresh current work' }).click()
  await expect(main.getByText('Quiet · check progress')).toBeVisible()
  await expect(main.getByText('Tokens not reported yet')).toBeVisible()
  await expect(main).toContainText('4 tools · 2 assistant updates · 3 reasoning updates')
  await page.screenshot({ path: 'test-results/review-progress-quiet.png', fullPage: true })
})
