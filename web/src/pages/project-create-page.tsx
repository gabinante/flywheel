import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Check, Layers, TicketCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
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
import { useAPI } from '@/contexts/use-api'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { formatApiError } from '@/lib/api/client'

interface WorkstreamTemplate {
  id: string
  name: string
  slug: string
  description: string
  tickets: { title: string }[]
}

interface ProjectTemplate {
  id: string
  name: string
  slug: string
  description: string
  workstream_template_ids: string[]
  workstream_templates: WorkstreamTemplate[]
}

export function ProjectCreatePage() {
  const { orgId, orgParam } = useResolvedRouteParams()
  const navigate = useNavigate()
  const { client } = useAPI()

  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [repoUrl, setRepoUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [formErr, setFormErr] = useState<string | null>(null)

  const [mode, setMode] = useState<'blank' | 'template'>('blank')
  const [templates, setTemplates] = useState<ProjectTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(true)
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null)
  const [selectedWsIds, setSelectedWsIds] = useState<Set<string>>(new Set())

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch('/project-templates')
        if (res.ok) {
          const data = await res.json()
          if (!cancelled) setTemplates(Array.isArray(data) ? data : [])
        }
      } catch {
        // templates unavailable — blank mode only
      }
      if (!cancelled) setTemplatesLoading(false)
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const selectedTemplate = templates.find((t) => t.id === selectedTemplateId) ?? null

  function selectTemplate(id: string) {
    setSelectedTemplateId(id)
    const tpl = templates.find((t) => t.id === id)
    if (tpl) {
      setSelectedWsIds(new Set(tpl.workstream_template_ids))
    }
  }

  function toggleWs(wsId: string) {
    setSelectedWsIds((prev) => {
      const next = new Set(prev)
      if (next.has(wsId)) {
        next.delete(wsId)
      } else {
        next.add(wsId)
      }
      return next
    })
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!orgId) return
    const trimmed = name.trim()
    if (!trimmed) {
      setFormErr('Name is required.')
      return
    }
    setBusy(true)
    setFormErr(null)

    const body: Record<string, unknown> = { name: trimmed }
    if (slug.trim()) body.slug = slug.trim()
    if (repoUrl.trim()) body.repo_url = repoUrl.trim()

    if (mode === 'template' && selectedTemplateId) {
      body.template_id = selectedTemplateId
      body.work_stream_template_ids = Array.from(selectedWsIds)
    }

    const { data, error, response } = await client.POST('/orgs/{orgID}/projects', {
      params: { path: { orgID: orgId } },
      body: body as never,
    })
    setBusy(false)
    if (!response.ok) {
      setFormErr(formatApiError(error))
      return
    }
    if (data?.id) {
      const projectSlug = data.slug ?? data.id
      navigate(`/orgs/${orgParam}/projects/${projectSlug}/command`, { replace: true })
      return
    }
    navigate(`/orgs/${orgParam}/projects`, { replace: true })
  }

  if (!orgId) {
    return <p className="text-destructive text-sm">Missing org id.</p>
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <p className="text-muted-foreground text-xs">
          <Link to="/orgs" className="hover:underline">
            Organizations
          </Link>
          <span className="px-1">/</span>
          <Link to={`/orgs/${orgParam}/projects`} className="hover:underline">
            Projects
          </Link>
          <span className="px-1">/</span>
          <span className="text-foreground" aria-current="page">
            New project
          </span>
        </p>
        <h1 className="text-xl font-semibold tracking-tight">New project</h1>
        <p className="text-muted-foreground text-sm">
          Create a blank project or start from a template with pre-configured workstreams and tickets.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        <Button asChild variant="outline" size="sm">
          <Link to={`/orgs/${orgParam}/projects`}>Cancel</Link>
        </Button>
      </div>

      {/* Mode toggle */}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => setMode('blank')}
          className={`rounded-lg border px-4 py-2 text-sm font-medium transition-all duration-200 ${
            mode === 'blank'
              ? 'border-primary/40 bg-primary/10 text-primary'
              : 'border-white/10 bg-white/[0.03] text-muted-foreground hover:bg-white/[0.06]'
          }`}
        >
          Blank
        </button>
        <button
          type="button"
          onClick={() => setMode('template')}
          disabled={templatesLoading || templates.length === 0}
          className={`rounded-lg border px-4 py-2 text-sm font-medium transition-all duration-200 ${
            mode === 'template'
              ? 'border-primary/40 bg-primary/10 text-primary'
              : 'border-white/10 bg-white/[0.03] text-muted-foreground hover:bg-white/[0.06]'
          } disabled:opacity-40 disabled:cursor-not-allowed`}
        >
          From Template
        </button>
      </div>

      <Card className="bg-white/[0.03] backdrop-blur-md border-white/10">
        <CardHeader>
          <CardTitle className="text-sm">Project details</CardTitle>
          <CardDescription>Name is required. Slug and repo URL are optional.</CardDescription>
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
                placeholder="e.g. My SaaS App"
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

            <Label>
              <span>Repo URL (optional)</span>
              <Input
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                disabled={busy}
                placeholder="https://github.com/org/repo"
                className="font-mono"
              />
            </Label>

            {/* Template selector */}
            {mode === 'template' && (
              <div className="flex flex-col gap-4">
                <div className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Choose a template</span>
                  <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    {templates.map((tpl) => (
                      <button
                        key={tpl.id}
                        type="button"
                        onClick={() => selectTemplate(tpl.id)}
                        className={`relative flex flex-col gap-2 rounded-xl border p-4 text-left transition-all duration-200 ${
                          selectedTemplateId === tpl.id
                            ? 'border-primary/40 bg-primary/[0.08] ring-1 ring-primary/20'
                            : 'border-white/10 bg-white/[0.02] hover:bg-white/[0.05]'
                        }`}
                      >
                        {selectedTemplateId === tpl.id && (
                          <div className="absolute right-3 top-3">
                            <Check className="h-4 w-4 text-primary" />
                          </div>
                        )}
                        <span className="text-sm font-semibold">{tpl.name}</span>
                        <span className="text-xs text-muted-foreground leading-relaxed">
                          {tpl.description}
                        </span>
                        <div className="flex items-center gap-3 pt-1">
                          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground/70">
                            <Layers className="h-3 w-3" />
                            {tpl.workstream_templates?.length ?? 0} workstreams
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>

                {/* Workstream checklist */}
                {selectedTemplate && (
                  <div className="flex flex-col gap-2">
                    <span className="text-sm font-medium">
                      Workstreams to include
                    </span>
                    <div className="flex flex-col gap-1.5">
                      {selectedTemplate.workstream_templates?.map((ws) => (
                        <label
                          key={ws.id}
                          className={`flex items-start gap-3 rounded-lg border p-3 transition-all duration-150 cursor-pointer ${
                            selectedWsIds.has(ws.id)
                              ? 'border-primary/30 bg-primary/[0.05]'
                              : 'border-white/10 bg-white/[0.02] opacity-60'
                          }`}
                        >
                          <input
                            type="checkbox"
                            checked={selectedWsIds.has(ws.id)}
                            onChange={() => toggleWs(ws.id)}
                            className="mt-0.5 h-4 w-4 rounded border-white/20 bg-white/10 accent-primary"
                          />
                          <div className="flex flex-1 flex-col gap-1 min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-medium">{ws.name}</span>
                              <Badge
                                variant="outline"
                                className="text-[10px] px-1.5 py-0 shrink-0"
                              >
                                <TicketCheck className="mr-0.5 h-2.5 w-2.5" />
                                {ws.tickets?.length ?? 0}
                              </Badge>
                            </div>
                            <span className="text-xs text-muted-foreground leading-relaxed">
                              {ws.description}
                            </span>
                          </div>
                        </label>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}

            <Button type="submit" disabled={busy} className="self-start">
              {busy ? 'Creating...' : 'Create project'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
