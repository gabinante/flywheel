import { describe, expect, it } from 'vitest'
import { reviewStatus, type CodeReviewRequest } from './codereview-format'

describe('review status separates agent recommendations from GitHub outcomes', () => {
  const review = { state: 'failed', verdict: 'approve', error: 'post review: GitHub rejected submission', watch: true } as CodeReviewRequest
  it('never describes a failed approval as posted', () => {
    expect(reviewStatus(review)).toEqual({ label: 'Posting failed', detail: expect.stringContaining('Agent recommendation: approve. No successful GitHub submission') })
  })
  it('distinguishes stopping a review from closing a PR', () => {
    expect(reviewStatus({ ...review, state: 'closed', watch: false }, 'OPEN').label).toBe('Review stopped')
    expect(reviewStatus({ ...review, state: 'closed' }, 'OPEN').label).toBe('Review inactive')
  })
  it('only claims a GitHub outcome when a posted review is recorded', () => {
    expect(reviewStatus({ ...review, state: 'watching', dry_run: true }).label).toBe('Dry run complete')
    expect(reviewStatus({ ...review, state: 'watching', error: '' }).label).toBe('Watching PR')
    expect(reviewStatus({ ...review, state: 'watching', review_url: 'https://github.com/test/repo/pull/1#review', error: '' }).label).toBe('Approved on GitHub')
  })
})
