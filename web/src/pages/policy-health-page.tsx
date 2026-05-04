import { useEffect, useState } from 'react'
import {
  Activity,
  ChevronDown,
  ChevronRight,
  CircleDot,
  GitCommit,
  Shield,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  Zap,
} from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PolicyHealthSkeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

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

type HealthLevel = 'healthy' | 'moderate' | 'critical' | 'unknown'

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function getHealthLevel(metrics: Metrics): HealthLevel {
  if (!metrics.sample_size_sufficient) return 'unknown'
  if (metrics.incident_rate > 0 || metrics.rollback_rate > 0.15) return 'critical'
  if (metrics.success_rate >= 0.9 && metrics.rollback_rate <= 0.05) return 'healthy'
  return 'moderate'
}

const HEALTH_CONFIG: Record<
  HealthLevel,
  {
    label: string
    dot: string
    text: string
    border: string
    glow: string
    bg: string
    icon: typeof ShieldCheck
  }
> = {
  healthy: {
    label: 'Healthy',
    dot: 'bg-green-400',
    text: 'text-green-400',
    border: 'border-green-500/30',
    glow: 'shadow-[0_0_8px_rgba(74,222,128,0.3)]',
    bg: 'bg-green-500/5',
    icon: ShieldCheck,
  },
  moderate: {
    label: 'Moderate',
    dot: 'bg-amber-400',
    text: 'text-amber-400',
    border: 'border-amber-500/30',
    glow: 'shadow-[0_0_8px_rgba(251,191,36,0.3)]',
    bg: 'bg-amber-500/5',
    icon: ShieldAlert,
  },
  critical: {
    label: 'Needs Review',
    dot: 'bg-red-400',
    text: 'text-red-400',
    border: 'border-red-500/30',
    glow: 'shadow-[0_0_8px_rgba(248,113,113,0.3)]',
    bg: 'bg-red-500/5',
    icon: ShieldAlert,
  },
  unknown: {
    label: 'Collecting Data',
    dot: 'bg-gray-400',
    text: 'text-muted-foreground',
    border: 'border-border/50',
    glow: '',
    bg: 'bg-muted/5',
    icon: ShieldQuestion,
  },
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

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })
}

/* ------------------------------------------------------------------ */
/*  Main Page                                                          */
/* ------------------------------------------------------------------ */

