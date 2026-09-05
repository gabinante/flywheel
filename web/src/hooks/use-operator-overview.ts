import { useQuery } from '@tanstack/react-query'

export type RunProgressData = {
  worker_state: 'waiting' | 'running' | 'exited' | 'preparing' | 'untracked'
  health?: string
  pid?: number
  process_started_at?: string
  deadline_at?: string
  last_output_at?: string
  last_activity_at?: string
  last_usage_at?: string
  output_events: number
  tool_calls: number
  assistant_messages: number
  reasoning_updates: number
  completed_turns: number
  tokens_in?: number
  tokens_out?: number
  recent?: { at: string; kind: string; summary: string }[]
}

export type WorkItem = {
  id: string
  kind: string
  title: string
  ref?: string
  href: string
  project_name?: string
  project_id?: string
  ticket_id?: string
  review_id?: string
  progress?: RunProgressData
  harness?: string
  worker?: string
  status: string
  reason?: string
  action?: string
  session_href?: string
  started_at: string
}
export type OperatorOverview = {
  in_flight: WorkItem[]
  attention: WorkItem[]
  updated_at: string
  dispatch: { enabled: boolean; active_workers: number; max_workers: number }
}

export function useOperatorOverview() {
  return useQuery<OperatorOverview>({
    queryKey: ['operator-overview'],
    queryFn: async ({ signal }) => {
      const response = await fetch('/api/overview', { signal })
      if (!response.ok) throw new Error('Could not refresh current work')
      return response.json()
    },
    staleTime: 2_000,
  })
}
