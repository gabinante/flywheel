import type { components } from '@/lib/api/v1'

type Ticket = components['schemas']['Ticket']
type TraceStep = components['schemas']['TraceStep']

export type ChangeStreamEvent = {
  id?: string
  change_type?: string
  source?: string
  affected_entities?: string[]
  before_state?: Record<string, unknown>
  after_state?: Record<string, unknown>
  metadata?: Record<string, unknown>
  initiator_id?: string
  initiator_type?: string
  environment?: string
  timestamp?: string
  created_at?: string
}

export type TraceActivityStep = TraceStep & { ticketId: string }

export type ActivityItem = {
  id: string
  kind: 'ticket' | 'review' | 'plan' | 'deploy' | 'git' | 'pr' | 'work_stream' | 'system'
  title: string
  detail?: string
  ticketId: string | null
  timestamp?: string
  source: 'change_stream' | 'trace'
}

export function ticketIdFromChangeEvent(event: ChangeStreamEvent): string | null {
  const ticketID = readString(event.metadata, 'ticket_id')
  if (ticketID) return ticketID
  for (const entity of event.affected_entities ?? []) {
    if (entity) return entity
  }
  return null
}

export function buildActivityItems({
  changeEvents,
  traceSteps,
  ticketsById,
}: {
  changeEvents: ChangeStreamEvent[]
  traceSteps: TraceActivityStep[]
  ticketsById: Map<string, Ticket>
}): ActivityItem[] {
  const items = [
    ...changeEvents
      .map((event) => summarizeChangeEvent(event, ticketsById))
      .filter((item): item is ActivityItem => item !== null),
    ...traceSteps
      .map((step) => summarizeTraceStep(step, ticketsById))
      .filter((item): item is ActivityItem => item !== null),
  ]

  items.sort((a, b) => timestampValue(b.timestamp) - timestampValue(a.timestamp))
  return dedupeActivityItems(items).slice(0, 25)
}

