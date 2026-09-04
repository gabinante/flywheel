import { useEffect, useState } from 'react'
import { ExternalLink, Link2, RefreshCw, Unlink } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import { relativeTime } from '@/lib/sessions-format'

type ProjectLinearLink = components['schemas']['ProjectLinearLink']

export function ProjectLinearCard({ projectId }: { projectId: string }) {
  const { client } = useAuth()
  const [link, setLink] = useState<ProjectLinearLink | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [ref, setRef] = useState('')
  const [linking, setLinking] = useState(false)

  const linkProject = async () => {
    if (!ref.trim()) return
    setLinking(true)
    const { data, error, response } = await client.PUT('/projects/{projectID}/linear', {
      params: { path: { projectID: projectId } },
      body: { linear_project: ref.trim() },
    })
    setLinking(false)
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setRef('')
    setLink(data)
  }

  const unlinkProject = async () => {
    const { error, response } = await client.DELETE('/projects/{projectID}/linear', { params: { path: { projectID: projectId } } })
    if (!response.ok) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setLink((prev) => (prev ? { ...prev, linked: false, linear_project_id: undefined, linear_project_name: undefined, linear_project_url: undefined, team_keys: [], ticket_count: 0 } : prev))
  }

  const linkForm = (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        value={ref}
        onChange={(e) => setRef(e.target.value)}
        placeholder="https://linear.app/<workspace>/project/<slug>-<id>  or project UUID"
        className="h-8 min-w-[22rem] flex-1 text-sm"
        aria-label="Linear project URL or id"
      />
      <Button size="sm" onClick={linkProject} disabled={linking || !ref.trim()}>
        <Link2 className="size-4" /> {link?.linked ? 'Change link' : 'Link'}
      </Button>
    </div>
  )

  useEffect(() => {
    let cancelled = false
    void client
      .GET('/projects/{projectID}/linear', { params: { path: { projectID: projectId } } })
      .then(({ data, error, response }) => {
        if (cancelled) return
        if (!response.ok || !data) {
          setErr(formatApiError(error))
          return
        }
        setErr(null)
        setLink(data)
      })
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  const syncNow = async () => {
    setSyncing(true)
    const { data, error, response } = await client.POST('/projects/{projectID}/linear/sync', {
      params: { path: { projectID: projectId } },
    })
    setSyncing(false)
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setLink(data)
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <CardTitle className="text-sm">Linear</CardTitle>
        <CardDescription>
          Linear is the ticket store. Projects you lead in Linear appear here automatically and their issues become
          tickets; tickets Flywheel files are created as Linear issues.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 text-sm">
        {err && <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-200">{err}</div>}
        {link && !link.linked && (
          <>
            <p className="text-muted-foreground">
              Not linked. Projects you lead in Linear link automatically once <code>LINEAR_API_KEY</code> is set; paste a
              Linear project URL or id here to link this one by hand (optional).
            </p>
            {linkForm}
          </>
        )}
        {link?.linked && (
          <>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div>
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Linear project</div>
                <a
                  href={link.linear_project_url ?? '#'}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1 text-foreground hover:underline"
                >
                  {link.linear_project_name} <ExternalLink className="size-3" />
                </a>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Teams</div>
                <div className="font-mono text-xs">{link.team_keys.join(', ') || '—'}</div>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Mirrored issues</div>
                <div>{link.ticket_count}</div>
              </div>
              <div>
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">Last sync</div>
                <div>{link.synced_at ? relativeTime(link.synced_at) : 'never'}</div>
              </div>
            </div>
            {link.last_error && (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                Last sync error: {link.last_error}
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" variant="ghost" onClick={syncNow} disabled={syncing}>
                <RefreshCw className={syncing ? 'size-4 animate-spin' : 'size-4'} /> Sync now
              </Button>
              <Button size="sm" variant="ghost" onClick={unlinkProject} className="text-muted-foreground">
                <Unlink className="size-4" /> Unlink
              </Button>
            </div>
            <details className="text-xs text-muted-foreground">
              <summary className="cursor-pointer">Link a different Linear project</summary>
              <div className="mt-2">{linkForm}</div>
            </details>
          </>
        )}
      </CardContent>
    </Card>
  )
}
