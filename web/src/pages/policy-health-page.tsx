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

export function PolicyHealthPage() {
  const { orgId, projectId } = useParams<{
    orgId: string
    projectId: string
  }>()
  const { token } = useAuth()
  const [health, setHealth] = useState<PolicyHealth[] | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [calibrating, setCalibrating] = useState(false)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    void (async () => {
      try {
        const res = await fetch(`/projects/${projectId}/policies/health`, {
          headers: { Authorization: `Bearer ${token ?? ''}` },
        })
        if (!res.ok) {
          setErr(`Failed to load policies: ${res.statusText}`)
          return
        }
        const data = await res.json()
        if (!cancelled) {
          setHealth(data.policies ?? [])
        }
      } catch (e) {
        if (!cancelled) setErr(String(e))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [token, projectId])

  async function runCalibration() {
    if (!projectId) return
    setCalibrating(true)
    try {
      const res = await fetch(`/projects/${projectId}/policies/calibrate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token ?? ''}`,
        },
      })
      if (res.ok) {
        // Refresh health data
        const healthRes = await fetch(`/projects/${projectId}/policies/health`, {
          headers: { Authorization: `Bearer ${token ?? ''}` },
        })
        if (healthRes.ok) {
          const data = await healthRes.json()
          setHealth(data.policies ?? [])
        }
      }
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
          <Button size="sm" onClick={runCalibration} disabled={calibrating}>
            {calibrating ? 'Calibrating…' : 'Run Calibration'}
          </Button>
        </div>
      </div>

      {health.length === 0 ? (
        <Card>
          <CardContent className="py-8 text-center">
            <p className="text-muted-foreground text-sm">
              No policies configured. Create a policy to start tracking
              calibration metrics.
            </p>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4">
        {health.map((ph) => (
          <PolicyCard key={ph.policy.id} health={ph} />
        ))}
      </div>
    </div>
  )
}

function PolicyCard({ health }: { health: PolicyHealth }) {
  const { policy, metrics, proposals, history } = health

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
              Insufficient sample
            </Badge>
          ) : null}
        </div>

        {/* Proposals */}
        {proposals.length > 0 ? (
          <div className="flex flex-col gap-2">
            <h4 className="text-sm font-medium">Pending Proposals</h4>
            {proposals.map((p) => (
              <ProposalCard key={p.id} proposal={p} />
            ))}
          </div>
        ) : null}

        {/* Edit History */}
        {history.length > 0 ? (
          <div className="flex flex-col gap-2">
            <h4 className="text-sm font-medium">Recent Changes</h4>
            <div className="flex flex-col gap-1">
              {history.slice(0, 5).map((e) => (
                <div
                  key={e.id}
                  className="text-muted-foreground flex items-center gap-2 text-xs"
                >
                  <Badge variant="outline" className="text-xs">
                    {e.change_type}
                  </Badge>
                  <span>{e.notes || 'No notes'}</span>
                  <span className="ml-auto">
                    {new Date(e.created_at).toLocaleDateString()}
                  </span>
                </div>
              ))}
            </div>
          </div>
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

function ProposalCard({ proposal }: { proposal: PolicyProposal }) {
  const isBroaden = proposal.proposal_type === 'broaden'

  return (
    <div
      className={`rounded-lg border p-3 ${
        isBroaden
          ? 'border-green-500/30 bg-green-950/20'
          : 'border-red-500/30 bg-red-950/20'
      }`}
    >
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
        <span className="text-sm">
          {(proposal.suggestion as Record<string, string>).recommendation ??
            'System proposal'}
        </span>
      </div>
      {proposal.suggestion?.rationale ? (
        <p className="text-muted-foreground mt-1 text-xs">
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
      <p className="text-muted-foreground mt-2 text-xs italic">
        Broadening is never automatic — human approval required.
      </p>
    </div>
  )
}
