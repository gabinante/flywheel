import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
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
import { useProjectBreadcrumbLabel } from '@/hooks/use-project-breadcrumb-label'
import { formatApiError } from '@/lib/api/client'

export function WorkStreamCreatePage() {
  const { orgId, projectId } = useParams<{
    orgId: string
    projectId: string
  }>()
  const navigate = useNavigate()
  const { client } = useAuth()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [plan, setPlan] = useState('')
  const [busy, setBusy] = useState(false)
  const [formErr, setFormErr] = useState<string | null>(null)
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
        `/orgs/${orgId}/projects/${projectId}/work-streams/${data.id}`,
        { replace: true },
      )
      return
    }
    navigate(`/orgs/${orgId}/projects/${projectId}`, { replace: true })
  }

  if (!orgId || !projectId) {
    return <p className="text-destructive text-sm">Missing route params.</p>
  }

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
          <Link to={`/orgs/${orgId}/projects/${projectId}`}>Cancel</Link>
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Details</CardTitle>
          <CardDescription>
            Name is required. Slug is optional (URL-safe id).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-4" onSubmit={handleSubmit}>
            {formErr ? (
              <p className="text-destructive text-sm">{formErr}</p>
            ) : null}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="wsc-name">Name</Label>
              <Input
                id="wsc-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
                placeholder="e.g. Web UI rollout"
                required
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="wsc-slug">Slug (optional)</Label>
              <Input
                id="wsc-slug"
                className="font-mono"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                disabled={busy}
                placeholder="url-safe-id"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="wsc-plan">Plan — Markdown (optional)</Label>
              <span className="text-muted-foreground text-xs">
                Optional implementation plan: GFM, code fences with language,
                Mermaid diagrams in <code className="font-mono">{'```mermaid'}</code>{' '}
                blocks.
              </span>
              <Textarea
                id="wsc-plan"
                className="min-h-[120px] font-mono"
                value={plan}
                onChange={(e) => setPlan(e.target.value)}
                disabled={busy}
                rows={8}
                spellCheck={false}
              />
            </div>
            <Button type="submit" disabled={busy}>
              {busy ? 'Creating…' : 'Create work stream'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
