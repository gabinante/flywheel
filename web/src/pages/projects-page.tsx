import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  Activity,
  CircleDot,
  FolderGit2,
  GitBranch,
  Layers,
  Plus,
  Sparkles,
  TicketCheck,
} from 'lucide-react'

import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type Ticket = components['schemas']['Ticket']
type WorkStream = components['schemas']['WorkStream']

/** Quick stats fetched per project. */
interface ProjectStats {
  totalTickets: number
  activeTickets: number
  activeStreams: number
}

/** Deterministic hue from a string. */
function stringToHue(s: string): number {
  let hash = 0
  for (let i = 0; i < s.length; i++) {
    hash = s.charCodeAt(i) + ((hash << 5) - hash)
  }
  return Math.abs(hash) % 360
}

const ACCENT_HUES = [145, 160, 130, 175, 120, 155, 140, 170]

function ProjectAvatar({ name, id }: { name: string; id: string }) {
  const hue = ACCENT_HUES[stringToHue(id) % ACCENT_HUES.length]
  const initial = (name || id).charAt(0).toUpperCase()
  return (
    <div
      className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl text-lg font-bold"
      style={{
        background: `oklch(0.35 0.08 ${hue} / 60%)`,
        color: `oklch(0.82 0.14 ${hue})`,
      }}
    >
      {initial}
    </div>
  )
}

/** Status indicator with semantic colors. */
function StatusIndicator({ status }: { status: string | undefined }) {
  const isActive = status === 'active' || !status
  return (
    <span className="inline-flex items-center gap-1.5">
      <CircleDot
        className={`h-3 w-3 ${
          isActive ? 'text-primary' : 'text-muted-foreground/50'
        }`}
      />
      <span
        className={`text-xs font-medium ${
          isActive ? 'text-primary' : 'text-muted-foreground/60'
        }`}
      >
        {isActive ? 'Active' : 'Closed'}
      </span>
    </span>
  )
}

/** Stat pill for displaying a count with an icon. */
function StatPill({
  icon: Icon,
  value,
  label,
  loading,
}: {
  icon: React.ComponentType<{ className?: string }>
  value: number | undefined
  label: string
  loading: boolean
}) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground" title={label}>
      <Icon className="h-3.5 w-3.5" />
      {loading ? (
        <span className="inline-block h-3 w-8 animate-pulse rounded bg-white/[0.06]" />
      ) : (
        <span>{value ?? 0}</span>
      )}
    </span>
  )
}

function EmptyProjectsState({ orgId }: { orgId: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: [0.25, 0.1, 0.25, 1] }}
      className="flex flex-col items-center justify-center gap-6 py-16 text-center"
    >
      <div className="relative">
        <div className="flex h-20 w-20 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04] backdrop-blur-md">
          <FolderGit2 className="h-10 w-10 text-primary/60" />
        </div>
        <motion.div
          className="absolute -right-1 -top-1"
          animate={{ rotate: [0, 15, -15, 0], scale: [1, 1.2, 1] }}
          transition={{ duration: 2.5, repeat: Infinity, repeatDelay: 3 }}
        >
          <Sparkles className="h-5 w-5 text-primary/40" />
        </motion.div>
      </div>
      <div className="flex max-w-sm flex-col gap-2">
        <h2 className="text-lg font-semibold tracking-tight">
          No projects yet
        </h2>
        <p className="text-sm text-muted-foreground leading-relaxed">
          Projects are where your code lives. Each project has its own tickets,
          work streams, and agents. Create one to get started.
        </p>
      </div>
      <Link
        to={`/orgs/${orgId}/projects/new`}
        className="inline-flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/10 px-5 py-2.5 text-sm font-medium text-primary backdrop-blur-sm transition-all duration-200 hover:bg-primary/20 hover:border-primary/30"
      >
        <Plus className="h-4 w-4" />
        Create project
      </Link>
    </motion.div>
  )
}

