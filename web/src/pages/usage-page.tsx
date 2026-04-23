import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import type { ApexOptions } from 'apexcharts'
import ReactApexChart from 'react-apexcharts'
import { AlertTriangle, Bot, Coins, Cpu, PieChart, TrendingUp } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { useAuth } from '@/contexts/use-auth'
import {
  getProjectUsage,
  type UsageBreakdown,
  type UsageResponse,
} from '@/lib/api/usage-client'

const DAY_WINDOWS = [7, 30, 90] as const
const CHART_COLORS = ['#14b8a6', '#f97316', '#22c55e', '#8b5cf6', '#38bdf8', '#f43f5e', '#facc15']
const chartFont = "'Geist Variable', sans-serif"
const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 })
const dollars = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 2 })
const integers = new Intl.NumberFormat('en-US')

function formatTokens(value: number) {
  if (!Number.isFinite(value)) return '0'
  if (Math.abs(value) < 1000) return `${integers.format(Math.round(value))} tok`
  return `${compact.format(value)} tok`
}

function formatPercent(value: number) {
  return `${Math.round(value * 100)}%`
}

function shortDate(isoDate: string) {
  return new Date(`${isoDate}T00:00:00Z`).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
}

function labelForTaskType(label: string) {
  switch (label) {
    case 'claude':
      return 'Claude'
    case 'conflict_resolution':
      return 'Conflict resolution'
    case 'codex':
      return 'Codex'
    case 'implementation':
      return 'Implementation'
    case 'investigation':
      return 'Investigation'
    case 'openai':
      return 'OpenAI'
    case 'orchestrator':
      return 'Orchestrator'
    case 'planning':
      return 'Planning'
    case 'deployment':
      return 'Deployment'
    case 'review':
      return 'Review'
    case 'reviewer':
      return 'Reviewer'
    case 'unknown':
      return 'Unknown'
    case 'worker':
      return 'Worker'
    default:
      return label
        .replaceAll('_', ' ')
        .replace(/\b\w/g, (match) => match.toUpperCase())
  }
}

function StatCard({
  icon: Icon,
  label,
  value,
  detail,
}: {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: string
  detail: string
}) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardContent className="flex items-start gap-4 px-5 py-5">
        <div className="rounded-2xl border border-white/10 bg-white/[0.06] p-3">
          <Icon className="size-5 text-foreground" />
        </div>
        <div className="min-w-0">
          <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">{label}</p>
          <p className="mt-2 text-2xl font-semibold tracking-tight text-foreground">{value}</p>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{detail}</p>
        </div>
      </CardContent>
    </Card>
  )
}

function BreakdownList({
  title,
  description,
  items,
}: {
  title: string
  description: string
  items: UsageBreakdown[]
}) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="text-sm">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 pt-5">
        {items.length === 0 ? (
          <p className="text-sm text-muted-foreground">No usage recorded in this window.</p>
        ) : (
          items.slice(0, 6).map((item) => (
            <div key={item.key} className="space-y-2">
              <div className="flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {labelForTaskType(item.label)}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {formatTokens(item.total_tokens)} · {item.calls} runs
                  </p>
                </div>
                <div className="text-right">
                  <p className="text-sm font-medium text-foreground">{dollars.format(item.cost_dollars)}</p>
                  <p className="text-xs text-muted-foreground">{formatPercent(item.share)} of tokens</p>
                </div>
              </div>
              <Progress value={item.share * 100} />
            </div>
          ))
        )}
      </CardContent>
    </Card>
  )
}

