import { useActivityVersion } from '@/contexts/use-activity'
import { useDraft } from '@/hooks/use-draft'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  Activity,
  CircleDot,
  FolderGit2,
  GitBranch,
  Layers,
  Pencil,
  Plus,
  Settings,
  Sparkles,
  TicketCheck,
} from 'lucide-react'

import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { ItemControls, LayoutToolbar, SectionHeader } from '@/components/project-sections'
import { UNSECTIONED, layoutOps, pruneLayout, useArranged, useProjectLayout } from '@/lib/project-layout'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { useAPI } from '@/contexts/use-api'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type Ticket = components['schemas']['Ticket']
type WorkStream = components['schemas']['WorkStream']

interface ProjectStats {
  totalTickets: number
  activeTickets: number
  activeStreams: number
}

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

function InlineRename({
  projectId,
  currentName,
  onSaved,
}: {
  projectId: string
  currentName: string
  onSaved: (newName: string) => void
}) {
  const { client } = useAPI()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useDraft(currentName)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      // Focus on next tick after render
      requestAnimationFrame(() => inputRef.current?.select())
    }
  }, [editing, currentName])

  const save = async () => {
    const trimmed = draft.trim()
    if (!trimmed || trimmed === currentName) {
      setEditing(false)
      return
    }
    const { response } = await client.PATCH('/projects/{projectID}', {
      params: { path: { projectID: projectId } },
      body: { name: trimmed },
    })
    if (response.ok) {
      onSaved(trimmed)
    }
    setEditing(false)
  }

  if (editing) {
    return (
      <Input
        ref={inputRef}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void save()}
        onKeyDown={(e) => {
          if (e.key === 'Enter') void save()
          if (e.key === 'Escape') setEditing(false)
        }}
        className="h-7 max-w-xs text-base font-semibold"
        onClick={(e) => e.preventDefault()}
      />
    )
  }

  return (
    <button
      onClick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        setEditing(true)
      }}
      className="inline-flex items-center gap-1.5 group/rename"
      title="Rename project"
    >
      <span className="text-base font-semibold leading-tight">{currentName}</span>
      <Pencil className="h-3 w-3 text-muted-foreground/0 group-hover/rename:text-muted-foreground transition-colors" />
    </button>
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
  const activityVersion = useActivityVersion('projects')
  const statsVersion = useActivityVersion('tickets', 'sessions')
  const { orgId, orgParam } = useResolvedRouteParams()
  const { client } = useAPI()
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [togglingById, setTogglingById] = useState<Record<string, boolean>>({})
  const [orgName, setOrgName] = useState<string | null>(null)
  const [stats, setStats] = useState<Record<string, ProjectStats>>({})
  const [statsLoading, setStatsLoading] = useState(true)
  const [err, setErr] = useState<string | null>(null)
  const { layout, update: updateLayout } = useProjectLayout()
  const [customizing, setCustomizing] = useState(false)
  const projectItems = useMemo(() => (projects ?? []).map((p) => ({ id: p.id ?? '', p })), [projects])
  const arranged = useArranged(projectItems, layout)

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
  }, [client, orgId, activityVersion])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      if (!projects || projects.length === 0) {
        if (!cancelled) {
          setStats({})
          setStatsLoading(false)
        }
        return
      }
      setStatsLoading(true)
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
            'draft',
            'planning',
            'executing',
            'awaiting_validation',
            'awaiting_input',
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
  }, [client, projects, statsVersion])

  const toggleDispatch = useCallback(
    async (project: Project) => {
      if (!project.id) return
      const projectID = project.id
      const previousValue = project.dispatch_enabled !== false
      const nextValue = !previousValue

      setTogglingById((prev) => ({ ...prev, [projectID]: true }))
      setProjects((prev) =>
        prev?.map((entry) =>
          entry.id === projectID
            ? { ...entry, dispatch_enabled: nextValue }
            : entry,
        ) ?? prev,
      )

      const { error, response } = await client.PATCH('/projects/{projectID}', {
        params: { path: { projectID } },
        body: { dispatch_enabled: nextValue },
      })

      if (!response.ok) {
        setProjects((prev) =>
          prev?.map((entry) =>
            entry.id === projectID
              ? { ...entry, dispatch_enabled: previousValue }
              : entry,
          ) ?? prev,
        )
        setErr(formatApiError(error))
      } else {
        setErr(null)
      }

      setTogglingById((prev) => {
        const next = { ...prev }
        delete next[projectID]
        return next
      })
    },
    [client],
  )

  const handleRenamed = useCallback((projectId: string, newName: string) => {
    setProjects((prev) =>
      prev?.map((entry) =>
        entry.id === projectId ? { ...entry, name: newName } : entry,
      ) ?? prev,
    )
  }, [])

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
        <div className="flex items-center justify-between">
          <h1 className="text-xl font-semibold tracking-tight">
            Projects{orgName ? ` — ${orgName}` : ''}
          </h1>
          <div className="flex items-center gap-2">
            {layout && projects.length > 0 && (
              <LayoutToolbar
                customizing={customizing}
                onToggle={() => setCustomizing((v) => !v)}
                onAdd={(name) => updateLayout(layoutOps.addSection(layout, name))}
              />
            )}
            <Link
              to={`/orgs/${orgParam}/projects/new`}
              className="inline-flex items-center gap-1.5 rounded-lg border border-primary/20 bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary transition-colors hover:bg-primary/20 hover:border-primary/30"
            >
              <Plus className="size-3.5" />
              Create project
            </Link>
          </div>
        </div>
      </motion.div>
      {projects.length === 0 ? (
        <EmptyProjectsState orgId={orgParam ?? orgId ?? ''} />
      ) : (
        <div className="flex flex-col gap-6">
          {arranged.map(({ section, items }, sIdx) => (
            <section key={section.id} className="flex flex-col gap-3">
              <SectionHeader
                section={section}
                index={sIdx}
                total={arranged.length}
                customizing={customizing}
                count={items.length}
                onChange={(patch) => layout && section.id !== UNSECTIONED && updateLayout(layoutOps.patchSection(layout, section.id, patch))}
                onMove={(dir) => layout && updateLayout(layoutOps.moveSection(layout, section.id, dir))}
                onDelete={() => layout && updateLayout(layoutOps.deleteSection(layout, section.id))}
              />
              {section.collapsed ? null : items.length === 0 && customizing ? (
                <p className="text-xs text-muted-foreground">Empty section — move projects here with “Move to”.</p>
              ) : (
        <StaggerList className="flex flex-col gap-4">
          {items.map(({ p }, pIdx) => {
            const name = p.name ?? p.slug ?? p.id ?? ''
            const id = p.id ?? ''
            const projectStats = stats[id]
            const isStatsLoading = statsLoading && !projectStats
            const dispatchOn = p.dispatch_enabled !== false
            const dispatchToggleID = `dispatch-${id}`
            const isDispatchToggling = togglingById[id] === true
            const description = p.description ?? ''
            return (
              <StaggerItem key={id}>
                <Card className="group hover:bg-white/[0.06]">
                  <CardContent className="p-0">
                    <div className="flex items-center gap-4 p-5">
                      {/* Left: avatar + name/description/status/tech */}
                      <Link
                        to={`/orgs/${orgParam}/projects/${p.slug ?? id}`}
                        className="flex min-w-0 flex-1 items-start gap-3"
                      >
                        <ProjectAvatar name={name} id={id} />
                        <div className="flex min-w-0 flex-1 flex-col gap-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <InlineRename
                              projectId={id}
                              currentName={name}
                              onSaved={(newName) => handleRenamed(id, newName)}
                            />
                          </div>
                          {description && (
                            <p className="text-xs text-muted-foreground line-clamp-2">
                              {description}
                            </p>
                          )}
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
                      </Link>

                      {/* Middle: stats + repo info */}
                      <div className="hidden sm:flex flex-col gap-1.5 shrink-0">
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
                        {(p.repo_url || p.default_branch) && (
                          <div className="flex items-center gap-3 text-xs text-muted-foreground/70">
                            {p.repo_url ? (
                              <span className="inline-flex items-center gap-1 truncate max-w-[200px]">
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
                        )}
                      </div>

                      {/* Right: dispatch toggle + settings */}
                      <div className="flex items-center gap-3 shrink-0">
                        <div className="flex flex-col items-end gap-0.5">
                          <label
                            htmlFor={dispatchToggleID}
                            className="text-xs font-medium text-foreground cursor-pointer"
                          >
                            {dispatchOn ? 'Dispatch on' : 'Dispatch off'}
                          </label>
                          <Switch
                            id={dispatchToggleID}
                            checked={dispatchOn}
                            onCheckedChange={() => void toggleDispatch(p)}
                            disabled={isDispatchToggling}
                          />
                        </div>
                        {customizing && layout ? (
                          <ItemControls
                            sections={layout.project_sections}
                            currentSectionId={section.id}
                            index={pIdx}
                            total={items.length}
                            onMoveWithin={(dir) =>
                              updateLayout(
                                layoutOps.moveWithin(
                                  pruneLayout(layout, new Set(projectItems.map((i) => i.id))),
                                  section.id,
                                  items.map((i) => i.id),
                                  pIdx,
                                  dir,
                                ),
                              )
                            }
                            onMoveTo={(sectionId) => updateLayout(layoutOps.moveTo(layout, id, sectionId, []))}
                          />
                        ) : null}
                        <Link
                          to={`/orgs/${orgParam}/projects/${p.slug ?? id}/settings`}
                          className="rounded-lg p-1.5 text-muted-foreground/60 transition-colors hover:bg-white/10 hover:text-foreground"
                          title="Project settings"
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Settings className="h-4 w-4" />
                        </Link>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </StaggerItem>
            )
          })}
        </StaggerList>
              )}
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
