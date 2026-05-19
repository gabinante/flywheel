import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  LoaderCircle,
  Sparkles,
  TerminalSquare,
} from 'lucide-react'

import { PlanMarkdown } from '@/components/plan-markdown'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import {
  getOrchestratorThread,
  sendOrchestratorMessage,
  type OrchestratorMessage,
  type OrchestratorRun,
  type OrchestratorRunEvent,
  type OrchestratorThread,
} from '@/lib/api/orchestrator-client'
import { cn } from '@/lib/utils'

const POLL_INTERVAL = 15_000
const LIVE_POLL_INTERVAL = 1_200
const GENERATION_HINT_ROTATION_MS = 2_400
const GENERATION_HINTS = [
  'Routing the request to the planner.',
  'Inspecting project context and recent work.',
  'Working through ticket and work-stream updates.',
  'Composing the response.',
]

type PendingUserMessage = {
  id: string
  content: string
  createdAt: string
}

function elapsed(isoDate: string | undefined): string {
  if (!isoDate) return ''
  const diff = Date.now() - new Date(isoDate).getTime()
  if (diff < 0) return 'just now'
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

function formatDuration(startedAt: string | undefined, completedAt?: string): string {
  if (!startedAt) return ''
  const start = new Date(startedAt).getTime()
  const end = completedAt ? new Date(completedAt).getTime() : Date.now()
  const diff = Math.max(0, end - start)
  if (diff < 1000) return '<1s'
  if (diff < 60_000) return `${Math.round(diff / 1000)}s`
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m`
  return `${Math.round(diff / 3_600_000)}h`
}

function getLatestUserMessage(messages: OrchestratorMessage[]): OrchestratorMessage | null {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.role === 'user') {
      return messages[index]
    }
  }
  return null
}

function isPendingMessageAcknowledged(
  messages: OrchestratorMessage[],
  pending: PendingUserMessage | null,
): boolean {
  if (!pending) return false
  const latestUser = getLatestUserMessage(messages)
  return latestUser?.content === pending.content
}

function normalizeThread(next: OrchestratorThread | null): OrchestratorThread | null {
  if (!next) return null
  return {
    ...next,
    messages: next.messages ?? [],
    runs: (next.runs ?? []).map((run) => ({
      ...run,
      events: run.events ?? [],
    })),
    playbook: {
      ...next.playbook,
      principles: next.playbook?.principles ?? [],
      ticket_sop: next.playbook?.ticket_sop ?? [],
      worker_lanes: next.playbook?.worker_lanes ?? [],
      starter_prompts: next.playbook?.starter_prompts ?? [],
    },
  }
}

function eventLabel(event: OrchestratorRunEvent): string {
  const payload = event.payload ?? {}
  if (typeof payload.message === 'string' && payload.message.trim().length > 0) {
    return payload.message.trim()
  }
  if (typeof payload.text === 'string' && payload.text.trim().length > 0) {
    return payload.text.trim()
  }
  return event.kind
}

function MessageBubble({
  message,
  pending = false,
}: {
  message: OrchestratorMessage
  pending?: boolean
}) {
  const isAssistant = message.role === 'assistant'

  return (
    <div
      className={cn(
        'flex flex-col gap-2 rounded-2xl border px-4 py-3 shadow-sm backdrop-blur-sm transition-opacity',
        isAssistant
          ? 'border-white/10 bg-white/[0.04] text-foreground'
          : 'border-teal-500/20 bg-teal-500/10 text-foreground',
        pending && 'opacity-80',
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Badge variant={isAssistant ? 'outline' : 'secondary'}>
            {isAssistant ? 'Planner' : pending ? 'Sending' : 'Input'}
          </Badge>
        </div>
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {pending ? 'just now' : elapsed(message.created_at)}
        </span>
      </div>

      {isAssistant ? (
        <PlanMarkdown
          markdown={message.content}
          className="max-w-none text-sm text-foreground [&_p]:my-1 [&_pre]:my-2"
        />
      ) : (
        <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {message.content}
        </p>
      )}
    </div>
  )
}

function LivePlannerPanel({
  run,
  sending,
  fallbackHint,
}: {
  run: OrchestratorRun | null
  sending: boolean
  fallbackHint: string
}) {
  const events = run?.events ?? []
  const recentEvents = events.slice(-5)
  const latestEvent = recentEvents[recentEvents.length - 1]
  const headline = latestEvent ? eventLabel(latestEvent) : fallbackHint
  const metadata = [run?.worker_name, run?.model, run?.runner]
    .filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
    .join(' · ')

  return (
    <div className="rounded-3xl border border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.05),rgba(255,255,255,0.025))] p-4 shadow-sm backdrop-blur-sm">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline" className="gap-1 border-primary/25 text-primary">
              <LoaderCircle className="size-3 animate-spin" />
              Generating
            </Badge>
            {metadata ? (
              <Badge variant="outline" className="border-white/10 text-muted-foreground">
                {metadata}
              </Badge>
            ) : null}
            {run?.started_at ? (
              <Badge variant="outline" className="border-white/10 text-muted-foreground">
                {formatDuration(run.started_at)}
              </Badge>
            ) : null}
          </div>

          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              {headline}
            </p>
            <p className="text-xs leading-relaxed text-muted-foreground">
              {sending && !run
                ? 'Your message is on the wire. The planner will appear here as soon as it acknowledges the run.'
                : 'Live planner activity updates stream in here while the orchestrator is working.'}
            </p>
          </div>
        </div>

        <div className="inline-flex items-center gap-2 text-[11px] uppercase tracking-[0.22em] text-muted-foreground">
          <Sparkles className="size-3.5" />
          Live
        </div>
      </div>

      <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-white/8">
        <div className="h-full w-2/5 rounded-full bg-primary/70 animate-pulse" />
      </div>

      {recentEvents.length > 1 ? (
        <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {recentEvents.slice(0, -1).map((event) => (
            <span key={event.id} className="flex items-center gap-1.5">
              <span className={cn(
                'size-1.5 rounded-full',
                event.kind === 'error' ? 'bg-destructive' : 'bg-primary/50',
              )} />
              {eventLabel(event)}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
}

export function OrchestratorConsole({
  projectId,
  onMessageComplete,
}: {
  projectId: string
  onMessageComplete?: () => void
}) {
  const { token } = useAuth()
  const [thread, setThread] = useState<OrchestratorThread | null>(null)
  const [draft, setDraft] = useState('')
  const [loading, setLoading] = useState(true)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pendingMessage, setPendingMessage] = useState<PendingUserMessage | null>(null)
  const [hintIndex, setHintIndex] = useState(0)
  const transcriptRef = useRef<HTMLDivElement | null>(null)
  const messages = thread?.messages ?? []
  const runs = thread?.runs ?? []
  const playbook = thread?.playbook
  const starterPrompts = playbook?.starter_prompts ?? []
  const activeRun = useMemo(
    () => runs.findLast((run) => run.status === 'running') ?? null,
    [runs],
  )
  const showLivePlanner = sending || activeRun !== null
  const pollInterval = showLivePlanner ? LIVE_POLL_INTERVAL : POLL_INTERVAL

  const applyThread = useCallback((next: OrchestratorThread | null) => {
    setThread(normalizeThread(next))
  }, [])

  const fetchThread = useCallback(async () => {
    if (!token) return
    const { data, error: requestError } = await getOrchestratorThread(token, projectId)
    if (requestError) {
      setError(requestError)
    } else {
      applyThread(data)
      setError(null)
    }
    setLoading(false)
  }, [applyThread, projectId, token])

  useEffect(() => {
    if (!token) return
    const immediate = window.setTimeout(() => void fetchThread(), 0)
    const interval = window.setInterval(() => void fetchThread(), pollInterval)
    return () => {
      window.clearTimeout(immediate)
      window.clearInterval(interval)
    }
  }, [fetchThread, pollInterval, token])

  useEffect(() => {
    if (!showLivePlanner) {
      setHintIndex(0)
      return
    }
    if (activeRun?.events?.length) return
    const interval = window.setInterval(() => {
      setHintIndex((current) => (current + 1) % GENERATION_HINTS.length)
    }, GENERATION_HINT_ROTATION_MS)
    return () => window.clearInterval(interval)
  }, [activeRun?.events?.length, showLivePlanner])

  const pendingAcknowledged = isPendingMessageAcknowledged(messages, pendingMessage)
  const displayMessages = useMemo(() => {
    if (!pendingMessage || pendingAcknowledged) return messages
    const optimisticMessage: OrchestratorMessage = {
      id: pendingMessage.id,
      project_id: projectId,
      role: 'user',
      content: pendingMessage.content,
      created_at: pendingMessage.createdAt,
    }
    return [...messages, optimisticMessage]
  }, [messages, pendingAcknowledged, pendingMessage, projectId])

  useEffect(() => {
    const node = transcriptRef.current
    if (!node) return
    node.scrollTo({ top: node.scrollHeight, behavior: 'smooth' })
  }, [displayMessages.length, sending, activeRun?.events.length])

  const submit = useCallback(async () => {
    const content = draft.trim()
    if (!content || !token || sending) return

    const optimisticMessage: PendingUserMessage = {
      id: `pending-${Date.now()}`,
      content,
      createdAt: new Date().toISOString(),
    }

    setPendingMessage(optimisticMessage)
    setDraft('')
    setSending(true)
    setError(null)
    void fetchThread()
    const followupFetch = window.setTimeout(() => void fetchThread(), 250)
    const { data, error: requestError } = await sendOrchestratorMessage(token, projectId, content)
    window.clearTimeout(followupFetch)
    setSending(false)

    if (requestError) {
      setError(requestError)
      setDraft(content)
      setPendingMessage(null)
      await fetchThread()
      return
    }

    applyThread(data)
    setPendingMessage(null)
    onMessageComplete?.()
  }, [applyThread, draft, fetchThread, onMessageComplete, projectId, sending, token])

  return (
    <Card className="flex h-[calc(100vh-12rem)] min-h-[480px] max-h-[900px] flex-col overflow-hidden border-white/12 bg-[radial-gradient(circle_at_top_left,rgba(20,184,166,0.12),transparent_32%),radial-gradient(circle_at_top_right,rgba(251,146,60,0.08),transparent_28%),linear-gradient(180deg,rgba(255,255,255,0.04),rgba(255,255,255,0.02))]">
      <CardHeader className="shrink-0 border-b border-white/10 pb-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline">Planner</Badge>
              <Badge variant="secondary">Scope and Tickets</Badge>
            </div>
            {playbook?.summary ? (
              <CardDescription className="max-w-2xl text-sm leading-relaxed">
                {playbook.summary}
              </CardDescription>
            ) : null}
          </div>
          <div className="text-[11px] uppercase tracking-[0.24em] text-muted-foreground">
            Cmd/Ctrl+Enter to send
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden py-5">
        <div
          ref={transcriptRef}
          className="flex min-h-[400px] flex-1 flex-col gap-3 overflow-y-auto pr-1"
        >
          {loading ? (
            <div className="grid gap-3">
              <div className="h-24 animate-pulse rounded-2xl bg-white/[0.05]" />
              <div className="h-20 animate-pulse rounded-2xl bg-white/[0.04]" />
              <div className="h-28 animate-pulse rounded-2xl bg-white/[0.05]" />
            </div>
          ) : displayMessages.length ? (
            displayMessages.map((message) => (
              <MessageBubble
                key={message.id}
                message={message}
                pending={pendingMessage?.id === message.id}
              />
            ))
          ) : (
            <div className="grid gap-4 rounded-3xl border border-dashed border-white/12 bg-black/10 p-5">
              <div className="space-y-2">
                <p className="text-sm font-medium text-foreground">
                  No messages yet.
                </p>
                <p className="max-w-2xl text-sm leading-relaxed text-muted-foreground">
                  Use the planner to refine scope, inspect the project, and create or update work streams and tickets.
                </p>
              </div>

              {starterPrompts.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {starterPrompts.map((prompt) => (
                    <Button
                      key={prompt}
                      type="button"
                      variant="outline"
                      size="sm"
                      className="h-auto whitespace-normal px-3 py-2 text-left leading-relaxed"
                      onClick={() => setDraft(prompt)}
                    >
                      {prompt}
                    </Button>
                  ))}
                </div>
              )}

              {playbook?.worker_lanes?.length ? (
                <div className="grid gap-3 md:grid-cols-3">
                  {playbook.worker_lanes.map((lane) => (
                    <div
                      key={lane.name}
                      className="rounded-2xl border border-white/10 bg-white/[0.03] p-3"
                    >
                      <p className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                        {lane.name}
                      </p>
                      <p className="mt-2 text-sm text-foreground">{lane.purpose}</p>
                      <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
                        {lane.default_style}
                      </p>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          )}
        </div>

        {showLivePlanner ? (
          <LivePlannerPanel
            run={activeRun}
            sending={sending}
            fallbackHint={GENERATION_HINTS[hintIndex]}
          />
        ) : null}

        <div className="shrink-0 space-y-3 border-t border-white/10 pt-4">
          {error ? (
            <div className="rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          ) : null}

          <div className="grid gap-3">
            <textarea
              ref={(el) => {
                if (!el) return
                el.style.height = 'auto'
                el.style.height = `${Math.min(el.scrollHeight, 200)}px`
              }}
              value={draft}
              onChange={(event) => {
                setDraft(event.target.value)
                const el = event.target
                el.style.height = 'auto'
                el.style.height = `${Math.min(el.scrollHeight, 200)}px`
              }}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
                  event.preventDefault()
                  void submit()
                }
              }}
              placeholder="Describe the task, scope, constraints, or ask what should be ticketed next."
              className="min-h-[40px] max-h-[200px] resize-none rounded-xl border border-white/10 bg-black/10 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary/40 focus:ring-2 focus:ring-primary/15"
              rows={1}
              disabled={sending}
              aria-busy={showLivePlanner}
            />
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="max-w-2xl text-xs leading-relaxed text-muted-foreground">
                The planner will surface live activity as it works, and completion means a real response, not a hung request.
              </p>
              <Button
                type="button"
                onClick={() => void submit()}
                disabled={sending || !draft.trim()}
                className="min-w-[148px] gap-2"
              >
                {sending ? (
                  <>
                    <LoaderCircle className="size-4 animate-spin" />
                    Generating
                  </>
                ) : (
                  <>
                    <TerminalSquare className="size-4" />
                    Send
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
