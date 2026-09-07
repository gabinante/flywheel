import { useId, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { GitPullRequest, LoaderCircle } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'
import type { CodeReviewRequest } from '@/lib/codereview-format'

export function QueuePRReviews({ onQueued, children }: { onQueued?: () => void; children?: ReactNode }) {
  const id = useId()
  const { client } = useAPI()
  const queryClient = useQueryClient()
  const [text, setText] = useState('')
  const [dryRun, setDryRun] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [requests, setRequests] = useState<CodeReviewRequest[]>([])

  const submit = async () => {
    if (submitting || !text.trim()) return
    setSubmitting(true)
    setError(null)
    setRequests([])
    try {
      const result = await client.POST('/code-reviews', { body: { text: text.trim(), dry_run: dryRun || undefined } })
      if (!result.response.ok || !result.data) throw new Error(formatApiError(result.error))
      setRequests(result.data.requests)
      setText('')
      for (const key of ['operator-overview', 'code-review-status', 'my-reviews']) {
        void queryClient.invalidateQueries({ queryKey: [key] })
      }
      onQueued?.()
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not queue the reviews. Try again.')
    } finally {
      setSubmitting(false)
    }
  }

  return <form aria-label="Queue PR reviews" className="rounded-2xl border border-white/10 bg-white/5 p-3 backdrop-blur-md" onSubmit={event => { event.preventDefault(); void submit() }}>
    <label htmlFor={id} className="mb-2 block text-sm font-medium">Review a PR</label>
    <Textarea id={id} value={text} disabled={submitting} onChange={event => setText(event.target.value)}
      placeholder="Paste PR URLs or owner/repo#123 references — one or many. Each becomes its own review."
      className="min-h-[3.5rem] text-sm" />
    <div className="mt-2 flex flex-wrap items-center gap-3">
      <Button type="submit" size="sm" disabled={submitting || !text.trim()}>
        {submitting ? <LoaderCircle className="size-4 animate-spin" /> : <GitPullRequest className="size-4" />}
        {submitting ? 'Queueing…' : 'Review'}
      </Button>
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        <Switch checked={dryRun} disabled={submitting} onCheckedChange={setDryRun} /> Dry run (do not post)
      </label>
      {children && <div className="ml-auto">{children}</div>}
    </div>
    {error && <p role="alert" className="mt-2 text-sm text-red-200">{error}</p>}
    {requests.length > 0 && <div role="status" className="mt-3 space-y-1 text-xs text-muted-foreground">
      <p>Review requests ready — open a review to follow its progress.</p>
      <ul className="flex flex-wrap gap-x-4 gap-y-1">{requests.map(request => <li key={request.id}>
        <Link to={`/code-reviews/${request.id}`} className="text-primary hover:underline">{request.repo}#{request.number}</Link>
      </li>)}</ul>
    </div>}
  </form>
}