export function ProjectsPage() {
  const { orgId } = useParams<{ orgId: string }>()
  const { client } = useAuth()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [orgName, setOrgName] = useState<string | null>(null)
  const [stats, setStats] = useState<Record<string, ProjectStats>>({})
  const [statsLoading, setStatsLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)

  // Fetch org name and projects
  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    ;(async () => {
      const orgRes = await client.GET('/orgs/{orgID}', {
        params: { path: { orgID: orgId } },
      })
      if (cancelled) return
      if (orgRes.response.ok && orgRes.data) {
        setOrgName(orgRes.data.name ?? orgRes.data.slug ?? orgId)
      } else {
        setOrgName(orgId)
      }

      const { data, error, response } = await client.GET('/orgs/{orgID}/projects', {
        params: { path: { orgID: orgId }, query: { status: 'all' } },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setProjects([])
        return
      }
      setErr(null)
      setProjects(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client, orgId])

  // Once projects are loaded, fetch ticket and work stream counts
  useEffect(() => {
    if (!projects || projects.length === 0) {
      setStatsLoading(false)
      return
    }
    let cancelled = false
    setStatsLoading(true)
    ;(async () => {
      const result: Record<string, ProjectStats> = {}
      await Promise.all(
        projects.map(async (p) => {
          if (!p.id) return
          const [ticketsRes, streamsRes] = await Promise.all([
            client.GET('/projects/{projectID}/tickets', {
              params: { path: { projectID: p.id } },
            }),
            client.GET('/projects/{projectID}/work-streams', {
              params: { path: { projectID: p.id }, query: { status: 'active' } },
            }),
          ])
          if (cancelled) return
          const tickets = (ticketsRes.response.ok ? ticketsRes.data : []) as Ticket[]
          const streams = (streamsRes.response.ok ? streamsRes.data : []) as WorkStream[]
          const activeStates = new Set([
            'pending',
            'claimed',
            'executing',
            'awaiting_review',
            'blocked',
            'needs_human',
          ])
          result[p.id] = {
            totalTickets: tickets.length,
            activeTickets: tickets.filter((t) => t.state && activeStates.has(t.state)).length,
            activeStreams: streams.length,
          }
        }),
      )
      if (!cancelled) {
        setStats(result)
        setStatsLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [client, projects])

  if (!orgId) {
    return <p className="text-destructive text-sm">Missing org id.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!projects) {
    return <ListPageSkeleton />
  }

  return (
    <div className="flex flex-col gap-6">
      <motion.div
        initial={{ opacity: 0, y: -8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
      >
        <p className="text-muted-foreground text-xs">
          <Link to="/orgs" className="hover:underline">
            Organizations
          </Link>
          <span className="px-1">/</span>
        </p>
        <h1 className="text-xl font-semibold tracking-tight">
          Projects{orgName ? ` — ${orgName}` : ''}
        </h1>
      </div>
      <ul className="flex flex-col gap-3">
        {projects.map((p) => (
          <li key={p.id}>
            <Link to={`/orgs/${orgId}/projects/${p.id}`}>
              <Card className="transition-colors hover:bg-white/[0.06]">
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>{p.name ?? p.slug ?? p.id}</CardTitle>
                    {p.status ? (
                      <Badge variant="outline">{p.status}</Badge>
                    ) : null}
                    {p.dispatch_enabled !== false ? (
                      <Badge variant="default" className="bg-emerald-600/80 text-xs">
                        Dispatch on
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="text-muted-foreground text-xs">
                        Dispatch off
                      </Badge>
                    )}
                  </div>
                  <CardDescription className="font-mono text-xs">
                    {p.id}
                  </CardDescription>
                </CardHeader>
              </Card>
            </Link>
          </li>
        ))}
      </ul>
      {projects.length === 0 ? (
        <EmptyProjectsState orgId={orgId} />
      ) : (
        <StaggerList className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map((p) => {
            const name = p.name ?? p.slug ?? p.id ?? ''
            const id = p.id ?? ''
            const projectStats = stats[id]
            const isStatsLoading = statsLoading && !projectStats
            return (
              <StaggerItem key={id}>
                <Link to={`/orgs/${orgId}/projects/${id}`} className="block h-full">
                  <Card className="group h-full hover:bg-white/[0.06]">
                    <CardHeader>
                      <div className="flex items-start gap-3">
                        <ProjectAvatar name={name} id={id} />
                        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                          <div className="flex flex-wrap items-center gap-2">
                            <CardTitle className="text-base font-semibold truncate">
                              {name}
                            </CardTitle>
                          </div>
                          <div className="flex flex-wrap items-center gap-2">
                            <StatusIndicator status={p.status} />
                            {p.tech_stack && p.tech_stack.length > 0 ? (
                              p.tech_stack.slice(0, 3).map((tech) => (
                                <Badge
                                  key={tech}
                                  variant="outline"
                                  className="text-[10px] px-1.5 py-0"
                                >
                                  {tech}
                                </Badge>
                              ))
                            ) : null}
                          </div>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <div className="flex flex-col gap-2.5">
                        {/* Stats row */}
                        <div className="flex items-center gap-4">
                          <StatPill
                            icon={TicketCheck}
                            value={projectStats?.totalTickets}
                            label="Total tickets"
                            loading={isStatsLoading}
                          />
                          <StatPill
                            icon={Activity}
                            value={projectStats?.activeTickets}
                            label="Active tickets"
                            loading={isStatsLoading}
                          />
                          <StatPill
                            icon={Layers}
                            value={projectStats?.activeStreams}
                            label="Active streams"
                            loading={isStatsLoading}
                          />
                        </div>

                        {/* Repo and branch info */}
                        {p.repo_url || p.default_branch ? (
                          <div className="flex items-center gap-3 text-xs text-muted-foreground/70">
                            {p.repo_url ? (
                              <span className="inline-flex items-center gap-1 truncate">
                                <FolderGit2 className="h-3 w-3 shrink-0" />
                                <span className="truncate font-mono">
                                  {p.repo_url.replace(/^https?:\/\/(www\.)?github\.com\//, '')}
                                </span>
                              </span>
                            ) : null}
                            {p.default_branch ? (
                              <span className="inline-flex items-center gap-1 shrink-0">
                                <GitBranch className="h-3 w-3" />
                                <span className="font-mono">{p.default_branch}</span>
                              </span>
                            ) : null}
                          </div>
                        ) : null}
                      </div>
                    </CardContent>
                  </Card>
                </Link>
              </StaggerItem>
            )
          })}
        </StaggerList>
      )}
    </div>
  )
}