export function UsagePage() {
  const { projectId } = useParams<{ orgId: string; projectId: string }>()
  const { token } = useAuth()
  const [days, setDays] = useState<number>(30)
  const [usage, setUsage] = useState<UsageResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!projectId || !token) {
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    ;(async () => {
      const { data, error: requestError } = await getProjectUsage(token, projectId, days)
      if (cancelled) return
      setUsage(data)
      setError(requestError)
      setLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [days, projectId, token])

  const daily = usage?.daily ?? []
  const taskType = usage?.by_task_type ?? []
  const workerGroups = usage?.by_worker_group ?? []
  const apiUsage = usage?.by_api ?? []
  const models = usage?.by_model ?? []
  const summary = usage?.summary
  const budgetStatus = usage?.budget_status

  const tokenSeries = [
    {
      name: 'Input tokens',
      data: daily.map((point) => point.input_tokens),
    },
    {
      name: 'Output tokens',
      data: daily.map((point) => point.output_tokens),
    },
  ]
  const tokenOptions: ApexOptions = {
    chart: {
      type: 'area',
      background: 'transparent',
      toolbar: { show: false },
      fontFamily: chartFont,
      foreColor: '#94a3b8',
    },
    colors: ['#14b8a6', '#f97316'],
    dataLabels: { enabled: false },
    fill: {
      type: 'gradient',
      gradient: {
        shadeIntensity: 0.4,
        opacityFrom: 0.35,
        opacityTo: 0.05,
      },
    },
    grid: {
      borderColor: 'rgba(255,255,255,0.08)',
      strokeDashArray: 4,
    },
    legend: { position: 'top', labels: { colors: '#cbd5e1' } },
    stroke: { curve: 'smooth', width: 3 },
    tooltip: {
      theme: 'dark',
      y: {
        formatter: (value) => formatTokens(Number(value)),
      },
    },
    xaxis: {
      categories: daily.map((point) => shortDate(point.date)),
      axisBorder: { color: 'rgba(255,255,255,0.08)' },
      axisTicks: { color: 'rgba(255,255,255,0.08)' },
      labels: { style: { colors: '#94a3b8' } },
    },
    yaxis: {
      labels: {
        formatter: (value) => compact.format(Number(value)),
        style: { colors: '#94a3b8' },
      },
    },
  }

  const taskTypeOptions: ApexOptions = {
    chart: {
      type: 'donut',
      background: 'transparent',
      fontFamily: chartFont,
      foreColor: '#cbd5e1',
    },
    colors: CHART_COLORS,
    dataLabels: { enabled: false },
    labels: taskType.map((item) => labelForTaskType(item.label)),
    legend: {
      position: 'bottom',
      labels: { colors: '#cbd5e1' },
    },
    tooltip: {
      theme: 'dark',
      y: {
        formatter: (value) => formatTokens(Number(value)),
      },
    },
    plotOptions: {
      pie: {
        donut: {
          size: '62%',
          labels: {
            show: true,
            total: {
              show: true,
              label: 'Task mix',
              color: '#94a3b8',
              formatter: () => formatTokens(summary?.total_tokens ?? 0),
            },
            value: {
              color: '#f8fafc',
              formatter: (value) => formatTokens(Number(value)),
            },
          },
        },
      },
    },
  }

  const apiOptions: ApexOptions = {
    chart: {
      type: 'bar',
      background: 'transparent',
      toolbar: { show: false },
      fontFamily: chartFont,
      foreColor: '#94a3b8',
    },
    colors: ['#38bdf8'],
    dataLabels: { enabled: false },
    grid: {
      borderColor: 'rgba(255,255,255,0.08)',
      strokeDashArray: 4,
    },
    plotOptions: {
      bar: {
        borderRadius: 6,
        horizontal: true,
      },
    },
    tooltip: {
      theme: 'dark',
      y: {
        formatter: (value) => formatTokens(Number(value)),
      },
    },
    xaxis: {
      categories: apiUsage.map((item) => labelForTaskType(item.label)),
      labels: {
        formatter: (value) => compact.format(Number(value)),
        style: { colors: '#94a3b8' },
      },
    },
    yaxis: {
      labels: { style: { colors: '#cbd5e1' } },
    },
  }

  const workerOptions: ApexOptions = {
    chart: {
      type: 'bar',
      background: 'transparent',
      toolbar: { show: false },
      fontFamily: chartFont,
      foreColor: '#94a3b8',
    },
    colors: ['#8b5cf6'],
    dataLabels: { enabled: false },
    grid: {
      borderColor: 'rgba(255,255,255,0.08)',
      strokeDashArray: 4,
    },
    plotOptions: {
      bar: {
        borderRadius: 6,
        columnWidth: '42%',
      },
    },
    tooltip: {
      theme: 'dark',
      y: {
        formatter: (value) => formatTokens(Number(value)),
      },
    },
    xaxis: {
      categories: workerGroups.map((item) => labelForTaskType(item.label)),
      labels: { style: { colors: '#cbd5e1' } },
    },
    yaxis: {
      labels: {
        formatter: (value) => compact.format(Number(value)),
        style: { colors: '#94a3b8' },
      },
    },
  }

  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">Usage</Badge>
            <Badge variant="secondary">Estimated session accounting</Badge>
          </div>
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">Model usage</h1>
            <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
              Track token burn over time, spot which APIs are active, and see whether orchestration,
              implementation, or conflict resolution is driving most of the load.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          {DAY_WINDOWS.map((window) => (
            <Button
              key={window}
              type="button"
              variant={days === window ? 'default' : 'outline'}
              size="sm"
              onClick={() => setDays(window)}
            >
              {window}d
            </Button>
          ))}
        </div>
      </div>

      {usage?.estimated ? (
        <Card className="border-amber-500/20 bg-amber-500/5 backdrop-blur-md">
          <CardContent className="flex items-start gap-3 px-5 py-4">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-300" />
            <div className="space-y-1">
              <p className="text-sm font-medium text-amber-100">Estimated usage</p>
              <p className="text-sm leading-relaxed text-amber-100/75">
                CLI-backed workers are currently measured from session prompt and response size, so the
                charts are best for relative trends and runaway-loop detection rather than exact billing.
              </p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {error ? (
        <Card className="border-destructive/30 bg-destructive/5">
          <CardContent className="px-5 py-4 text-sm text-destructive">{error}</CardContent>
        </Card>
      ) : null}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard
          icon={TrendingUp}
          label="Total tokens"
          value={loading ? '...' : formatTokens(summary?.total_tokens ?? 0)}
          detail={`${integers.format(summary?.calls ?? 0)} recorded runs in the selected window`}
        />
        <StatCard
          icon={Bot}
          label="Input tokens"
          value={loading ? '...' : formatTokens(summary?.input_tokens ?? 0)}
          detail={loading ? 'Loading window' : `${formatTokens(summary?.output_tokens ?? 0)} came back out`}
        />
        <StatCard
          icon={Coins}
          label="Estimated cost"
          value={loading ? '...' : dollars.format(summary?.cost_dollars ?? 0)}
          detail="Provider pricing is applied where known and conservatively estimated otherwise."
        />
        <StatCard
          icon={Cpu}
          label="API mix"
          value={loading ? '...' : apiUsage[0] ? labelForTaskType(apiUsage[0].label) : 'Idle'}
          detail={loading ? 'Loading mix' : apiUsage[0] ? `${formatPercent(apiUsage[0].share)} of window tokens` : 'No usage recorded'}
        />
      </div>

      {budgetStatus ? (
        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <CardTitle className="text-sm">Budget watch</CardTitle>
            <CardDescription>
              Current month budget status for this project.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 pt-5">
            <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
              <span className="text-muted-foreground">
                {dollars.format((budgetStatus.spent_millicent ?? 0) / 100000)} spent
              </span>
              <span className="text-muted-foreground">
                {dollars.format((budgetStatus.projected_millicent ?? 0) / 100000)} projected month-end
              </span>
            </div>
            <Progress
              value={(budgetStatus.used_fraction ?? 0) * 100}
              barClassName={
                budgetStatus.over_budget
                  ? 'bg-rose-500/80'
                  : budgetStatus.warning
                    ? 'bg-amber-400/80'
                    : 'bg-emerald-500/80'
              }
            />
          </CardContent>
        </Card>
      ) : null}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)]">
        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <CardTitle className="text-sm">Usage over time</CardTitle>
            <CardDescription>
              Daily input vs output token flow across the selected window.
            </CardDescription>
          </CardHeader>
          <CardContent className="pt-5">
            {loading ? (
              <div className="h-[320px] animate-pulse rounded-3xl bg-white/[0.04]" />
            ) : (
              <ReactApexChart options={tokenOptions} series={tokenSeries} type="area" height={320} />
            )}
          </CardContent>
        </Card>

        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <div className="flex items-center gap-2">
              <PieChart className="size-4 text-muted-foreground" />
              <CardTitle className="text-sm">Task type contribution</CardTitle>
            </div>
            <CardDescription>
              Orchestrator, implementation, review, and conflict resolution share of token usage.
            </CardDescription>
          </CardHeader>
          <CardContent className="pt-5">
            {loading ? (
              <div className="h-[320px] animate-pulse rounded-3xl bg-white/[0.04]" />
            ) : (
              <ReactApexChart
                options={taskTypeOptions}
                series={taskType.map((item) => item.total_tokens)}
                type="donut"
                height={320}
              />
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <CardTitle className="text-sm">API usage</CardTitle>
            <CardDescription>
              Token volume by API family such as Claude, Codex, or OpenAI-native runs.
            </CardDescription>
          </CardHeader>
          <CardContent className="pt-5">
            {loading ? (
              <div className="h-[320px] animate-pulse rounded-3xl bg-white/[0.04]" />
            ) : (
              <ReactApexChart
                options={apiOptions}
                series={[{ name: 'Tokens', data: apiUsage.map((item) => item.total_tokens) }]}
                type="bar"
                height={320}
              />
            )}
          </CardContent>
        </Card>

        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <CardTitle className="text-sm">Worker lane usage</CardTitle>
            <CardDescription>
              Split between orchestrator, reviewer, and worker-class sessions.
            </CardDescription>
          </CardHeader>
          <CardContent className="pt-5">
            {loading ? (
              <div className="h-[320px] animate-pulse rounded-3xl bg-white/[0.04]" />
            ) : (
              <ReactApexChart
                options={workerOptions}
                series={[{ name: 'Tokens', data: workerGroups.map((item) => item.total_tokens) }]}
                type="bar"
                height={320}
              />
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <BreakdownList
          title="Task breakdown"
          description="Which task types are consuming the most usage in this window."
          items={taskType}
        />

        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="border-b border-white/10 pb-4">
            <CardTitle className="text-sm">Top models</CardTitle>
            <CardDescription>
              Highest-usage models or CLI identities recorded in the selected window.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 pt-5">
            {models.length === 0 ? (
              <p className="text-sm text-muted-foreground">No usage recorded in this window.</p>
            ) : (
              models.slice(0, 8).map((item) => (
                <div key={item.key} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-foreground">{item.label}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {formatTokens(item.total_tokens)} · {item.calls} runs
                      </p>
                    </div>
                    <Badge variant="outline">{dollars.format(item.cost_dollars)}</Badge>
                  </div>
                  <Progress value={item.share * 100} className="mt-3" />
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
