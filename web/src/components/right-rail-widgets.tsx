import { useActivityStatus } from '@/contexts/use-activity'
import { Link } from 'react-router-dom'
import { AlertCircle, ArrowUpRight, CircleCheck, LoaderCircle, RefreshCw, TerminalSquare } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { RunProgress } from '@/components/run-progress'
import { useOperatorOverview, type WorkItem } from '@/hooks/use-operator-overview'
import { relativeTime } from '@/lib/sessions-format'

const labels: Record<string, string> = {
  dispatch: 'Dispatch', executor: 'Implementation', planner: 'Planning', operator: 'Operator',
  validator: 'Validation', investigator: 'Investigation', orchestrator: 'Project planning',
  code_review: 'Code review', feedback: 'Addressing feedback', conversation: 'Conversation', harness: 'Harness', external: 'External job',
}
const statusLabels: Record<string, string> = {
  starting: 'Starting', running: 'Running', fetching: 'Preparing review', reviewing: 'Reviewing', publishing: 'Publishing review',
}
function WorkCard({ item, attention = false }: { item: WorkItem; attention?: boolean }) {
  return (
    <li className="min-w-0 rounded-xl border border-white/10 bg-white/[0.025] p-3" data-work-id={item.id}>
      <div className="mb-1.5 flex items-center justify-between gap-2 text-[10px] text-muted-foreground">
        <span className="truncate">{item.project_name || 'Across projects'}</span>
        {!attention && <span className="shrink-0">{relativeTime(item.started_at)}</span>}
      </div>
      <Link to={item.href} className="group block rounded-sm text-sm leading-snug hover:text-primary focus-visible:outline-2 focus-visible:outline-primary">
        {item.ref && <span className="mb-1 block truncate font-mono text-[10px] text-primary/80" title={item.ref}>{item.ref}</span>}
        <span className="line-clamp-2 font-medium">{item.title}</span>
      </Link>
      {attention ? (
        <>
          <p className="mt-2 line-clamp-3 text-xs leading-relaxed text-muted-foreground" title={item.reason}>{item.reason}</p>
          <Link to={item.href} className="mt-2 inline-flex items-center gap-1 rounded text-xs font-medium text-amber-300 hover:text-amber-200">
            {item.action} <ArrowUpRight className="size-3" />
          </Link>
        </>
      ) : (
        <div className="mt-2 space-y-1 text-[11px] text-muted-foreground">
          <p>{labels[item.kind] || item.kind} · {statusLabels[item.status] || item.status}</p>
          {(item.harness || item.worker) && <p className="truncate" title={item.worker}>
            {item.harness === 'codex' ? 'Codex' : item.harness?.startsWith('claude') ? 'Claude Code' : item.harness}
            {item.worker && ` · ${item.worker}`}
          </p>}
        </div>
      )}
      {item.session_href ? (
        <Link to={item.session_href} className="mt-2 inline-flex items-center gap-1.5 rounded text-xs text-primary hover:underline">
          <TerminalSquare className="size-3.5" /> Open session
        </Link>
      ) : !attention && item.harness ? (
        <p className="mt-2 text-[10px] text-muted-foreground/70">Session not available yet.</p>
      ) : null}
      {item.progress && <div className="mt-2 border-t border-white/10 pt-2"><RunProgress progress={item.progress} compact /></div>}
    </li>
  )
}

export function RightRailWidgets() {
  const activityStatus = useActivityStatus()
  const { data, isPending, isError, refetch, isFetching } = useOperatorOverview()
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-sm font-semibold">Your work</h2>
          <p className="mt-0.5 text-[11px] text-muted-foreground">Across all projects</p>
          <p role="status" aria-label="Activity connection" className={`mt-1 text-[10px] ${activityStatus === 'live' ? 'text-primary' : 'text-amber-300'}`}>
            {activityStatus === 'live' ? 'Live updates connected' : activityStatus === 'connecting' ? 'Connecting live updates…' : 'Reconnecting · checking for updates every 15s'}
          </p>
        </div>
        <Button variant="ghost" size="icon-sm" aria-label="Refresh current work" disabled={isFetching} onClick={() => void refetch()}>
          <RefreshCw className={`size-3.5 ${isFetching ? 'animate-spin' : ''}`} />
        </Button>
      </div>
      {isError && <p role="alert" className="rounded-lg border border-amber-400/20 bg-amber-400/5 p-2 text-xs text-amber-200">
        Could not refresh current work.{data ? ' Showing the last successful update.' : ' Try refreshing.'}
      </p>}
      {isPending ? <p role="status" className="text-xs text-muted-foreground">Loading current work…</p> : data && <>
        <section aria-labelledby="in-flight-heading" className="space-y-3">
          <h3 id="in-flight-heading" className="flex items-center gap-2 text-xs font-semibold">
            <LoaderCircle className={`size-3.5 text-primary ${data.in_flight.length ? 'animate-spin' : ''}`} />
            In flight <span className="ml-auto tabular-nums text-muted-foreground">{data.in_flight.length}</span>
          </h3>
          <Link to="/settings?section=dispatch" className="block rounded-lg bg-white/[0.035] px-2.5 py-2 text-[11px] text-muted-foreground hover:bg-white/[0.06]">
            <span className={data.dispatch.enabled ? 'text-primary' : 'text-amber-300'}>Dispatch {data.dispatch.enabled ? 'on' : 'off'}</span>
            <span className="float-right tabular-nums">{data.dispatch.active_workers}/{data.dispatch.max_workers} workers</span>
          </Link>
          {data.in_flight.length ? <ul className="space-y-2">{data.in_flight.map(item => <WorkCard key={item.id} item={item} />)}</ul> :
            <p className="py-1 text-xs text-muted-foreground">No work is running.</p>}
        </section>
        <section aria-labelledby="attention-heading" className="space-y-3 border-t border-white/10 pt-4">
          <h3 id="attention-heading" className="flex items-center gap-2 text-xs font-semibold">
            <AlertCircle className="size-3.5 text-amber-300" />
            Needs attention <span className="ml-auto tabular-nums text-muted-foreground">{data.attention.length}</span>
          </h3>
          {data.attention.length ? <ul className="space-y-2">{data.attention.map(item => <WorkCard key={item.id} item={item} attention />)}</ul> :
            <p className="flex items-center gap-1.5 py-1 text-xs text-muted-foreground"><CircleCheck className="size-3.5" /> Nothing needs your input.</p>}
        </section>
      </>}
    </div>
  )
}
