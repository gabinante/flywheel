import { QueuePRReviews } from '@/components/queue-pr-reviews'
import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardCheck, Loader2, RefreshCw } from 'lucide-react'

import { AutoReviewToggle } from '@/components/auto-review-toggle'

import { RunProgress } from '@/components/run-progress'
import { useOperatorOverview } from '@/hooks/use-operator-overview'
import { useReviewServiceStatus } from '@/hooks/use-review-service-status'
import { ACTIVE_STATES, reviewStatus } from '@/lib/codereview-format'
import { PRCardRow } from '@/components/pr-card-row'
import { reviewStateLabel } from '@/lib/pr-format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

type MyReviews = components['schemas']['MyReviews']
type PullRequestCard = components['schemas']['PullRequestCard']

const reviewsKey = ['my-reviews']

export function MyReviewsPage() {
  const { client } = useAPI()
  const queryClient = useQueryClient()
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [queueing, setQueueing] = useState<Set<string>>(new Set())
  const pending = useRef(new Set<string>())
  const [queueErrors, setQueueErrors] = useState<Record<string, string>>({})
  const { data: overview } = useOperatorOverview()
  const { data: service } = useReviewServiceStatus()
  const load = async (refresh = false, signal?: AbortSignal) => {
    const result = await client.GET('/me/reviews', { params: { query: refresh ? { refresh: true } : {} }, signal })
    if (!result.response.ok || !result.data) throw new Error(formatApiError(result.error))
    return result.data
  }
  const { data, error } = useQuery({ queryKey: reviewsKey, queryFn: ({ signal }) => load(false, signal) })

  const refresh = async () => {
    setLoading(true)
    try {
      await queryClient.cancelQueries({ queryKey: reviewsKey })
      const fresh = await load(true)
      queryClient.setQueryData(reviewsKey, fresh)
      setErr(null)
    } catch (e) { setErr(e instanceof Error ? e.message : 'Could not refresh reviews.') }
    finally { setLoading(false) }
  }

  const enqueue = async (pr: PullRequestCard) => {
    const key = `${pr.repo}#${pr.number}`
    if (pending.current.has(key)) return
    pending.current.add(key)
    setQueueing(new Set(pending.current))
    setQueueErrors(prev => ({ ...prev, [key]: '' }))
    try {
      const result = await client.POST('/code-reviews', { body: { text: pr.url, watch: true } })
      if (!result.response.ok || !result.data) throw new Error(formatApiError(result.error))
      const review = result.data.requests.find(r => `${r.repo}#${r.number}` === key)
      if (!review) throw new Error('The server did not confirm a review. Refresh to check the queue.')
      await queryClient.cancelQueries({ queryKey: reviewsKey })
      queryClient.setQueryData<MyReviews>(reviewsKey, prev => {
        if (!prev) return prev
        const update = (cards: PullRequestCard[]) => cards.map(c => `${c.repo}#${c.number}` === key ? { ...c, review } : c)
        return { ...prev, requested: update(prev.requested), reviewed: update(prev.reviewed) }
      })
      void queryClient.invalidateQueries({ queryKey: ['operator-overview'] })
      void queryClient.invalidateQueries({ queryKey: ['code-review-status'] })
    } catch (e) {
      setQueueErrors(prev => ({ ...prev, [key]: e instanceof Error ? e.message : 'Could not queue review. Try again.' }))
    } finally {
      pending.current.delete(key)
      setQueueing(new Set(pending.current))
    }
  }

  const actions = (pr: PullRequestCard) => {
    const key = `${pr.repo}#${pr.number}`
    const active = pr.review && ACTIVE_STATES.has(pr.review.state)
    const live = pr.review && pr.review.state !== 'queued'
      ? overview?.in_flight.find(item => item.review_id === pr.review!.id)
      : undefined
    const sessionHref = live?.session_href || (!active && pr.review?.session_id ? `/sessions/${pr.review.session_id}` : undefined)
    return (
      <>
        {pr.my_review_state && (
          <Badge variant="outline" className="text-[11px] font-normal text-muted-foreground">
            you {reviewStateLabel(pr.my_review_state)}
          </Badge>
        )}
        <Button size="sm" variant="outline" disabled={queueing.has(key) || !!active} onClick={() => enqueue(pr)}>
          {queueing.has(key) && <Loader2 className="size-3.5 animate-spin" />}
          {queueing.has(key) ? 'Queueing…' : active ? reviewStatus(pr.review!).label : pr.review ? 'Re-review' : 'Review with harness'}
        </Button>
        {active && <p role="status" className="text-xs text-sky-300">
          {pr.review?.state === 'queued' && service?.enabled === false ? 'Queued — review service is paused. Enable Service above to start.' : `Attempt ${pr.review?.attempt} · ${reviewStatus(pr.review!).label}`}
        </p>}
        {pr.review && <Link className="text-xs text-sky-300 hover:underline" to={`/code-reviews/${pr.review.id}`}>View review</Link>}
        {sessionHref && <Link className="text-xs text-sky-300 hover:underline" to={sessionHref}>Open session</Link>}
        {active && live?.progress && <RunProgress progress={live.progress} compact />}
        {queueErrors[key] && <p role="alert" className="text-xs text-destructive">{queueErrors[key]}</p>}
      </>
    )
  }

  return (
    <div className="w-full space-y-6 p-2 xl:px-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <ClipboardCheck className="size-5 text-muted-foreground" />
            My Reviews
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Open PRs waiting on your review, and open PRs you have already reviewed, across every repo
            {data?.login ? ` (as ${data.login})` : ''}. Queue any of them for a harness review.
            {data && <span className="ml-2 text-xs">fetched {relativeTime(data.fetched_at)}</span>}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={loading}
          onClick={refresh}
        >
          <RefreshCw className={`mr-1.5 size-3.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      <AutoReviewToggle />
      <QueuePRReviews />

      {(err || error) && <p role="alert" className="text-sm text-destructive">{err || error?.message}{data ? " Showing the last successful update." : ""}</p>}
      {!data && !err && !error && <p className="text-sm text-muted-foreground">Asking GitHub…</p>}

      {data && (
        <>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
              Requested from me directly · {data.requested.filter((p) => p.request_kind === 'direct').length}
            </h2>
            {data.requested.filter((p) => p.request_kind === 'direct').length === 0 && <p className="text-sm text-muted-foreground">Nobody is waiting on you personally.</p>}
            {data.requested
              .filter((p) => p.request_kind === 'direct')
              .map((pr) => (
                <PRCardRow key={`${pr.repo}#${pr.number}`} pr={pr} showAuthor extra={actions(pr)} />
              ))}
          </section>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
              Requested via a team I'm on · {data.requested.filter((p) => p.request_kind === 'team').length}
            </h2>
            {data.requested.filter((p) => p.request_kind === 'team').length === 0 && <p className="text-sm text-muted-foreground">No team requests.</p>}
            {data.requested
              .filter((p) => p.request_kind === 'team')
              .map((pr) => (
                <PRCardRow key={`${pr.repo}#${pr.number}`} pr={pr} showAuthor extra={actions(pr)} />
              ))}
          </section>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Reviewed by me, still open · {data.reviewed.length}</h2>
            {data.reviewed.length === 0 && <p className="text-sm text-muted-foreground">No open PRs you have reviewed.</p>}
            {data.reviewed.map((pr) => (
              <PRCardRow key={`${pr.repo}#${pr.number}`} pr={pr} showAuthor extra={actions(pr)} />
            ))}
          </section>
        </>
      )}
    </div>
  )
}