function summarizeChangeEvent(
  event: ChangeStreamEvent,
  ticketsById: Map<string, Ticket>,
): ActivityItem | null {
  const type = event.change_type ?? ''
  const ticketId = ticketIdFromChangeEvent(event)
  const ticket = ticketId ? ticketsById.get(ticketId) : undefined
  const ticketLabel = ticketTitle(ticket, event)
  const actor = shortActor(
    readString(event.metadata, 'actor_id') ??
      readString(event.metadata, 'agent_id') ??
      readString(event.metadata, 'reviewer_id') ??
      event.initiator_id,
  )
  const backend = readString(event.metadata, 'backend')
  const changedFields = readStringArray(event.metadata, 'changed_fields').map(humanizeField)
  const ticketCount = readNumber(event.metadata, 'ticket_count')

  const activity: Omit<ActivityItem, 'id' | 'timestamp' | 'ticketId' | 'source'> | null = (() => {
    switch (type) {
      case 'ticket_created':
        return {
          kind: 'ticket',
          title: `Created ${ticketLabel}`,
          detail: actor ? `by ${actor}` : undefined,
        }
      case 'ticket_updated':
        return {
          kind: 'ticket',
          title: `Updated ${ticketLabel}`,
          detail: changedFields.length > 0 ? `Changed ${changedFields.join(', ')}` : undefined,
        }
      case 'ticket_planning':
        return { kind: 'ticket', title: `Planned ${ticketLabel}` }
      case 'ticket_started':
        return {
          kind: 'ticket',
          title: `Started work on ${ticketLabel}`,
          detail: actor ? `by ${actor}` : undefined,
        }
      case 'ticket_submitted':
        return { kind: 'review', title: `Submitted ${ticketLabel} for review` }
      case 'ticket_validated':
      case 'ticket_approved':
        return { kind: 'review', title: `Approved ${ticketLabel}` }
      case 'ticket_rejected':
        return { kind: 'review', title: `Rejected ${ticketLabel}` }
      case 'ticket_closed':
        return { kind: 'ticket', title: `Closed ${ticketLabel}` }
      case 'ticket_cancelled':
        return { kind: 'ticket', title: `Cancelled ${ticketLabel}` }
      case 'ticket_reopened':
        return { kind: 'ticket', title: `Reopened ${ticketLabel}` }
      case 'ticket_awaiting_input':
        return { kind: 'ticket', title: `${ticketLabel} is awaiting input` }
      case 'ticket_input_provided':
        return { kind: 'ticket', title: `Provided input for ${ticketLabel}` }
      case 'ticket_escalated':
        return { kind: 'ticket', title: `Escalated ${ticketLabel}` }
      case 'ticket_replanned':
        return { kind: 'ticket', title: `Sent ${ticketLabel} back to planning` }
      case 'ticket_invalidated':
        return { kind: 'ticket', title: `Invalidated ${ticketLabel}` }
      case 'ticket_rolled_back':
        return { kind: 'deploy', title: `Rolled back ${ticketLabel}` }
      case 'ticket_tests_passed':
        return { kind: 'review', title: `Checks passed for ${ticketLabel}` }
      case 'ticket_tests_failed':
        return { kind: 'review', title: `Checks failed for ${ticketLabel}` }
      case 'plan_created':
        return { kind: 'plan', title: `Created ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_submitted':
        return { kind: 'plan', title: `Submitted ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_classified':
        return { kind: 'plan', title: `Classified ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_approved':
        return { kind: 'plan', title: `Approved ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_applied':
        return { kind: 'plan', title: `Applied ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_rejected':
        return { kind: 'plan', title: `Rejected ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_superseded':
        return { kind: 'plan', title: `Superseded ${backendLabel(backend)} for ${ticketLabel}` }
      case 'plan_replan_identical':
        return { kind: 'plan', title: `Replan matched for ${ticketLabel}` }
      case 'plan_replan_diverged':
        return { kind: 'plan', title: `Replan diverged for ${ticketLabel}` }
      case 'work_stream_completed':
        return {
          kind: 'work_stream',
          title: 'Completed work stream',
          detail: ticketCount ? `${ticketCount} tickets finished` : undefined,
        }
      default:
        return null
    }
  })()

  if (!activity) return null
  return {
    id: event.id ?? `change:${type}:${ticketId ?? 'project'}:${event.timestamp ?? event.created_at ?? ''}`,
    ticketId,
    timestamp: event.timestamp ?? event.created_at,
    source: 'change_stream',
    ...activity,
  }
}

function summarizeTraceStep(
  step: TraceActivityStep,
  ticketsById: Map<string, Ticket>,
): ActivityItem | null {
  const payload = (step.payload ?? {}) as Record<string, unknown>
  const ticket = ticketsById.get(step.ticketId)
  const ticketLabel = ticketTitle(ticket, undefined, step.ticketId)
  const name = normalize(readString(payload, 'name') ?? '')
  const text = compactText(
    readString(payload, 'description'),
    readString(payload, 'summary'),
    readString(payload, 'details'),
    readString(payload, 'note'),
    readString(payload, 'result'),
    readString(payload, 'action'),
  )
  const combined = normalize(`${name} ${text}`)

  if (!combined) return null

  const prNumber = extractPrNumber(
    readString(payload, 'pr_url'),
    text,
  )

  if (looksLikePullRequestActivity(name, combined)) {
    const merged = /\bmerged\b/.test(combined)
    const updated = /\balready exists\b/.test(combined) || /\bupdated\b/.test(combined)
    const action = merged ? 'Merged' : updated ? 'Updated' : 'Created'
    return {
      id: step.id ?? `trace:${step.ticketId}:${step.created_at ?? ''}:${action.toLowerCase()}:pr`,
      kind: 'pr',
      title: `${action} pull request${prNumber ? ` #${prNumber}` : ''} for ${ticketLabel}`,
      detail: text || undefined,
      ticketId: step.ticketId,
      timestamp: step.created_at,
      source: 'trace',
    }
  }

  if (looksLikePushActivity(name, combined)) {
    return {
      id: step.id ?? `trace:${step.ticketId}:${step.created_at ?? ''}:push`,
      kind: 'git',
      title: `Pushed changes for ${ticketLabel}`,
      detail: text || undefined,
      ticketId: step.ticketId,
      timestamp: step.created_at,
      source: 'trace',
    }
  }

  if (looksLikeCommitActivity(name, combined)) {
    return {
      id: step.id ?? `trace:${step.ticketId}:${step.created_at ?? ''}:commit`,
      kind: 'git',
      title: `Committed changes for ${ticketLabel}`,
      detail: text || undefined,
      ticketId: step.ticketId,
      timestamp: step.created_at,
      source: 'trace',
    }
  }

  if (looksLikeDeployActivity(name, combined)) {
    return {
      id: step.id ?? `trace:${step.ticketId}:${step.created_at ?? ''}:deploy`,
      kind: 'deploy',
      title: `Recorded deploy activity for ${ticketLabel}`,
      detail: text || undefined,
      ticketId: step.ticketId,
      timestamp: step.created_at,
      source: 'trace',
    }
  }

  return null
}

function dedupeActivityItems(items: ActivityItem[]): ActivityItem[] {
  const seen = new Map<string, number>()
  const deduped: ActivityItem[] = []

  for (const item of items) {
    const ts = timestampValue(item.timestamp)
    const signature = [item.kind, item.ticketId ?? '', item.title, item.detail ?? ''].join('|')
    const prior = seen.get(signature)
    if (prior != null && Math.abs(prior - ts) < 2_000) {
      continue
    }
    seen.set(signature, ts)
    deduped.push(item)
  }

  return deduped
}

function looksLikePullRequestActivity(name: string, combined: string): boolean {
  return (
    name.includes('pr') ||
    name.includes('pull request') ||
    /\bpull request\b/.test(combined) ||
    /\bcreated pr\b/.test(combined) ||
    /\bpr #\d+\b/.test(combined) ||
    /github\.com\/.+\/pull\/\d+/.test(combined)
  )
}

function looksLikePushActivity(name: string, combined: string): boolean {
  return name.includes('push') || /\bpushed\b/.test(combined) || /\bgit push\b/.test(combined)
}

function looksLikeCommitActivity(name: string, combined: string): boolean {
  return name.includes('commit') || /\bcommitted\b/.test(combined) || /\bgit commit\b/.test(combined)
}

function looksLikeDeployActivity(name: string, combined: string): boolean {
  return name.includes('deploy') || /\bdeployed\b/.test(combined) || /\bdeploy\b/.test(combined)
}

function extractPrNumber(prURL: string | undefined, text: string): string | null {
  const urlMatch = prURL?.match(/\/pull\/(\d+)/)
  if (urlMatch?.[1]) return urlMatch[1]
  const textMatch = text.match(/\bPR\s*#(\d+)\b/i)
  return textMatch?.[1] ?? null
}

function backendLabel(backend: string | undefined): string {
  if (!backend) return 'plan'
  return `${backend.replace(/_/g, ' ')} plan`
}

function ticketTitle(
  ticket: Ticket | undefined,
  event?: ChangeStreamEvent,
  fallbackTicketId?: string,
): string {
  return ticket?.title ?? readString(event?.metadata, 'title') ?? fallbackTicketId ?? 'ticket'
}

function shortActor(actorID: string | undefined): string | null {
  if (!actorID) return null
  const parts = actorID.split('-')
  if (parts.length > 2) return parts.slice(-2).join('-')
  return actorID.slice(0, 12)
}

function humanizeField(field: string): string {
  return field.replace(/_/g, ' ')
}

function compactText(...parts: Array<string | undefined>): string {
  return parts
    .filter((part): part is string => Boolean(part))
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function normalize(text: string): string {
  return text.toLowerCase().replace(/\s+/g, ' ').trim()
}

function timestampValue(timestamp?: string): number {
  if (!timestamp) return 0
  const value = new Date(timestamp).getTime()
  return Number.isNaN(value) ? 0 : value
}

function readString(record: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = record?.[key]
  return typeof value === 'string' && value.length > 0 ? value : undefined
}

function readNumber(record: Record<string, unknown> | undefined, key: string): number | undefined {
  const value = record?.[key]
  return typeof value === 'number' ? value : undefined
}

function readStringArray(record: Record<string, unknown> | undefined, key: string): string[] {
  const value = record?.[key]
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string' && item.length > 0)
}
