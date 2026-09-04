import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Activity, ChevronLeft, ChevronRight, GitBranch, Search, TerminalSquare } from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { StyledSelect } from '@/components/ui/styled-select'
import { Switch } from '@/components/ui/switch'
import { useAuth } from '@/contexts/use-auth'
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
  shortRepo,
  type AgentSession,
} from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

type CollectorStatus = components['schemas']['SessionCollectorStatus']

const PAGE_SIZE = 40

const HARNESS_OPTIONS = [
  { value: 'all', label: 'All harnesses' },
  { value: 'claude_code', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
]
const ORIGIN_OPTIONS = [
  { value: 'all', label: 'Any origin' },
  { value: 'interactive', label: 'Interactive' },
  { value: 'dispatched', label: 'Dispatched' },
  { value: 'automation', label: 'Automation' },
  { value: 'subagent', label: 'Subagent' },
]
const STATUS_OPTIONS = [
  { value: 'all', label: 'Any status' },
  { value: 'active', label: 'Active' },
  { value: 'idle', label: 'Idle' },
  { value: 'ended', label: 'Ended' },
]

function SessionRow({ session, base }: { session: AgentSession; base: string }) {
  const linkCount = session.links.length
  return (
    <Link
      to={`${base}/sessions/${session.id}`}
      className="group grid grid-cols-[auto_1fr_auto] items-start gap-3 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3 transition-colors hover:border-white/20 hover:bg-white/[0.06]"
    >
      <span
        className={cn('mt-1.5 inline-block size-2 rounded-full', STATUS_DOT[session.status] ?? STATUS_DOT.ended)}
        title={session.status}
      />
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className={cn('h-5 px-1.5 text-[10px] font-medium', HARNESS_CLASS[session.harness])}>
            {HARNESS_LABEL[session.harness] ?? session.harness}
          </Badge>
          {session.origin !== 'interactive' && (
            <Badge variant="muted" className="h-5 px-1.5 text-[10px]">
              {session.origin}
            </Badge>
          )}
          <span className="truncate text-sm font-medium text-foreground">
            {session.title || session.first_prompt || session.external_id}
          </span>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {session.repo && (
            <span className="inline-flex items-center gap-1">
              <GitBranch className="size-3" />
              {shortRepo(session.repo)}
              {session.branch && <span className="text-foreground/60">@{session.branch}</span>}
            </span>
          )}
          {session.model && <span>{session.model}</span>}
          <span>{session.prompt_count} prompts</span>
          <span>{session.tool_call_count} tool calls</span>
          <span>
            {compact(session.tokens_in)} in / {compact(session.tokens_out)} out
          </span>
        </div>
        {linkCount > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {session.links.slice(0, 6).map((l) => {
              const href = linkHref(l)
              const chip = (
                <span
                  key={`${l.kind}:${l.ref}`}
                  className={cn(
                    'rounded-md border px-1.5 py-0.5 font-mono text-[10px]',
                    l.kind === 'pr'
                      ? 'border-violet-500/30 bg-violet-500/10 text-violet-200'
                      : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200',
                  )}
                >
                  {l.ref}
                </span>
              )
              return href ? (
                <a
                  key={`${l.kind}:${l.ref}`}
                  href={href}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className="hover:opacity-80"
                >
                  {chip}
                </a>
              ) : (
                chip
              )
            })}
            {linkCount > 6 && <span className="text-[10px] text-muted-foreground">+{linkCount - 6}</span>}
          </div>
        )}
      </div>
      <div className="text-right text-xs text-muted-foreground">
        <div>{relativeTime(session.last_activity_at)}</div>
        <div className="mt-0.5 text-foreground/50">{duration(session.started_at, session.last_activity_at)}</div>
      </div>
    </Link>
  )
}

