import { useActivityVersion } from '@/contexts/use-activity'
import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, ExternalLink, RefreshCw, XCircle } from 'lucide-react'

import { AgentConversation, type ConversationMessage } from '@/components/agent-conversation'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { RunProgress } from '@/components/run-progress'
import { CodeReviewStatus } from '@/components/code-review-status'
import { useReviewServiceStatus } from '@/hooks/use-review-service-status'
import { useOperatorOverview } from '@/hooks/use-operator-overview'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import { ACTIVE_STATES, SEVERITY_CLASS, shortSha, type CodeReviewRequest } from '@/lib/codereview-format'
import { relativeTime } from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

function Meta({ label, value }: { label: string; value?: string | number | null }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="truncate text-sm">{value === undefined || value === null || value === '' ? '—' : value}</span>
    </div>
  )
}

function ReviewConversation({ reviewId, harness }: { reviewId: string; harness: string }) {
  const activityVersion = useActivityVersion('reviews')
  const { client } = useAPI()
  const [messages, setMessages] = useState<ConversationMessage[]>([])

  useEffect(() => {
    let cancelled = false
    void client.GET('/code-reviews/{reviewID}/messages', { params: { path: { reviewID: reviewId } } }).then(({ data }) => {
      if (!cancelled && data) setMessages(previous => [...data.messages, ...previous.filter(m => m.id.startsWith('tmp-'))])
    })
    return () => {
      cancelled = true
    }
  }, [client, reviewId, activityVersion])

  const send = async (text: string) => {
    const optimistic: ConversationMessage = { id: `tmp-${Date.now()}`, role: 'user', content: text, created_at: new Date().toISOString() }
    setMessages((prev) => [...prev, optimistic])
    const { data, error, response } = await client.POST('/code-reviews/{reviewID}/messages', { params: { path: { reviewID: reviewId } }, body: { message: text } })
    if (!response.ok || !data) {
      setMessages((prev) => prev.filter((m) => m.id !== optimistic.id))
      return formatApiError(error)
    }
    setMessages((prev) => [...new Map([...prev.filter((m) => m.id !== optimistic.id), ...data.messages].map(m => [m.id, m])).values()])
    return null
  }

  return (
    <AgentConversation
      title={`Talk to the reviewer (${harness === 'codex' ? 'Codex' : 'Claude Code'})`}
      description="Interrogate the review: ask why something was flagged, what it would take to fix, or tell the agent to post a comment or reply on the PR. It resumes its own review session with the PR checked out."
      messages={messages}
      onSend={send}
      emptyHint="Nothing asked yet. Try: “Why is finding 2 a P1?” or “Post finding 1 as an inline comment.”"
    />
  )
}

