import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { Building2, FolderKanban, Plus, Sparkles } from 'lucide-react'

import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Org = components['schemas']['Org']
type Project = components['schemas']['Project']

/** Deterministic hue from a string — keeps avatar colors stable per-org. */
function stringToHue(s: string): number {
  let hash = 0
  for (let i = 0; i < s.length; i++) {
    hash = s.charCodeAt(i) + ((hash << 5) - hash)
  }
  return Math.abs(hash) % 360
}

/** Accent colors in the green family, cycling by index. */
const ACCENT_HUES = [145, 160, 130, 175, 120, 155, 140, 170]

function OrgAvatar({ name, id }: { name: string; id: string }) {
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

function EmptyOrgsState() {
  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: [0.25, 0.1, 0.25, 1] }}
      className="flex flex-col items-center justify-center gap-6 py-16 text-center"
    >
      <div className="relative">
        <div className="flex h-20 w-20 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04] backdrop-blur-md">
          <Building2 className="h-10 w-10 text-primary/60" />
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
          No organizations yet
        </h2>
        <p className="text-sm text-muted-foreground leading-relaxed">
          Organizations group your projects and teams together.
          Create your first org to start managing work with Flywheel.
        </p>
      </div>
      <Link
        to="/orgs/new"
        className="inline-flex items-center gap-2 rounded-xl border border-primary/20 bg-primary/10 px-5 py-2.5 text-sm font-medium text-primary backdrop-blur-sm transition-all duration-200 hover:bg-primary/20 hover:border-primary/30"
      >
        <Plus className="h-4 w-4" />
        Create organization
      </Link>
    </motion.div>
  )
}

export function OrgsPage() {
  const { client } = useAuth()
  const [orgs, setOrgs] = useState<Org[] | null>(null)
  const [projectCounts, setProjectCounts] = useState<Record<string, number>>({})
  const [err, setErr] = useState<string | null>(null)

  // Fetch orgs
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/orgs', {})
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setOrgs([])
        return
      }
      setErr(null)
      setOrgs(data ?? [])
    })()
    return () => {
      cancelled = true
    }
  }, [client])

  // Once orgs are loaded, fetch project counts for each
  useEffect(() => {
    if (!orgs || orgs.length === 0) return
    let cancelled = false
    ;(async () => {
      const counts: Record<string, number> = {}
      await Promise.all(
        orgs.map(async (org) => {
          if (!org.id) return
          const { data, response } = await client.GET('/orgs/{orgID}/projects', {
            params: { path: { orgID: org.id }, query: { status: 'all' } },
          })
          if (cancelled) return
          if (response.ok && data) {
            counts[org.id] = (data as Project[]).length
          }
        }),
      )
      if (!cancelled) setProjectCounts(counts)
    })()
    return () => {
      cancelled = true
    }
  }, [client, orgs])

  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!orgs) {
    return <ListPageSkeleton />
  }

  return (
    <div className="flex flex-col gap-6">
      <motion.div
        initial={{ opacity: 0, y: -8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3 }}
        className="flex items-center justify-between"
      >
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Organizations
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {orgs.length > 0
              ? `${orgs.length} organization${orgs.length === 1 ? '' : 's'}`
              : 'Manage your teams and projects'}
          </p>
        </div>
      </motion.div>

      {orgs.length === 0 ? (
        <EmptyOrgsState />
      ) : (
        <StaggerList className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {orgs.map((o) => {
            const name = o.name ?? o.slug ?? o.id ?? ''
            const id = o.id ?? ''
            const count = projectCounts[id]
            return (
              <StaggerItem key={id}>
                <Link to={`/orgs/${id}/projects`} className="block h-full">
                  <Card className="group h-full hover:bg-white/[0.06]">
                    <CardHeader>
                      <div className="flex items-start gap-3">
                        <OrgAvatar name={name} id={id} />
                        <div className="flex min-w-0 flex-1 flex-col gap-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <CardTitle className="text-base font-semibold truncate">
                              {name}
                            </CardTitle>
                            {o.slug ? (
                              <Badge variant="outline" className="text-xs shrink-0">
                                {o.slug}
                              </Badge>
                            ) : null}
                          </div>
                          <span className="font-mono text-xs text-muted-foreground truncate">
                            {id}
                          </span>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="pt-0">
                      <div className="flex items-center gap-4 text-xs text-muted-foreground">
                        <span className="inline-flex items-center gap-1.5">
                          <FolderKanban className="h-3.5 w-3.5" />
                          {count !== undefined ? (
                            <span>
                              {count} project{count === 1 ? '' : 's'}
                            </span>
                          ) : (
                            <span className="inline-block h-3 w-12 animate-pulse rounded bg-white/[0.06]" />
                          )}
                        </span>
                        {o.created_at ? (
                          <span>
                            Created{' '}
                            {new Date(o.created_at).toLocaleDateString(undefined, {
                              month: 'short',
                              day: 'numeric',
                              year: 'numeric',
                            })}
                          </span>
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
