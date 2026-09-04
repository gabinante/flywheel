import { useCallback, useEffect, useState } from 'react'
import { ClipboardCheck, RefreshCw } from 'lucide-react'

import { PRCardRow } from '@/components/pr-card-row'
import { reviewStateLabel } from '@/lib/pr-format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

type MyReviews = components['schemas']['MyReviews']
type PullRequestCard = components['schemas']['PullRequestCard']

const ACTIVE_REVIEW_STATES = new Set(['queued', 'fetching', 'reviewing', 'publishing'])

export function MyReviewsPage() {
  const { client } = useAuth()
  const [data, setData] = useState<MyReviews | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [queueing, setQueueing] = useState<string | null>(null)

  const load = useCallback(
    async (refresh = false) => {
      const { data, error, response } = await client.GET('/me/reviews', { params: { query: refresh ? { refresh: true } : {} } })
      if (!response.ok || !data) {
        setErr(formatApiError(error))
        return
      }
      setErr(null)
      setData(data)
    },
    [client],
  )

  useEffect(() => {
    // Deferred so the effect body itself stays free of state updates.
    void Promise.resolve().then(() => load())
  }, [load])

  const enqueue = async (pr: PullRequestCard) => {
    const key = `${pr.repo}#${pr.number}`
    setQueueing(key)
    const { error, response } = await client.POST('/code-reviews', { body: { text: pr.url, watch: true } })
    setQueueing(null)
    if (!response.ok) {
      setErr(formatApiError(error))
      return
    }
    void load(true)
  }

  const actions = (pr: PullRequestCard) => {
    const key = `${pr.repo}#${pr.number}`
    const active = pr.review && ACTIVE_REVIEW_STATES.has(pr.review.state)
    return (
      <>
        {pr.my_review_state && (
          <Badge variant="outline" className="text-[11px] font-normal text-muted-foreground">
            you {reviewStateLabel(pr.my_review_state)}
          </Badge>
        )}
        {active && <span className="text-[11px] text-sky-300">reviewing…</span>}
        <Button size="sm" variant="outline" disabled={queueing === key || !!active} onClick={() => enqueue(pr)}>
          {queueing === key ? 'Queueing…' : pr.review ? 'Re-review' : 'Review with harness'}
        </Button>
      </>
    )
  }

  return (
    <div className="mx-auto w-full max-w-[1500px] space-y-6 p-6 xl:px-10">
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
          onClick={async () => {
            setLoading(true)
            await load(true)
            setLoading(false)
          }}
        >
          <RefreshCw className={`mr-1.5 size-3.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {err && <p className="text-sm text-destructive">{err}</p>}
      {!data && !err && <p className="text-sm text-muted-foreground">Asking GitHub…</p>}

      {data && (
        <>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Requested from me · {data.requested.length}</h2>
            {data.requested.length === 0 && <p className="text-sm text-muted-foreground">Nobody is waiting on you.</p>}
            {data.requested.map((pr) => (
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
