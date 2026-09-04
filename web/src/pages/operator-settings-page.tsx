import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Activity, Bot, Cpu, FileText, GitPullRequest, KeyRound, MessageSquareReply, Save, Settings as SettingsIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StyledSelect } from '@/components/ui/styled-select'
import { Switch } from '@/components/ui/switch'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type OperatorSettings = components['schemas']['OperatorSettings']
type UpdateRequest = components['schemas']['UpdateOperatorSettingsRequest']
type LinearStatus = components['schemas']['LinearStatus']
type CodeReviewStatus = components['schemas']['CodeReviewStatus']
type Project = components['schemas']['Project']

type SectionID = 'models' | 'dispatch' | 'linear' | 'review' | 'feedback' | 'reports'

const SECTIONS: Array<{ id: SectionID; label: string; description: string; icon: React.ComponentType<{ className?: string }> }> = [
  { id: 'models', label: 'Models & harnesses', description: 'Claude Code and Codex binaries and the default model and effort each uses.', icon: Cpu },
  { id: 'dispatch', label: 'Dispatch', description: 'Whether tickets are picked up by implementation workers, and which harness runs them.', icon: Bot },
  { id: 'linear', label: 'Linear', description: 'Personal API key and sync of the projects you lead.', icon: KeyRound },
  { id: 'review', label: 'Code review', description: 'Harness, publishing, and the review-requested / re-review watchers.', icon: GitPullRequest },
  { id: 'feedback', label: 'PR feedback', description: 'How review comments on your PRs get addressed.', icon: MessageSquareReply },
  { id: 'reports', label: 'Reports', description: 'Linear project status updates and the weekly roundup.', icon: FileText },
]

const HARNESSES = [
  { value: 'codex', label: 'Codex' },
  { value: 'claude', label: 'Claude Code' },
]
const DRIVERS = [...HARNESSES, { value: 'generic', label: 'Generic CLI' }]
const EFFORTS = ['', 'low', 'medium', 'high', 'xhigh'].map((v) => ({ value: v || 'default', label: v || 'Harness default' }))
const WEEKDAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'].map((d) => ({ value: d, label: d }))
const HEALTHS = [
  { value: 'onTrack', label: 'On track' },
  { value: 'atRisk', label: 'At risk' },
  { value: 'offTrack', label: 'Off track' },
]

function toRequest(s: OperatorSettings, apiKey: string, clearKey: boolean): UpdateRequest {
  return {
    linear: {
      enabled: s.linear.enabled,
      project_ids: s.linear.project_ids,
      default_team_key: s.linear.default_team_key,
      sync_interval_seconds: s.linear.sync_interval_seconds,
      ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
      ...(clearKey ? { clear_api_key: true } : {}),
    },
    review: s.review,
    feedback: s.feedback,
    report: s.report,
    dispatch: s.dispatch,
    harnesses: s.harnesses,
  }
}

function Field({ label, hint, children, htmlFor }: { label: string; hint?: string; children: React.ReactNode; htmlFor?: string }) {
  return (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor={htmlFor} className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground/80">{hint}</p>}
    </div>
  )
}

