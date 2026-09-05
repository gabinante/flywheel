import { useActivityVersion } from '@/contexts/use-activity'
import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, Copy, ExternalLink, GitBranch } from 'lucide-react'

import { AgentConversation, type ConversationMessage } from '@/components/agent-conversation'
import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { RunProgress } from '@/components/run-progress'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useOperatorOverview, type RunProgressData } from '@/hooks/use-operator-overview'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import {
  HARNESS_CLASS,
  HARNESS_LABEL,
  STATUS_DOT,
  compact,
  duration,
  linkHref,
  relativeTime,
} from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

type SessionDetail = components['schemas']['SessionDetail']

function Meta({ label, value, mono }: { label: string; value: string | number | undefined; mono?: boolean }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className={cn('truncate text-sm text-foreground', mono && 'font-mono text-xs')} title={String(value ?? '')}>
        {value === undefined || value === '' ? '—' : value}
      </span>
    </div>
  )
}

function ContinueSession({ sessionId, harness, onReplied }: { sessionId: string; harness: string; onReplied: () => void }) {
  const { client } = useAPI()
  const [messages, setMessages] = useState<ConversationMessage[]>([])
  const send = async (text: string) => {
    const mine: ConversationMessage = { id: `u-${Date.now()}`, role: 'user', content: text, created_at: new Date().toISOString() }
    setMessages((prev) => [...prev, mine])
    const { data, error, response } = await client.POST('/sessions/{sessionID}/continue', { params: { path: { sessionID: sessionId } }, body: { message: text } })
    if (!response.ok || !data) {
      setMessages((prev) => prev.filter((m) => m.id !== mine.id))
      return formatApiError(error)
    }
    setMessages((prev) => [...prev, { id: `a-${Date.now()}`, role: 'assistant', content: data.reply, created_at: new Date().toISOString() }])
    onReplied()
    return null
  }
  return (
    <AgentConversation
      title={`Continue this session (${harness === 'codex' ? 'Codex' : 'Claude Code'})`}
      description="Sends a new message into the same harness session, in its working directory, with full permissions. New turns show up in the transcript after the next ingestion pass."
      messages={messages}
      onSend={send}
      emptyHint="Pick up where this session left off."
    />
  )
}

