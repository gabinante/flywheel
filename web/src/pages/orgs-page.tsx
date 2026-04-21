import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'

import { StaggerItem, StaggerList } from '@/components/stagger-list'
import { Badge } from '@/components/ui/badge'
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Org = components['schemas']['Org']

export function OrgsPage() {
  const { client } = useAuth()
  const [orgs, setOrgs] = useState<Org[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

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

  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (!orgs) {
    return <ListPageSkeleton />
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold tracking-tight">Organizations</h1>
      <StaggerList className="flex flex-col gap-3">
        {orgs.map((o) => (
          <StaggerItem key={o.id}>
            <Link to={`/orgs/${o.id}/projects`}>
              <Card className="hover:bg-muted/40">
                <CardHeader>
                  <div className="flex flex-wrap items-center gap-2">
                    <CardTitle>{o.name ?? o.slug ?? o.id}</CardTitle>
                    {o.slug ? (
                      <Badge variant="outline">{o.slug}</Badge>
                    ) : null}
                  </div>
                  <CardDescription className="font-mono text-xs">
                    {o.id}
                  </CardDescription>
                </CardHeader>
              </Card>
            </Link>
          </StaggerItem>
        ))}
      </StaggerList>
      {orgs.length === 0 ? (
        <p className="text-muted-foreground text-sm">No organizations yet.</p>
      ) : null}
    </div>
  )
}
