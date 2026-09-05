import { useDraft } from '@/hooks/use-draft'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import {
  ArrowRight,
  Check,
  Copy,
  FileText,
  GitBranch,
  GitMerge,
  Link2,
  Monitor,
  Save,
  Settings,
  SlidersHorizontal,
  Workflow,
} from 'lucide-react'

import { OrgProjectCrumbs } from '@/components/org-project-crumbs'
import { ProjectBasePromptCard } from '@/components/project-base-prompt-card'
import { ProjectLinearCard } from '@/components/project-linear-card'
import { ProjectReportsCard } from '@/components/project-reports-card'
import { ProjectMergeCard } from '@/components/project-merge-card'
import { WorkflowTimelineEditor } from '@/components/workflow-timeline-editor'
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
import { ProjectPageSkeleton } from '@/components/ui/skeleton'
import { useAPI } from '@/contexts/use-api'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']

type SettingsSectionID = 'dispatch' | 'workflow' | 'prompts' | 'linear' | 'reports' | 'merge'

const SETTINGS_SECTIONS: Array<{
  id: SettingsSectionID
  label: string
  description: string
  icon: React.ComponentType<{ className?: string }>
}> = [
  {
    id: 'prompts',
    label: 'Base prompt',
    description: 'Project-wide system prompt, conventions, and key files injected into all worker prompts.',
    icon: FileText,
  },
  {
    id: 'dispatch',
    label: 'Server-side dispatch',
    description: 'Enable or pause automatic server-side ticket pickup. Users can still claim tickets via local Claude Code + MCP when disabled.',
    icon: SlidersHorizontal,
  },
  {
    id: 'workflow',
    label: 'Workflow stages',
    description: 'Define the staged ticket lifecycle for this project.',
    icon: Workflow,
  },
  {
    id: 'linear',
    label: 'Linear',
    description: 'The Linear project this project mirrors, sync status, and manual sync.',
    icon: Link2,
  },
  {
    id: 'reports',
    label: 'Reports',
    description: 'Preview and post the Linear project status update and the weekly roundup.',
    icon: FileText,
  },
  {
    id: 'merge',
    label: 'Merge',
    description: 'Fold this project into another one (for duplicates) and delete it.',
    icon: GitMerge,
  },
]

function isSettingsSection(value: string | undefined): value is SettingsSectionID {
  return SETTINGS_SECTIONS.some((section) => section.id === value)
}

