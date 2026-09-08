import { test, expect, type APIRequestContext, type Page, type TestInfo } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { randomUUID } from 'node:crypto'
import { writeFileSync, rmSync } from 'node:fs'
import { join } from 'node:path'

let api: APIRequestContext
let org: { id: string; slug: string }
let project: { id: string; slug: string }
const prefix = `journey-${randomUUID().slice(0, 8)}`
const projectPath = () => `/orgs/${org.slug}/projects/${project.slug}`
async function json(method: string, path: string, data?: unknown) {
  const res = await api.fetch(path, { method, data })
  expect(res.ok(), `${method} ${path}: ${await res.text()}`).toBeTruthy()
  return res.json()
}
async function evidence(page: Page, info: TestInfo) {
  const path = info.outputPath('audit.png')
  await page.screenshot({ path, fullPage: true })
  await info.attach('UI audit', { path, contentType: 'image/png' })
}
test.beforeAll(async ({ playwright, baseURL }) => {
  if (new URL(baseURL!).port !== '8091' || !process.env.FLYWHEEL_E2E_PG_CONTAINER?.startsWith('flywheel-hardening-')) throw new Error('Requires disposable hardening environment')
  api = await playwright.request.newContext({ baseURL })
  org = await json('POST', '/orgs', { name: 'Journey Audit', slug: prefix })
  project = await json('POST', `/orgs/${org.id}/projects`, { name: 'Journey Project', slug: prefix })
})
test.beforeEach(async ({ page }) => {
  await page.addInitScript(id => localStorage.setItem('flywheel.preferredOrgId', id), org.id)
})
test.afterAll(async () => { await api?.dispose() })

