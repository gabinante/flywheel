import type { components } from '@/lib/api/v1'

export type CodeReviewRequest = components['schemas']['CodeReviewRequest']
export type CodeReviewFinding = components['schemas']['CodeReviewFinding']
export type FeedbackRound = components['schemas']['FeedbackRound']

export const STATE_CLASS: Record<string, string> = {
  queued: 'border-slate-500/30 bg-slate-500/10 text-slate-300',
  fetching: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  reviewing: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  publishing: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  approved: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200',
  changes_requested: 'border-amber-500/30 bg-amber-500/10 text-amber-200',
  commented: 'border-violet-500/30 bg-violet-500/10 text-violet-200',
  watching: 'border-teal-500/30 bg-teal-500/10 text-teal-200',
  superseded: 'border-slate-500/30 bg-slate-500/10 text-slate-400',
  closed: 'border-slate-500/30 bg-slate-500/10 text-slate-400',
  failed: 'border-red-500/30 bg-red-500/10 text-red-200',
}

export const SEVERITY_CLASS: Record<string, string> = {
  P0: 'border-red-500/40 bg-red-500/15 text-red-200',
  P1: 'border-amber-500/40 bg-amber-500/15 text-amber-200',
  P2: 'border-sky-500/30 bg-sky-500/10 text-sky-200',
  P3: 'border-slate-500/30 bg-slate-500/10 text-slate-300',
}

export const VERDICT_LABEL: Record<string, string> = {
  approve: 'Approved',
  request_changes: 'Changes requested',
  comment: 'Commented',
}

export const ACTIVE_STATES = new Set(['queued', 'fetching', 'reviewing', 'publishing'])

export function findingCounts(findings: CodeReviewFinding[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const f of findings) out[f.severity] = (out[f.severity] ?? 0) + 1
  return out
}

export function shortSha(sha?: string): string {
  return sha ? sha.slice(0, 8) : ''
}
