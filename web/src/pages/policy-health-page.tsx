import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'

interface Condition {
  field: string
  operator: string
  value: unknown
  label?: string
}

interface Rules {
  auto_approve_conditions?: Condition[]
  review_conditions?: Condition[]
  block_conditions?: Condition[]
}

interface Metrics {
  policy_id: string
  total_decisions: number
  auto_approval_rate: number
  rollback_rate: number
  incident_rate: number
  success_rate: number
  pending_outcomes: number
  sample_size_sufficient: boolean
}

interface PolicyProposal {
  id: string
  policy_id: string
  proposal_type: 'broaden' | 'review'
  suggestion: Record<string, unknown>
  statistics: Record<string, unknown>
  status: string
  created_at: string
}

interface PolicyChangeEvent {
  id: string
  policy_id: string
  actor_id: string
  change_type: string
  notes: string
  created_at: string
}

interface SimulationSummary {
  total_tickets: number
  would_auto_approve: number
  would_require_review: number
  would_block: number
  changed_decisions: number
  change_rate: number
}

interface SimulationResult {
  policy_id: string
  tickets_simulated: number
  summary: SimulationSummary
}

interface PolicyHealth {
  policy: {
    id: string
    project_id: string
    name: string
    description: string
    rules: Rules
    enabled: boolean
    min_sample: number
    created_at: string
    updated_at: string
  }
  metrics: Metrics
  proposals: PolicyProposal[]
  history: PolicyChangeEvent[]
}

