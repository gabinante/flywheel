import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, ExternalLink, RefreshCw, XCircle } from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import { ACTIVE_STATES, SEVERITY_CLASS, STATE_CLASS, VERDICT_LABEL, shortSha, type CodeReviewRequest } from '@/lib/codereview-format'
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

export function CodeReviewDetailPage() {
  const { client } = useAuth()
  const { reviewId } = useParams<{ reviewId: string }>()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const [r, setR] = useState<CodeReviewRequest | null>(null)
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
    const t = setInterval(refresh, 8_000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [client, reviewId, tick, refresh])

  const act = async (action: 'rerun' | 'close') => {
    if (!reviewId) return
    const path = action === 'rerun' ? '/code-reviews/{reviewID}/rerun' : '/code-reviews/{reviewID}/close'
    const { data, error, response } = await client.POST(path, { params: { path: { reviewID: reviewId } } })
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setR(data)
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
            <Button variant="ghost" size="sm" onClick={() => act('rerun')} disabled={ACTIVE_STATES.has(r.state)}>
              <RefreshCw className="size-4" /> Re-review
            </Button>
            {r.watch && r.state !== 'closed' && (
              <Button variant="ghost" size="sm" onClick={() => act('close')}>
                <XCircle className="size-4" /> Stop watching
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
              <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', STATE_CLASS[r.state])}>
                {r.state.replace('_', ' ')}
              </Badge>
              {r.verdict && <Badge variant="muted" className="h-5 px-1.5 text-[10px]">{VERDICT_LABEL[r.verdict] ?? r.verdict}</Badge>}
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
            {r.session_id && (
              <div className="border-t border-white/10 px-4 py-2 text-xs">
                <Link to={`${base}/sessions/${r.session_id}`} className="text-muted-foreground hover:underline">
                  Open the reviewing session →
                </Link>
              </div>
            )}
            {r.error && <div className="border-t border-white/10 px-4 py-2 text-xs text-red-200">{r.error}</div>}
          </Card>

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
