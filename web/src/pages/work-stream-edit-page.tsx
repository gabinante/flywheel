import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { DetailPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type WorkStream = components['schemas']['WorkStream']

export function WorkStreamEditPage() {
  const { orgId, projectId, workStreamId } = useParams<{
    orgId: string
    projectId: string
    workStreamId: string
  }>()
  const navigate = useNavigate()
  const { client } = useAuth()
  const [stream, setStream] = useState<WorkStream | null | undefined>(undefined)
  const [name, setName] = useState('')
  const [plan, setPlan] = useState('')
  const [branch, setBranch] = useState('')
  const [status, setStatus] = useState<'active' | 'closed'>('active')
  const [err, setErr] = useState<string | null>(null)
  const [formErr, setFormErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  useEffect(() => {
    if (!projectId || !workStreamId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET(
        '/projects/{projectID}/work-streams/{workStreamID}',
        {
          params: {
            path: { projectID: projectId, workStreamID: workStreamId },
          },
        },
      )
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setStream(null)
        return
      }
      setErr(null)
      const s = data ?? null
      setStream(s)
      if (s) {
        setName(s.name ?? '')
        setPlan(s.plan ?? '')
        setBranch(s.branch ?? '')
        setStatus(s.status === 'closed' ? 'closed' : 'active')
      }
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId, workStreamId])

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    if (!projectId || !workStreamId) return
    const trimmed = name.trim()
    if (!trimmed) {
      setFormErr('Name is required.')
      return
    }
    setBusy(true)
    setFormErr(null)
    setSaved(false)
    const { data, error, response } = await client.PATCH(
      '/projects/{projectID}/work-streams/{workStreamID}',
      {
        params: {
          path: { projectID: projectId, workStreamID: workStreamId },
        },
        body: {
          name: trimmed,
          plan: plan.trim(),
          branch: branch.trim() || undefined,
          status,
        },
      },
    )
    setBusy(false)
    if (!response.ok) {
      setFormErr(formatApiError(error))
      return
    }
    setSaved(true)
    if (data) {
      setStream(data)
      setName(data.name ?? '')
      setPlan(data.plan ?? '')
      setBranch(data.branch ?? '')
      setStatus(data.status === 'closed' ? 'closed' : 'active')
    }
  }

  if (!orgId || !projectId || !workStreamId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }
  if (err) {
    return <p className="text-destructive text-sm">{err}</p>
  }
  if (stream === undefined) {
    return <DetailPageSkeleton />
  }
  if (!stream) {
    return <p className="text-muted-foreground text-sm">Work stream not found.</p>
  }

  const streamTicketsHref = `/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(workStreamId)}`
  const streamCrumbLabel = stream.name ?? stream.slug ?? stream.id

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgId}
            projectId={projectId}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          <Link to={streamTicketsHref} className="hover:underline">
            {streamCrumbLabel}
          </Link>
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            Manage
          </span>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-xl font-semibold tracking-tight">
            Manage work stream
          </h1>
          <Badge variant="outline">{stream.slug ?? stream.id}</Badge>
        </div>
        <p className="text-muted-foreground font-mono text-xs">{stream.id}</p>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => navigate(-1)}
        >
          Back
        </Button>
        <Button asChild variant="secondary" size="sm">
          <Link
            to={`/orgs/${orgId}/projects/${projectId}/tickets?work_stream_id=${encodeURIComponent(workStreamId)}`}
          >
            Tickets in this stream
          </Link>
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Edit work stream</CardTitle>
          <CardDescription>
            Update display fields, Git branch name, or close the stream when
            work is finished.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-4" onSubmit={handleSave}>
            {formErr ? (
              <p className="text-destructive text-sm">{formErr}</p>
            ) : null}
            {saved ? (
              <p className="text-muted-foreground text-sm">Saved.</p>
            ) : null}
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-muted-foreground">Name</span>
              <input
                className="border-input bg-background h-8 rounded-md border px-2 text-sm disabled:opacity-50"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
                required
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-muted-foreground">Plan (Markdown)</span>
              <span className="text-muted-foreground text-xs">
                GFM, fenced code with a language for highlighting,{' '}
                <code className="font-mono">{'```mermaid'}</code> for diagrams.
                Leave empty to clear.
              </span>
              <textarea
                className="border-input bg-background font-mono min-h-[200px] rounded-md border px-2 py-2 text-sm disabled:opacity-50"
                value={plan}
                onChange={(e) => setPlan(e.target.value)}
                disabled={busy}
                rows={12}
                spellCheck={false}
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-muted-foreground">Branch</span>
              <input
                className="border-input bg-background h-8 rounded-md border px-2 font-mono text-sm disabled:opacity-50"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                disabled={busy}
                placeholder="e.g. feature/my-stream"
              />
            </label>
            <label className="flex flex-col gap-1 text-sm">
              <span className="text-muted-foreground">Status</span>
              <select
                className="border-input bg-background h-8 rounded-md border px-2 text-sm disabled:opacity-50"
                value={status}
                onChange={(e) =>
                  setStatus(e.target.value as 'active' | 'closed')
                }
                disabled={busy}
              >
                <option value="active">active</option>
                <option value="closed">closed</option>
              </select>
            </label>
            <Button type="submit" disabled={busy}>
              {busy ? 'Saving…' : 'Save changes'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