export function SessionsPage() {
  const { client } = useAuth()
  const { base, projectId, orgSlug, projectSlug } = useProjectPaths()
  const projectLabel = useProjectBreadcrumbLabel(projectId)
  const [params, setParams] = useSearchParams()

  const harness = params.get('harness') ?? 'all'
  const origin = params.get('origin') ?? 'all'
  const status = params.get('status') ?? 'all'
  const repo = params.get('repo') ?? ''
  const q = params.get('q') ?? ''
  const includeSubagents = params.get('subagents') === '1'
  const offset = Number(params.get('offset') ?? '0') || 0

  const [qDraft, setQDraft] = useState(q)
  const [sessions, setSessions] = useState<AgentSession[] | null>(null) // null = first load pending
  const [total, setTotal] = useState(0)
  const [err, setErr] = useState<string | null>(null)
  const [collector, setCollector] = useState<CollectorStatus | null>(null)

  const setParam = useCallback(
    (key: string, value: string | null, resetOffset = true) => {
      const next = new URLSearchParams(params)
      if (value === null || value === '' || value === 'all') next.delete(key)
      else next.set(key, value)
      if (resetOffset) next.delete('offset')
      setParams(next, { replace: true })
    },
    [params, setParams],
  )

  // Debounce the search box into the URL.
  useEffect(() => {
    const t = setTimeout(() => {
      if (qDraft !== q) setParam('q', qDraft)
    }, 300)
    return () => clearTimeout(t)
  }, [qDraft, q, setParam])

  const query = useMemo(
    () => ({
      harness: harness === 'all' ? undefined : (harness as 'claude_code' | 'codex'),
      origin: origin === 'all' ? undefined : (origin as 'interactive' | 'dispatched' | 'automation' | 'subagent'),
      status: status === 'all' ? undefined : (status as 'active' | 'idle' | 'ended'),
      repo: repo || undefined,
      q: q || undefined,
      include_subagents: includeSubagents || undefined,
      limit: PAGE_SIZE,
      offset: offset || undefined,
    }),
    [harness, origin, status, repo, q, includeSubagents, offset],
  )

  useEffect(() => {
    let cancelled = false
    void client.GET('/sessions', { params: { query } }).then(({ data, error, response }) => {
      if (cancelled) return
      if (!response.ok || !data) {
        setErr(formatApiError(error))
        setSessions((prev) => prev ?? [])
        return
      }
      setErr(null)
      setSessions(data.sessions)
      setTotal(data.total)
    })
    return () => {
      cancelled = true
    }
  }, [client, query])

  useEffect(() => {
    let cancelled = false
    const load = () =>
      client.GET('/sessions/collector').then(({ data, response }) => {
        if (!cancelled && response.ok && data) setCollector(data)
      })
    void load()
    const t = setInterval(load, 30_000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [client])

  const pageEnd = Math.min(offset + PAGE_SIZE, total)

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        <OrgProjectCrumbs orgId={orgSlug} projectId={projectSlug} projectLabel={projectLabel} />
        <span className="px-1">/</span>
        <span className="text-foreground" aria-current="page">
          Sessions
        </span>
      </p>

      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
            <TerminalSquare className="size-5 text-muted-foreground" />
            Sessions
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Every Claude Code and Codex session, linked to the PRs and issues it touched.
          </p>
        </div>
        {collector && (
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <Activity className={cn('size-3.5', collector.last_error ? 'text-red-400' : 'text-emerald-400')} />
            <span>
              {collector.sessions_total} sessions · Claude {collector.by_harness.claude_code ?? 0} · Codex{' '}
              {collector.by_harness.codex ?? 0}
            </span>
            {collector.last_run_at && <span>· synced {relativeTime(collector.last_run_at)}</span>}
            {collector.last_error && (
              <span className="text-red-400" title={collector.last_error}>
                · collector error
              </span>
            )}
          </div>
        )}
      </header>

      <section className="flex flex-wrap items-center gap-2 rounded-2xl border border-white/10 bg-white/5 p-3 backdrop-blur-md">
        <div className="relative min-w-[16rem] flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={qDraft}
            onChange={(e) => setQDraft(e.target.value)}
            placeholder="Search prompts, titles, branches, PRs, issues…"
            className="h-8 pl-8 text-sm"
            aria-label="Search sessions"
          />
        </div>
        <StyledSelect value={harness} onValueChange={(v) => setParam('harness', v)} options={HARNESS_OPTIONS} aria-label="Harness" />
        <StyledSelect value={origin} onValueChange={(v) => setParam('origin', v)} options={ORIGIN_OPTIONS} aria-label="Origin" />
        <StyledSelect value={status} onValueChange={(v) => setParam('status', v)} options={STATUS_OPTIONS} aria-label="Status" />
        <Input
          value={repo}
          onChange={(e) => setParam('repo', e.target.value)}
          placeholder="repo"
          className="h-8 w-40 text-sm"
          aria-label="Repository filter"
        />
        <label className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          <Switch checked={includeSubagents} onCheckedChange={(v) => setParam('subagents', v ? '1' : null)} />
          Show subagents
        </label>
      </section>

      {err && (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">{err}</div>
      )}

      <section className="flex flex-col gap-2">
        {sessions === null ? (
          <div className="py-12 text-center text-sm text-muted-foreground">Loading sessions…</div>
        ) : sessions.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/10 py-12 text-center text-sm text-muted-foreground">
            No sessions match. The collector tails <code>~/.claude/projects</code> and <code>~/.codex</code>; new
            sessions appear within seconds.
          </div>
        ) : (
          sessions.map((s) => <SessionRow key={s.id} session={s} base={base} />)
        )}
      </section>

      {total > PAGE_SIZE && (
        <footer className="flex items-center justify-between text-xs text-muted-foreground">
          <span>
            {offset + 1}–{pageEnd} of {total}
          </span>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              disabled={offset === 0}
              onClick={() => setParam('offset', String(Math.max(0, offset - PAGE_SIZE)), false)}
            >
              <ChevronLeft className="size-4" /> Prev
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={pageEnd >= total}
              onClick={() => setParam('offset', String(offset + PAGE_SIZE), false)}
            >
              Next <ChevronRight className="size-4" />
            </Button>
          </div>
        </footer>
      )}
    </div>
  )
}