export function CodeReviewDetailPage() {
  const activityVersion = useActivityVersion('reviews')
  const { client } = useAPI()
  const queryClient = useQueryClient()
  const { data: service } = useReviewServiceStatus()
  const [actionPending, setActionPending] = useState<'rerun' | 'close' | null>(null)
  const { reviewId } = useParams<{ reviewId: string }>()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const [r, setR] = useState<CodeReviewRequest | null>(null)
  const { data: overview } = useOperatorOverview()
  const live = overview?.in_flight.find(item => item.review_id === reviewId)
    ?? overview?.attention.find(item => item.review_id === reviewId)
  const [err, setErr] = useState<string | null>(null)
  const [tick, setTick] = useState(0)
  const refresh = useCallback(() => setTick((t) => t + 1), [])

  useEffect(() => {
    if (!reviewId) return
    let cancelled = false
    void client.GET('/code-reviews/{reviewID}', { params: { path: { reviewID: reviewId } } }).then(({ data, error, response }) => {
      if (cancelled) return
      if (!response.ok || !data) {
        setErr(formatApiError(error))
        return
      }
      setErr(null)
      setR(data)
    })
    return () => {
      cancelled = true
    }
  }, [client, reviewId, tick, activityVersion])

  const act = async (action: 'rerun' | 'close') => {
    if (!reviewId || actionPending) return
    setActionPending(action)
    setErr(null)
    const path = action === 'rerun' ? '/code-reviews/{reviewID}/rerun' : '/code-reviews/{reviewID}/close'
    try {
    const { data, error, response } = await client.POST(path, { params: { path: { reviewID: reviewId } } })
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setR(data)
    refresh()
    void queryClient.invalidateQueries({ queryKey: ['my-reviews'] })
    void queryClient.invalidateQueries({ queryKey: ['operator-overview'] })
    void queryClient.invalidateQueries({ queryKey: ['code-review-status'] })
    } catch (e) { setErr(e instanceof Error ? e.message : 'Could not update review.') }
    finally { setActionPending(null) }
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        <OrgProjectCrumbs orgId={orgSlug} projectId={projectSlug} projectLabel={projectLabel} />
        <span className="px-1">/</span>
        <Link to={`${base}/code-reviews`} className="hover:underline">
          Code Reviews
        </Link>
        <span className="px-1">/</span>
        <span className="text-foreground" aria-current="page">
          {r ? `${r.repo}#${r.number}` : 'Review'}
        </span>
      </p>
      <div className="flex items-center gap-2">
        <Button asChild variant="ghost" size="sm">
          <Link to={`${base}/code-reviews`}>
            <ArrowLeft className="size-4" /> All reviews
          </Link>
        </Button>
        {r && (
          <>
            <Button variant="ghost" size="sm" onClick={() => act('rerun')} disabled={!!actionPending || ACTIVE_STATES.has(r.state)}>
              <RefreshCw className={`size-4 ${actionPending === 'rerun' ? 'animate-spin' : ''}`} /> {actionPending === 'rerun' ? 'Queueing…' : 'Re-review'}
            </Button>
            {r.watch && r.state !== 'closed' && (
              <Button variant="ghost" size="sm" onClick={() => act('close')} disabled={!!actionPending}>
                <XCircle className="size-4" /> {actionPending === 'close' ? 'Stopping…' : ACTIVE_STATES.has(r.state) ? 'Stop review' : 'Stop watching'}
              </Button>
            )}
          </>
        )}
      </div>
      {err && <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">{err}</div>}
      {r && (
        <>
          <header className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <CodeReviewStatus review={r} />
              {r.dry_run && <Badge variant="muted" className="h-5 px-1.5 text-[10px]">dry run</Badge>}
              <Badge variant="muted" className="h-5 px-1.5 text-[10px]">{r.harness}</Badge>
              <a href={r.url} target="_blank" rel="noreferrer" className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground hover:underline">
                Open PR <ExternalLink className="size-3" />
              </a>
              {r.review_url && (
                <a href={r.review_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:underline">
                  Posted review <ExternalLink className="size-3" />
                </a>
              )}
            </div>
            <h1 className="text-lg font-semibold tracking-tight">
              <span className="font-mono text-muted-foreground">{r.repo}#{r.number}</span> {r.title}
            </h1>
          </header>
          {r.state === 'queued' && <p role="status" className="text-sm text-sky-300">{service?.enabled === false ? 'Queued — review service is paused. Enable it in review settings to start.' : `Attempt ${r.attempt} queued. Waiting for a review worker.`}</p>}

          <Card className="border-white/10 bg-white/5 backdrop-blur-md">
            <CardContent className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
              <Meta label="Author" value={r.author} />
              <Meta label="Branch" value={r.head_ref ? `${r.head_ref} → ${r.base_ref}` : undefined} />
              <Meta label="Head" value={shortSha(r.head_sha)} />
              <Meta label="Attempt" value={r.attempt} />
              <Meta label="Origin" value={r.origin.replace('_', ' ')} />
              <Meta label="My review on GitHub" value={r.my_review_state} />
              <Meta label="Reviewed" value={r.reviewed_at ? relativeTime(r.reviewed_at) : undefined} />
              <Meta label="Last checked" value={r.last_checked_at ? relativeTime(r.last_checked_at) : undefined} />
            </CardContent>
            {(live?.session_href || r.session_id) && (
              <div className="border-t border-white/10 px-4 py-2 text-xs">
                <Link to={live?.session_href || `${base}/sessions/${r.session_id}`} className="text-muted-foreground hover:underline">
                  Open the reviewing session →
                </Link>
              </div>
            )}
            {r.error && <div className="border-t border-white/10 px-4 py-2 text-xs text-red-200">{r.error}</div>}
          </Card>

          {live?.progress && <RunProgress progress={live.progress} />}

          {r.summary && (
            <Card className="border-white/10 bg-white/5 backdrop-blur-md">
              <CardHeader className="py-3">
                <CardTitle className="text-sm">Summary</CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <p className="whitespace-pre-wrap text-sm text-foreground/90">{r.summary}</p>
              </CardContent>
            </Card>
          )}

          <ReviewConversation reviewId={r.id} harness={r.harness} />

          <Card className="border-white/10 bg-white/5 backdrop-blur-md">
            <CardHeader className="py-3">
              <CardTitle className="text-sm">Findings ({r.findings.length})</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 pt-0">
              {r.findings.length === 0 && <p className="text-xs italic text-muted-foreground">No findings recorded.</p>}
              {r.findings.map((f) => (
                <div key={f.id} className="rounded-lg border border-white/10 bg-black/20 p-3">
                  <div className="mb-1 flex flex-wrap items-center gap-2 text-xs">
                    <span className={cn('rounded border px-1.5 font-mono text-[10px]', SEVERITY_CLASS[f.severity])}>{f.severity}</span>
                    <span className="font-medium">{f.title}</span>
                    <span className="font-mono text-muted-foreground">
                      {f.path}
                      {f.line ? `:${f.line}` : ''}
                    </span>
                    <Badge variant="muted" className="ml-auto h-5 px-1.5 text-[10px]">{f.status.replace('_', ' ')}</Badge>
                  </div>
                  <p className="whitespace-pre-wrap text-sm text-foreground/90">{f.body}</p>
                </div>
              ))}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
