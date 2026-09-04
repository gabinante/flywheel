import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { CalendarClock, Play, RefreshCw, Settings as SettingsIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

type ScheduledAction = components['schemas']['ScheduledAction']

const KIND_LABEL: Record<ScheduledAction['kind'], string> = {
  pending: 'Pending now',
  scheduled: 'Scheduled',
  recurring: 'Recurring watchers',
}

function interval(seconds?: number) {
  if (!seconds) return ''
  if (seconds % 3600 === 0) return `every ${seconds / 3600}h`
  if (seconds % 60 === 0) return `every ${seconds / 60}m`
  return `every ${seconds}s`
}

function untilLabel(iso: string | undefined, now: number) {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (t <= now) return 'due now'
  const mins = Math.round((t - now) / 60000)
  if (mins < 60) return `in ${mins}m`
  const hours = Math.round(mins / 60)
  if (hours < 48) return `in ${hours}h`
  return `in ${Math.round(hours / 24)}d`
}

export function SchedulePage() {
  const { client } = useAuth()
  const [items, setItems] = useState<ScheduledAction[] | null>(null)
  const [now, setNow] = useState(0)
  const [err, setErr] = useState<string | null>(null)
  const [running, setRunning] = useState<string | null>(null)

  const load = useCallback(async () => {
    const { data, error, response } = await client.GET('/schedule')
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setItems(data.items)
    setNow(new Date(data.now).getTime())
  }, [client])

  useEffect(() => {
    void Promise.resolve().then(() => load())
    const t = window.setInterval(() => void load(), 30_000)
    return () => window.clearInterval(t)
  }, [load])

  const run = async (a: ScheduledAction) => {
    if (a.outward && !window.confirm(`Run “${a.name}” now? This posts to ${a.settings_section === 'reports' ? 'Linear' : 'GitHub or Linear'}.`)) return
    setRunning(a.id)
    const { error, response } = await client.POST('/schedule/{actionID}/run', { params: { path: { actionID: a.id } } })
    setRunning(null)
    if (!response.ok) {
      setErr(formatApiError(error))
    }
    void load()
  }

  const groups = (['pending', 'scheduled', 'recurring'] as ScheduledAction['kind'][]).map((k) => ({ kind: k, items: (items ?? []).filter((i) => i.kind === k) }))

  return (
    <div className="w-full space-y-6 p-2 xl:px-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <CalendarClock className="size-5 text-muted-foreground" />
            Scheduled actions
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Everything Flywheel does on its own — watchers, syncs, and report posts — with when it last ran and when it runs next.
            Anything can be run now; actions that post to Linear or GitHub ask first.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => load()}>
          <RefreshCw className="mr-1.5 size-3.5" />
          Refresh
        </Button>
      </div>

      {err && <p className="text-sm text-destructive">{err}</p>}
      {!items && !err && <p className="text-sm text-muted-foreground">Loading…</p>}

      {groups.map(
        (g) =>
          g.items.length > 0 && (
            <section key={g.kind} className="space-y-2">
              <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">{KIND_LABEL[g.kind]}</h2>
              {g.items.map((a) => (
                <div
                  key={a.id}
                  className={`flex flex-col gap-2 rounded-xl border px-4 py-3 sm:flex-row sm:items-center sm:justify-between ${
                    a.enabled ? 'border-white/5 bg-white/[0.02]' : 'border-white/5 bg-transparent opacity-70'
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{a.name}</span>
                      <Badge variant="outline" className={`text-[11px] font-normal ${a.enabled ? 'border-emerald-400/30 text-emerald-300' : 'text-muted-foreground'}`}>
                        {a.enabled ? 'on' : 'off'}
                      </Badge>
                      {a.interval_seconds ? <span className="text-xs text-muted-foreground">{interval(a.interval_seconds)}</span> : null}
                      {a.outward && <Badge variant="outline" className="border-amber-400/30 text-[11px] font-normal text-amber-300">posts externally</Badge>}
                      {a.count ? <Badge variant="outline" className="text-[11px] font-normal">{a.count}</Badge> : null}
                    </div>
                    <p className="mt-0.5 text-xs text-muted-foreground">{a.description}</p>
                    <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      {a.detail && <span>{a.detail}</span>}
                      {a.last_run_at && <span>last {relativeTime(a.last_run_at)}</span>}
                      {a.next_run_at && a.enabled && <span>next {untilLabel(a.next_run_at, now)}</span>}
                      {a.last_error && <span className="text-destructive">{a.last_error}</span>}
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {a.settings_section && (
                      <Button asChild variant="ghost" size="sm" title="Open settings">
                        <Link to={`/settings?section=${a.settings_section}`}>
                          <SettingsIcon className="size-3.5" />
                        </Link>
                      </Button>
                    )}
                    {a.runnable && (
                      <Button size="sm" variant="outline" disabled={running === a.id} onClick={() => run(a)}>
                        <Play className="mr-1.5 size-3.5" />
                        {running === a.id ? 'Running…' : 'Run now'}
                      </Button>
                    )}
                  </div>
                </div>
              ))}
            </section>
          ),
      )}
    </div>
  )
}
