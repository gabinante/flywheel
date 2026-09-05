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

/** An agent's recommendation is separate from a review actually posted to GitHub. */
export function reviewStatus(r: CodeReviewRequest, prState?: string) {
  const recommendation = r.verdict ? `Agent recommendation: ${r.verdict === 'approve' ? 'approve' : r.verdict === 'request_changes' ? 'request changes' : 'comment'}.` : ''
  switch (r.state) {
    case 'queued': return { label: 'Queued', detail: 'Waiting for an available review worker.' }
    case 'fetching': return { label: 'Preparing review', detail: 'Fetching the PR and preparing its checkout. The agent has not started yet.' }
    case 'reviewing': return { label: 'Agent reviewing', detail: 'Review in progress. Open the session to follow the agent.' }
    case 'publishing': return { label: 'Posting review', detail: `${recommendation} Submitting the review to GitHub.` }
    case 'failed': return {
      label: r.error?.startsWith('post review:') ? 'Posting failed' : 'Review failed',
      detail: `${recommendation}${recommendation ? ' ' : ''}No successful GitHub submission was recorded for this attempt. Open the review for the error and retry.`,
    }
    case 'closed': return {
      label: r.watch ? (prState === 'OPEN' ? 'Review inactive' : 'PR closed or merged') : 'Review stopped',
      detail: r.watch ? 'The PR was closed when Flywheel last checked. A new explicit request or manual review can restart it.' : 'This review was stopped in Flywheel. A new explicit request or manual review can restart it.',
    }
    default:
      if (r.dry_run) return { label: 'Dry run complete', detail: `${recommendation} Nothing was posted to GitHub.` }
      if (r.review_url) return { label: `${VERDICT_LABEL[r.verdict] ?? 'Review posted'} on GitHub`, detail: r.watch ? 'Watching for new commits and explicit review requests.' : 'Review submitted successfully.' }
      return { label: r.state === 'watching' ? 'Watching PR' : r.state.replaceAll('_', ' '), detail: r.error || 'No active review worker. Open the review for details.' }
  }
}

export function findingCounts(findings: CodeReviewFinding[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const f of findings) out[f.severity] = (out[f.severity] ?? 0) + 1
  return out
}

export function shortSha(sha?: string): string {
  return sha ? sha.slice(0, 8) : ''
}