function ScopeModelCard() {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Settings className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Scope model</CardTitle>
        </div>
        <CardDescription>
          Some settings are inherited from the installation; this page is for
          project-specific overrides.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 md:grid-cols-3">
        <div className="rounded-2xl border border-white/10 bg-black/10 p-4">
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="outline">Global</Badge>
            <span className="text-sm font-medium">Runtime defaults</span>
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">
            Server-level runner defaults, provider credentials, and fallback worker
            behavior are still operator-managed through environment/config.
          </p>
        </div>
        <div className="rounded-2xl border border-white/10 bg-black/10 p-4">
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="outline">Project</Badge>
            <span className="text-sm font-medium">Overrides</span>
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">
            Active worker limits, configured executors, custom roles, routing
            workflows, and worker routing are stored on this project.
          </p>
        </div>
        <div className="rounded-2xl border border-white/10 bg-black/10 p-4">
          <div className="mb-2 flex items-center gap-2">
            <Badge variant="outline">Org</Badge>
            <span className="text-sm font-medium">Default layer</span>
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">
            The right long-term home for shared worker pools and reusable workflows
            is an org/default layer that projects can inherit or override.
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

function DispatchControlCard({
  projectId,
  project,
  saving,
  onToggle,
  onProjectChange,
}: {
  projectId: string
  project: Project
  saving: boolean
  onToggle: () => void
  onProjectChange: (project: Project) => void
}) {
  const { client } = useAPI()
  const dispatchOn = project.dispatch_enabled !== false
  const currentMax =
    typeof project.dispatch_config?.max_active_workers === 'number' &&
    project.dispatch_config.max_active_workers > 0
      ? project.dispatch_config.max_active_workers
      : 0
  const [maxWorkers, setMaxWorkers] = useDraft(currentMax)
  const [limitSaving, setLimitSaving] = useState(false)
  const [limitSavedAt, setLimitSavedAt] = useState<number | null>(null)
  const [limitError, setLimitError] = useState<string | null>(null)



  async function saveLimit() {
    setLimitSaving(true)
    setLimitError(null)
    setLimitSavedAt(null)
    const nextConfig = {
      ...(project.dispatch_config ?? {}),
      max_active_workers: maxWorkers > 0 ? maxWorkers : undefined,
    }
    const { data, error: apiError, response } = await client.PATCH(
      '/projects/{projectID}',
      {
        params: { path: { projectID: projectId } },
        body: { dispatch_config: nextConfig },
      },
    )
    if (!response.ok || !data) {
      setLimitError(formatApiError(apiError))
      setLimitSaving(false)
      return
    }
    onProjectChange(data)
    setLimitSavedAt(Date.now())
    setLimitSaving(false)
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <SlidersHorizontal className="size-4 text-muted-foreground" />
            <CardTitle className="text-sm">Server-side auto-dispatch</CardTitle>
          </div>
          <CardDescription>
            Controls server-side automatic ticket pickup. When disabled, users
            can still claim tickets via local Claude Code + MCP.
          </CardDescription>
        </div>
        <Button
          variant={dispatchOn ? 'default' : 'outline'}
          size="sm"
          className={
            dispatchOn ? 'shrink-0 bg-emerald-600 hover:bg-emerald-700' : 'shrink-0'
          }
          disabled={saving}
          onClick={onToggle}
        >
          {saving ? 'Saving...' : dispatchOn ? 'Dispatch enabled' : 'Dispatch disabled'}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs leading-relaxed text-muted-foreground">
          Turning dispatch off leaves existing tickets and settings unchanged.
        </p>
        <section className="rounded-2xl border border-white/10 bg-black/10 p-4">
          <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_12rem] md:items-end">
            <div className="space-y-1">
              <h3 className="text-sm font-medium text-foreground">
                Active worker limit
              </h3>
              <p className="text-xs leading-relaxed text-muted-foreground">
                Maximum workers this project can run at once across implementation,
                review, and conflict-resolution work. Use 0 to inherit the server
                default.
              </p>
            </div>
            <Label>
              Max active
              <Input
                type="number"
                min={0}
                step={1}
                value={maxWorkers}
                onChange={(event) =>
                  setMaxWorkers(Math.max(0, Math.floor(Number(event.target.value) || 0)))
                }
              />
            </Label>
          </div>
          {maxWorkers !== currentMax ? (
            <div className="mt-3 flex items-center gap-2">
              <Button size="xs" onClick={() => void saveLimit()} disabled={limitSaving}>
                <Save className="size-3.5" />
                {limitSaving ? 'Saving…' : 'Save'}
              </Button>
              {limitError ? (
                <span className="text-xs text-destructive">{limitError}</span>
              ) : null}
            </div>
          ) : null}
          {limitSavedAt ? (
            <span className="mt-2 block text-xs text-emerald-400">Saved</span>
          ) : null}
        </section>
      </CardContent>
    </Card>
  )
}

