import { test, expect } from '@playwright/test'

// Executable acceptance criteria for known missing/broken journeys. An unexpected
// pass tells us to remove the expected-failure annotation and close the audit item.
test('G01 library workflow save must identify its actual destination', async ({ page }) => {
  await page.goto('/#/workflows')
  await page.locator('a[href*="/workflows/new?template="]').first().click()
  await expect(page.getByLabel('Pipeline name', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save to library', exact: true })).toHaveCount(1)
  await expect(page.getByRole('button', { name: 'Save org default', exact: true })).toHaveCount(0)
})

test('G02 an unknown workflow template should show a recoverable error', async ({ page }) => {
  await page.goto('/#/workflows/new?template=nonexistent-audit-template')
  await expect(page.getByText(/template not found|could not load.*template/i)).toBeVisible({ timeout: 3000 })
})

test('G03 a failed scheduled action should retain an actionable error', async ({ page }) => {
  await page.goto('/#/schedule')
  await page.route('**/schedule/*/run', route => route.fulfill({ status: 503, json: { error: 'Audit action failed' } }))
  page.on('dialog', d => d.accept())
  await page.getByRole('button', { name: 'Run now', exact: true }).first().click()
  // Allow the refresh response to render before checking persistent feedback.
  await page.waitForTimeout(300)
  await expect(page.locator('main')).toContainText('Audit action failed', { timeout: 2000 })
})

test('G04 schedule a specific PR review for a chosen time', async ({ page }) => {
  test.fail(true, 'AUDIT-04: no custom scheduled review creation surface exists')
  await page.goto('/#/schedule')
  await expect(page.getByRole('heading', { name: 'Scheduled actions', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: /Schedule review|New scheduled action/i })).toBeVisible({ timeout: 2000 })
})

test('G05 create a workflow from scratch through the library', async ({ page }) => {
  test.fail(true, 'AUDIT-05: only template-derived creation is exposed')
  await page.goto('/#/workflows')
  await expect(page.getByRole('heading', { name: 'Workflows', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: /New workflow|Create workflow/i })).toBeVisible({ timeout: 2000 })
})

for (const width of [1440, 390]) {
  test(`J15 inspect navigation, control names and layout at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 900 })
    const errors: string[] = []
    page.on('pageerror', e => errors.push(e.message))
    const audit = []
    for (const path of ['/my/prs', '/my/reviews', '/schedule', '/workflows', '/settings?section=workers', '/sessions']) {
      await page.goto(`/#${path}`)
      await expect(page.locator('main')).toBeVisible()
      await expect(page.locator('main h1').first()).toBeVisible()
      const layout = await page.evaluate(() => ({
        width: innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        unnamedButtons: [...document.querySelectorAll('main button')].filter(b => !b.textContent?.trim() && !b.getAttribute('aria-label') && !b.getAttribute('title') && !b.getAttribute('aria-labelledby') && !b.labels?.length).map(b => b.outerHTML.slice(0, 200)),
      }))
      audit.push({ path, ...layout })
      await page.screenshot({ path: info.outputPath(`${path.split('?')[0].replaceAll('/', '_')}.png`), fullPage: true })
    }
    await info.attach('Layout and control audit', { body: JSON.stringify(audit, null, 2), contentType: 'application/json' })
    expect(errors).toEqual([])
  })
}

test('G06 an unknown page should provide a way back to the workspace', async ({ page }) => {
  await page.goto('/#/nonexistent-audit-page')
  await expect(page.getByText(/page not found|could not find.*page/i)).toBeVisible({ timeout: 2000 })
})

test('G07 a failed save to the workflow library should explain the failure', async ({ page }) => {
  await page.goto('/#/workflows/org')
  await expect(page.getByLabel('Pipeline name', { exact: true })).toBeVisible()
  await page.route('**/workflow-library', route => route.request().method() === 'POST'
    ? route.fulfill({ status: 503, json: { error: 'Audit library unavailable' } }) : route.continue())
  await page.getByRole('button', { name: 'Save to library', exact: true }).click()
  await expect(page.locator('main')).toContainText('Audit library unavailable', { timeout: 2000 })
})

test('G08 a missing saved workflow must not masquerade as a new default', async ({ page }) => {
  await page.goto('/#/workflows/nonexistent-audit-workflow')
  await expect(page.getByRole('alert')).toContainText('Could not load this workflow')
  await expect(page.getByRole('button', { name: 'Save workflow', exact: true })).toBeDisabled()
  await page.getByRole('link', { name: 'Workflow library', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Workflows', exact: true })).toBeVisible()
})
