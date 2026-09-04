import { useCallback, useEffect, useState } from 'react'
import { GitPullRequest, RefreshCw } from 'lucide-react'

import { PRCardRow } from '@/components/pr-card-row'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

type MyPullRequests = components['schemas']['MyPullRequests']

export function MyPRsPage() {
  const { client } = useAuth()
  const [data, setData] = useState<MyPullRequests | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [addressing, setAddressing] = useState<string | null>(null)

  const load = useCallback(
    async (refresh = false) => {
      const { data, error, response } = await client.GET('/me/prs', { params: { query: refresh ? { refresh: true } : {} } })
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

  const address = async (roundId: string) => {
    setAddressing(roundId)
    const { error, response } = await client.POST('/code-reviews/feedback/{roundID}/address', { params: { path: { roundID: roundId } } })
    setAddressing(null)
    if (!response.ok) {
      setErr(formatApiError(error))
      return
    }
    void load(true)
  }

  return (
    <div className="mx-auto w-full max-w-[1500px] space-y-6 p-6 xl:px-10">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <GitPullRequest className="size-5 text-muted-foreground" />
            My PRs
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Open pull requests you authored, across every repo{data?.login ? ` (as ${data.login})` : ''}. Feedback rounds Flywheel has seen
            are flagged and can be addressed from here.
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
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Open · {data.open.length}</h2>
            {data.open.length === 0 && <p className="text-sm text-muted-foreground">No open PRs.</p>}
            {data.open.map((pr) => (
              <PRCardRow
                key={`${pr.repo}#${pr.number}`}
                pr={pr}
                extra={
                  pr.feedback && pr.feedback.new > 0 && pr.feedback.latest_id ? (
                    <Button size="sm" variant="outline" disabled={addressing === pr.feedback.latest_id} onClick={() => address(pr.feedback!.latest_id)}>
                      {addressing === pr.feedback.latest_id ? 'Dispatching…' : 'Address feedback'}
                    </Button>
                  ) : null
                }
              />
            ))}
          </section>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Merged in the last 7 days · {data.merged.length}</h2>
            {data.merged.length === 0 && <p className="text-sm text-muted-foreground">Nothing merged this week.</p>}
            {data.merged.map((pr) => (
              <PRCardRow key={`${pr.repo}#${pr.number}`} pr={pr} />
            ))}
          </section>
        </>
      )}
    </div>
  )
}
