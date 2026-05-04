import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { PlanMarkdown } from '@/components/plan-markdown'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from '@/contexts/use-auth'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'

export function WorkStreamCreatePage() {
  const { orgId, projectId, orgSlug, projectSlug, base } = useProjectPaths()
  const navigate = useNavigate()
  const { client } = useAuth()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [plan, setPlan] = useState('')
  const [busy, setBusy] = useState(false)
  const [formErr, setFormErr] = useState<string | null>(null)
  const [showPreview, setShowPreview] = useState(false)
  const projectLabel = useProjectBreadcrumbLabel(projectId)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!projectId) return
    const trimmed = name.trim()
    if (!trimmed) {
      setFormErr('Name is required.')
      return
    }
    setBusy(true)
    setFormErr(null)
    const { data, error, response } = await client.POST(
      '/projects/{projectID}/work-streams',
      {
        params: { path: { projectID: projectId } },
        body: {
          name: trimmed,
          slug: slug.trim() || undefined,
          plan: plan.trim() || undefined,
        },
      },
    )
    setBusy(false)
    if (!response.ok) {
      setFormErr(formatApiError(error))
      return
    }
    if (data?.id) {
      navigate(
        `${base}/work-streams/${data.id}`,
        { replace: true },
      )
      return
    }
    navigate(base, { replace: true })
  }

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }

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
          <span className="text-foreground" aria-current="page">
            New work stream
          </span>
        </p>
        <h1 className="text-xl font-semibold tracking-tight">
          New work stream
        </h1>
        <p className="text-muted-foreground text-sm">
          Create a stream to group tickets. You can set the Git branch on the
          next screen.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button asChild variant="outline" size="sm">
          <Link to={base}>Cancel</Link>
        </Button>
      </div>

      <Card className="bg-white/[0.03] backdrop-blur-md border-white/10">
        <CardHeader>
          <CardTitle className="text-sm">Details</CardTitle>
          <CardDescription>
            Name is required. Slug is optional (URL-safe id).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
            {formErr ? (
              <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive backdrop-blur-sm">
                {formErr}
              </div>
            ) : null}

            <Label>
              <span>Name</span>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
                placeholder="e.g. Web UI rollout"
                required
              />
            </Label>

            <Label>
              <span>Slug (optional)</span>
              <Input
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                disabled={busy}
                placeholder="url-safe-id"
                className="font-mono"
              />
            </Label>

            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <Label className="flex-row items-center gap-0" htmlFor="plan-field">
                  <span>Plan — Markdown (optional)</span>
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
                GFM, code fences with language,{' '}
                <code className="font-mono text-[10px] rounded bg-white/10 px-1 py-0.5">
                  {'```mermaid'}
                </code>{' '}
                for diagrams.
              </p>
              {showPreview && plan.trim() ? (
                <div className="min-h-[120px] rounded-lg border border-white/10 bg-white/[0.03] p-3 backdrop-blur-sm">
                  <PlanMarkdown markdown={plan} />
                </div>
              ) : (
                <Textarea
                  id="plan-field"
                  className="font-mono min-h-[120px]"
                  value={plan}
                  onChange={(e) => setPlan(e.target.value)}
                  disabled={busy}
                  rows={8}
                  spellCheck={false}
                />
              )}
            </div>

            <Button type="submit" disabled={busy} className="self-start">
              {busy ? 'Creating...' : 'Create work stream'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
