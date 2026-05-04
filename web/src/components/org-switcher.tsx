import { useEffect, useMemo, useState } from 'react'
import { Building2 } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAuth } from '@/contexts/use-auth'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import {
  orgsWithIDs,
  resolvePreferredOrgId,
  setPreferredOrgId,
  type OrgWithID,
} from '@/lib/org-preferences'
import { cn } from '@/lib/utils'

type OrgSwitcherProps = {
  expanded: boolean
}

type OrgLoadState = {
  token: string | null
  orgs: OrgWithID[]
  status: 'idle' | 'loaded' | 'failed'
}

const EMPTY_ORGS: OrgWithID[] = []

function orgLabel(org: OrgWithID): string {
  return org.name || org.slug || org.id
}

export function OrgSwitcher({ expanded }: OrgSwitcherProps) {
  const { token, client } = useAuth()
  const navigate = useNavigate()
  const { orgId } = useResolvedRouteParams()
  const [loadState, setLoadState] = useState<OrgLoadState>({
    token: null,
    orgs: [],
    status: 'idle',
  })

  useEffect(() => {
    if (!token) return

    let cancelled = false
    ;(async () => {
      const { data, response } = await client.GET('/orgs', {})
      if (cancelled) return
      if (!response.ok) {
        setLoadState({ token, orgs: [], status: 'failed' })
        return
      }
      setLoadState({ token, orgs: orgsWithIDs(data), status: 'loaded' })
    })()
    return () => {
      cancelled = true
    }
  }, [client, token])

  const isCurrentLoad = loadState.token === token
  const orgs = isCurrentLoad ? loadState.orgs : EMPTY_ORGS
  const loaded = isCurrentLoad && loadState.status !== 'idle'
  const failed = isCurrentLoad && loadState.status === 'failed'

  const currentOrg = useMemo(
    () => orgs.find((org) => org.id === orgId) ?? null,
    [orgId, orgs],
  )
  const selectedOrgId = currentOrg?.id ?? resolvePreferredOrgId(orgs) ?? ''

  useEffect(() => {
    if (currentOrg) setPreferredOrgId(currentOrg.id)
  }, [currentOrg])

  if (!token) return null

  if (!expanded) {
    const collapsedSlug = currentOrg?.slug ?? selectedOrgId
    return (
      <Link
        to={collapsedSlug ? `/orgs/${collapsedSlug}/projects` : '/orgs'}
        className="flex h-9 items-center justify-center rounded-lg text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
        title={currentOrg ? orgLabel(currentOrg) : 'Organizations'}
      >
        <Building2 className="size-4" />
      </Link>
    )
  }

  if (!loaded) {
    return (
      <div className="space-y-2 px-1">
        <div className="h-3 w-20 animate-pulse rounded bg-white/[0.06]" />
        <div className="h-8 animate-pulse rounded-lg bg-white/[0.06]" />
      </div>
    )
  }

  if (failed || orgs.length === 0) {
    return (
      <Link
        to="/orgs"
        className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground"
      >
        <Building2 className="size-4 shrink-0" />
        <span className="truncate">Organizations</span>
      </Link>
    )
  }

  return (
    <div className="space-y-1 px-1">
      <span className="px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
        Organization
      </span>
      <Select
        value={selectedOrgId}
        onValueChange={(nextOrgId) => {
          setPreferredOrgId(nextOrgId)
          const nextOrg = orgs.find((o) => o.id === nextOrgId)
          const slug = nextOrg?.slug ?? nextOrgId
          navigate(`/orgs/${slug}/projects`)
        }}
      >
        <SelectTrigger
          size="sm"
          className={cn(
            'h-8 w-full border-sidebar-border bg-white/[0.03] text-sidebar-foreground hover:bg-sidebar-accent/50',
            '[&_[data-slot=select-value]]:min-w-0',
          )}
          aria-label="Switch organization"
        >
          <SelectValue placeholder="Organization" />
        </SelectTrigger>
        <SelectContent align="start" className="min-w-56">
          {orgs.map((org) => (
            <SelectItem key={org.id} value={org.id}>
              <span className="truncate">{orgLabel(org)}</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
