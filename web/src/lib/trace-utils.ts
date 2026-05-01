import type { components } from '@/lib/api/v1'

type TraceStep = components['schemas']['TraceStep']

export const WORKER_OUTPUT_KIND = 'worker_output'

export const STEP_TYPE_META: Record<string, { label: string; dotColor: string }> = {
  tool_call:   { label: 'Tool Call',   dotColor: 'bg-blue-400' },
  observation: { label: 'Observation', dotColor: 'bg-emerald-400' },
  thought:     { label: 'Thought',     dotColor: 'bg-purple-400' },
  error:       { label: 'Error',       dotColor: 'bg-red-400' },
}

export function getPayloadRecord(step: TraceStep): Record<string, unknown> | null {
  if (!step.payload || typeof step.payload !== 'object') return null
  return step.payload as Record<string, unknown>
}

export function getWorkerOutput(step: TraceStep): { stream: string; text: string } | null {
  const payload = getPayloadRecord(step)
  if (!payload || payload.kind !== WORKER_OUTPUT_KIND) return null
  if (typeof payload.text !== 'string') return null
  return {
    stream: typeof payload.stream === 'string' ? payload.stream : 'stdout',
    text: payload.text,
  }
}

export function summarizePayload(step: TraceStep): string | null {
  const payload = getPayloadRecord(step)
  if (!payload) return null
  if (payload.kind === WORKER_OUTPUT_KIND) return null
  if ('message' in payload && typeof payload.message === 'string') return payload.message
  if ('summary' in payload && typeof payload.summary === 'string') return payload.summary
  if ('name' in payload && typeof payload.name === 'string') return payload.name
  if ('text' in payload && typeof payload.text === 'string') return payload.text
  if ('command' in payload && typeof payload.command === 'string') return payload.command
  const keys = Object.keys(payload)
  if (keys.length === 0) return null
  return keys.slice(0, 3).join(', ')
}
