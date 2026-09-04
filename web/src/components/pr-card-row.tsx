import { ExternalLink, GitPullRequest, GitPullRequestDraft, MessageSquareReply, TerminalSquare } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { checksBadge, decisionBadge, reviewStateLabel } from '@/lib/pr-format'
import { relativeTime } from '@/lib/sessions-format'
import type { components } from '@/lib/api/v1'

export type PullRequestCard = components['schemas']['PullRequestCard']

/** One PR row shared by My PRs and My Reviews. `extra` renders trailing actions. */
export function PRCardRow({ pr, extra, showAuthor }: { pr: PullRequestCard; extra?: React.ReactNode; showAuthor?: boolean }) {
  const checks = checksBadge(pr.checks)
  const decision = decisionBadge(pr.review_decision)
  const Icon = pr.is_draft ? GitPullRequestDraft : GitPullRequest
  const merged = pr.state === 'MERGED'
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-white/5 bg-white/[0.02] px-4 py-3 hover:bg-white/[0.04] sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <Icon className={`size-4 shrink-0 ${merged ? 'text-violet-300' : pr.is_draft ? 'text-muted-foreground' : 'text-emerald-300'}`} />
          <a href={pr.url} target="_blank" rel="noreferrer" className="truncate text-sm font-medium hover:underline">
            {pr.title}
          </a>
          <a href={pr.url} target="_blank" rel="noreferrer" className="text-xs text-muted-foreground hover:underline">
            {pr.repo.split('/')[1] ?? pr.repo}#{pr.number}
          </a>
          <ExternalLink className="size-3 text-muted-foreground/60" />
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {showAuthor && <span>by {pr.author}</span>}
          <span>
            {merged ? 'merged' : 'updated'} {relativeTime(pr.merged_at ?? pr.updated_at)}
          </span>
          <span className="font-mono text-[11px]">{pr.head_ref}</span>
          {pr.changed_files > 0 && (
            <span>
              <span className="text-emerald-300/80">+{pr.additions}</span> <span className="text-red-300/80">−{pr.deletions}</span> · {pr.changed_files} files
            </span>
          )}
          {pr.sessions > 0 && (
            <span className="inline-flex items-center gap-1" title="Agent sessions linked to this PR">
              <TerminalSquare className="size-3" />
              {pr.sessions} session{pr.sessions === 1 ? '' : 's'}
            </span>
          )}
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {pr.is_draft && <Badge variant="outline" className="text-[11px] font-normal text-muted-foreground">draft</Badge>}
          {checks && (
            <Badge variant="outline" className={`text-[11px] font-normal ${checks.cls}`}>
              <checks.Icon className="mr-1 size-3" />
              {checks.label}
            </Badge>
          )}
          {decision && !merged && <Badge variant="outline" className={`text-[11px] font-normal ${decision.cls}`}>{decision.label}</Badge>}
          {pr.mergeable === 'CONFLICTING' && <Badge variant="outline" className="border-red-400/30 text-[11px] font-normal text-red-300">conflicts</Badge>}
          {pr.linear_refs.map((r) => (
            <Badge key={r} variant="outline" className="border-indigo-400/30 text-[11px] font-mono font-normal text-indigo-200">
              {r}
            </Badge>
          ))}
          {pr.feedback && pr.feedback.new > 0 && (
            <Badge variant="outline" className="border-amber-400/30 text-[11px] font-normal text-amber-300">
              <MessageSquareReply className="mr-1 size-3" />
              {pr.feedback.new} new feedback
            </Badge>
          )}
          {pr.review && (
            <Badge variant="outline" className="text-[11px] font-normal text-muted-foreground" title={pr.review.summary ?? ''}>
              flywheel: {pr.review.state.replace('_', ' ')}
              {pr.review.verdict ? ` · ${pr.review.verdict}` : ''}
              {pr.review.dry_run ? ' (dry run)' : ''}
            </Badge>
          )}
          {pr.reviews
            .filter((r) => r.state !== 'PENDING')
            .slice(0, 4)
            .map((r) => (
              <span key={r.login} className="text-[11px] text-muted-foreground">
                {r.login} {reviewStateLabel(r.state)}
              </span>
            ))}
          {pr.request_kind === 'direct' && <Badge variant="outline" className="border-sky-400/30 text-[11px] font-normal text-sky-200">requested you</Badge>}
          {pr.request_kind === 'team' && (
            <Badge variant="outline" className="text-[11px] font-normal text-muted-foreground">via team {pr.requested_teams.join(', ')}</Badge>
          )}
          {pr.requested_reviewers.length > 0 && (
            <span className="text-[11px] text-muted-foreground">awaiting {pr.requested_reviewers.join(', ')}</span>
          )}
        </div>
      </div>
      {extra && <div className="flex shrink-0 items-center gap-2 sm:pl-3">{extra}</div>}
    </div>
  )
}
