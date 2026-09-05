import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { GitMerge, TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { StyledSelect } from '@/components/ui/styled-select'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']

/**
 * Merge this project into another one. Everything owned by this project (tickets,
 * work streams, repositories, reports, orchestrator threads, and the Linear link when
 * the target has none) moves to the target; this project is then deleted.
 */
export function ProjectMergeCard({ project, orgId, orgSlug }: { project: Project; orgId: string; orgSlug: string }) {
  const { client } = useAPI()
  const navigate = useNavigate()
  const [candidates, setCandidates] = useState<Project[]>([])
  const [target, setTarget] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void client
      .GET('/orgs/{orgID}/projects', { params: { path: { orgID: orgId }, query: { status: 'all' } } })
      .then(({ data }) => {
        if (cancelled || !data) return
        setCandidates(data.filter((p) => p.id !== project.id))
      })
    return () => {
      cancelled = true
    }
  }, [client, orgId, project.id])

  const options = useMemo(
    () => candidates.map((p) => ({ value: p.id ?? '', label: `${p.name ?? p.slug ?? p.id}${p.status === 'closed' ? ' (closed)' : ''}` })),
    [candidates],
  )
  const targetProject = candidates.find((p) => p.id === target)

  const merge = async () => {
    if (!targetProject || !project.id) return
    const ok = window.confirm(
      `Merge “${project.name}” into “${targetProject.name}”?\n\nAll tickets, work streams, repositories, reports, and the Linear link move to “${targetProject.name}”. “${project.name}” is deleted. This cannot be undone.`,
    )
    if (!ok) return
    setBusy(true)
    setErr(null)
    const { data, error, response } = await client.POST('/projects/{projectID}/merge', {
      params: { path: { projectID: project.id } },
      body: { into: targetProject.id ?? '' },
    })
    setBusy(false)
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    const slug = data.target.slug ?? data.target.id ?? ''
    navigate(`/orgs/${orgSlug}/projects/${slug}/settings`, {
      replace: true,
      state: {
        mergeNotice: `Merged “${data.source_name}” into “${data.target.name}”: ${data.tickets_moved} tickets, ${data.work_streams_moved} work streams, ${data.repos_moved} repositories${data.linear_link_moved ? ', Linear link' : ''}.`,
      },
    })
  }

  return (
    <Card className="border-amber-400/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <GitMerge className="size-4 text-amber-300" />
          Merge into another project
        </CardTitle>
        <CardDescription>
          Use this to fold a duplicate into the project you want to keep — for example a hand-made project and the one Linear sync
          created for the same work. Tickets, work streams, repositories, reports, command-center threads, and the Linear link (if the
          keeper has none) move over; empty fields on the keeper are filled from this project; then this project is deleted.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="merge-target" className="text-xs uppercase tracking-wide text-muted-foreground">
            Keep this project
          </Label>
          <StyledSelect
            id="merge-target"
            className="h-9 w-full max-w-md min-w-0"
            value={target}
            onValueChange={setTarget}
            options={options}
            placeholder={options.length ? 'Choose the project to merge into…' : 'No other projects'}
            disabled={!options.length}
          />
        </div>
        {targetProject && (
          <div className="flex items-start gap-2 rounded-lg border border-amber-400/20 bg-amber-400/5 px-3 py-2 text-xs text-amber-100/90">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0 text-amber-300" />
            <span>
              <strong>{project.name}</strong> will be deleted and everything in it will belong to <strong>{targetProject.name}</strong>. If both are
              linked to different Linear projects, unlink one first.
            </span>
          </div>
        )}
        {err && <p className="text-sm text-destructive">{err}</p>}
        <Button variant="destructive" disabled={!targetProject || busy} onClick={merge}>
          <GitMerge className="mr-1.5 size-4" />
          {busy ? 'Merging…' : targetProject ? `Merge into ${targetProject.name}` : 'Merge'}
        </Button>
      </CardContent>
    </Card>
  )
}