export function PolicyHealthPage() {
  const { orgId, projectId, orgSlug, projectSlug } = useProjectPaths()
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
    return <PolicyHealthSkeleton />
  }

  const activePolicies = health.filter((h) => h.policy.enabled)
  const proposedPolicies = health.filter((h) => !h.policy.enabled)
  const totalDecisions = health.reduce(
    (sum, h) => sum + h.metrics.total_decisions,
    0,
  )
  const pendingProposals = health.reduce(
    (sum, h) =>
      sum + (h.proposals?.filter((p) => p.status === 'pending')?.length ?? 0),
    0,
  )

  /* Health summary counts */
  const healthCounts = health.reduce(
    (acc, h) => {
      const level = getHealthLevel(h.metrics)
      acc[level] = (acc[level] ?? 0) + 1
      return acc
    },
    {} as Record<HealthLevel, number>,
  )

  return (
    <div className="flex flex-col gap-6">
      {/* Header */}
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgSlug}
            projectId={projectSlug}
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
            <Zap className="mr-1.5 size-3.5" />
            {calibrating ? 'Calibrating...' : 'Run Calibration'}
          </Button>
        </div>
      </div>

      {/* Summary cards strip */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <SummaryTile
          label="Active policies"
          value={String(activePolicies.length)}
          icon={<Shield className="size-4 text-green-400" />}
        />
        <SummaryTile
          label="Total decisions"
          value={String(totalDecisions)}
          icon={<Activity className="size-4 text-blue-400" />}
          mono
        />
        <SummaryTile
          label="Health overview"
          value=""
          icon={<ShieldCheck className="size-4 text-green-400" />}
          custom={
            <div className="mt-1 flex items-center gap-2">
              {healthCounts.healthy ? (
                <span className="flex items-center gap-1 text-xs text-green-400">
                  <span className="inline-block size-1.5 rounded-full bg-green-400" />
                  {healthCounts.healthy}
                </span>
              ) : null}
              {healthCounts.moderate ? (
                <span className="flex items-center gap-1 text-xs text-amber-400">
                  <span className="inline-block size-1.5 rounded-full bg-amber-400" />
                  {healthCounts.moderate}
                </span>
              ) : null}
              {healthCounts.critical ? (
                <span className="flex items-center gap-1 text-xs text-red-400">
                  <span className="inline-block size-1.5 rounded-full bg-red-400" />
                  {healthCounts.critical}
                </span>
              ) : null}
              {healthCounts.unknown ? (
                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                  <span className="inline-block size-1.5 rounded-full bg-gray-400" />
                  {healthCounts.unknown}
                </span>
              ) : null}
            </div>
          }
        />
        <SummaryTile
          label="Pending proposals"
          value={String(pendingProposals)}
          icon={<GitCommit className="size-4 text-amber-400" />}
          highlight={pendingProposals > 0}
        />
      </div>

      {calibrationMsg && (
        <p className="text-muted-foreground text-sm">{calibrationMsg}</p>
      )}

      {health.length === 0 ? (
        <Card className="border-border/50 bg-white/5 backdrop-blur-md">
          <CardContent className="py-8 text-center">
            <ShieldQuestion className="mx-auto mb-3 size-8 text-muted-foreground" />
            <p className="text-muted-foreground text-sm">
              No policies configured. Create a policy via the API to start
              tracking calibration metrics.
            </p>
          </CardContent>
        </Card>
      ) : null}

      {/* Active Policies */}
      {activePolicies.length > 0 ? (
        <section className="flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-sm font-medium tracking-tight text-foreground">
            <span className="inline-block size-2 rounded-full bg-green-400" />
            Active Policies
            <Badge variant="secondary" className="ml-1">{activePolicies.length}</Badge>
          </h2>
          <div className="grid gap-4">
            {activePolicies.map((ph) => (
              <PolicyCard
                key={ph.policy.id}
                health={ph}
                token={token}
                onRefresh={loadHealth}
              />
            ))}
          </div>
        </section>
      ) : null}

      {/* Proposed / Disabled Policies */}
      {proposedPolicies.length > 0 ? (
        <section className="flex flex-col gap-3">
          <h2 className="flex items-center gap-2 text-sm font-medium tracking-tight text-muted-foreground">
            <span className="inline-block size-2 rounded-full bg-gray-500" />
            Proposed / Disabled
            <Badge variant="outline" className="ml-1">{proposedPolicies.length}</Badge>
          </h2>
          <div className="grid gap-4">
            {proposedPolicies.map((ph) => (
              <PolicyCard
                key={ph.policy.id}
                health={ph}
                token={token}
                onRefresh={loadHealth}
                isProposed
              />
            ))}
          </div>
        </section>
      ) : null}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Summary Tile (top strip)                                           */
/* ------------------------------------------------------------------ */

function SummaryTile({
  label,
  value,
  icon,
  mono,
  highlight,
  custom,
}: {
  label: string
  value: string
  icon: React.ReactNode
  mono?: boolean
  highlight?: boolean
  custom?: React.ReactNode
}) {
  return (
    <div
      className={`rounded-xl border border-border/50 bg-white/5 p-3 backdrop-blur-md transition-colors duration-200 ${
        highlight ? 'border-amber-500/30 bg-amber-500/5' : ''
      }`}
    >
      <div className="flex items-center gap-2">
        {icon}
        <span className="text-muted-foreground text-xs">{label}</span>
      </div>
      {custom ?? (
        <p
          className={`mt-1 text-lg font-semibold ${mono ? 'font-mono' : ''} ${
            highlight ? 'text-amber-400' : 'text-foreground'
          }`}
        >
          {value}
        </p>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Policy Card                                                        */
/* ------------------------------------------------------------------ */

function PolicyCard({
  health,
  token,
  onRefresh,
  isProposed,
}: {
  health: PolicyHealth
  token: string | null
  onRefresh: () => void
  isProposed?: boolean
}) {
  const { policy, metrics, proposals, history } = health
  const [busyProposal, setBusyProposal] = useState<string | null>(null)
  const [showHistory, setShowHistory] = useState(false)
  const [showConditions, setShowConditions] = useState(false)
  const [simResult, setSimResult] = useState<SimulationResult | null>(null)
  const [simExpanded, setSimExpanded] = useState(false)
  const [simBusy, setSimBusy] = useState(false)
  const [simError, setSimError] = useState<string | null>(null)

  const healthLevel = getHealthLevel(metrics)
  const config = HEALTH_CONFIG[healthLevel]
  const HealthIcon = config.icon
  const pendingProposals = proposals?.filter((p) => p.status === 'pending') ?? []

  const hasConditions =
    (policy.rules.auto_approve_conditions?.length ?? 0) > 0 ||
    (policy.rules.review_conditions?.length ?? 0) > 0 ||
    (policy.rules.block_conditions?.length ?? 0) > 0

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
      setSimExpanded(true)
    } catch (e) {
      setSimError(String(e))
    } finally {
      setSimBusy(false)
    }
  }

  return (
    <Card
      className={`overflow-hidden transition-all duration-200 ${
        isProposed
          ? 'border-dashed border-border/40 bg-white/[0.02] opacity-75 backdrop-blur-md hover:opacity-100'
          : `border-border/50 bg-white/5 backdrop-blur-md ${config.border}`
      }`}
    >
      {/* Colored health stripe at top of card */}
      {!isProposed && (
        <div
          className={`h-0.5 w-full transition-colors duration-300 ${config.dot}`}
        />
      )}
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            {/* Health icon with glow */}
            <div
              className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${config.bg} ${config.glow} transition-shadow duration-300`}
            >
              <HealthIcon className={`size-5 ${config.text}`} />
            </div>
            <div className="flex flex-col gap-0.5">
              <div className="flex items-center gap-2">
                <CardTitle className="text-base">{policy.name}</CardTitle>
                {isProposed ? (
                  <Badge
                    variant="outline"
                    className="border-dashed border-gray-500/50 text-muted-foreground"
                  >
                    Proposed
                  </Badge>
                ) : (
                  <Badge
                    variant="outline"
                    className={`border-green-500/40 text-green-400`}
                  >
                    Active
                  </Badge>
                )}
              </div>
              {policy.description ? (
                <p className="text-muted-foreground text-xs leading-relaxed">
                  {policy.description}
                </p>
              ) : null}
            </div>
          </div>

          {/* Health status badge */}
          <HealthStatusBadge level={healthLevel} />
        </div>
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

        {/* Condition Visualization */}
        {hasConditions ? (
          <ExpandableSection
            label="Conditions"
            count={
              (policy.rules.auto_approve_conditions?.length ?? 0) +
              (policy.rules.review_conditions?.length ?? 0) +
              (policy.rules.block_conditions?.length ?? 0)
            }
            expanded={showConditions}
            onToggle={() => setShowConditions(!showConditions)}
          >
            <ConditionVisualizer rules={policy.rules} />
          </ExpandableSection>
        ) : null}

        {/* Proposals with resolution actions */}
        {pendingProposals.length > 0 ? (
          <div className="flex flex-col gap-2">
            <h4 className="flex items-center gap-2 text-sm font-medium">
              <CircleDot className="size-3.5 text-amber-400" />
              Proposed Adjustments
              <Badge className="bg-amber-800/50 text-amber-200">
                {pendingProposals.length}
              </Badge>
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
            <Activity className="mr-1.5 size-3.5" />
            {simBusy ? 'Simulating...' : 'Preview rules (last 30d)'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setShowHistory(!showHistory)}
          >
            <GitCommit className="mr-1.5 size-3.5" />
            {showHistory ? 'Hide history' : 'Change history'}
          </Button>
        </div>

        {/* Simulation Results — Expandable Panel */}
        {simError && (
          <p className="text-destructive text-sm">{simError}</p>
        )}
        {simResult && (
          <ExpandableSection
            label="Simulation Preview (last 30 days)"
            expanded={simExpanded}
            onToggle={() => setSimExpanded(!simExpanded)}
            accent="blue"
          >
            <SimulationPanel result={simResult} />
          </ExpandableSection>
        )}

        {/* Change History Timeline */}
        {showHistory && history.length > 0 ? (
          <PolicyTimeline events={history.slice(0, 10)} />
        ) : null}
        {showHistory && history.length === 0 ? (
          <p className="text-muted-foreground text-sm">No change history.</p>
        ) : null}
      </CardContent>
    </Card>
  )
}

/* ------------------------------------------------------------------ */
/*  Health Status Badge (prominent indicator)                          */
/* ------------------------------------------------------------------ */

function HealthStatusBadge({ level }: { level: HealthLevel }) {
  const config = HEALTH_CONFIG[level]

  return (
    <div
      className={`flex items-center gap-2 rounded-lg border px-3 py-1.5 ${config.border} ${config.bg} transition-all duration-200`}
    >
      <div className="relative flex items-center justify-center">
        <div
          className={`size-2.5 rounded-full ${config.dot} ${
            level === 'critical' ? 'animate-pulse' : ''
          }`}
        />
        {level === 'critical' && (
          <div className="absolute size-2.5 animate-ping rounded-full bg-red-400/50" />
        )}
      </div>
      <span className={`text-xs font-medium ${config.text}`}>
        {config.label}
      </span>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Expandable Section                                                 */
/* ------------------------------------------------------------------ */

function ExpandableSection({
  label,
  count,
  expanded,
  onToggle,
  accent,
  children,
}: {
  label: string
  count?: number
  expanded: boolean
  onToggle: () => void
  accent?: 'blue' | 'default'
  children: React.ReactNode
}) {
  const accentBorder =
    accent === 'blue' ? 'border-blue-500/20' : 'border-border/50'
  const accentBg = accent === 'blue' ? 'bg-blue-500/5' : 'bg-white/[0.02]'

  return (
    <div
      className={`overflow-hidden rounded-lg border ${accentBorder} ${accentBg} transition-all duration-200`}
    >
      <button
        type="button"
        className="flex w-full items-center gap-2 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-white/5"
        onClick={onToggle}
        aria-expanded={expanded}
      >
        {expanded ? (
          <ChevronDown className="size-3.5 text-muted-foreground transition-transform duration-200" />
        ) : (
          <ChevronRight className="size-3.5 text-muted-foreground transition-transform duration-200" />
        )}
        <span className="text-sm font-medium">{label}</span>
        {count != null && (
          <Badge variant="secondary" className="ml-auto text-xs">
            {count}
          </Badge>
        )}
      </button>
      <div
        className={`grid transition-all duration-200 ease-in-out ${
          expanded ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0'
        }`}
      >
        <div className="overflow-hidden">
          <div className="border-t border-border/30 px-3 pb-3 pt-2">
            {children}
          </div>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Condition Visualizer                                               */
/* ------------------------------------------------------------------ */

function ConditionVisualizer({ rules }: { rules: Rules }) {
  const sections: {
    key: string
    label: string
    conditions: Condition[]
    color: string
    dot: string
    bg: string
    border: string
  }[] = [
    {
      key: 'auto_approve',
      label: 'Auto-approve when',
      conditions: rules.auto_approve_conditions ?? [],
      color: 'text-green-400',
      dot: 'bg-green-400',
      bg: 'bg-green-500/10',
      border: 'border-green-500/20',
    },
    {
      key: 'review',
      label: 'Require review when',
      conditions: rules.review_conditions ?? [],
      color: 'text-amber-400',
      dot: 'bg-amber-400',
      bg: 'bg-amber-500/10',
      border: 'border-amber-500/20',
    },
    {
      key: 'block',
      label: 'Block when',
      conditions: rules.block_conditions ?? [],
      color: 'text-red-400',
      dot: 'bg-red-400',
      bg: 'bg-red-500/10',
      border: 'border-red-500/20',
    },
  ]

  return (
    <div className="flex flex-col gap-3">
      {sections
        .filter((s) => s.conditions.length > 0)
        .map((section) => (
          <div key={section.key} className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <span
                className={`inline-block size-1.5 rounded-full ${section.dot}`}
              />
              <span className={`text-xs font-medium ${section.color}`}>
                {section.label}
              </span>
            </div>
            <div className="flex flex-wrap gap-1.5 pl-3.5">
              {section.conditions.map((c, i) => (
                <span
                  key={`${c.field}-${i}`}
                  className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs ${section.border} ${section.bg}`}
                >
                  <span className="font-mono font-medium text-foreground">
                    {c.label ?? c.field}
                  </span>
                  <span className="text-muted-foreground">{c.operator}</span>
                  <span className="font-mono text-foreground">
                    {String(c.value)}
                  </span>
                </span>
              ))}
            </div>
          </div>
        ))}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Simulation Panel                                                   */
