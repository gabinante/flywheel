import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  Database,
  Globe,
  Layers,
  Search,
  Server,
  GitBranch,
  Box,
  Plug,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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

// The valid entity types from the backend catalog model.
const ENTITY_TYPES = [
  'service',
  'datastore',
  'integration',
  'infrastructure',
  'repository',
  'environment',
] as const

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

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function EntityCardSkeleton() {
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-white/10 bg-white/[0.03] p-4 backdrop-blur-md">
      <div className="flex items-center gap-3">
        <Skeleton className="size-8 rounded-lg" />
        <div className="flex-1 space-y-1.5">
          <Skeleton className="h-4 w-2/5" />
          <Skeleton className="h-3 w-1/4" />
        </div>
      </div>
      <Skeleton className="h-3 w-3/4" />
      <Skeleton className="h-3 w-1/2" />
    </div>
  )
}

function EntityCard({
  entity,
  onClick,
}: {
  entity: CatalogEntity
  onClick?: () => void
}) {
  const Icon = entityTypeIcon(entity.type)
  const labelEntries = entity.labels ? Object.entries(entity.labels) : []

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick?.()
        }
      }}
      className="group flex cursor-pointer flex-col gap-3 rounded-xl border border-white/10 bg-white/5 p-4 backdrop-blur-md transition-all duration-200 hover:border-white/20 hover:bg-white/[0.08]"
    >
      <div className="flex items-start gap-3">
        <div className="rounded-lg border border-white/10 bg-white/[0.06] p-2">
          <Icon className="size-4 text-foreground" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="truncate text-sm font-medium text-foreground">
              {entity.name}
            </p>
            <Badge variant="outline" className="shrink-0 text-[10px]">
              {entityTypeLabel(entity.type)}
            </Badge>
            {entity.source && (
              <Badge variant="muted" className="shrink-0 text-[10px]">
                {entity.source}
              </Badge>
            )}
          </div>
          {entity.description && (
            <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
              {entity.description}
            </p>
          )}
        </div>
      </div>

      {labelEntries.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {labelEntries.slice(0, 5).map(([key, value]) => (
            <span
              key={key}
              className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-[10px] text-muted-foreground"
            >
              {key}: {value}
            </span>
          ))}
          {labelEntries.length > 5 && (
            <span className="rounded-md bg-white/[0.06] px-1.5 py-0.5 text-[10px] text-muted-foreground">
              +{labelEntries.length - 5} more
            </span>
          )}
        </div>
      )}

      <p className="text-[10px] text-muted-foreground/70">
        Last updated {formatTimestamp(entity.updated_at)}
      </p>
    </div>
  )
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-4 py-12 text-center">
      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4">
        <Layers className="size-8 text-muted-foreground/50" />
      </div>
      <div className="max-w-md space-y-2">
        <p className="text-sm font-medium text-foreground">
          No catalog entities yet
        </p>
        <p className="text-xs leading-relaxed text-muted-foreground">
          Catalog entities represent your infrastructure: services, datastores,
          integrations, repositories, and environments. They get populated
          automatically via a bootstrap scan of your repository, or you can
          create them manually through the API.
        </p>
      </div>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <Card className="border-destructive/30 bg-destructive/5">
      <CardContent className="px-5 py-4 text-sm text-destructive">
        Failed to load catalog entities: {message}
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function CatalogEntityList() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  const { token } = useAuth()
  const navigate = useNavigate()

  const [entities, setEntities] = useState<CatalogEntity[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Filters
  const [searchQuery, setSearchQuery] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('all')

  // Fetch entities from API
  useEffect(() => {
    if (!projectId || !token) {
      setLoading(false)
      return
    }

    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const params = new URLSearchParams({ project_id: projectId })
        if (typeFilter && typeFilter !== 'all') {
          params.set('type', typeFilter)
        }

        const response = await fetch(
          `/api/v1/catalog/entities?${params.toString()}`,
          {
            headers: {
              Authorization: `Bearer ${token}`,
              'Content-Type': 'application/json',
            },
          },
        )

        if (cancelled) return

        if (!response.ok) {
          const body = await response.json().catch(() => null)
          const msg =
            body && typeof body === 'object' && 'error' in body
              ? String(body.error)
              : `HTTP ${response.status}`
          setError(msg)
          setEntities([])
          setLoading(false)
          return
        }

        const data = await response.json()
        if (cancelled) return

        setEntities(data.entities ?? [])
        setError(null)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Network error')
        setEntities([])
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()

    return () => {
      cancelled = true
    }
  }, [projectId, token, typeFilter])

  // Client-side text search filter
  const filteredEntities = useMemo(() => {
    if (!searchQuery.trim()) return entities
    const query = searchQuery.toLowerCase()
    return entities.filter(
      (entity) =>
        entity.name.toLowerCase().includes(query) ||
        (entity.description ?? '').toLowerCase().includes(query),
    )
  }, [entities, searchQuery])

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="border-b border-white/10 pb-4">
        <CardTitle className="text-sm">Catalog</CardTitle>
        <CardDescription>
          Registered entities and their relationships across the project.
        </CardDescription>
      </CardHeader>

      <CardContent className="pt-4">
        {/* Filter bar */}
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
            <Input
              placeholder="Search entities..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8"
            />
          </div>
          <Select value={typeFilter} onValueChange={setTypeFilter}>
            <SelectTrigger className="w-full sm:w-44">
              <SelectValue placeholder="All types" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All types</SelectItem>
              {ENTITY_TYPES.map((type) => (
                <SelectItem key={type} value={type}>
                  {entityTypeLabel(type)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Content states */}
        {error ? (
          <ErrorState message={error} />
        ) : loading ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <EntityCardSkeleton key={i} />
            ))}
          </div>
        ) : filteredEntities.length === 0 && entities.length === 0 ? (
          <EmptyState />
        ) : filteredEntities.length === 0 ? (
          <div className="py-8 text-center">
            <p className="text-sm text-muted-foreground">
              No entities match your search.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {filteredEntities.map((entity) => (
              <EntityCard
                key={entity.id}
                entity={entity}
                onClick={() =>
                  navigate(
                    `/orgs/${orgId}/projects/${projectId}/infrastructure/entities/${entity.id}`,
                  )
                }
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
