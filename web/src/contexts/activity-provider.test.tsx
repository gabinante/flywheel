import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { ActivityProvider } from './activity-provider'
import { useActivityStatus, useActivityVersion } from './use-activity'
import { parseActivity, queryAffected } from '@/lib/activity'

class FakeSource extends EventTarget {
  static instances: FakeSource[] = []
  onerror?: () => void
  closed = false
  url: string
  constructor(url: string) { super(); this.url = url; FakeSource.instances.push(this) }
  close() { this.closed = true }
  activity(topics: string[] = [], resync = true, ready = true) {
    this.dispatchEvent(new MessageEvent('activity', { data: JSON.stringify({ topics, resync, ready }) }))
  }
}

function Probe() {
  const status = useActivityStatus()
  const version = useActivityVersion('sessions')
  return <output>{status} {version}</output>
}

beforeEach(() => { vi.useFakeTimers(); FakeSource.instances = []; vi.stubGlobal('EventSource', FakeSource) })
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals() })

test('one connection handles invalidations, offline fallback, recovery, and cleanup', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['operator-overview'], {})
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const view = render(<QueryClientProvider client={client}><ActivityProvider><Probe /></ActivityProvider></QueryClientProvider>)
  const source = FakeSource.instances[0]
  expect(source.url).toBe('/api/activity/events')
  await act(async () => { source.activity() })
  expect(screen.getByRole('status')).toHaveTextContent('live 1:0')
  await act(async () => { source.activity(['sessions'], false) })
  expect(screen.getByRole('status')).toHaveTextContent('live 1:1')
  await act(async () => { source.onerror?.() })
  expect(screen.getByRole('status')).toHaveTextContent('reconnecting')
  invalidate.mockClear()
  await act(async () => { await vi.advanceTimersByTimeAsync(15_000) })
  expect(invalidate).toHaveBeenCalledTimes(1)
  await act(async () => { source.activity() })
  expect(screen.getByRole('status')).toHaveTextContent('live')
  expect(FakeSource.instances).toHaveLength(1)
  view.unmount()
  expect(source.closed).toBe(true)
  invalidate.mockClear()
  await act(async () => { await vi.advanceTimersByTimeAsync(60_000) })
  expect(invalidate).not.toHaveBeenCalled()
})

test('a silent connection is replaced and normal heartbeats keep it alive', async () => {
  const client = new QueryClient()
  render(<QueryClientProvider client={client}><ActivityProvider><Probe /></ActivityProvider></QueryClientProvider>)
  const source = FakeSource.instances[0]
  await act(async () => { source.activity() })
  for (let i = 0; i < 4; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000)
      source.dispatchEvent(new MessageEvent('heartbeat', { data: '{}' }))
    })
  }
  expect(FakeSource.instances).toHaveLength(1)
  await act(async () => { await vi.advanceTimersByTimeAsync(50_000) })
  expect(FakeSource.instances).toHaveLength(2)
  expect(source.closed).toBe(true)
  expect(screen.getByRole('status')).toHaveTextContent('reconnecting')
})

test('topic mapping isolates progress from expensive review reads and rejects malformed notices', () => {
  expect(queryAffected(['operator-overview'], new Set(['runs']))).toBe(true)
  expect(queryAffected(['my-reviews'], new Set(['runs']))).toBe(false)
  expect(queryAffected(['my-reviews'], new Set(['reviews']))).toBe(true)
  expect(parseActivity('not json')).toBeNull()
  expect(parseActivity('{"topics":"sessions","ready":true}')).toBeNull()
  expect(parseActivity('{"topics":["sessions","unknown"],"ready":true,"resync":false}')?.topics).toEqual(['sessions'])
})

test('a slow review read does not delay worker updates to the tray', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let finishReviews!: () => void
  const reviews = new Promise<void>(resolve => { finishReviews = resolve })
  let current = 0
  function Queries() {
    useQuery({ queryKey: ['my-reviews'], queryFn: () => reviews })
    const { data } = useQuery({ queryKey: ['operator-overview'], queryFn: async () => current })
    return <span data-testid="work">{data}</span>
  }
  render(<QueryClientProvider client={client}><ActivityProvider><Queries /></ActivityProvider></QueryClientProvider>)
  await act(async () => { FakeSource.instances[0].activity(); await vi.advanceTimersByTimeAsync(1) })
  expect(screen.getByTestId('work')).toHaveTextContent('0')
  current = 1
  await act(async () => { FakeSource.instances[0].activity(['runs'], false); await vi.advanceTimersByTimeAsync(1) })
  expect(screen.getByTestId('work')).toHaveTextContent('1')
  expect(client.isFetching({ queryKey: ['my-reviews'] })).toBe(1)
  await act(async () => { finishReviews(); await vi.advanceTimersByTimeAsync(1) })
})
