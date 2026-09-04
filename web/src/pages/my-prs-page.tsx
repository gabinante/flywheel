import { useCallback, useEffect, useState } from 'react'
import { ChevronDown, ChevronRight, GitPullRequest, RefreshCw } from 'lucide-react'

import { PRCardRow } from '@/components/pr-card-row'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

type MyPullRequests = components['schemas']['MyPullRequests']
type PullRequestCard = components['schemas']['PullRequestCard']

const COLLAPSE_KEY = 'flywheel.myprs.collapsed'

function readCollapsed(): Record<string, boolean> {
  try {
    return JSON.parse(window.localStorage.getItem(COLLAPSE_KEY) ?? '{}') as Record<string, boolean>
  } catch {
    return {}
  }
}

function groupByRepo(prs: PullRequestCard[]): Array<{ repo: string; items: PullRequestCard[] }> {
  const m = new Map<string, PullRequestCard[]>()
  for (const pr of prs) {
    const list = m.get(pr.repo) ?? []
    list.push(pr)
    m.set(pr.repo, list)
  }
  return [...m.entries()]
    .map(([repo, items]) => ({ repo, items }))
    .sort((a, b) => b.items.length - a.items.length || a.repo.localeCompare(b.repo))
}

function RepoSection({
  id,
  repo,
  items,
  collapsed,
  onToggle,
  renderExtra,
}: {
  id: string
  repo: string
  items: PullRequestCard[]
  collapsed: boolean
  onToggle: (id: string) => void
  renderExtra?: (pr: PullRequestCard) => React.ReactNode
}) {
  const Chevron = collapsed ? ChevronRight : ChevronDown
  return (
    <div className="rounded-xl border border-white/5">
      <button
        type="button"
        onClick={() => onToggle(id)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-white/[0.03]"
        aria-expanded={!collapsed}
      >
        <Chevron className="size-4 text-muted-foreground" />
        <span className="font-medium">{repo}</span>
        <span className="text-xs text-muted-foreground">{items.length}</span>
      </button>
      {!collapsed && (
        <div className="flex flex-col gap-2 px-2 pb-2">
          {items.map((pr) => (
            <PRCardRow key={`${pr.repo}#${pr.number}`} pr={pr} extra={renderExtra?.(pr)} />
          ))}
        </div>
      )}
    </div>
  )
}

export function MyPRsPage() {
  const { client } = useAuth()
  const [data, setData] = useState<MyPullRequests | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [addressing, setAddressing] = useState<string | null>(null)
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => readCollapsed())

  const toggle = (id: string) => {
    setCollapsed((prev) => {
      const next = { ...prev, [id]: !prev[id] }
      try {
        window.localStorage.setItem(COLLAPSE_KEY, JSON.stringify(next))
      } catch {
        /* per-viewer convenience only */
      }
      return next
    })
  }

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
            {groupByRepo(data.open).map((g) => (
              <RepoSection
                key={g.repo}
                id={`open:${g.repo}`}
                repo={g.repo}
                items={g.items}
                collapsed={!!collapsed[`open:${g.repo}`]}
                onToggle={toggle}
                renderExtra={(pr) =>
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
            {groupByRepo(data.merged).map((g) => (
              <RepoSection
                key={g.repo}
                id={`merged:${g.repo}`}
                repo={g.repo}
                items={g.items}
                collapsed={collapsed[`merged:${g.repo}`] ?? true}
                onToggle={toggle}
              />
            ))}
          </section>
        </>
      )}
    </div>
  )
}