/** Helper for policy API calls (endpoints are outside the OpenAPI spec). */
async function policyApiFetch(
  path: string,
  token: string | null,
  init?: RequestInit,
): Promise<Response> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`
  return fetch(path, { ...init, headers: { ...headers, ...init?.headers } })
}

export function PolicyHealthPage() {
  const { orgId, projectId } = useParams<{
    orgId: string
    projectId: string
  }>()
  const { token } = useAuth()
  const [health, setHealth] = useState<PolicyHealth[] | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [calibrating, setCalibrating] = useState(false)
  const [calibrationMsg, setCalibrationMsg] = useState<string | null>(null)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  async function loadHealth() {
    if (!projectId) return
    try {
      const res = await policyApiFetch(
        `/projects/${projectId}/policies/health`,
        token,
      )
      if (!res.ok) {
        setErr(`Failed to load policies: ${res.statusText}`)
        return
      }
      const data = await res.json()
      setHealth(data.policies ?? [])
      setErr(null)
    } catch (e) {
      setErr(String(e))
    }
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      await loadHealth()
      if (cancelled) return
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, projectId])

  async function handleCalibrate() {
    if (!projectId) return
    setCalibrating(true)
    setCalibrationMsg(null)
    try {
      const res = await policyApiFetch(
        `/projects/${projectId}/policies/calibrate`,
        token,
        { method: 'POST' },
      )
      if (res.ok) {
        const data = await res.json()
        const count = data.proposals?.length ?? 0
        setCalibrationMsg(
          count > 0
            ? `Calibration generated ${count} proposal${count > 1 ? 's' : ''}.`
            : 'Calibration complete. No new proposals.',
        )
      }
      await loadHealth()
    } finally {
      setCalibrating(false)
    }
  }

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!health) {
    return <p className="text-muted-foreground text-sm">Loading…</p>
  }

  const totalDecisions = health.reduce(
    (sum, h) => sum + h.metrics.total_decisions,
    0,
  )
  const pendingProposals = health.reduce(
    (sum, h) =>
      sum + (h.proposals?.filter((p) => p.status === 'pending')?.length ?? 0),
    0,
  )

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgId}
            projectId={projectId}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Policy Health
          </span>
        </p>
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold tracking-tight">
            Policy Health
          </h1>
          <Button size="sm" onClick={handleCalibrate} disabled={calibrating}>
            {calibrating ? 'Calibrating…' : 'Run Calibration'}
          </Button>
        </div>
      </div>

      {/* Summary strip */}
      <div className="flex flex-wrap items-center gap-4 text-sm">
        <span>
          <span className="text-muted-foreground">Active policies:</span>{' '}
          <span className="font-medium">{health.length}</span>
        </span>
        <span>
          <span className="text-muted-foreground">Total decisions:</span>{' '}
          <span className="font-mono font-medium">{totalDecisions}</span>
        </span>
        {pendingProposals > 0 && (
          <Badge className="bg-amber-800/50 text-amber-200">
            {pendingProposals} pending proposal{pendingProposals > 1 ? 's' : ''}
          </Badge>
        )}
      </div>

      {calibrationMsg && (
        <p className="text-muted-foreground text-sm">{calibrationMsg}</p>
      )}

      {health.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-muted-foreground text-sm">
              No policies configured. Create a policy via the API to start
              tracking calibration metrics.
            </p>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4">
        {health.map((ph) => (
          <PolicyCard
            key={ph.policy.id}
            health={ph}
            token={token}
            onRefresh={loadHealth}
          />
        ))}
      </div>
    </div>
  )
}

function PolicyCard({
  health,
  token,
  onRefresh,
}: {
  health: PolicyHealth
  token: string | null
  onRefresh: () => void
}) {
  const { policy, metrics, proposals, history } = health
  const [busyProposal, setBusyProposal] = useState<string | null>(null)
  const [showHistory, setShowHistory] = useState(false)
  const [simResult, setSimResult] = useState<SimulationResult | null>(null)
  const [simBusy, setSimBusy] = useState(false)
  const [simError, setSimError] = useState<string | null>(null)

  const pendingProposals = proposals?.filter((p) => p.status === 'pending') ?? []

  async function handleResolveProposal(
    proposalId: string,
    status: 'accepted' | 'rejected' | 'dismissed',
  ) {
    setBusyProposal(proposalId)
    try {
      await policyApiFetch(`/proposals/${proposalId}/resolve`, token, {
        method: 'POST',
        body: JSON.stringify({ status }),
      })
      onRefresh()
    } finally {
      setBusyProposal(null)
    }
  }

  async function handleSimulate() {
    setSimBusy(true)
    setSimError(null)
    try {
      const res = await policyApiFetch(
        `/policies/${policy.id}/simulate`,
        token,
        {
          method: 'POST',
          body: JSON.stringify({
            candidate_rules: policy.rules,
            days: 30,
          }),
        },
      )
      if (!res.ok) {
        setSimError(`Simulation failed: ${res.statusText}`)
        return
      }
      const data = await res.json()
      setSimResult(data)
    } catch (e) {
      setSimError(String(e))
    } finally {
      setSimBusy(false)
    }
  }

  return (
    <Card className="border-border/50 bg-card/80 backdrop-blur-sm">
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CardTitle className="text-base">{policy.name}</CardTitle>
            <Badge variant={policy.enabled ? 'default' : 'secondary'}>
              {policy.enabled ? 'Active' : 'Disabled'}
            </Badge>
          </div>
          <HealthIndicator metrics={metrics} />
        </div>
        {policy.description ? (
          <p className="text-muted-foreground text-sm">{policy.description}</p>
        ) : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Metrics Grid */}
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <MetricTile
            label="Auto-approval"
            value={`${(metrics.auto_approval_rate * 100).toFixed(1)}%`}
          />
          <MetricTile
            label="Rollback rate"
            value={`${(metrics.rollback_rate * 100).toFixed(1)}%`}
            variant={
              metrics.rollback_rate > 0.15
                ? 'danger'
                : metrics.rollback_rate > 0.05
                  ? 'warning'
                  : 'success'
            }
          />
          <MetricTile
            label="Incident rate"
            value={`${(metrics.incident_rate * 100).toFixed(1)}%`}
            variant={metrics.incident_rate > 0 ? 'danger' : 'success'}
          />
          <MetricTile
            label="Success rate"
            value={`${(metrics.success_rate * 100).toFixed(1)}%`}
            variant={
              metrics.success_rate >= 0.9
                ? 'success'
                : metrics.success_rate >= 0.7
                  ? 'warning'
                  : 'danger'
            }
          />
        </div>

        <div className="text-muted-foreground flex items-center gap-4 text-xs">
          <span>{metrics.total_decisions} total decisions</span>
          <span>{metrics.pending_outcomes} pending outcomes</span>
          {!metrics.sample_size_sufficient ? (
            <Badge variant="outline" className="text-xs">
              Insufficient sample ({metrics.total_decisions}/{policy.min_sample})
            </Badge>
          ) : null}
        </div>

        {/* Proposals with resolution actions */}
        {pendingProposals.length > 0 ? (
          <div className="flex flex-col gap-2">
            <h4 className="text-sm font-medium">
              Proposed Adjustments ({pendingProposals.length})
            </h4>
            {pendingProposals.map((p) => (
              <ProposalCard
                key={p.id}
                proposal={p}
                onResolve={handleResolveProposal}
                busy={busyProposal === p.id}
              />
            ))}
          </div>
        ) : null}

        {/* Actions */}
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={simBusy}
            onClick={handleSimulate}
          >
            {simBusy ? 'Simulating…' : 'Preview rules (last 30d)'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setShowHistory(!showHistory)}
          >
            {showHistory ? 'Hide history' : 'Edit history'}
          </Button>
        </div>

        {/* Simulation Results */}
        {simError && (
          <p className="text-destructive text-sm">{simError}</p>
        )}
        {simResult && (
          <div className="border-border rounded-lg border p-3">
            <h4 className="mb-2 text-sm font-medium">
              Simulation Preview (last 30 days)
            </h4>
            <div className="flex flex-wrap gap-4 text-xs">
              <div>
                <span className="text-muted-foreground">Tickets:</span>{' '}
                <span className="font-mono font-medium">
                  {simResult.summary.total_tickets}
                </span>
              </div>
              <div>
                <span className="text-muted-foreground">Would auto-approve:</span>{' '}
                <span className="font-mono font-medium text-green-400">
                  {simResult.summary.would_auto_approve}
                </span>
              </div>
              <div>
                <span className="text-muted-foreground">Would need review:</span>{' '}
                <span className="font-mono font-medium text-amber-400">
                  {simResult.summary.would_require_review}
                </span>
              </div>
              <div>
                <span className="text-muted-foreground">Would block:</span>{' '}
                <span className="font-mono font-medium text-red-400">
                  {simResult.summary.would_block}
                </span>
              </div>
              <div>
                <span className="text-muted-foreground">Changed:</span>{' '}
                <span className="font-mono font-medium">
                  {simResult.summary.changed_decisions} (
                  {(simResult.summary.change_rate * 100).toFixed(1)}%)
                </span>
              </div>
            </div>
          </div>
        )}

        {/* Edit History */}
        {showHistory && history.length > 0 ? (
          <div className="flex flex-col gap-2">
            <h4 className="text-sm font-medium">Recent Changes</h4>
            <div className="flex flex-col gap-1">
              {history.slice(0, 10).map((e) => (
                <div
                  key={e.id}
                  className="text-muted-foreground border-border flex items-center gap-2 rounded border px-3 py-2 text-xs"
                >
                  <Badge variant="outline" className="shrink-0 text-xs">
                    {e.change_type}
                  </Badge>
                  <span className="flex-1">{e.notes || 'No notes'}</span>
                  <span className="shrink-0 font-mono">
                    {new Date(e.created_at).toLocaleDateString()}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ) : null}
        {showHistory && history.length === 0 ? (
          <p className="text-muted-foreground text-sm">No edit history.</p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function MetricTile({
  label,
  value,
  variant = 'default',
}: {
  label: string
  value: string
  variant?: 'default' | 'success' | 'warning' | 'danger'
}) {
  const colorClass =
    variant === 'success'
      ? 'text-green-400'
      : variant === 'warning'
        ? 'text-amber-400'
        : variant === 'danger'
          ? 'text-red-400'
          : 'text-foreground'

  return (
    <div className="bg-muted/30 rounded-lg p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className={`text-lg font-semibold ${colorClass}`}>{value}</p>
    </div>
  )
}

function HealthIndicator({ metrics }: { metrics: Metrics }) {
  if (!metrics.sample_size_sufficient) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-2 w-2 rounded-full bg-gray-400" />
        <span className="text-muted-foreground text-xs">Collecting data</span>
      </div>
    )
  }
  if (metrics.incident_rate > 0 || metrics.rollback_rate > 0.15) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-2 w-2 animate-pulse rounded-full bg-red-400" />
        <span className="text-xs text-red-400">Needs review</span>
      </div>
    )
  }
  if (metrics.success_rate >= 0.9 && metrics.rollback_rate <= 0.05) {
    return (
      <div className="flex items-center gap-1">
        <div className="h-2 w-2 rounded-full bg-green-400" />
        <span className="text-xs text-green-400">Healthy</span>
      </div>
    )
  }
  return (
    <div className="flex items-center gap-1">
      <div className="h-2 w-2 rounded-full bg-amber-400" />
      <span className="text-xs text-amber-400">Moderate</span>
    </div>
  )
}

function ProposalCard({
  proposal,
  onResolve,
  busy,
}: {
  proposal: PolicyProposal
  onResolve: (id: string, status: 'accepted' | 'rejected' | 'dismissed') => void
  busy: boolean
}) {
  const isBroaden = proposal.proposal_type === 'broaden'

  return (
    <div
      className={`rounded-lg border p-3 ${
        isBroaden
          ? 'border-green-500/30 bg-green-950/20'
          : 'border-red-500/30 bg-red-950/20'
      }`}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <Badge
              variant="outline"
              className={
                isBroaden
                  ? 'border-green-500/50 text-green-400'
                  : 'border-red-500/50 text-red-400'
              }
            >
              {isBroaden ? 'Broaden' : 'Review'}
            </Badge>
            <span className="text-muted-foreground text-xs">
              {new Date(proposal.created_at).toLocaleDateString()}
            </span>
          </div>
          <span className="text-sm">
            {(proposal.suggestion as Record<string, string>).recommendation ??
              'System proposal'}
          </span>
          {proposal.suggestion?.rationale ? (
            <p className="text-muted-foreground text-xs">
              {String(proposal.suggestion.rationale)}
            </p>
          ) : null}
          {proposal.suggestion?.concerns ? (
            <ul className="text-muted-foreground mt-1 list-inside list-disc text-xs">
              {(proposal.suggestion.concerns as string[]).map((c, i) => (
                <li key={i}>{c}</li>
              ))}
            </ul>
          ) : null}
        </div>
        {/* Resolution actions — broadening is never automatic */}
        <div className="flex flex-wrap gap-1.5">
          <Button
            size="xs"
            disabled={busy}
            onClick={() => onResolve(proposal.id, 'accepted')}
          >
            Accept
          </Button>
          <Button
            size="xs"
            variant="outline"
            disabled={busy}
            onClick={() => onResolve(proposal.id, 'rejected')}
          >
            Reject
          </Button>
          <Button
            size="xs"
            variant="ghost"
            disabled={busy}
            onClick={() => onResolve(proposal.id, 'dismissed')}
          >
            Dismiss
          </Button>
        </div>
      </div>
      <p className="text-muted-foreground mt-2 text-xs italic">
        Broadening is never automatic — human approval required.
      </p>
    </div>
  )
}
