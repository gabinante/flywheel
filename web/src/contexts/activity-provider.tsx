import { useEffect, useState, type ReactNode } from 'react'
import { useQueryClient, type Query } from '@tanstack/react-query'
import { ActivityContext, type ActivityState } from './use-activity'
import { parseActivity, queryAffected, type ActivityTopic } from '@/lib/activity'

/** One EventSource per app. Events invalidate REST snapshots; they never mutate
 * domain objects or drafts. A fresh snapshot on reconnect covers missed events.
 */
export function ActivityProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [state, setState] = useState<ActivityState>({ status: 'connecting', all: 0, versions: {} })

  useEffect(() => {
    let stopped = false
    let source: EventSource | undefined
    let live = false
    let lastMessage = Date.now()
    let lastReconcile = Date.now()
    const jobs = new Map<Query, { dirty: boolean }>()

    // Serialize per query, not across the app: a slow GitHub read must not hold
    // up the work tray. Every notice during a fetch schedules another fetch.
    const refreshQueries = (topics: ReadonlySet<ActivityTopic>, all: boolean) => {
      for (const query of queryClient.getQueryCache().findAll({ predicate: q => all || queryAffected(q.queryKey, topics) })) {
        const existing = jobs.get(query)
        if (existing) { existing.dirty = true; continue }
        const job = { dirty: true }
        jobs.set(query, job)
        void (async () => {
          try {
            while (!stopped && job.dirty) {
              job.dirty = false
              // A fetch already in flight may have read before this commit.
              if (query.state.fetchStatus === 'fetching') await query.promise?.catch(() => undefined)
              if (!stopped) await queryClient.invalidateQueries({ predicate: q => q === query }, { cancelRefetch: false })
            }
          } finally { jobs.delete(query) }
        })()
      }
    }

    const update = (topics: ActivityTopic[], all = false) => {
      if (stopped) return
      setState(previous => {
        const versions = { ...previous.versions }
        for (const topic of topics) versions[topic] = (versions[topic] ?? 0) + 1
        return { status: live ? 'live' : 'reconnecting', all: previous.all + (all ? 1 : 0), versions }
      })
      refreshQueries(new Set(topics), all)
    }
    const disconnected = () => {
      live = false
      setState(previous => ({ ...previous, status: 'reconnecting' }))
    }
    const connect = () => {
      source?.close()
      lastMessage = Date.now()
      source = new EventSource('/api/activity/events')
      source.addEventListener('activity', event => {
        const e = parseActivity((event as MessageEvent<string>).data)
        if (!e) return
        lastMessage = Date.now()
        const recovered = !live && e.ready
        live = e.ready
        update(e.topics, e.resync || recovered)
      })
      source.addEventListener('heartbeat', () => { lastMessage = Date.now() })
      source.onerror = disconnected // native EventSource retries with server delay
    }
    connect()

    const reconcile = () => {
      if (document.visibilityState === 'hidden') return
      if (!live) connect()
      lastReconcile = Date.now()
      update([], true)
    }
    const visible = () => { if (document.visibilityState === 'visible') reconcile() }
    const offline = () => { source?.close(); disconnected() }
    window.addEventListener('offline', offline)
    window.addEventListener('online', reconcile)
    window.addEventListener('focus', reconcile)
    document.addEventListener('visibilitychange', visible)
    const timer = window.setInterval(() => {
      const now = Date.now()
      // Also recover from a half-open connection that never emits an error.
      if (now - lastMessage > 45_000) { disconnected(); connect() }
      // Slow reconciliation covers time-derived health, upstream GitHub caches,
      // and failed REST fetches. Disconnected clients get a faster fallback.
      if (document.visibilityState !== 'hidden' && now - lastReconcile >= (live ? 60_000 : 15_000)) {
        lastReconcile = now
        update([], true)
      }
    }, 5_000)
    return () => {
      stopped = true
      source?.close()
      window.clearInterval(timer)
      window.removeEventListener('offline', offline)
      window.removeEventListener('online', reconcile)
      window.removeEventListener('focus', reconcile)
      document.removeEventListener('visibilitychange', visible)
    }
  }, [queryClient])

  return <ActivityContext.Provider value={state}>{children}</ActivityContext.Provider>
}
