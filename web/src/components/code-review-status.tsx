import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { reviewStatus, STATE_CLASS, type CodeReviewRequest } from '@/lib/codereview-format'
import { cn } from '@/lib/utils'

export function CodeReviewStatus({ review, prState, linked = false }: { review: CodeReviewRequest; prState?: string; linked?: boolean }) {
  const status = reviewStatus(review, prState)
  const badge = <Badge variant="outline" className={cn('text-[11px] font-normal', STATE_CLASS[review.state])}>{status.label}</Badge>
  return <div className="space-y-1" aria-label="Flywheel review status">
    {linked ? <Link to={`/code-reviews/${review.id}`} className="hover:underline">{badge}</Link> : badge}
    <p className="text-xs text-muted-foreground">{status.detail}</p>
  </div>
}
