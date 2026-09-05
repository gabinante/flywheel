import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { ClipboardCheck, ExternalLink, GitPullRequest, MessageSquareWarning, RefreshCw } from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { CodeReviewStatus as ReviewStatus } from '@/components/code-review-status'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { StyledSelect } from '@/components/ui/styled-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import {
  ACTIVE_STATES,
  SEVERITY_CLASS,
  STATE_CLASS,
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
  { value: 'closed', label: 'Stopped / PR closed' },
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
          <ReviewStatus review={r} />
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

type FeedbackGroup = {
  key: string
  repo: string
  number: number
  url: string
  title: string
  rounds: FeedbackRound[]
  newIds: string[]
  latest: FeedbackRound
  changesRequested: boolean
  comments: number
  reviewers: string[]
  dispatched: boolean
}

/** One card per PR: Bugbot posts a review per push and several people may review, but the
 * fix is one pass over the PR — so rounds are grouped and actions apply to all of them. */
function groupFeedback(rounds: FeedbackRound[]): FeedbackGroup[] {
  const byPR = new Map<string, FeedbackGroup>()
  for (const f of rounds) {
    const key = `${f.repo}#${f.number}`
    let g = byPR.get(key)
    if (!g) {
      g = { key, repo: f.repo, number: f.number, url: f.url, title: f.title, rounds: [], newIds: [], latest: f, changesRequested: false, comments: 0, reviewers: [], dispatched: false }
      byPR.set(key, g)
    }
    g.rounds.push(f)
    if (f.state === 'new') g.newIds.push(f.id)
    if (f.state === 'dispatched') g.dispatched = true
    if (f.review_state === 'CHANGES_REQUESTED') g.changesRequested = true
    g.comments += f.comment_count
    if (!g.reviewers.includes(f.reviewer)) g.reviewers.push(f.reviewer)
    const at = (x: FeedbackRound) => new Date(x.submitted_at ?? x.observed_at).getTime()
    if (at(f) > at(g.latest)) g.latest = f
  }
  return [...byPR.values()].sort((a, b) => new Date(b.latest.submitted_at ?? b.latest.observed_at).getTime() - new Date(a.latest.submitted_at ?? a.latest.observed_at).getTime())
}

