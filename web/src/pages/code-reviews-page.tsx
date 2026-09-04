import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ClipboardCheck, ExternalLink, GitPullRequest, MessageSquareWarning, RefreshCw } from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StyledSelect } from '@/components/ui/styled-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import {
  ACTIVE_STATES,
  SEVERITY_CLASS,
  STATE_CLASS,
  VERDICT_LABEL,
  findingCounts,
  type CodeReviewRequest,
  type FeedbackRound,
} from '@/lib/codereview-format'
import { relativeTime } from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

type CodeReviewStatus = components['schemas']['CodeReviewStatus']

const STATE_OPTIONS = [
  { value: 'all', label: 'All states' },
  { value: 'queued', label: 'Queued' },
  { value: 'reviewing', label: 'Reviewing' },
  { value: 'watching', label: 'Watching' },
  { value: 'approved', label: 'Approved' },
  { value: 'changes_requested', label: 'Changes requested' },
  { value: 'commented', label: 'Commented / dry run' },
  { value: 'failed', label: 'Failed' },
  { value: 'closed', label: 'Closed' },
]

function ReviewRow({ r, base }: { r: CodeReviewRequest; base: string }) {
  const counts = findingCounts(r.findings)
  const active = ACTIVE_STATES.has(r.state)
  return (
    <Link
      to={`${base}/code-reviews/${r.id}`}
      className="grid grid-cols-[auto_1fr_auto] items-start gap-3 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3 transition-colors hover:border-white/20 hover:bg-white/[0.06]"
    >
      <GitPullRequest className={cn('mt-0.5 size-4', active ? 'animate-pulse text-sky-300' : 'text-muted-foreground')} />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-xs text-muted-foreground">
            {r.repo}#{r.number}
          </span>
          <span className="truncate text-sm font-medium">{r.title || '(fetching title…)'}</span>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs">
          <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', STATE_CLASS[r.state])}>
            {r.state.replace('_', ' ')}
          </Badge>
          {r.verdict && (
            <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', STATE_CLASS[r.verdict === 'approve' ? 'approved' : r.verdict === 'request_changes' ? 'changes_requested' : 'commented'])}>
              {VERDICT_LABEL[r.verdict] ?? r.verdict}
            </Badge>
          )}
          {r.dry_run && <Badge variant="muted" className="h-5 px-1.5 text-[10px]">dry run</Badge>}
          <Badge variant="muted" className="h-5 px-1.5 text-[10px]">{r.harness}</Badge>
          <span className="text-muted-foreground">{r.origin.replace('_', ' ')}</span>
          {r.author && <span className="text-muted-foreground">by {r.author}</span>}
          {r.attempt > 1 && <span className="text-muted-foreground">attempt {r.attempt}</span>}
          {(['P0', 'P1', 'P2', 'P3'] as const).map((sev) =>
            counts[sev] ? (
              <span key={sev} className={cn('rounded border px-1 font-mono text-[10px]', SEVERITY_CLASS[sev])}>
                {counts[sev]} {sev}
              </span>
            ) : null,
          )}
          {r.error && <span className="truncate text-red-300" title={r.error}>· {r.error.slice(0, 80)}</span>}
        </div>
      </div>
      <div className="text-right text-xs text-muted-foreground">{relativeTime(r.updated_at)}</div>
    </Link>
  )
}

function FeedbackRow({
  f,
  onState,
  onAddress,
  harness,
}: {
  f: FeedbackRound
  onState: (id: string, state: 'ignored' | 'addressed') => void
  onAddress: (id: string) => void
  harness: string
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-2.5 text-xs">
      <MessageSquareWarning className={cn('size-4', f.review_state === 'CHANGES_REQUESTED' ? 'text-amber-300' : 'text-muted-foreground')} />
      <a href={f.url} target="_blank" rel="noreferrer" className="font-mono text-muted-foreground hover:underline">
        {f.repo}#{f.number}
      </a>
      <span className="truncate text-sm">{f.title}</span>
      <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', f.review_state === 'CHANGES_REQUESTED' ? STATE_CLASS.changes_requested : f.review_state === 'APPROVED' ? STATE_CLASS.approved : STATE_CLASS.commented)}>
        {f.review_state.toLowerCase().replace('_', ' ')}
      </Badge>
      <span className="text-muted-foreground">
        by {f.reviewer} · {f.comment_count} comment{f.comment_count === 1 ? '' : 's'} · {relativeTime(f.submitted_at ?? f.observed_at)}
      </span>
      <Badge variant="muted" className="h-5 px-1.5 text-[10px]">{f.state}</Badge>
      {f.state === 'dispatched' && <span className="ml-auto text-[11px] text-sky-300">addressing…</span>}
      {f.state === 'new' && (
        <span className="ml-auto flex gap-1">
          <Button size="sm" className="h-6 px-2 text-[11px]" onClick={() => onAddress(f.id)}>
            Address with {harness}
          </Button>
          <Button size="sm" variant="ghost" className="h-6 px-2 text-[11px]" onClick={() => onState(f.id, 'addressed')}>
            Mark addressed
          </Button>
          <Button size="sm" variant="ghost" className="h-6 px-2 text-[11px]" onClick={() => onState(f.id, 'ignored')}>
            Ignore
          </Button>
        </span>
      )}
    </div>
  )
}

