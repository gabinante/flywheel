import type { FlywheelClient } from '@/contexts/auth-context'
import type { components } from '@/lib/api/v1'

export type OperatorSettings = components['schemas']['OperatorSettings']
export type UpdateOperatorSettingsRequest = components['schemas']['UpdateOperatorSettingsRequest']

/** Build a full PUT body from the current settings (the API replaces the whole document). */
export function toUpdateRequest(s: OperatorSettings): UpdateOperatorSettingsRequest {
  return {
    linear: {
      enabled: s.linear.enabled,
      project_ids: s.linear.project_ids,
      default_team_key: s.linear.default_team_key,
      sync_interval_seconds: s.linear.sync_interval_seconds,
    },
    review: s.review,
    feedback: s.feedback,
    report: s.report,
  }
}

/** Read → mutate → write operator settings. Returns the saved settings or an error message. */
export async function patchSettings(
  client: FlywheelClient,
  mutate: (s: OperatorSettings) => OperatorSettings,
): Promise<{ data?: OperatorSettings; error?: string }> {
  const cur = await client.GET('/settings')
  if (!cur.response.ok || !cur.data) return { error: 'could not load settings' }
  const next = mutate(cur.data)
  const res = await client.PUT('/settings', { body: toUpdateRequest(next) })
  if (!res.response.ok || !res.data) {
    const err = res.error as { error?: string } | undefined
    return { error: err?.error ?? `save failed (${res.response.status})` }
  }
  return { data: res.data }
}