function FeedbackGroupRow({
  g,
  onState,
  onAddress,
  harness,
}: {
  g: FeedbackGroup
  onState: (ids: string[], state: 'ignored' | 'addressed') => void
  onAddress: (id: string) => void
  harness: string
}) {
  const [open, setOpen] = useState(false)
  const latestAt = g.latest.submitted_at ?? g.latest.observed_at
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] px-4 py-2.5 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <MessageSquareWarning className={cn('size-4', g.changesRequested ? 'text-amber-300' : 'text-muted-foreground')} />
        <a href={g.url} target="_blank" rel="noreferrer" className="font-mono text-muted-foreground hover:underline">
          {g.repo}#{g.number}
        </a>
        <span className="truncate text-sm">{g.title}</span>
        <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', g.changesRequested ? STATE_CLASS.changes_requested : STATE_CLASS.commented)}>
          {g.changesRequested ? 'changes requested' : 'commented'}
        </Badge>
        <button type="button" onClick={() => setOpen((v) => !v)} className="text-muted-foreground hover:text-foreground hover:underline">
          {g.rounds.length === 1
            ? `by ${g.reviewers[0]} · ${g.comments} comment${g.comments === 1 ? '' : 's'}`
            : `${g.rounds.length} reviews by ${g.reviewers.join(', ')} · ${g.comments} comment${g.comments === 1 ? '' : 's'}`}
          {' · '}
          {relativeTime(latestAt)}
        </button>
        {g.dispatched && <span className="ml-auto text-[11px] text-sky-300">addressing…</span>}
        {!g.dispatched && g.newIds.length > 0 && (
          <span className="ml-auto flex gap-1">
            <Button size="sm" className="h-6 px-2 text-[11px]" onClick={() => onAddress(g.latest.state === 'new' ? g.latest.id : g.newIds[0])}>
              Address with {harness}
            </Button>
            <Button size="sm" variant="ghost" className="h-6 px-2 text-[11px]" onClick={() => onState(g.newIds, 'addressed')}>
              Mark addressed
            </Button>
            <Button size="sm" variant="ghost" className="h-6 px-2 text-[11px]" onClick={() => onState(g.newIds, 'ignored')}>
              Ignore
            </Button>
          </span>
        )}
      </div>
      {open && g.rounds.length > 1 && (
        <ul className="mt-2 space-y-1 border-t border-white/5 pt-2 text-muted-foreground">
          {g.rounds.map((f) => (
            <li key={f.id} className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-[10px]">{(f.head_sha ?? "").slice(0, 7)}</span>
              <span>{f.reviewer}</span>
              <span>{f.review_state.toLowerCase().replace('_', ' ')}</span>
              <span>{f.comment_count} comment{f.comment_count === 1 ? '' : 's'}</span>
              <span>{relativeTime(f.submitted_at ?? f.observed_at)}</span>
              <Badge variant="muted" className="h-4 px-1 text-[10px]">{f.state}</Badge>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function CodeReviewsPage() {
  const { client } = useAPI()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const [params, setParams] = useSearchParams()
  const state = params.get('state') ?? 'all'
  const offset = Math.max(0, Number(params.get('offset')) || 0)
  const [total, setTotal] = useState(0)

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
    const query = { project_id: projectId || undefined, state: state === 'all' ? undefined : state, limit: 100, offset }
    void Promise.all([
      client.GET('/code-reviews', { params: { query } }),
      client.GET('/code-reviews/status'),
      client.GET('/code-reviews/feedback', { params: { query: { limit: 50, project_id: projectId || undefined } } }),
    ]).then(([list, st, fb]) => {
      if (cancelled) return
      if (!list.response.ok || !list.data) {
        setErr(formatApiError(list.error))
        setRequests((prev) => prev ?? [])
      } else {
        setErr(null)
        setRequests(list.data.requests)
        setTotal(list.data.total ?? 0)
      }
      if (st.response.ok && st.data) setStatus(st.data)
      if (fb.response.ok && fb.data) setFeedback(fb.data.rounds.filter((r) => (r.state === 'new' || r.state === 'dispatched')))
    })
    const t = setInterval(refresh, 10_000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [client, state, tick, refresh, projectId, offset])

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

  const setFeedbackState = async (ids: string[], st: 'ignored' | 'addressed') => {
    await Promise.all(ids.map((id) => client.POST('/code-reviews/feedback/{roundID}/state', { params: { path: { roundID: id } }, body: { state: st } })))
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
          {groupFeedback(feedback).map((g) => (
            <FeedbackGroupRow key={g.key} g={g} onState={setFeedbackState} onAddress={addressFeedback} harness="Claude Code" />
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
      {total > 100 && <nav aria-label="Review pagination" className="flex items-center justify-between">
        <Button variant="outline" disabled={offset === 0} onClick={() => setParams((p) => { p.set('offset', String(Math.max(0, offset - 100))); return p })}>Previous reviews</Button>
        <span className="text-xs text-muted-foreground">{offset + 1}–{Math.min(offset + 100, total)} of {total}</span>
        <Button variant="outline" disabled={offset + 100 >= total} onClick={() => setParams((p) => { p.set('offset', String(offset + 100)); return p })}>Next reviews</Button>
      </nav>}
      <p className="text-[11px] text-muted-foreground">
        <ExternalLink className="mr-1 inline size-3" />
        Reviews run in detached worktrees under <code>{status?.repo_root ?? '~/git'}/&lt;repo&gt;-worktrees/review-&lt;n&gt;-&lt;run&gt;</code>. Clean checkouts are removed afterwards; unfinished work is preserved.
      </p>
    </div>
  )
}
