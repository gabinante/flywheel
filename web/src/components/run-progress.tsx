import { useEffect, useState } from 'react'
import type { RunProgressData } from '@/hooks/use-operator-overview'
import { compact, duration, relativeTime } from '@/lib/sessions-format'
import { cn } from '@/lib/utils'

const count = (n: number | undefined, label: string) => `${n || 0} ${label}${n === 1 ? '' : 's'}`

export function RunProgress({ progress: p, compact: small = false }: { progress: RunProgressData; compact?: boolean }) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 5_000)
    return () => clearInterval(timer)
  }, [])
  const quiet = p.health === 'quiet' || (p.worker_state === 'running' && now - Date.parse(p.last_output_at || p.process_started_at || '') >= 120_000)
  const label = p.worker_state === 'untracked' ? 'No live worker'
    : p.worker_state === 'waiting' ? 'Waiting for capacity'
      : p.worker_state === 'preparing' ? 'Review service active'
        : p.worker_state === 'exited' ? 'Run finished'
          : quiet ? 'Quiet · check progress' : 'Worker connected'
  const latest = p.recent?.at(-1)
  const counts = `${count(p.tool_calls, 'tool')} · ${count(p.assistant_messages, 'assistant update')} · ${count(p.reasoning_updates, 'reasoning update')}`
  const reported = p.tokens_in !== undefined || p.tokens_out !== undefined
  const tokens = reported ? `${compact(p.tokens_in)} in / ${compact(p.tokens_out)} out tokens` : 'Tokens not reported yet'
  return (
    <section aria-label="Run progress" className={cn('space-y-2 text-xs', !small && 'rounded-xl border border-white/10 bg-white/[0.025] p-4')}>
      {!small && <h2 className="text-sm font-semibold">Run progress</h2>}
      <div className={cn('font-medium', quiet || p.worker_state === 'untracked' ? 'text-amber-300' : 'text-muted-foreground')}>
        {label}{!small && p.pid ? <span className="ml-2 font-mono text-[10px] text-muted-foreground">PID {p.pid}</span> : null}
      </div>
      {p.worker_state === 'untracked' ? <p className="text-muted-foreground">The saved review state has no worker attached to this server.</p> : <>
        {p.worker_state !== 'preparing' && <p className="text-muted-foreground">
          {p.last_output_at ? `Last output ${relativeTime(p.last_output_at, now)}` : 'No harness output yet'}
          {!small && p.process_started_at && p.worker_state === 'running' && ` · Runtime ${duration(p.process_started_at, new Date(now).toISOString())}`}
        </p>}
        {latest && <p className="text-foreground/80">{latest.summary}</p>}
        <p className="text-[10px] leading-relaxed text-muted-foreground">{counts}</p>
        <p className="font-mono text-[10px] text-muted-foreground">{tokens}</p>
        {!small && <>
          {p.last_activity_at && <p className="text-muted-foreground">Last work event {relativeTime(p.last_activity_at, now)}</p>}
          {p.last_usage_at && <p className="text-muted-foreground">Usage reported {relativeTime(p.last_usage_at, now)}</p>}
          {p.deadline_at && p.worker_state === 'running' && <p className="text-muted-foreground">Time limit {new Date(p.deadline_at).toLocaleTimeString()}</p>}
          <p className="text-[11px] leading-relaxed text-muted-foreground">Token usage arrives when the harness reports it, often at turn completion. A quiet worker may still be computing or waiting on a tool.</p>
          {!!p.recent?.length && <ol className="max-h-48 space-y-1.5 overflow-y-auto border-t border-white/10 pt-3">
            {[...p.recent].reverse().map((event, i) => <li key={`${event.at}-${i}`} className="flex justify-between gap-3">
              <span>{event.summary}</span><time dateTime={event.at} className="shrink-0 text-muted-foreground">{relativeTime(event.at, now)}</time>
            </li>)}
          </ol>}
        </>}
      </>}
    </section>
  )
}