function Toggle({ id, label, hint, checked, onChange }: { id: string; label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-start justify-between gap-4 rounded-lg border border-white/5 bg-white/[0.02] px-3 py-2.5">
      <div className="min-w-0">
        <Label htmlFor={id} className="text-sm">
          {label}
        </Label>
        {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </div>
  )
}

export function OperatorSettingsPage() {
  const { client } = useAuth()
  const [settings, setSettings] = useState<OperatorSettings | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [clearKey, setClearKey] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [params] = useSearchParams()
  const initial = params.get('section')
  const [section, setSection] = useState<SectionID>(SECTIONS.some((s) => s.id === initial) ? (initial as SectionID) : 'linear')
  const [linearStatus, setLinearStatus] = useState<LinearStatus | null>(null)
  const [reviewStatus, setReviewStatus] = useState<CodeReviewStatus | null>(null)
  const [projects, setProjects] = useState<Project[]>([])

  const loadStatus = useCallback(() => {
    void client.GET('/linear/status').then(({ data }) => setLinearStatus(data ?? null))
    void client.GET('/code-reviews/status').then(({ data }) => setReviewStatus(data ?? null))
  }, [client])

  useEffect(() => {
    let cancelled = false
    void client.GET('/settings').then(({ data, error, response }) => {
      if (cancelled) return
      if (!response.ok || !data) {
        setErr(formatApiError(error))
        return
      }
      setSettings(data)
    })
    // Projects live under the (single) org; gather them across whatever orgs exist.
    void client.GET('/orgs', {}).then(async ({ data: orgs }) => {
      if (cancelled || !orgs) return
      const lists = await Promise.all(
        orgs.map((o) =>
          client.GET('/orgs/{orgID}/projects', { params: { path: { orgID: o.id ?? '' } } }).then(({ data }) => data ?? []),
        ),
      )
      if (!cancelled) setProjects(lists.flat())
    })
    loadStatus()
    return () => {
      cancelled = true
    }
  }, [client, loadStatus])

  const update = <K extends keyof OperatorSettings>(key: K, patch: Partial<OperatorSettings[K]>) => {
    setSettings((prev) => (prev ? { ...prev, [key]: { ...(prev[key] as object), ...patch } } : prev))
    setDirty(true)
  }

  const save = async () => {
    if (!settings) return
    setSaving(true)
    setErr(null)
    const { data, error, response } = await client.PUT('/settings', { body: toRequest(settings, apiKey, clearKey) })
    setSaving(false)
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setSettings(data)
    setApiKey('')
    setClearKey(false)
    setDirty(false)
    setSavedAt(Date.now())
    // Give the services a beat to reconfigure before refreshing status.
    window.setTimeout(loadStatus, 1500)
  }

  const projectOptions = useMemo(
    () => [{ value: 'none', label: 'None' }, ...projects.map((p) => ({ value: p.id ?? '', label: p.name ?? p.id ?? '' }))],
    [projects],
  )

  if (!settings) {
    return (
      <div className="mx-auto max-w-5xl p-6">
        {err ? <p className="text-sm text-destructive">{err}</p> : <p className="text-sm text-muted-foreground">Loading settings…</p>}
      </div>
    )
  }

  const { linear, review, feedback, report, dispatch, harnesses } = settings
  const harnessDefault = (h: string) => (h === 'codex' ? harnesses.codex : harnesses.claude)

  return (
    <div className="mx-auto w-full max-w-[1500px] space-y-6 p-6 xl:px-10">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <SettingsIcon className="size-5 text-muted-foreground" />
            Settings
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Operator-level configuration. Changes apply to the running server immediately — no restart needed.{' '}
            {!settings.saved && <span className="text-amber-300/90">Currently showing values from the environment; save once to take over.</span>}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {err && <span className="text-sm text-destructive">{err}</span>}
          {savedAt && !dirty && !err && <span className="text-xs text-emerald-300/80">Saved</span>}
          <Button onClick={save} disabled={saving || (!dirty && !apiKey && !clearKey)}>
            <Save className="mr-1.5 size-4" />
            {saving ? 'Saving…' : 'Save changes'}
          </Button>
        </div>
      </div>

      <div className="grid gap-8 lg:grid-cols-[240px_minmax(0,1fr)]">
        <nav className="space-y-1">
          {SECTIONS.map((s) => {
            const Icon = s.icon
            const active = section === s.id
            return (
              <button
                key={s.id}
                type="button"
                onClick={() => setSection(s.id)}
                className={`flex w-full items-start gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors ${
                  active ? 'bg-white/[0.06] text-foreground' : 'text-muted-foreground hover:bg-white/[0.04] hover:text-foreground'
                }`}
              >
                <Icon className="mt-0.5 size-4 shrink-0" />
                <span>
                  <span className="block font-medium">{s.label}</span>
                  <span className="block text-xs text-muted-foreground">{s.description}</span>
                </span>
              </button>
            )
          })}
        </nav>

        <div className="space-y-6">
          {section === 'models' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">Models &amp; harnesses</CardTitle>
                <CardDescription>
                  Flywheel never calls a model provider directly: it runs the two local harnesses, which carry their own logins. Set the
                  executable and the default model and reasoning effort for each here. Code review, PR feedback, and dispatch choose a
                  harness and inherit these unless they set their own.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6">
                {(['claude', 'codex'] as const).map((h) => {
                  const d = harnesses[h]
                  const label = h === 'claude' ? 'Claude Code' : 'Codex'
                  return (
                    <div key={h} className="space-y-3 rounded-xl border border-white/5 bg-white/[0.02] p-4">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{label}</span>
                        <span className="text-xs text-muted-foreground">
                          used by{' '}
                          {[review.harness === h && 'code review', feedback.harness === h && 'PR feedback', dispatch.driver === h && 'dispatch'].filter(Boolean).join(', ') ||
                            'nothing yet'}
                        </span>
                      </div>
                      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                        <Field label="Executable" htmlFor={`${h}-bin`} hint={h === 'claude' ? 'e.g. claude, or an absolute path' : 'e.g. codex, or an absolute path'}>
                          <Input id={`${h}-bin`} value={d.bin} onChange={(e) => update('harnesses', { [h]: { ...d, bin: e.target.value } })} />
                        </Field>
                        <Field label="Default model" htmlFor={`${h}-model`} hint={h === 'claude' ? 'Blank uses the CLI default (e.g. claude-fable-5-1).' : 'Blank uses the CLI default (e.g. gpt-5.6-sol).'}>
                          <Input id={`${h}-model`} value={d.model} onChange={(e) => update('harnesses', { [h]: { ...d, model: e.target.value } })} />
                        </Field>
                        <Field label="Default reasoning effort" htmlFor={`${h}-effort`}>
                          <StyledSelect
                            className="h-9 w-full min-w-0"
                            id={`${h}-effort`}
                            value={d.reasoning_effort || 'default'}
                            onValueChange={(v) => update('harnesses', { [h]: { ...d, reasoning_effort: v === 'default' ? '' : v } })}
                            options={EFFORTS}
                          />
                        </Field>
                      </div>
                    </div>
                  )
                })}
              </CardContent>
            </Card>
          )}

          {section === 'dispatch' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  Dispatch
                  <StatusBadge ok={dispatch.enabled} okLabel="Picking up tickets" offLabel="Off — tickets wait" />
                </CardTitle>
                <CardDescription>
                  The dispatcher runs implementation workers for tickets in agent phases, in a worktree per ticket under{' '}
                  <code>{dispatch.worktree_dir || '~/git'}/&lt;repo&gt;-worktrees/</code>. Projects can still opt out individually. Workers reach
                  Flywheel over MCP with a key that is minted automatically{dispatch.worker_key_set ? ' (present)' : ' (missing — restart the server to mint one)'}.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Toggle
                  id="dispatch-enabled"
                  label="Dispatch enabled"
                  hint="Off = nothing is picked up; tickets stay where they are. On = waiting tickets start immediately."
                  checked={dispatch.enabled}
                  onChange={(v) => update('dispatch', { enabled: v })}
                />
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <Field label="Default harness" htmlFor="dispatch-driver" hint="Used unless a project's worker roles say otherwise.">
                    <StyledSelect className="h-9 w-full min-w-0" id="dispatch-driver" value={dispatch.driver} onValueChange={(v) => update('dispatch', { driver: v })} options={DRIVERS} />
                  </Field>
                  <Field label="Model" htmlFor="dispatch-model" hint={`Blank uses the harness default${harnessDefault(dispatch.driver).model ? ` (${harnessDefault(dispatch.driver).model})` : ''}.`}>
                    <Input id="dispatch-model" value={dispatch.model} onChange={(e) => update('dispatch', { model: e.target.value })} />
                  </Field>
                  <Field label="Reasoning effort" htmlFor="dispatch-effort">
                    <StyledSelect
                      className="h-9 w-full min-w-0"
                      id="dispatch-effort"
                      value={dispatch.reasoning_effort || 'default'}
                      onValueChange={(v) => update('dispatch', { reasoning_effort: v === 'default' ? '' : v })}
                      options={EFFORTS}
                    />
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Max concurrent workers" htmlFor="dispatch-max" hint="Across all projects; each project can set a lower cap.">
                    <Input id="dispatch-max" type="number" min={1} value={dispatch.max_workers} onChange={(e) => update('dispatch', { max_workers: Number(e.target.value) || 1 })} />
                  </Field>
                  <Field label="Worktree root" htmlFor="dispatch-root" hint="Applies to new workers after a server restart.">
                    <Input id="dispatch-root" value={dispatch.worktree_dir} onChange={(e) => update('dispatch', { worktree_dir: e.target.value })} />
                  </Field>
                </div>
              </CardContent>
            </Card>
          )}

          {section === 'linear' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  Linear
                  <StatusBadge ok={!!linearStatus?.enabled} okLabel={linearStatus?.viewer_name ? `Connected as ${linearStatus.viewer_name}` : 'Connected'} offLabel="Not connected" />
                </CardTitle>
                <CardDescription>
                  Linear is the ticket store. Flywheel mirrors the projects you lead, files tickets for implementation work, and posts
                  status updates. Code reviews never create tickets.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Field
                  label="Personal API key"
                  htmlFor="linear-key"
                  hint={
                    linear.api_key_set
                      ? `A key ending in ${linear.api_key_hint || '…'} is stored. Paste a new one to replace it.`
                      : 'Create one at Linear → Settings → Security & access → Personal API keys.'
                  }
                >
                  <div className="flex gap-2">
                    <Input
                      id="linear-key"
                      type="password"
                      autoComplete="off"
                      placeholder={linear.api_key_set ? '•••••••••••••••• (unchanged)' : 'lin_api_…'}
                      value={apiKey}
                      onChange={(e) => {
                        setApiKey(e.target.value)
                        setClearKey(false)
                      }}
                    />
                    {linear.api_key_set && (
                      <Button
                        variant={clearKey ? 'destructive' : 'outline'}
                        type="button"
                        onClick={() => {
                          setClearKey((v) => !v)
                          setApiKey('')
                        }}
                      >
                        {clearKey ? 'Will remove on save' : 'Remove'}
                      </Button>
                    )}
                  </div>
                </Field>
                <Toggle
                  id="linear-enabled"
                  label="Sync enabled"
                  hint="Poll Linear for the projects you lead and mirror their issues as tickets."
                  checked={linear.enabled}
                  onChange={(v) => update('linear', { enabled: v })}
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Default team key" htmlFor="linear-team" hint="Team used when filing tickets for a project that isn't linked yet (e.g. RLETD).">
                    <Input id="linear-team" value={linear.default_team_key} onChange={(e) => update('linear', { default_team_key: e.target.value })} />
                  </Field>
                  <Field label="Sync interval (seconds)" htmlFor="linear-interval">
                    <Input
                      id="linear-interval"
                      type="number"
                      min={15}
                      value={linear.sync_interval_seconds}
                      onChange={(e) => update('linear', { sync_interval_seconds: Number(e.target.value) || 60 })}
                    />
                  </Field>
                </div>
                <Field
                  label="Extra project IDs"
                  htmlFor="linear-projects"
                  hint="Comma-separated Linear project IDs to mirror in addition to the ones you lead. Individual projects can also be linked from their settings page."
                >
                  <Input
                    id="linear-projects"
                    value={linear.project_ids.join(', ')}
                    onChange={(e) => update('linear', { project_ids: e.target.value.split(',').map((v) => v.trim()).filter(Boolean) })}
                  />
                </Field>
                {linearStatus && (
                  <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
                    <span>{linearStatus.projects_linked} projects linked</span>
                    <span>{linearStatus.tickets_linked} tickets linked</span>
                    {linearStatus.last_run_at && <span>last sync {new Date(linearStatus.last_run_at).toLocaleTimeString()}</span>}
                    {linearStatus.last_error && <span className="text-destructive">{linearStatus.last_error}</span>}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {section === 'review' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  Code review
                  <StatusBadge ok={!!reviewStatus?.enabled} okLabel={reviewStatus?.login ? `gh as ${reviewStatus.login}` : 'Enabled'} offLabel="Disabled" />
                </CardTitle>
                <CardDescription>
                  PR-keyed reviews run in a detached worktree at the PR head and post a single review through your <code>gh</code> CLI: inline
                  conversational comments, request changes on P0/P1, approve otherwise.{' '}
                  <Link to="/code-reviews" className="underline">
                    Open the queue
                  </Link>
                  .
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Toggle id="review-enabled" label="Code review enabled" checked={review.enabled} onChange={(v) => update('review', { enabled: v })} />
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <Field label="Harness" htmlFor="review-harness">
                    <StyledSelect className="h-9 w-full min-w-0" id="review-harness" value={review.harness} onValueChange={(v) => update('review', { harness: v })} options={HARNESSES} />
                  </Field>
                  <Field label="Model" htmlFor="review-model" hint={`Blank uses the harness default${harnessDefault(review.harness).model ? ` (${harnessDefault(review.harness).model})` : ''}.`}>
                    <Input id="review-model" value={review.model} onChange={(e) => update('review', { model: e.target.value })} placeholder="e.g. gpt-5.6-sol" />
                  </Field>
                  <Field label="Reasoning effort" htmlFor="review-effort">
                    <StyledSelect
                      className="h-9 w-full min-w-0"
                      id="review-effort"
                      value={review.reasoning_effort || 'default'}
                      onValueChange={(v) => update('review', { reasoning_effort: v === 'default' ? '' : v })}
                      options={EFFORTS}
                    />
                  </Field>
                </div>
                <Toggle
                  id="review-publish"
                  label="Publish reviews to GitHub"
                  hint="Off = dry run: findings are recorded in Flywheel only. On = the review is posted on the PR."
                  checked={review.publish}
                  onChange={(v) => update('review', { publish: v })}
                />
                <Toggle
                  id="review-watch-requested"
                  label="Watch review-requested:@me"
                  hint="Queue a review for every open PR that requests your review, and re-review watched PRs on new commits or when your review is dismissed."
                  checked={review.watch_requested}
                  onChange={(v) => update('review', { watch_requested: v })}
                />
                <Toggle
                  id="review-watch-authored"
                  label="Watch feedback on my PRs"
                  hint="Detect actionable reviews (changes requested, inline comments) landing on PRs you authored so they can be addressed."
                  checked={review.watch_authored}
                  onChange={(v) => update('review', { watch_authored: v })}
                />
                <Toggle id="review-skip-drafts" label="Skip draft PRs" checked={review.skip_drafts} onChange={(v) => update('review', { skip_drafts: v })} />
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <Field label="Max concurrent reviews" htmlFor="review-max">
                    <Input id="review-max" type="number" min={1} value={review.max_concurrent} onChange={(e) => update('review', { max_concurrent: Number(e.target.value) || 1 })} />
                  </Field>
                  <Field label="Poll interval (seconds)" htmlFor="review-poll" hint="Applies from the next scheduled tick.">
                    <Input id="review-poll" type="number" min={30} value={review.poll_interval_seconds} onChange={(e) => update('review', { poll_interval_seconds: Number(e.target.value) || 120 })} />
                  </Field>
                  <Field label="Repo root" htmlFor="review-root" hint="Primary checkouts live here; review worktrees go in <repo>-worktrees/.">
                    <Input id="review-root" value={review.repo_root} onChange={(e) => update('review', { repo_root: e.target.value })} />
                  </Field>
                </div>
                {reviewStatus && (
                  <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
                    <span>{reviewStatus.active} active</span>
                    <span>{reviewStatus.queued} queued</span>
                    <span>{reviewStatus.watching} watching</span>
                    <span>{reviewStatus.new_feedback} new feedback</span>
                    <span>{reviewStatus.reviews_posted} posted</span>
                    {reviewStatus.last_error && <span className="text-destructive">{reviewStatus.last_error}</span>}
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          {section === 'feedback' && (
            <Card>
              <CardHeader>
                <CardTitle>PR feedback</CardTitle>
                <CardDescription>
                  When a review lands on one of your PRs, Flywheel can run a harness in a worktree on the PR branch to address the comments,
                  push, and reply. Requires “Watch feedback on my PRs” under Code review.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Toggle
                  id="feedback-auto"
                  label="Address feedback automatically"
                  hint="Off = feedback rounds appear in the Code Reviews page with an “Address” button. On = they are addressed as they arrive."
                  checked={feedback.auto_address}
                  onChange={(v) => update('feedback', { auto_address: v })}
                />
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <Field label="Harness" htmlFor="feedback-harness">
                    <StyledSelect className="h-9 w-full min-w-0" id="feedback-harness" value={feedback.harness} onValueChange={(v) => update('feedback', { harness: v })} options={HARNESSES} />
                  </Field>
                  <Field label="Model" htmlFor="feedback-model" hint={`Blank uses the harness default${harnessDefault(feedback.harness).model ? ` (${harnessDefault(feedback.harness).model})` : ''}.`}>
                    <Input id="feedback-model" value={feedback.model} onChange={(e) => update('feedback', { model: e.target.value })} />
                  </Field>
                  <Field label="Reasoning effort" htmlFor="feedback-effort">
                    <StyledSelect
                      className="h-9 w-full min-w-0"
                      id="feedback-effort"
                      value={feedback.reasoning_effort || 'default'}
                      onValueChange={(v) => update('feedback', { reasoning_effort: v === 'default' ? '' : v })}
                      options={EFFORTS}
                    />
                  </Field>
                </div>
              </CardContent>
            </Card>
          )}

          {section === 'reports' && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  Reports
                  <Badge variant="outline" className="text-xs">
                    <Activity className="mr-1 size-3" />
                    {linearStatus?.enabled ? 'Posts to Linear' : 'Preview only until Linear is connected'}
                  </Badge>
                </CardTitle>
                <CardDescription>
                  Delta project status updates and a weekly roundup, rendered from tickets, merged PRs, and agent sessions. Previews are always
                  available from each project's settings; these toggles control automatic posting.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <Toggle
                  id="report-updates"
                  label="Post project status updates"
                  hint="For every Linear-linked project, post a delta update when the interval has elapsed."
                  checked={report.project_updates_enabled}
                  onChange={(v) => update('report', { project_updates_enabled: v })}
                />
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  <Field label="Update interval (hours)" htmlFor="report-interval">
                    <Input id="report-interval" type="number" min={1} value={report.project_update_interval_hours} onChange={(e) => update('report', { project_update_interval_hours: Number(e.target.value) || 48 })} />
                  </Field>
                  <Field label="Default health" htmlFor="report-health">
                    <StyledSelect className="h-9 w-full min-w-0" id="report-health" value={report.default_health} onValueChange={(v) => update('report', { default_health: v })} options={HEALTHS} />
                  </Field>
                </div>
                <Toggle
                  id="report-weekly"
                  label="Post the weekly roundup"
                  hint="Prepends a section to the roundup document and/or posts a project update."
                  checked={report.weekly_enabled}
                  onChange={(v) => update('report', { weekly_enabled: v })}
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Weekly day" htmlFor="report-day">
                    <StyledSelect className="h-9 w-full min-w-0" id="report-day" value={report.weekly_day} onValueChange={(v) => update('report', { weekly_day: v })} options={WEEKDAYS} />
                  </Field>
                  <Field label="Hour (local, 0–23)" htmlFor="report-hour">
                    <Input id="report-hour" type="number" min={0} max={23} value={report.weekly_hour} onChange={(e) => update('report', { weekly_hour: Number(e.target.value) || 0 })} />
                  </Field>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <Field label="Roundup document ID" htmlFor="report-doc" hint="Linear document that receives a new section each week.">
                    <Input id="report-doc" value={report.roundup_document_id} onChange={(e) => update('report', { roundup_document_id: e.target.value })} />
                  </Field>
                  <Field label="Roundup project" htmlFor="report-project" hint="Flywheel project whose Linear project also gets the roundup as an update.">
                    <StyledSelect
                      className="h-9 w-full min-w-0"
                      id="report-project"
                      value={report.roundup_project_id || 'none'}
                      onValueChange={(v) => update('report', { roundup_project_id: v === 'none' ? '' : v })}
                      options={projectOptions}
                    />
                  </Field>
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}

function StatusBadge({ ok, okLabel, offLabel }: { ok: boolean; okLabel: string; offLabel: string }) {
  return (
    <Badge variant="outline" className={`text-xs font-normal ${ok ? 'border-emerald-400/30 text-emerald-300' : 'border-white/10 text-muted-foreground'}`}>
      <span className={`mr-1.5 inline-block size-1.5 rounded-full ${ok ? 'bg-emerald-400' : 'bg-muted-foreground/50'}`} />
      {ok ? okLabel : offLabel}
    </Badge>
  )
}
