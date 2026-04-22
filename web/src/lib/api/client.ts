import createClient from 'openapi-fetch'

import type { paths } from '@/lib/api/v1'

export function createFlywheelClient(getToken: () => string | null) {
  const client = createClient<paths>({ baseUrl: '' })
  client.use({
    onRequest({ request }) {
      const t = getToken()
      if (t) request.headers.set('Authorization', `Bearer ${t}`)
    },
  })
  return client
}

export function formatApiError(data: unknown): string {
  if (data && typeof data === 'object' && 'error' in data) {
    const err = (data as { error?: string }).error
    if (typeof err === 'string') return err
  }
  return 'Request failed'
}