export function CodeReviewsPage() {
  const { client } = useAuth()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const [params, setParams] = useSearchParams()
  const state = params.get('state') ?? 'all'

  const [requests, setRequests] = useState<CodeReviewRequest[] | null>(null)
  const [feedback, setFeedback] = useState<FeedbackRound[]>([])
  const [status, setStatus] = useState<CodeReviewStatus | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [text, setText] = useState('')
  const [dryRun, setDryRun] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [tick, setTick] = useState(0)

  const refresh = useCallback(() => setTick((t) => t + 1), [])

  useEffect(() => {
    let cancelled = false
    const query = { state: state === 'all' ? undefined : state, limit: 100 }
    void Promise.all([
      client.GET('/code-reviews', { params: { query } }),
      client.GET('/code-reviews/status'),
      client.GET('/code-reviews/feedback', { params: { query: { limit: 50 } } }),
    ]).then(([list, st, fb]) => {
      if (cancelled) return
      if (!list.response.ok || !list.data) {
        setErr(formatApiError(list.error))
        setRequests((prev) => prev ?? [])
      } else {
        setErr(null)
        setRequests(list.data.requests)
      }
      if (st.response.ok && st.data) setStatus(st.data)
      if (fb.response.ok && fb.data) setFeedback(fb.data.rounds.filter((r) => r.state === 'new' || r.state === 'dispatched'))
    })
    const t = setInterval(refresh, 10_000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [client, state, tick, refresh])

  const submit = async () => {
    if (!text.trim()) return
    setSubmitting(true)
    const { error, response } = await client.POST('/code-reviews', { body: { text, dry_run: dryRun || undefined } })
    setSubmitting(false)
    if (!response.ok) {
      setErr(formatApiError(error))
      return
    }
    setText('')
    refresh()
  }

  const setFeedbackState = async (id: string, st: 'ignored' | 'addressed') => {
    await client.POST('/code-reviews/feedback/{roundID}/state', { params: { path: { roundID: id } }, body: { state: st } })
    refresh()
  }

  const addressFeedback = async (id: string) => {
    const { error, response } = await client.POST('/code-reviews/feedback/{roundID}/address', { params: { path: { roundID: id } } })
    if (!response.ok) setErr(formatApiError(error))
    refresh()
  }

  const publishOff = status && !status.publish

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        <OrgProjectCrumbs orgId={orgSlug} projectId={projectSlug} projectLabel={projectLabel} />
        <span className="px-1">/</span>
        <span className="text-foreground" aria-current="page">
          Code Reviews
        </span>
      </p>

      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
            <ClipboardCheck className="size-5 text-muted-foreground" />
            Code Reviews
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            In-depth PR reviews by {status?.harness ?? 'the configured harness'}: inline conversational comments, changes requested on any P0/P1,
            approved otherwise.
          </p>
        </div>
        {status && (
          <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <span>{status.login ? `gh: ${status.login}` : 'gh: not signed in'}</span>
            <span>
              {status.active} running · {status.queued} queued · {status.watching} watching · {status.reviews_posted} posted
            </span>
            {status.last_poll_at && <span>polled {relativeTime(status.last_poll_at)}</span>}
            {status.last_error && (
              <span className="text-red-300" title={status.last_error}>
                error
              </span>
            )}
            <Button size="sm" variant="ghost" className="h-6 px-2" onClick={refresh}>
              <RefreshCw className="size-3" />
            </Button>
          </div>
        )}
      </header>

      {publishOff && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-xs text-amber-100">
          Publishing is off (<code>REVIEW_PUBLISH=false</code>): reviews run and record findings but nothing is posted to GitHub.
        </div>
      )}
      {err && <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">{err}</div>}

      <section className="rounded-2xl border border-white/10 bg-white/5 p-3 backdrop-blur-md">
        <Textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Paste PR URLs or owner/repo#123 references — one or many. Each becomes its own review."
          className="min-h-[3.5rem] text-sm"
        />
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <Button size="sm" onClick={submit} disabled={submitting || !text.trim()}>
            <GitPullRequest className="size-4" /> Review
          </Button>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Switch checked={dryRun} onCheckedChange={setDryRun} /> Dry run (do not post)
          </label>
          <div className="ml-auto">
            <StyledSelect value={state} onValueChange={(v) => setParams(v === 'all' ? {} : { state: v }, { replace: true })} options={STATE_OPTIONS} aria-label="State" />
          </div>
        </div>
      </section>

      {feedback.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-medium text-muted-foreground">Reviews landed on your PRs</h2>
          {feedback.map((f) => (
            <FeedbackRow key={f.id} f={f} onState={setFeedbackState} onAddress={addressFeedback} harness="Claude Code" />
          ))}
        </section>
      )}

      <section className="flex flex-col gap-2">
        {requests === null ? (
          <div className="py-12 text-center text-sm text-muted-foreground">Loading…</div>
        ) : requests.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/10 py-12 text-center text-sm text-muted-foreground">
            No review requests yet. Paste a PR above, or enable the review-requested watcher.
          </div>
        ) : (
          requests.map((r) => <ReviewRow key={r.id} r={r} base={base} />)
        )}
      </section>
      <p className="text-[11px] text-muted-foreground">
        <ExternalLink className="mr-1 inline size-3" />
        Reviews run in detached worktrees under <code>{status?.repo_root ?? '~/git'}/&lt;repo&gt;-worktrees/review-&lt;n&gt;</code> and are removed afterwards.
      </p>
    </div>
  )
}
