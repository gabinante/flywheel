import { CheckCircle2, CircleDashed, XCircle } from 'lucide-react'

export function checksBadge(state: string) {
  switch (state) {
    case 'SUCCESS':
      return { label: 'checks pass', cls: 'border-emerald-400/30 text-emerald-300', Icon: CheckCircle2 }
    case 'FAILURE':
    case 'ERROR':
      return { label: 'checks failing', cls: 'border-red-400/30 text-red-300', Icon: XCircle }
    case 'PENDING':
    case 'EXPECTED':
      return { label: 'checks running', cls: 'border-amber-400/30 text-amber-300', Icon: CircleDashed }
    default:
      return null
  }
}

export function decisionBadge(decision: string) {
  switch (decision) {
    case 'APPROVED':
      return { label: 'approved', cls: 'border-emerald-400/30 text-emerald-300' }
    case 'CHANGES_REQUESTED':
      return { label: 'changes requested', cls: 'border-red-400/30 text-red-300' }
    case 'REVIEW_REQUIRED':
      return { label: 'review required', cls: 'border-white/10 text-muted-foreground' }
    default:
      return null
  }
}

export function reviewStateLabel(state: string) {
  switch (state) {
    case 'APPROVED':
      return 'approved'
    case 'CHANGES_REQUESTED':
      return 'requested changes'
    case 'COMMENTED':
      return 'commented'
    case 'DISMISSED':
      return 'dismissed'
    case 'PENDING':
      return 'pending'
    default:
      return state.toLowerCase()
  }
}
