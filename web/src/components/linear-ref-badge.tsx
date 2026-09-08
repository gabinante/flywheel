import { ExternalLink } from 'lucide-react'

import type { components } from '@/lib/api/v1'
import { cn } from '@/lib/utils'

type TicketExternalRef = components['schemas']['TicketExternalRef']

const STATE_TYPE_CLASS: Record<string, string> = {
  triage: 'border-slate-500/30 bg-slate-500/10 text-slate-300',
  backlog: 'border-slate-500/30 bg-slate-500/10 text-slate-300',
  unstarted: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  started: 'border-amber-500/30 bg-amber-500/10 text-amber-200',
  completed: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200',
  canceled: 'border-red-500/30 bg-red-500/10 text-red-200',
}

/** Compact "ENG-465 · In Review" chip linking to the Linear issue. */
export function LinearRefBadge({ external, showState = true, className }: { external?: TicketExternalRef; showState?: boolean; className?: string }) {
  const ref = external
  if (!ref) return null
  const inner = (
    <>
      <span className="font-mono text-[11px] font-medium">{ref.identifier}</span>
      {showState && ref.state_name && <span className="text-[10px] opacity-80">· {ref.state_name}</span>}
      {ref.url && <ExternalLink className="size-3 opacity-60" />}
    </>
  )
  const cls = cn(
    'inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5',
    STATE_TYPE_CLASS[ref.state_type ?? ''] ?? 'border-white/10 bg-white/5 text-muted-foreground',
    className,
  )
  return ref.url ? (
    <a href={ref.url} target="_blank" rel="noreferrer" className={cn(cls, 'hover:opacity-90')} onClick={(e) => e.stopPropagation()} title="Open in Linear">
      {inner}
    </a>
  ) : (
    <span className={cls}>{inner}</span>
  )
}
