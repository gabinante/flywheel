import { useCallback, useEffect, useRef, useState } from 'react'

import { PlanMarkdown } from '@/components/plan-markdown'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import {
  getOrchestratorThread,
  sendOrchestratorMessage,
  type OrchestratorMessage,
  type OrchestratorThread,
} from '@/lib/api/orchestrator-client'
import { cn } from '@/lib/utils'

const POLL_INTERVAL = 15_000

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

function MessageBubble({ message }: { message: OrchestratorMessage }) {
  const isAssistant = message.role === 'assistant'

  return (
    <div
      className={cn(
        'flex flex-col gap-2 rounded-2xl border px-4 py-3 shadow-sm backdrop-blur-sm',
        isAssistant
          ? 'border-white/10 bg-white/[0.04] text-foreground'
          : 'border-teal-500/20 bg-teal-500/10 text-foreground',
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Badge variant={isAssistant ? 'outline' : 'secondary'}>
            {isAssistant ? 'Planner' : 'Input'}
          </Badge>
        </div>
        <span className="text-[11px] tabular-nums text-muted-foreground">
          {elapsed(message.created_at)}
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
  const transcriptRef = useRef<HTMLDivElement | null>(null)
  const hasLoaded = useRef(false)
  const messages = thread?.messages ?? []
  const playbook = thread?.playbook
  const starterPrompts = playbook?.starter_prompts ?? []

  const applyThread = useCallback((next: OrchestratorThread | null) => {
    if (!next) {
      setThread(null)
      return
    }
    setThread({
      ...next,
      messages: next.messages ?? [],
      playbook: {
        ...next.playbook,
        principles: next.playbook?.principles ?? [],
        ticket_sop: next.playbook?.ticket_sop ?? [],
        worker_lanes: next.playbook?.worker_lanes ?? [],
        starter_prompts: next.playbook?.starter_prompts ?? [],
      },
    })
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
    hasLoaded.current = true
    setLoading(false)
  }, [applyThread, projectId, token])

  useEffect(() => {
    if (!token) return
    const immediate = window.setTimeout(() => void fetchThread(), 0)
    const interval = window.setInterval(() => void fetchThread(), POLL_INTERVAL)
    return () => {
      window.clearTimeout(immediate)
      window.clearInterval(interval)
    }
  }, [fetchThread, token])

  useEffect(() => {
    const node = transcriptRef.current
    if (!node) return
    node.scrollTo({ top: node.scrollHeight, behavior: 'smooth' })
  }, [messages.length, sending])

  const submit = useCallback(async () => {
    const content = draft.trim()
    if (!content || !token || sending) return

    setSending(true)
    setError(null)
    const { data, error: requestError } = await sendOrchestratorMessage(token, projectId, content)
    setSending(false)

    if (requestError) {
      setError(requestError)
      await fetchThread()
      return
    }

    applyThread(data)
    setDraft('')
    onMessageComplete?.()
  }, [applyThread, draft, fetchThread, onMessageComplete, projectId, sending, token])

  return (
    <Card className="min-h-[720px] border-white/12 bg-[radial-gradient(circle_at_top_left,rgba(20,184,166,0.12),transparent_32%),radial-gradient(circle_at_top_right,rgba(251,146,60,0.08),transparent_28%),linear-gradient(180deg,rgba(255,255,255,0.04),rgba(255,255,255,0.02))]">
      <CardHeader className="border-b border-white/10 pb-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline">Planner</Badge>
              <Badge variant="secondary">Scope and Tickets</Badge>
            </div>
            <CardTitle className="text-base tracking-tight">
              Orchestrator
            </CardTitle>
            <CardDescription className="max-w-2xl text-sm leading-relaxed">
              {playbook?.summary ??
                'Use this pane to clarify scope, inspect project context, and create or update work streams and tickets.'}
            </CardDescription>
          </div>
          <div className="text-[11px] uppercase tracking-[0.24em] text-muted-foreground">
            Cmd/Ctrl+Enter to send
          </div>
        </div>
      </CardHeader>

      <CardContent className="flex h-full flex-1 flex-col gap-4 py-5">
        <div
          ref={transcriptRef}
          className="flex min-h-[430px] flex-1 flex-col gap-3 overflow-y-auto pr-1"
        >
          {loading ? (
            <div className="grid gap-3">
              <div className="h-24 animate-pulse rounded-2xl bg-white/[0.05]" />
              <div className="h-20 animate-pulse rounded-2xl bg-white/[0.04]" />
              <div className="h-28 animate-pulse rounded-2xl bg-white/[0.05]" />
            </div>
          ) : messages.length ? (
            messages.map((message) => (
              <MessageBubble key={message.id} message={message} />
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

        <div className="space-y-3 border-t border-white/10 pt-4">
          {error ? (
            <div className="rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          ) : null}

          <div className="grid gap-3">
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
                  event.preventDefault()
                  void submit()
                }
              }}
              placeholder="Describe the task, scope, constraints, or ask what should be ticketed next."
              className="min-h-[120px] resize-y rounded-xl border border-white/10 bg-black/10 px-3 py-2 text-sm text-foreground outline-none transition focus:border-primary/40 focus:ring-2 focus:ring-primary/15"
              disabled={sending}
            />
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="max-w-2xl text-xs leading-relaxed text-muted-foreground">
                Keep requests at the planning layer: scope review, ticket creation, queue triage, and re-planning. Workers handle implementation.
              </p>
              <Button
                type="button"
                onClick={() => void submit()}
                disabled={sending || !draft.trim()}
                className="min-w-[140px]"
              >
                {sending ? 'Running...' : 'Send'}
              </Button>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
