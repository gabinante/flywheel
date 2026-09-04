import { useCallback, useEffect, useState } from 'react'
import { ExternalLink, RefreshCw } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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

  const load = useCallback(async () => {
    const { data, error, response } = await client.GET('/projects/{projectID}/linear', {
      params: { path: { projectID: projectId } },
    })
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setLink(data)
  }, [client, projectId])

  useEffect(() => {
    void load()
  }, [load])

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
          <p className="text-muted-foreground">
            This project is not linked to a Linear project. Set <code>LINEAR_API_KEY</code> and lead the project in
            Linear, or list it in <code>LINEAR_PROJECT_IDS</code>.
          </p>
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
            <div>
              <Button size="sm" variant="ghost" onClick={syncNow} disabled={syncing}>
                <RefreshCw className={syncing ? 'size-4 animate-spin' : 'size-4'} /> Sync now
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