export function SessionDetailPage() {
  const activityVersion = useActivityVersion('sessions')
  const { client } = useAPI()
  const { sessionId } = useParams<{ sessionId: string }>()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const { data: overview } = useOperatorOverview()
  const live = overview?.in_flight.find(item => item.session_href === `/sessions/${sessionId}`)
  const [reloadTick, setReloadTick] = useState(0)
  const [detail, setDetail] = useState<SessionDetail | null>(null)
  const running = (overview?.in_flight.some(item => item.session_href === `/sessions/${sessionId}`) ?? false)
    || (detail?.session.id === sessionId && detail?.session.metadata?.flywheel_running === true)
  const [err, setErr] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!sessionId) return
    let cancelled = false
    client
      .GET('/sessions/{sessionID}', { params: { path: { sessionID: sessionId } } })
      .then(({ data, error, response }) => {
        if (cancelled) return
        if (!response.ok || !data) {
          setErr(formatApiError(error))
          return
        }
        setErr(null)
        setDetail(data)
      })
    return () => {
      cancelled = true
    }
  }, [client, sessionId, reloadTick, activityVersion])



  const s = detail?.session
  const savedProgress = s?.metadata?.flywheel_progress as RunProgressData | undefined
  const progress = live?.progress ?? (savedProgress && { ...savedProgress, worker_state: running ? savedProgress.worker_state : 'exited' as const })

  const copyPath = () => {
    if (!s?.transcript_path) return
    void navigator.clipboard?.writeText(s.transcript_path).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div className="flex h-[calc(100vh-7.5rem)] min-h-[32rem] flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        <OrgProjectCrumbs orgId={orgSlug} projectId={projectSlug} projectLabel={projectLabel} />
        <span className="px-1">/</span>
        <span className="text-foreground" aria-current="page">
          Session
        </span>
      </p>
      <div>
        <Button asChild variant="ghost" size="sm">
          <Link to={`${base}/sessions`}>
            <ArrowLeft className="size-4" /> All sessions
          </Link>
        </Button>
      </div>

      {err && <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">{err}</div>}

      {s && (
        <div className="flex min-h-0 flex-1 flex-col gap-4">
          <header className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className={cn('inline-block size-2 rounded-full', STATUS_DOT[s.status] ?? STATUS_DOT.ended)} />
              <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px]', HARNESS_CLASS[s.harness])}>
                {HARNESS_LABEL[s.harness] ?? s.harness}
              </Badge>
              <Badge variant="muted" className="h-5 px-1.5 text-[10px]">
                {s.origin}
              </Badge>
              <Badge variant="muted" className="h-5 px-1.5 text-[10px]">
                {s.status}
              </Badge>
              {s.parent_session_id && (
                <Link to={`${base}/sessions/${s.parent_session_id}`} className="text-xs text-muted-foreground underline-offset-2 hover:underline">
                  parent session
                </Link>
              )}
            </div>
            <h1 className="text-lg font-semibold tracking-tight">{s.title || s.first_prompt || s.external_id}</h1>
          </header>

          {progress && <RunProgress progress={progress} />}

          <Card className="border-white/10 bg-white/5 backdrop-blur-md">
            <CardContent className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
              <Meta label="Repository" value={s.repo} />
              <Meta label="Branch" value={s.branch} mono />
              <Meta label="Model" value={s.model} />
              <Meta label="Effort" value={s.reasoning_effort} />
              <Meta label="Started" value={`${relativeTime(s.started_at)} · ${new Date(s.started_at).toLocaleString()}`} />
              <Meta label="Last activity" value={relativeTime(running ? progress?.last_activity_at ?? s.last_activity_at : s.last_activity_at)} />
              <Meta label="Duration" value={duration(s.started_at, running ? overview?.updated_at ?? s.last_activity_at : s.ended_at ?? s.last_activity_at)} />
              <Meta label={running ? 'Tokens (this run)' : 'Tokens'} value={running && progress ? progress.tokens_in === undefined && progress.tokens_out === undefined ? 'Not reported yet' : `${compact(progress.tokens_in)} in / ${compact(progress.tokens_out)} out` : `${compact(s.tokens_in)} in / ${compact(s.tokens_out)} out`} />
              <Meta label="Prompts" value={s.prompt_count} />
              <Meta label={running ? 'Tool calls (this run)' : 'Tool calls'} value={running && progress ? progress.tool_calls : s.tool_call_count} />
              <Meta label="Working directory" value={s.cwd} mono />
              <Meta label="External id" value={s.external_id} mono />
            </CardContent>
            {s.transcript_path && (
              <div className="flex items-center gap-2 border-t border-white/10 px-4 py-2 text-xs text-muted-foreground">
                <span className="truncate font-mono">{s.transcript_path}</span>
                <Button variant="ghost" size="sm" className="ml-auto h-6 px-2" onClick={copyPath}>
                  <Copy className="size-3" /> {copied ? 'Copied' : 'Copy path'}
                </Button>
              </div>
            )}
          </Card>

          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto pr-1">
            {s.links.length > 0 && (
              <Card className="border-white/10 bg-white/5 backdrop-blur-md">
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">Linked</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2 pt-0">
                  {s.links.map((l) => {
                    const href = linkHref(l)
                    const inner = (
                      <>
                        <span className="text-[10px] uppercase text-muted-foreground">{l.kind.replace('_', ' ')}</span>
                        <span className="font-mono text-xs">{l.ref}</span>
                        {href && <ExternalLink className="size-3 text-muted-foreground" />}
                      </>
                    )
                    const cls = 'inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.04] px-2 py-1'
                    return href ? (
                      <a key={`${l.kind}:${l.ref}`} href={href} target="_blank" rel="noreferrer" className={cn(cls, 'hover:bg-white/[0.08]')}>
                        {inner}
                      </a>
                    ) : (
                      <span key={`${l.kind}:${l.ref}`} className={cls}>
                        {inner}
                      </span>
                    )
                  })}
                </CardContent>
              </Card>
            )}

            <Card className="border-white/10 bg-white/5 backdrop-blur-md">
              <CardHeader className="py-3">
                <CardTitle className="text-sm">Prompts ({detail.prompts.length})</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3 pt-0">
                {detail.prompts.length === 0 && (
                  <p className="text-xs italic text-muted-foreground">No operator prompts captured for this session.</p>
                )}
                {detail.prompts.map((p) => (
                  <div key={p.seq} className="rounded-lg border border-white/10 bg-black/20 p-3">
                    <div className="mb-1 flex items-center gap-2 text-[10px] uppercase tracking-wide text-muted-foreground">
                      <span>{p.role}</span>
                      <span>#{p.seq}</span>
                      <span className="ml-auto normal-case">{new Date(p.ts).toLocaleString()}</span>
                    </div>
                    <pre className="whitespace-pre-wrap break-words font-sans text-sm text-foreground/90">{p.text}</pre>
                  </div>
                ))}
              </CardContent>
            </Card>

            {detail.children.length > 0 && (
              <Card className="border-white/10 bg-white/5 backdrop-blur-md">
                <CardHeader className="py-3">
                  <CardTitle className="text-sm">Subagents ({detail.children.length})</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-2 pt-0">
                  {detail.children.map((c) => (
                    <Link
                      key={c.id}
                      to={`${base}/sessions/${c.id}`}
                      className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2 text-sm hover:bg-white/[0.06]"
                    >
                      <span className={cn('size-2 rounded-full', STATUS_DOT[c.status] ?? STATUS_DOT.ended)} />
                      <span className="truncate">{c.title || c.first_prompt || c.external_id}</span>
                      <span className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
                        {c.branch && (
                          <span className="inline-flex items-center gap-1">
                            <GitBranch className="size-3" />
                            {c.branch}
                          </span>
                        )}
                        {relativeTime(c.last_activity_at)}
                      </span>
                    </Link>
                  ))}
                </CardContent>
              </Card>
            )}
          </div>

          {running ? <p role="status" className="text-xs text-muted-foreground">This session is running. You can continue it after the current run finishes.</p> :
            <ContinueSession sessionId={s.id} harness={s.harness} onReplied={() => setReloadTick((t) => t + 1)} />}
        </div>
      )}
    </div>
  )
}
