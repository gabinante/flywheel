import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'
import type { CodeReviewRequest } from '@/lib/codereview-format'

export function PrioritizeReviewButton({ review, onChanged }: { review: CodeReviewRequest; onChanged?: (r: CodeReviewRequest) => void }) {
  const { client } = useAPI()
  const queries = useQueryClient()
  const [pending, setPending] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState('')
  if (review.state !== 'queued') return null
  const prioritized = (!!review.priority_at && !review.retry_at) || confirmed
  const prioritize = async () => {
    setPending(true); setError('')
    try {
      const result = await client.POST('/code-reviews/{reviewID}/prioritize', { params: { path: { reviewID: review.id } } })
      if (!result.response.ok || !result.data) throw new Error(formatApiError(result.error))
      setConfirmed(!!result.data.priority_at)
      onChanged?.(result.data)
      void queries.invalidateQueries({ queryKey: ['my-reviews'] })
      void queries.invalidateQueries({ queryKey: ['operator-overview'] })
      void queries.invalidateQueries({ queryKey: ['code-review-status'] })
    } catch (e) { setError(e instanceof Error ? e.message : 'Could not prioritize this review.') }
    finally { setPending(false) }
  }
  return <span className="inline-flex flex-col items-start gap-1">
    <Button size="sm" variant="outline" disabled={pending || prioritized} onClick={prioritize}
      title="Review next; allows one extra reviewer when normal slots are full. Respects paused reviews.">
      {pending ? 'Prioritizing…' : prioritized ? 'Priority queued' : 'Jump the queue'}
    </Button>
    {error && <span role="alert" className="text-xs text-destructive">{error}</span>}
  </span>
}