function RepositorySettingsCard({
  projectId,
  project,
  onProjectChange,
}: {
  projectId: string
  project: Project
  onProjectChange: (project: Project) => void
}) {
  const { client } = useAPI()
  const [repoUrl, setRepoUrl] = useDraft(project.repo_url ?? '')
  const [defaultBranch, setDefaultBranch] = useDraft(project.default_branch ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)



  async function saveRepo() {
    setSaving(true)
    setError(null)
    setSavedAt(null)
    const { data, error: apiError, response } = await client.PATCH(
      '/projects/{projectID}',
      {
        params: { path: { projectID: projectId } },
        body: { repo_url: repoUrl || undefined, default_branch: defaultBranch || undefined },
      },
    )
    if (!response.ok || !data) {
      setError(formatApiError(apiError))
      setSaving(false)
      return
    }
    onProjectChange(data)
    setSavedAt(Date.now())
    setSaving(false)
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <div className="flex items-center gap-2">
          <GitBranch className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Repository</CardTitle>
        </div>
        <CardDescription>
          Git repository for this project. Workers clone and push to this repo.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              Repository URL
            </span>
            <Input
              value={repoUrl}
              onChange={(e) => setRepoUrl(e.target.value)}
              placeholder="git@github.com:org/repo.git"
            />
          </label>
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              Default branch
            </span>
            <Input
              value={defaultBranch}
              onChange={(e) => setDefaultBranch(e.target.value)}
              placeholder="main"
            />
          </label>
        </div>
        <div className="flex items-center gap-2">
          <Button size="xs" onClick={() => void saveRepo()} disabled={saving}>
            <Save className="size-3.5" />
            {saving ? 'Saving…' : 'Save'}
          </Button>
          {savedAt ? (
            <span className="text-xs text-emerald-400">Saved</span>
          ) : null}
          {error ? (
            <span className="text-xs text-destructive">{error}</span>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

function ConnectLocalWorkerCard() {
  const [config, setConfig] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [loading, setLoading] = useState(false)

  async function fetchConfig() {
    setLoading(true)
    const resp = await fetch('/worker-config')
    if (resp.ok) {
      const data = await resp.json()
      setConfig(JSON.stringify(data, null, 2))
    }
    setLoading(false)
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Monitor className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Connect local worker</CardTitle>
        </div>
        <CardDescription>
          Run Claude Code locally against this Flywheel server. Paste the MCP config
          into your <code className="text-xs">~/.claude/settings.json</code> or project settings.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {config === null ? (
          <Button size="sm" variant="outline" onClick={() => void fetchConfig()} disabled={loading}>
            {loading ? 'Loading...' : 'Show MCP config'}
          </Button>
        ) : (
          <>
            <pre className="overflow-x-auto rounded-lg border border-white/10 bg-black/30 p-3 text-xs text-muted-foreground">
              {config}
            </pre>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => {
                navigator.clipboard.writeText(config)
                setCopied(true)
                setTimeout(() => setCopied(false), 2000)
              }}
            >
              {copied ? <Check className="size-3.5 text-emerald-400" /> : <Copy className="size-3.5" />}
              {copied ? 'Copied' : 'Copy to clipboard'}
            </Button>
          </>
        )}
        <p className="text-xs leading-relaxed text-muted-foreground">
          After adding the config, use <code>claim_ticket</code> and other MCP tools
          from your local Claude Code session to work on tickets.
        </p>
      </CardContent>
    </Card>
  )
}

function SettingsOverview({
  basePath,
  projectId,
  project,
  onProjectChange,
}: {
  basePath: string
  projectId: string
  project: Project
  onProjectChange: (project: Project) => void
}) {
  return (
    <div className="space-y-6">
      <RepositorySettingsCard projectId={projectId} project={project} onProjectChange={onProjectChange} />
      <ConnectLocalWorkerCard />
      <ScopeModelCard />
      <div className="grid gap-3 md:grid-cols-2">
        {SETTINGS_SECTIONS.map((section) => (
          <Link
            key={section.id}
            to={`${basePath}/settings/${section.id}`}
            className="group"
          >
            <Card className="h-full border-white/10 bg-white/5 backdrop-blur-md transition-colors hover:border-white/20 hover:bg-white/10">
              <CardContent className="flex h-full items-start gap-4 px-5 py-5">
                <div className="rounded-xl border border-white/10 bg-black/20 p-2.5">
                  <section.icon className="size-5 text-foreground" />
                </div>
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex items-center justify-between gap-3">
                    <h2 className="text-sm font-semibold text-foreground">
                      {section.label}
                    </h2>
                    <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    {section.description}
                  </p>
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>
    </div>
  )
}

export function ProjectSettingsPage() {
  const { section } = useResolvedRouteParams()
  const { orgId, projectId, orgSlug, projectSlug, base } = useProjectPaths()
  const { client } = useAPI()
  const [project, setProject] = useState<Project | null | undefined>(undefined)
  const [err, setErr] = useState<string | null>(null)
  const [dispatchSaving, setDispatchSaving] = useState(false)

  useEffect(() => {
    if (!projectId) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/projects/{projectID}', {
        params: { path: { projectID: projectId } },
      })
      if (cancelled) return
      if (!response.ok) {
        setErr(formatApiError(error))
        setProject(null)
        return
      }
      setErr(null)
      setProject(data ?? null)
    })()
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  const toggleDispatch = useCallback(async () => {
    if (!projectId || !project) return
    setDispatchSaving(true)
    const nextValue = !(project.dispatch_enabled !== false)
    const { error, response } = await client.PATCH('/projects/{projectID}', {
      params: { path: { projectID: projectId } },
      body: { dispatch_enabled: nextValue },
    })
    if (response.ok) {
      setProject((prev) =>
        prev ? { ...prev, dispatch_enabled: nextValue } : prev,
      )
      setErr(null)
    } else {
      setErr(formatApiError(error))
    }
    setDispatchSaving(false)
  }, [client, project, projectId])

  const projectLabel = useMemo(
    () => project?.name ?? project?.slug ?? project?.id ?? 'Project',
    [project],
  )
  const selectedSection = isSettingsSection(section) ? section : undefined
  const selectedSectionMeta = SETTINGS_SECTIONS.find(
    (entry) => entry.id === selectedSection,
  )

  if (section === 'workers') return <Navigate to="/settings?section=workers" replace />
  if (!orgId || !projectId) {
    return <p className="text-sm text-destructive">Missing route params.</p>
  }
  if (err) {
    return <p className="text-sm text-destructive">{err}</p>
  }
  if (project === undefined) {
    return <ProjectPageSkeleton />
  }
  if (!project) {
    return <p className="text-sm text-muted-foreground">Project not found.</p>
  }

  const title = selectedSectionMeta?.label ?? 'Project settings'
  const description =
    selectedSectionMeta?.description ??
    "Configure this project's dispatch behavior, worker overrides, and workflow stages."

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <p className="text-xs text-muted-foreground">
          <OrgProjectCrumbs
            orgId={orgSlug}
            projectId={projectSlug}
            projectLabel={projectLabel}
          />
          <span className="px-1">/</span>
          {selectedSectionMeta ? (
            <>
              <Link
                to={`${base}/settings`}
                className="hover:underline"
              >
                Settings
              </Link>
              <span className="px-1">/</span>
              <span>{selectedSectionMeta.label}</span>
            </>
          ) : (
            <span>Settings</span>
          )}
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">
            {title}
          </h1>
          {project.status ? <Badge variant="outline">{project.status}</Badge> : null}
        </div>
        <p className="max-w-3xl text-sm leading-relaxed text-muted-foreground">
          {description}
        </p>
      </div>

      {selectedSection === undefined ? (
        <SettingsOverview basePath={base} projectId={projectId} project={project} onProjectChange={setProject} />
      ) : (
        <div className="space-y-6">
          {selectedSection === 'prompts' ? (
            <ProjectBasePromptCard
              projectId={projectId}
              project={project}
              onProjectChange={setProject}
            />
          ) : null}

          {selectedSection === 'dispatch' ? (
            <DispatchControlCard
              projectId={projectId}
              project={project}
              saving={dispatchSaving}
              onToggle={toggleDispatch}
              onProjectChange={setProject}
            />
          ) : null}


          {selectedSection === 'workflow' ? (
            <WorkflowTimelineEditor projectId={projectId} orgId={orgId} />
          ) : null}

          {selectedSection === 'linear' ? <ProjectLinearCard projectId={projectId} /> : null}

          {selectedSection === 'reports' ? <ProjectReportsCard projectId={projectId} /> : null}
          {selectedSection === 'merge' ? <ProjectMergeCard project={project} orgId={orgId} orgSlug={orgSlug} /> : null}

          <div className="flex justify-end">
            <Button asChild variant="ghost" size="sm">
              <Link to={`${base}/settings`}>
                Back to settings
              </Link>
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
