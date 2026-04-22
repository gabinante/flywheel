import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'

/** Remount when `token` changes (see keyed wrapper in App) so verification state resets. */
export function HomePage() {
  const { token, client, signOut } = useAuth()
  const [verified, setVerified] = useState<boolean | null>(null)

  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/orgs', {})
      if (cancelled) return
      if (response.status === 401) {
        signOut()
        setVerified(false)
        return
      }
      if (error) {
        setVerified(false)
        return
      }
      setVerified(Array.isArray(data))
    })()
    return () => {
      cancelled = true
    }
  }, [token, client, signOut])

  if (token && verified === true) {
    return <Navigate to="/orgs" replace />
  }

  if (token && verified === null) {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-4 w-36" />
        <Skeleton className="h-3 w-48" />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Welcome</CardTitle>
          <CardDescription>
            Sign in with GitHub to manage organizations, projects, tickets, and
            reviews. The UI talks to the same REST API as the TUI and MCP,
            using typed calls generated from{' '}
            <code className="text-foreground">api/openapi.yaml</code>.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          <Button asChild>
            <a href="/auth/github">Sign in with GitHub</a>
          </Button>
        </CardContent>
      </Card>
      {token && verified === false ? (
        <p className="text-destructive text-sm">
          Session invalid. Sign in again.
        </p>
      ) : null}
    </div>
  )
}