/* ------------------------------------------------------------------ */

function SimulationPanel({ result }: { result: SimulationResult }) {
  const { summary } = result
  const total = summary.total_tickets || 1

  /* Stacked bar data */
  const approvePercent = (summary.would_auto_approve / total) * 100
  const reviewPercent = (summary.would_require_review / total) * 100
  const blockPercent = (summary.would_block / total) * 100

  return (
    <div className="flex flex-col gap-4">
      {/* Visual bar */}
      <div className="flex flex-col gap-1.5">
        <div className="flex h-3 overflow-hidden rounded-full bg-muted/30">
          <div
            className="bg-green-500/80 transition-all duration-500"
            style={{ width: `${approvePercent}%` }}
            title={`Auto-approve: ${summary.would_auto_approve}`}
          />
          <div
            className="bg-amber-500/80 transition-all duration-500"
            style={{ width: `${reviewPercent}%` }}
            title={`Review: ${summary.would_require_review}`}
          />
          <div
            className="bg-red-500/80 transition-all duration-500"
            style={{ width: `${blockPercent}%` }}
            title={`Block: ${summary.would_block}`}
          />
        </div>
        <div className="flex flex-wrap gap-4 text-xs">
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-2 rounded-sm bg-green-500/80" />
            <span className="text-muted-foreground">Auto-approve:</span>
            <span className="font-mono font-medium text-green-400">
              {summary.would_auto_approve}
            </span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-2 rounded-sm bg-amber-500/80" />
            <span className="text-muted-foreground">Review:</span>
            <span className="font-mono font-medium text-amber-400">
              {summary.would_require_review}
            </span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-2 rounded-sm bg-red-500/80" />
            <span className="text-muted-foreground">Block:</span>
            <span className="font-mono font-medium text-red-400">
              {summary.would_block}
            </span>
          </span>
        </div>
      </div>

      {/* Stats row */}
      <div className="grid grid-cols-3 gap-3">
        <div className="rounded-lg bg-muted/20 p-2.5">
          <p className="text-muted-foreground text-xs">Tickets simulated</p>
          <p className="font-mono text-sm font-medium">{summary.total_tickets}</p>
        </div>
        <div className="rounded-lg bg-muted/20 p-2.5">
          <p className="text-muted-foreground text-xs">Decisions changed</p>
          <p className="font-mono text-sm font-medium">
            {summary.changed_decisions}
          </p>
        </div>
        <div className="rounded-lg bg-muted/20 p-2.5">
          <p className="text-muted-foreground text-xs">Change rate</p>
          <p className="font-mono text-sm font-medium">
            {(summary.change_rate * 100).toFixed(1)}%
          </p>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Policy Change Timeline                                             */
/* ------------------------------------------------------------------ */

function PolicyTimeline({ events }: { events: PolicyChangeEvent[] }) {
  const changeTypeStyles: Record<string, { dot: string; badge: string }> = {
    created: {
      dot: 'bg-green-400',
      badge: 'border-green-500/30 bg-green-500/10 text-green-400',
    },
    updated: {
      dot: 'bg-blue-400',
      badge: 'border-blue-500/30 bg-blue-500/10 text-blue-400',
    },
    enabled: {
      dot: 'bg-green-400',
      badge: 'border-green-500/30 bg-green-500/10 text-green-400',
    },
    disabled: {
      dot: 'bg-gray-400',
      badge: 'border-gray-500/30 bg-gray-500/10 text-gray-400',
    },
    broadened: {
      dot: 'bg-amber-400',
      badge: 'border-amber-500/30 bg-amber-500/10 text-amber-400',
    },
    tightened: {
      dot: 'bg-red-400',
      badge: 'border-red-500/30 bg-red-500/10 text-red-400',
    },
  }

  const defaultStyle = {
    dot: 'bg-muted-foreground',
    badge: 'border-border/50 bg-muted/20 text-muted-foreground',
  }

  return (
    <div className="flex flex-col gap-0">
      <h4 className="mb-2 flex items-center gap-2 text-sm font-medium">
        <GitCommit className="size-3.5 text-muted-foreground" />
        Change History
      </h4>
      <div className="relative ml-3">
        {/* Vertical timeline line */}
        <div className="absolute bottom-0 left-0 top-0 w-px bg-border/50" />

        {events.map((event, index) => {
          const style = changeTypeStyles[event.change_type] ?? defaultStyle
          const isLast = index === events.length - 1

          return (
            <div
              key={event.id}
              className={`relative flex gap-4 pl-6 ${isLast ? 'pb-0' : 'pb-4'}`}
            >
              {/* Timeline dot */}
              <div
                className={`absolute left-0 top-1.5 z-10 size-2 -translate-x-1/2 rounded-full ${style.dot} ring-2 ring-background`}
              />

              {/* Content */}
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <div className="flex items-center gap-2">
                  <span
                    className={`inline-flex items-center rounded-md border px-1.5 py-0.5 text-xs font-medium ${style.badge}`}
                  >
                    {event.change_type}
                  </span>
                  <span className="text-muted-foreground text-xs font-mono">
                    {formatDate(event.created_at)}
                  </span>
                  <span className="text-muted-foreground/60 text-xs font-mono">
                    {formatTime(event.created_at)}
                  </span>
                </div>
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {event.notes || 'No notes'}
                </p>
                {event.actor_id ? (
                  <p className="text-xs text-muted-foreground/60">
                    by {event.actor_id}
                  </p>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Metric Tile                                                        */
/* ------------------------------------------------------------------ */

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

  const bgClass =
    variant === 'success'
      ? 'bg-green-500/5'
      : variant === 'warning'
        ? 'bg-amber-500/5'
        : variant === 'danger'
          ? 'bg-red-500/5'
          : 'bg-muted/30'

  return (
    <div className={`rounded-lg ${bgClass} p-3 transition-colors duration-200`}>
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className={`text-lg font-semibold ${colorClass}`}>{value}</p>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Proposal Card                                                      */
/* ------------------------------------------------------------------ */

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
      className={`rounded-lg border p-3 transition-colors duration-200 ${
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
              {formatDate(proposal.created_at)}
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
