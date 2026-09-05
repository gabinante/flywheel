import { createContext, useContext } from 'react'
import type { ActivityTopic } from '@/lib/activity'

export type ActivityState = {
  status: 'connecting' | 'live' | 'reconnecting'
  all: number
  versions: Partial<Record<ActivityTopic, number>>
}
export const ActivityContext = createContext<ActivityState>({ status: 'connecting', all: 0, versions: {} })

export function useActivityStatus() { return useContext(ActivityContext).status }

/** Include this stable value in an existing data-loading effect's dependencies.
 * It changes for matching events, reconnect/focus, and the reconciliation fallback.
 * Never use it to reset an editor's local draft. React Query data is wired centrally.
 */
export function useActivityVersion(...topics: ActivityTopic[]) {
  const state = useContext(ActivityContext)
  return `${state.all}:${topics.map(topic => state.versions[topic] ?? 0).join(':')}`
}
