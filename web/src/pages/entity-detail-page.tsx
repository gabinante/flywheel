import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  ArrowLeft,
  Box,
  Clock,
  Database,
  GitBranch,
  Globe,
  Layers,
  Link2,
  Plug,
  Server,
  Activity,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface CatalogEntity {
  id: string
  project_id: string
  type: string
  name: string
  description?: string
  labels?: Record<string, string>
  metadata?: Record<string, string>
  source?: string
  created_at: string
  updated_at: string
}

interface CatalogEdge {
  id: string
  project_id: string
  from_id: string
  to_id: string
  type: string
  metadata?: Record<string, string>
  source?: string
  created_at: string
}

interface EntityInstance {
  id: string
  entity_id: string
  environment: string
  attributes?: Record<string, unknown>
  retired_at?: string | null
  created_at: string
  updated_at: string
}

interface StreamEvent {
  id: string
  entity_id: string
  event_type: string
  payload?: Record<string, unknown>
  actor: string
  created_at: string
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function entityTypeIcon(type: string) {
  switch (type) {
    case 'service':
      return Server
    case 'datastore':
      return Database
    case 'integration':
      return Plug
    case 'infrastructure':
      return Layers
    case 'repository':
      return GitBranch
    case 'environment':
      return Globe
    default:
      return Box
  }
}

function entityTypeLabel(type: string): string {
  return type.charAt(0).toUpperCase() + type.slice(1)
}

function formatTimestamp(iso: string): string {
  try {
    const date = new Date(iso)
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

function formatRelativeTime(iso: string): string {
  try {
    const date = new Date(iso)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    const diffMin = Math.floor(diffMs / 60000)
    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin}m ago`
    const diffHr = Math.floor(diffMin / 60)
    if (diffHr < 24) return `${diffHr}h ago`
    const diffDays = Math.floor(diffHr / 24)
    if (diffDays < 30) return `${diffDays}d ago`
    return formatTimestamp(iso)
  } catch {
    return iso
  }
}

function edgeTypeLabel(type: string): string {
  return type.replace(/_/g, ' ')
}

const EVENT_TYPE_COLORS: Record<string, string> = {
  created: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
  renamed: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
  retired: 'bg-red-500/20 text-red-400 border-red-500/30',
  instance_created: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
  instance_retired: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
  attributes_updated: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
}

// ---------------------------------------------------------------------------
// Section skeletons
// ---------------------------------------------------------------------------

function SectionSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="flex flex-col gap-2 rounded-lg border border-white/10 bg-white/[0.03] p-3"
        >
          <Skeleton className="h-4 w-2/5" />
          <Skeleton className="h-3 w-3/4" />
        </div>
      ))}
    </div>
  )
}

function HeaderSkeleton() {
  return (
    <div className="flex flex-col gap-3 animate-in fade-in duration-300">
      <Skeleton className="h-3 w-32" />
      <div className="flex items-center gap-3">
        <Skeleton className="size-10 rounded-xl" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="h-3 w-96" />
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Instances section
// ---------------------------------------------------------------------------

function InstancesSection({
  entityId,
  token,
}: {
  entityId: string
  token: string
}) {
  const [instances, setInstances] = useState<EntityInstance[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const resp = await fetch(`/api/v1/entities/${entityId}/instances`, {
          headers: { Authorization: `Bearer ${token}` },
        })
        if (cancelled) return
        if (!resp.ok) {
          const body = await resp.json().catch(() => null)
          setError(
            body && typeof body === 'object' && 'error' in body
              ? String(body.error)
              : `HTTP ${resp.status}`,
          )
          setInstances([])
          return
        }
        const data = await resp.json()
        if (cancelled) return
        // API returns array directly (not wrapped)
        setInstances(Array.isArray(data) ? data : [])
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Network error')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [entityId, token])

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Server className="size-4 text-muted-foreground" />
          Instances
        </CardTitle>
        <CardDescription>
          Running instances of this entity across environments.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        {error ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
            Failed to load instances: {error}
          </div>
        ) : loading ? (
          <SectionSkeleton rows={2} />
        ) : instances.length === 0 ? (
          <div className="py-6 text-center">
            <p className="text-sm text-muted-foreground">
              No instances found for this entity.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {instances.map((inst) => (
              <div
                key={inst.id}
                className="flex flex-col gap-2 rounded-lg border border-white/10 bg-white/[0.03] p-3 transition-colors hover:bg-white/[0.05]"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="text-[10px]">
                      {inst.environment}
                    </Badge>
                    {inst.retired_at && (
                      <Badge
                        variant="muted"
                        className="text-[10px] text-amber-400"
                      >
                        Retired
                      </Badge>
                    )}
                  </div>
                  <span className="text-[10px] text-muted-foreground/70">
                    {formatTimestamp(inst.created_at)}
                  </span>
                </div>
                {inst.attributes &&
                  Object.keys(inst.attributes).length > 0 && (
                    <div className="flex flex-wrap gap-1.5">
                      {Object.entries(inst.attributes)
                        .slice(0, 6)
                        .map(([key, value]) => (
                          <span
                            key={key}
                            className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-[10px] text-muted-foreground"
                          >
                            {key}: {String(value)}
                          </span>
                        ))}
                      {Object.keys(inst.attributes).length > 6 && (
                        <span className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-[10px] text-muted-foreground">
                          +{Object.keys(inst.attributes).length - 6} more
                        </span>
                      )}
                    </div>
                  )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Edges section
// ---------------------------------------------------------------------------

function EdgesSection({
  entityId,
  projectId,
  token,
}: {
  entityId: string
  projectId: string
  token: string
}) {
  const [edges, setEdges] = useState<CatalogEdge[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Also fetch entity names for edge references
  const [entityNames, setEntityNames] = useState<Record<string, string>>({})

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const params = new URLSearchParams({
          project_id: projectId,
          entity_id: entityId,
        })
        const resp = await fetch(`/api/v1/catalog/edges?${params.toString()}`, {
          headers: { Authorization: `Bearer ${token}` },
        })
        if (cancelled) return
        if (!resp.ok) {
          const body = await resp.json().catch(() => null)
          setError(
            body && typeof body === 'object' && 'error' in body
              ? String(body.error)
              : `HTTP ${resp.status}`,
          )
          setEdges([])
          return
        }
        const data = await resp.json()
        if (cancelled) return
        const edgeList: CatalogEdge[] = data.edges ?? []
        setEdges(edgeList)

        // Resolve entity names for connected entities
        const otherIds = new Set<string>()
        for (const edge of edgeList) {
          if (edge.from_id !== entityId) otherIds.add(edge.from_id)
          if (edge.to_id !== entityId) otherIds.add(edge.to_id)
        }
        const names: Record<string, string> = {}
        // Fetch each related entity name (parallel)
        await Promise.all(
          Array.from(otherIds).map(async (id) => {
            try {
              const entResp = await fetch(
                `/api/v1/catalog/entities/${id}?project_id=${encodeURIComponent(projectId)}`,
                { headers: { Authorization: `Bearer ${token}` } },
              )
              if (entResp.ok) {
                const ent = await entResp.json()
                names[id] = ent.name ?? id
              }
            } catch {
              // fallback to ID
            }
          }),
        )
        if (!cancelled) setEntityNames(names)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Network error')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [entityId, projectId, token])

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Link2 className="size-4 text-muted-foreground" />
          Relationships
        </CardTitle>
        <CardDescription>
          Edges connecting this entity to others in the catalog.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        {error ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
            Failed to load edges: {error}
          </div>
        ) : loading ? (
          <SectionSkeleton rows={2} />
        ) : edges.length === 0 ? (
          <div className="py-6 text-center">
            <p className="text-sm text-muted-foreground">
              No relationships found for this entity.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {edges.map((edge) => {
              const isOutgoing = edge.from_id === entityId
              const otherId = isOutgoing ? edge.to_id : edge.from_id
              const otherName = entityNames[otherId] ?? otherId

              return (
                <div
                  key={edge.id}
                  className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.03] p-3 transition-colors hover:bg-white/[0.05]"
                >
                  <div className="flex min-w-0 flex-1 items-center gap-2">
                    <Badge
                      variant="outline"
                      className="shrink-0 text-[10px] capitalize"
                    >
                      {edgeTypeLabel(edge.type)}
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {isOutgoing ? '\u2192' : '\u2190'}
                    </span>
                    <span className="truncate text-sm text-foreground">
                      {otherName}
                    </span>
                  </div>
                  {edge.source && (
                    <Badge variant="muted" className="shrink-0 text-[10px]">
                      {edge.source}
                    </Badge>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Stream section
// ---------------------------------------------------------------------------

function StreamSection({
  entityId,
  token,
}: {
  entityId: string
  token: string
}) {
  const [events, setEvents] = useState<StreamEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const resp = await fetch(
          `/api/v1/entities/${entityId}/stream?limit=50`,
          { headers: { Authorization: `Bearer ${token}` } },
        )
        if (cancelled) return
        if (!resp.ok) {
          const body = await resp.json().catch(() => null)
          setError(
            body && typeof body === 'object' && 'error' in body
              ? String(body.error)
              : `HTTP ${resp.status}`,
          )
          setEvents([])
          return
        }
        const data = await resp.json()
        if (cancelled) return
        // API returns array directly
        setEvents(Array.isArray(data) ? data : [])
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Network error')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [entityId, token])

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Activity className="size-4 text-muted-foreground" />
          Event Stream
        </CardTitle>
        <CardDescription>
          Recent lifecycle events and state changes.
        </CardDescription>
      </CardHeader>
      <CardContent className="pt-4">
        {error ? (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
            Failed to load stream: {error}
          </div>
        ) : loading ? (
          <SectionSkeleton rows={3} />
        ) : events.length === 0 ? (
          <div className="py-6 text-center">
            <p className="text-sm text-muted-foreground">
              No events recorded for this entity yet.
            </p>
          </div>
        ) : (
          <div className="relative flex flex-col gap-0">
            {/* Timeline line */}
            <div className="absolute left-[11px] top-3 bottom-3 w-px bg-white/10" />

            {events.map((event, idx) => {
              const colorClass =
                EVENT_TYPE_COLORS[event.event_type] ??
                'bg-white/10 text-muted-foreground border-white/20'
              const isLast = idx === events.length - 1

              return (
                <div
                  key={event.id}
                  className={`relative flex items-start gap-3 py-2.5 ${isLast ? '' : ''}`}
                >
                  {/* Timeline dot */}
                  <div className="relative z-10 mt-1 size-[9px] shrink-0 rounded-full border border-white/20 bg-white/10" />

                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <div className="flex items-center gap-2">
                      <Badge
                        variant="outline"
                        className={`text-[10px] ${colorClass}`}
                      >
                        {event.event_type.replace(/_/g, ' ')}
                      </Badge>
                      <span className="text-[10px] text-muted-foreground/70">
                        {formatRelativeTime(event.created_at)}
                      </span>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span>by {event.actor}</span>
                      {event.payload &&
                        Object.keys(event.payload).length > 0 && (
                          <span className="truncate rounded bg-white/[0.04] px-1.5 py-0.5 font-mono text-[10px]">
                            {JSON.stringify(event.payload).slice(0, 80)}
                            {JSON.stringify(event.payload).length > 80
                              ? '...'
                              : ''}
                          </span>
                        )}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export function EntityDetailPage() {
  const { orgId, projectId, entityId } = useParams<{
    orgId: string
    projectId: string
    entityId: string
  }>()
  const { token } = useAuth()

  const [entity, setEntity] = useState<CatalogEntity | null | undefined>(
    undefined,
  )
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!entityId || !projectId || !token) return
    let cancelled = false
    ;(async () => {
      try {
        const resp = await fetch(
          `/api/v1/catalog/entities/${entityId}?project_id=${encodeURIComponent(projectId)}`,
          { headers: { Authorization: `Bearer ${token}` } },
        )
        if (cancelled) return
        if (!resp.ok) {
          const body = await resp.json().catch(() => null)
          setError(
            body && typeof body === 'object' && 'error' in body
              ? String(body.error)
              : `HTTP ${resp.status}`,
          )
          setEntity(null)
          return
        }
        const data = await resp.json()
        if (cancelled) return
        setEntity(data)
        setError(null)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Network error')
        setEntity(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [entityId, projectId, token])

  if (!orgId || !projectId || !entityId) {
    return <p className="text-sm text-destructive">Missing route params.</p>
  }

  const infraHref = `/orgs/${orgId}/projects/${projectId}/infrastructure`

  if (error) {
    return (
      <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
        <p className="text-xs text-muted-foreground">
          <Link to={infraHref} className="hover:underline">
            Infrastructure
          </Link>
          <span className="px-1">/</span>
          <span>Entity</span>
        </p>
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      </div>
    )
  }

  if (entity === undefined) {
    return (
      <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
        <HeaderSkeleton />
        <SectionSkeleton rows={3} />
        <SectionSkeleton rows={2} />
        <SectionSkeleton rows={3} />
      </div>
    )
  }

  if (!entity) {
    return (
      <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
        <p className="text-xs text-muted-foreground">
          <Link to={infraHref} className="hover:underline">
            Infrastructure
          </Link>
          <span className="px-1">/</span>
          <span>Entity</span>
        </p>
        <p className="text-sm text-muted-foreground">Entity not found.</p>
      </div>
    )
  }

  const Icon = entityTypeIcon(entity.type)
  const labelEntries = entity.labels ? Object.entries(entity.labels) : []

  return (
    <div className="mx-auto flex max-w-[1440px] flex-col gap-5 animate-in fade-in duration-300">
      {/* Breadcrumbs */}
      <p className="text-xs text-muted-foreground">
        <Link to={infraHref} className="hover:underline">
          Infrastructure
        </Link>
        <span className="px-1">/</span>
        <span className="text-foreground">{entity.name}</span>
      </p>

      {/* Header */}
      <div className="flex flex-col gap-3">
        <Link
          to={infraHref}
          className="flex w-fit items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3" />
          Back to Infrastructure
        </Link>

        <div className="flex items-start gap-4">
          <div className="rounded-xl border border-white/10 bg-white/[0.06] p-3">
            <Icon className="size-6 text-foreground" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-2xl font-semibold tracking-tight">
                {entity.name}
              </h1>
              <Badge variant="outline" className="text-[10px]">
                {entityTypeLabel(entity.type)}
              </Badge>
              {entity.source && (
                <Badge variant="muted" className="text-[10px]">
                  {entity.source}
                </Badge>
              )}
            </div>
            {entity.description && (
              <p className="mt-1 max-w-3xl text-sm leading-relaxed text-muted-foreground">
                {entity.description}
              </p>
            )}
            <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground/70">
              <span className="flex items-center gap-1">
                <Clock className="size-3" />
                Updated {formatTimestamp(entity.updated_at)}
              </span>
            </div>
          </div>
        </div>

        {/* Labels */}
        {labelEntries.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {labelEntries.map(([key, value]) => (
              <span
                key={key}
                className="rounded-md bg-white/[0.06] px-2 py-0.5 text-[10px] text-muted-foreground"
              >
                {key}: {value}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Instances */}
      <InstancesSection entityId={entityId} token={token!} />

      {/* Edges */}
      <EdgesSection
        entityId={entityId}
        projectId={projectId}
        token={token!}
      />

      {/* Stream */}
      <StreamSection entityId={entityId} token={token!} />
    </div>
  )
}
