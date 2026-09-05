import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { PlanMarkdown } from '@/components/plan-markdown'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { DetailPageSkeleton } from '@/components/ui/skeleton'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type WorkStream = components['schemas']['WorkStream']

export function WorkStreamEditPage() {
  const { workStreamId } = useResolvedRouteParams()
  const { orgId, projectId, orgSlug, projectSlug, base } = useProjectPaths()
  const navigate = useNavigate()
  const { client } = useAPI()
  const [stream, setStream] = useState<WorkStream | null | undefined>(undefined)
  const [name, setName] = useState('')
  const [plan, setPlan] = useState('')
  const [branch, setBranch] = useState('')
  const [status, setStatus] = useState<'active' | 'closed'>('active')
  const [err, setErr] = useState<string | null>(null)
  const [formErr, setFormErr] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [showPreview, setShowPreview] = useState(false)
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

  const streamTicketsHref = `${base}/tickets?work_stream_id=${encodeURIComponent(workStreamId)}`
  const streamCrumbLabel = stream.name ?? stream.slug ?? stream.id

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <OrgProjectCrumbs
            orgId={orgSlug}
            projectId={projectSlug}
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
            to={`${base}/tickets?work_stream_id=${encodeURIComponent(workStreamId)}`}
          >
            Tickets in this stream
          </Link>
        </Button>
      </div>

      <Card className="bg-white/[0.03] backdrop-blur-md border-white/10">
        <CardHeader>
          <CardTitle className="text-sm">Edit work stream</CardTitle>
          <CardDescription>
            Update display fields, Git branch name, or close the stream when
            work is finished.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={handleSave}>
            <AnimatePresence>
              {formErr ? (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="overflow-hidden"
                >
                  <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive backdrop-blur-sm">
                    {formErr}
                  </div>
                </motion.div>
              ) : null}
              {saved ? (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: 'auto' }}
                  exit={{ opacity: 0, height: 0 }}
                  className="overflow-hidden"
                >
                  <div className="rounded-lg border border-primary/30 bg-primary/10 px-3 py-2 text-sm text-primary backdrop-blur-sm">
                    Changes saved successfully.
                  </div>
                </motion.div>
              ) : null}
            </AnimatePresence>

            <Label>
              <span>Name</span>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
                required
              />
            </Label>

            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <Label className="flex-row items-center gap-0" htmlFor="plan-edit-field">
                  <span>Plan (Markdown)</span>
                </Label>
                {plan.trim() ? (
                  <button
                    type="button"
                    className="text-xs text-muted-foreground hover:text-foreground transition-colors duration-150"
                    onClick={() => setShowPreview(!showPreview)}
                  >
                    {showPreview ? 'Edit' : 'Preview'}
                  </button>
                ) : null}
              </div>
              <p className="text-muted-foreground text-xs">
                GFM, fenced code with a language for highlighting,{' '}
                <code className="font-mono text-[10px] rounded bg-white/10 px-1 py-0.5">
                  {'```mermaid'}
                </code>{' '}
                for diagrams. Leave empty to clear.
              </p>
              {showPreview && plan.trim() ? (
                <div className="min-h-[200px] rounded-lg border border-white/10 bg-white/[0.03] p-3 backdrop-blur-sm">
                  <PlanMarkdown markdown={plan} />
                </div>
              ) : (
                <Textarea
                  id="plan-edit-field"
                  className="font-mono min-h-[200px]"
                  value={plan}
                  onChange={(e) => setPlan(e.target.value)}
                  disabled={busy}
                  rows={12}
                  spellCheck={false}
                />
              )}
            </div>

            <Label>
              <span>Branch</span>
              <Input
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                disabled={busy}
                placeholder="e.g. feature/my-stream"
                className="font-mono"
              />
            </Label>

            <div className="flex flex-col gap-2">
              <span className="text-sm font-medium text-muted-foreground">Status</span>
              <div className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.03] px-4 py-3 backdrop-blur-sm">
                <Switch
                  id="status-switch"
                  checked={status === 'active'}
                  onCheckedChange={(checked) =>
                    setStatus(checked ? 'active' : 'closed')
                  }
                  disabled={busy}
                />
                <label htmlFor="status-switch" className="flex flex-col gap-0.5 cursor-pointer">
                  <span className={`text-sm font-medium ${status === 'active' ? 'text-primary' : 'text-muted-foreground'}`}>
                    {status === 'active' ? 'Active' : 'Closed'}
                  </span>
                  <span className="text-xs text-muted-foreground/70">
                    {status === 'active'
                      ? 'Tickets can be assigned and dispatched'
                      : 'Stream is archived, no new work'}
                  </span>
                </label>
              </div>
            </div>

            <Button type="submit" disabled={busy} className="self-start">
              {busy ? 'Saving...' : 'Save changes'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
