import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { TerminalSquare } from 'lucide-react'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { HARNESS_CLASS, HARNESS_LABEL, STATUS_DOT, relativeTime, type AgentSession } from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

/** Sessions linked to a ticket, by Linear identifier and by ticket id. */
export function TicketSessionsCard({ refs, base }: { refs: string[]; base: string }) {
  const { client } = useAuth()
  const [sessions, setSessions] = useState<AgentSession[] | null>(null)
  const key = refs.filter(Boolean).join('|')

  useEffect(() => {
    if (!key) return
    let cancelled = false
    Promise.all(
      key.split('|').map((ref) =>
        client.GET('/sessions', { params: { query: { ref, include_subagents: true, limit: 50 } } }).then(({ data }) => data?.sessions ?? []),
      ),
    ).then((lists) => {
      if (cancelled) return
      const seen = new Set<string>()
      const merged: AgentSession[] = []
      for (const s of lists.flat()) {
        if (!seen.has(s.id)) {
          seen.add(s.id)
          merged.push(s)
        }
      }
      merged.sort((a, b) => b.last_activity_at.localeCompare(a.last_activity_at))
      setSessions(merged)
    })
    return () => {
      cancelled = true
    }
  }, [client, key])

  if (!key || sessions === null || sessions.length === 0) return null
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="py-3">
        <CardTitle className="flex items-center gap-2 text-sm">
          <TerminalSquare className="size-4 text-muted-foreground" /> Sessions ({sessions.length})
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-1.5 pt-0">
        {sessions.slice(0, 12).map((s) => (
          <Link
            key={s.id}
            to={`${base}/sessions/${s.id}`}
            className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5 text-xs hover:bg-white/[0.06]"
          >
            <span className={cn('size-1.5 rounded-full', STATUS_DOT[s.status] ?? STATUS_DOT.ended)} />
            <span className={cn('rounded border px-1 text-[10px]', HARNESS_CLASS[s.harness])}>{HARNESS_LABEL[s.harness] ?? s.harness}</span>
            <span className="truncate">{s.title || s.first_prompt || s.external_id}</span>
            <span className="ml-auto shrink-0 text-muted-foreground">{relativeTime(s.last_activity_at)}</span>
          </Link>
        ))}
      </CardContent>
    </Card>
  )
}
