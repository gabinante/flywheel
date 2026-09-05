export const activityTopics = ['orchestrator', 'reviews', 'review-status', 'prs', 'sessions', 'collector', 'runs', 'tickets', 'projects', 'workflows', 'settings', 'dispatch'] as const
export type ActivityTopic = typeof activityTopics[number]
export type ActivityEvent = { topics: ActivityTopic[]; resync: boolean; ready: boolean }

// Only invalidation metadata belongs on this stream. Reject malformed events so
// the connection watchdog can recover instead of presenting a false live state.
export function parseActivity(data: string): ActivityEvent | null {
  try {
    const e = JSON.parse(data)
    if (!e || !Array.isArray(e.topics) || typeof e.resync !== 'boolean' || typeof e.ready !== 'boolean') return null
    return { topics: e.topics.filter((t: unknown) => activityTopics.includes(t as ActivityTopic)), resync: e.resync, ready: e.ready }
  } catch { return null }
}

const queryTopics: Record<string, ActivityTopic[]> = {
  'operator-overview': ['orchestrator', 'runs', 'reviews', 'review-status', 'tickets', 'projects', 'sessions', 'workflows', 'settings', 'dispatch'],
  'my-reviews': ['reviews', 'prs'],
  'code-review-status': ['reviews', 'review-status', 'settings'],
  'dispatch-status': ['dispatch', 'tickets', 'projects', 'settings'],
  'project-rail-data': ['dispatch', 'tickets', 'projects'],
  'project-escalations': ['tickets'],
}

export function queryAffected(key: readonly unknown[], topics: ReadonlySet<ActivityTopic>) {
  return (queryTopics[String(key[0])] ?? []).some(topic => topics.has(topic))
}