test('J03 create a project through the UI and reopen its settings', async ({ page }, info) => {
  await page.goto(`/#/orgs/${org.slug}/projects`)
  await page.getByRole('link', { name: 'Create project', exact: true }).click()
  await page.getByLabel('Name', { exact: true }).fill('Created through the browser')
  await page.getByLabel('Slug (optional)', { exact: true }).fill(`${prefix}-created`)
  await page.getByRole('button', { name: 'Create project', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`${prefix}-created`))
  const projects = await json('GET', `/orgs/${org.id}/projects`)
  const created = projects.find((p: { slug: string }) => p.slug === `${prefix}-created`)
  expect(created.name).toBe('Created through the browser')
  await page.goto(`/#/orgs/${org.slug}/projects/${created.slug}/settings`)
  await expect(page.getByRole('heading', { name: 'Project settings', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Project settings', exact: true })).toBeVisible()
  await evidence(page, info)
})

test('J04 create, preview, edit and close a work stream', async ({ page }, info) => {
  await page.goto(`/#${projectPath()}/work-streams/new`)
  await page.getByLabel('Name', { exact: true }).fill('Audit work stream')
  await page.getByLabel('Plan — Markdown (optional)', { exact: true }).fill('## Delivery plan\nValidate the core journey.')
  await page.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Delivery plan', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Create work stream', exact: true }).click()
  await page.getByLabel('Branch', { exact: true }).fill('audit-branch')
  await page.getByRole('button', { name: 'Save changes', exact: true }).click()
  await page.reload()
  await expect(page.getByLabel('Branch', { exact: true })).toHaveValue('audit-branch')
  await page.getByRole('switch').uncheck()
  await page.getByRole('button', { name: 'Save changes', exact: true }).click()
  await page.reload()
  await expect(page.getByRole('switch')).not.toBeChecked()
  await evidence(page, info)
})

test('J05 create and edit a custom workflow from a built-in template', async ({ page }, info) => {
  await page.goto('/#/workflows')
  await page.locator('a[href*="/workflows/new?template="]').first().click()
  await page.getByLabel('Pipeline name', { exact: true }).fill('Audit custom workflow')
  const saved = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/workflow-library'))
  await page.getByRole('button', { name: 'Create workflow', exact: true }).click()
  const entry = await (await saved).json()
  await expect(page).toHaveURL(new RegExp(`/workflows/${entry.id}$`))
  await page.getByLabel('Pipeline name', { exact: true }).fill('Audit renamed workflow')
  await page.getByRole('button', { name: 'Save workflow', exact: true }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Pipeline name', { exact: true })).toHaveValue('Audit renamed workflow')
  await evidence(page, info)
})

test('J06 select a library workflow and save a project-specific override', async ({ page }, info) => {
  const entry = await json('POST', `/orgs/${org.id}/workflow-library`, { name: 'Project audit template', phases: [{ id: 'plan', name: 'Plan', type: 'agent', config: { role: 'operator' } }] })
  await page.goto(`/#${projectPath()}/settings/workflow`)
  await page.getByRole('button', { name: 'Select from library', exact: true }).click()
  await page.getByPlaceholder('Search templates...').fill(entry.name)
  await page.getByRole('button', { name: entry.name, exact: true }).click()
  await page.getByLabel('Pipeline name', { exact: true }).fill('Project-only pipeline')
  await page.getByRole('button', { name: 'Save pipeline', exact: true }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Pipeline name', { exact: true })).toHaveValue('Project-only pipeline')
  expect((await json('GET', `/api/v1/workflows/${entry.id}`)).workflow.name).toBe('Project audit template')
  await evidence(page, info)
})

test('J07 edit the organization default without overwriting a project override', async ({ page }) => {
  await json('PUT', `/projects/${project.id}/workflow`, { name: 'Retained project override', phases: [{ id: 'work', name: 'Work', type: 'agent', config: { role: 'operator' } }] })
  await page.goto('/#/workflows/org')
  await page.getByLabel('Pipeline name', { exact: true }).fill('Audit organization default')
  await page.getByRole('button', { name: 'Save org default', exact: true }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Pipeline name', { exact: true })).toHaveValue('Audit organization default')
  await page.goto(`/#${projectPath()}/settings/workflow`)
  await expect(page.getByLabel('Pipeline name', { exact: true })).toHaveValue('Retained project override')
})

test('J08 delete a saved workflow with confirmation', async ({ page }) => {
  const entry = await json('POST', `/orgs/${org.id}/workflow-library`, { name: 'Delete this audit template', phases: [{ id: 'work', name: 'Work', type: 'agent', config: { role: 'operator' } }] })
  await page.goto('/#/workflows')
  const card = page.locator('div.group').filter({ has: page.getByRole('link', { name: /Delete this audit template/ }) })
  page.once('dialog', d => d.dismiss())
  await card.getByRole('button', { name: 'Delete from library' }).click()
  await expect(card).toBeVisible()
  page.once('dialog', d => d.accept())
  await card.getByRole('button', { name: 'Delete from library' }).click()
  await expect(card).toHaveCount(0)
  expect((await api.get(`/api/v1/workflows/${entry.id}`)).status()).toBe(404)
})

test('J09 define a worker and assign it to reviews and implementation', async ({ page }, info) => {
  const saved = await json('GET', '/settings')
  try {
    await page.goto('/#/settings?section=workers')
    await page.getByRole('button', { name: 'Add worker', exact: true }).click()
    await page.getByLabel('Name', { exact: true }).fill('Audit worker')
    await page.getByLabel('Model', { exact: true }).fill('audit-model')
    await page.getByLabel('Instructions', { exact: true }).fill('Keep reviews pragmatic.')
    await page.getByRole('button', { name: 'Save changes', exact: true }).click()
    await expect(page.getByText('Saved', { exact: true })).toBeVisible()
    await page.reload()
    await page.getByRole('button', { name: /Audit worker/ }).click()
    await expect(page.getByLabel('Instructions', { exact: true })).toHaveValue('Keep reviews pragmatic.')
    await evidence(page, info)
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(page.getByRole('button', { name: 'Expand sidebar', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Open panel', exact: true })).toBeVisible()
    await expect(page.getByLabel('Instructions', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await page.screenshot({ path: info.outputPath('worker-mobile.png'), fullPage: true, animations: 'disabled' })
    await page.setViewportSize({ width: 1440, height: 1000 })
    await page.getByRole('tab', { name: 'Task assignments' }).click()
    await page.getByLabel('Review worker', { exact: true }).click()
    await page.getByRole('option', { name: 'Audit worker', exact: true }).click()
    await page.getByLabel('Implementation', { exact: false }).first().click()
    await page.getByRole('option', { name: 'Audit worker', exact: true }).click()
    await page.getByRole('button', { name: 'Save changes', exact: true }).click()
    await expect(page.getByText('Saved', { exact: true }).first()).toBeVisible()
    const settings = await json('GET', '/settings')
    const worker = settings.workers.workers.find((w: { name: string }) => w.name === 'Audit worker')
    expect(settings.review.worker_id).toBe(worker.id)
    expect(settings.workers.policies.executor.worker_ids).toEqual([worker.id])
    await page.goto('/#/settings?section=review')
    await expect(page.getByLabel('Review worker', { exact: true })).toContainText('Audit worker')
    // An invalid assignment must be rejected, rather than silently running someone else.
    settings.workers.workers.find((w: { id: string }) => w.id === worker.id).enabled = false
    const invalid = await api.put('/settings', { data: settings })
    expect(invalid.status()).toBe(400)
  } finally { await json('PUT', '/settings', saved) }
})

test('J10 edit a built-in prompt and restore its default', async ({ page }) => {
  const original = (await json('GET', '/prompts')).items.find((p: { id: string }) => p.id === 'code_review')
  try {
    await page.goto('/#/settings?section=prompts')
    const editor = page.locator('div.rounded-xl').filter({ has: page.locator('textarea') }).filter({ has: page.getByText(original.name, { exact: true }) })
    await editor.locator('textarea').fill('Short pragmatic audit review instructions.')
    await editor.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(editor.getByText('Saved', { exact: true })).toBeVisible()
    await page.reload()
    await expect(editor.locator('textarea')).toHaveValue('Short pragmatic audit review instructions.')
    await editor.getByRole('button', { name: /Reset|Restore/ }).click()
    await expect(editor.locator('textarea')).toHaveValue(original.default_text)
  } finally { await json('PUT', '/prompts/code_review', { text: original.customized ? original.text : '' }) }
})

test('J11 discuss a change with the project orchestrator and retain the conversation', async ({ page }, info) => {
  test.setTimeout(60_000)
  const hold = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR!
  rmSync(join(hold, 'planner.release'), { force: true })
  await page.goto(`/#${projectPath()}/command`)
  const composer = page.getByPlaceholder('Describe the task, scope, constraints, or ask what should be ticketed next.')
  await composer.fill('How should we validate a potential workflow change?')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  try {
    await expect(page.getByText('How should we validate a potential workflow change?', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: /Cancel/ })).toBeVisible()
    await evidence(page, info)
  } finally { writeFileSync(join(hold, 'planner.release'), '') }
  await expect(page.getByText('Start with a small ticket to validate the proposed change.', { exact: false })).toBeVisible({ timeout: 20_000 })
  await page.reload()
  await expect(page.getByText('How should we validate a potential workflow change?', { exact: true })).toBeVisible()
  await expect(page.getByText('Start with a small ticket to validate the proposed change.', { exact: false })).toBeVisible()
})

test('J12 failed planner submission preserves the draft and allows retry', async ({ page }) => {
  await page.goto(`/#${projectPath()}/command`)
  const composer = page.getByPlaceholder('Describe the task, scope, constraints, or ask what should be ticketed next.')
  await page.route('**/orchestrator/messages', route => route.fulfill({ status: 503, json: { message: 'Planner temporarily unavailable' } }))
  await composer.fill('Keep this planning draft')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(composer).toHaveValue('Keep this planning draft')
  await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeEnabled()
})

test('J13 cancel an active planner run from the UI', async ({ page }) => {
  const hold = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR!
  rmSync(join(hold, 'planner.release'), { force: true })
  await page.goto(`/#${projectPath()}/command`)
  await page.getByPlaceholder('Describe the task, scope, constraints, or ask what should be ticketed next.').fill('Cancel this audit conversation turn')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  try {
    await page.getByRole('button', { name: /Cancel/ }).click()
    await expect(page.getByRole('button', { name: 'Send', exact: true })).toBeVisible()
    await expect.poll(async () => (await json('GET', `/api/command-center/projects/${project.id}/orchestrator`)).runs.at(-1).status).toBe('cancelled')
  } finally { writeFileSync(join(hold, 'planner.release'), '') }
})

test('J14 schedule overview links to review settings', async ({ page }, info) => {
  const saved = await json('GET', '/settings')
  try {
    await page.goto('/#/schedule')
    await expect(page.getByRole('heading', { name: 'Scheduled actions', exact: true })).toBeVisible()
    await page.locator('main a[href="#/settings?section=review"]').first().click()
    await expect(page).toHaveURL(/section=review/)
    await evidence(page, info)
  } finally { await json('PUT', '/settings', saved) }
})

test('J16 search sessions, open linked work and continue the same harness session', async ({ page }, info) => {
  const saved = await json('GET', '/settings')
  const id = randomUUID()
  const hold = process.env.FLYWHEEL_E2E_HARNESS_HOLD_DIR!
  const sql = (s: string) => `'${s.replaceAll("'", "''")}'`
  const container = process.env.FLYWHEEL_E2E_PG_CONTAINER!
  execFileSync('docker', ['exec', '-i', container, 'psql', '-U', 'flywheel_test', '-d', 'flywheel_test', '-v', 'ON_ERROR_STOP=1'], { input: `INSERT INTO agent_sessions(id,harness,external_id,cwd,repo,title,started_at,last_activity_at,ended_at) VALUES (${sql(id)},'claude_code',${sql(randomUUID())},${sql(hold)},'flywheel-tests/journey','Audit resumable session',now(),now(),now()); INSERT INTO session_links(session_id,kind,ref) VALUES (${sql(id)},'pr','flywheel-tests/journey#19');` })
  try {
    const settings = structuredClone(saved)
    settings.harnesses.claude.bin = fileURLToPath(new URL('./fake-planner.py', import.meta.url))
    await json('PUT', '/settings', settings)
    rmSync(join(hold, 'planner.release'), { force: true })
    await page.goto('/#/sessions')
    await page.getByRole('textbox', { name: 'Search sessions' }).fill('Audit resumable session')
    await page.getByRole('link').filter({ hasText: 'Audit resumable session' }).click()
    await expect(page).toHaveURL(new RegExp(`/sessions/${id}$`))
    await expect(page.locator('main a[href="https://github.com/flywheel-tests/journey/pull/19"]')).toBeVisible()
    await page.getByPlaceholder('Ask a question or give an instruction… (⌘↩ to send)').fill('Continue the audit in this session')
    await page.getByRole('button', { name: 'Send', exact: true }).click()
    await expect(page.locator('main')).toContainText('This session is running')
    writeFileSync(join(hold, 'planner.release'), '')
    await expect(page.getByText('Start with a small ticket to validate the proposed change.', { exact: false })).toBeVisible()
    await expect(page.getByText('This session is running.', { exact: false })).not.toBeVisible()
    await evidence(page, info)
  } finally { writeFileSync(join(hold, 'planner.release'), ''); await json('PUT', '/settings', saved) }
})

test('J17 project tickets show a ticket and open its detail', async ({ page }, info) => {
  const ticket = await json('POST', `/projects/${project.id}/tickets`, { title: 'Inspect this journey ticket', type: 'task', created_by: 'audit', objective: { description: 'Audit ticket navigation', success_criteria: ['Detail is accessible'] } })
  await page.goto(`/#${projectPath()}/tickets`)
  await page.getByRole('link').filter({ hasText: 'Inspect this journey ticket' }).first().click()
  await expect(page).toHaveURL(new RegExp(`/tickets/${ticket.id}$`))
  await expect(page.locator('main')).toContainText('Audit ticket navigation')
  await evidence(page, info)
})

test('J18 add, configure, reorder and remove workflow phases', async ({ page }) => {
  await json('PUT', `/projects/${project.id}/workflow`, { name: 'Phase editing audit', phases: [{ id: 'first', name: 'First', type: 'agent', config: { role: 'operator' } }] })
  await page.goto(`/#${projectPath()}/settings/workflow`)
  await page.getByRole('button', { name: 'Agent', exact: true }).click()
  await page.getByRole('textbox', { name: 'Phase name', exact: true }).fill('Audit implementation')
  await page.getByLabel('Phase ID', { exact: true }).fill('audit-implementation')
  await page.getByRole('button', { name: 'Gate', exact: true }).click()
  await page.getByRole('button', { name: 'Remove step', exact: true }).last().click()
  const handle = page.getByRole('button', { name: 'Move Audit implementation', exact: true })
  await handle.focus()
  await page.keyboard.press('Space')
  await expect(handle).toHaveAttribute('aria-pressed', 'true')
  // Let dnd-kit's deferred keyboard listener and layout measurement attach.
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  await page.keyboard.press('ArrowUp')
  await expect(page.getByRole('status').filter({ hasText: /over.*first|position 1/ })).toHaveCount(1)
  await page.keyboard.press('Space')
  await page.getByRole('button', { name: 'Save pipeline', exact: true }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  const phases = (await json('GET', `/projects/${project.id}/workflow`)).workflow.phases
  expect(phases.map((p: { id: string }) => p.id)).toEqual(['audit-implementation', 'first'])
  await page.reload()
  await expect(page.getByRole('button', { name: 'Move Audit implementation', exact: true })).toBeVisible()
})


test('J19 assign a custom worker directly to a workflow step', async ({ page }) => {
  const saved = await json('GET', '/settings')
  try {
    const settings = structuredClone(saved)
    settings.workers.workers = [...(settings.workers.workers ?? []), { id: 'audit-custom-worker', name: 'Audit custom worker', enabled: true, driver: 'codex', model: 'audit-model' }]
    await json('PUT', '/settings', settings)
    await page.goto(`/#${projectPath()}/settings/workflow`)
    await page.getByRole('button', { name: 'Agent', exact: true }).click()
    await page.getByLabel('Worker', { exact: true }).click()
    await expect(page.getByRole('option', { name: 'Audit custom worker', exact: true })).toBeVisible()
    settings.workers.workers.at(-1).name = 'Updated audit worker'
    await json('PUT', '/settings', settings)
    await page.getByRole('option', { name: 'Updated audit worker', exact: true }).click()
    await page.getByRole('button', { name: 'Save pipeline', exact: true }).click()
    await expect(page.getByText('Saved', { exact: true })).toBeVisible()
    const phase = (await json('GET', `/projects/${project.id}/workflow`)).workflow.phases.at(-1)
    expect(phase.config.worker_id).toBe('audit-custom-worker')
    expect(phase.config.role).toBe('executor')
  } finally { await json('PUT', '/settings', saved) }
})
