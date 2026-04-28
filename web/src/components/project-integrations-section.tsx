import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  CheckCircle2,
  Eye,
  EyeOff,
  GitBranch,
  Plus,
  Save,
  Server,
  Trash2,
  XCircle,
} from 'lucide-react'

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type Environment = components['schemas']['Environment']

// --- API response types (masked credentials) ---

type CredentialStatus = {
  flyio_configured?: boolean
  flyio_masked?: string
}

type ConfigResponse = {
  scm?: {
    provider?: string
  }
  infrastructure?: {
    provider?: string
    flyio?: {
      organization_slug?: string
      apps?: FlyIOApp[]
    }
  }
  credentials?: CredentialStatus
}

// --- Config payload for PUT (includes raw credentials) ---

type ConfigPayload = {
  scm?: {
    provider?: string
  }
  infrastructure?: {
    provider?: string
    flyio?: {
      organization_slug?: string
      apps?: FlyIOApp[]
    }
  }
  credentials?: {
    flyio_api_token?: string
  }
}

type FlyIOApp = {
  app_name: string
  environment_slug: string
  service?: string
}

type IntegrationsDraft = {
  scmProvider: string
  infrastructureProvider: string
  flyioOrganizationSlug: string
  flyioApps: FlyIOApp[]
  flyioApiToken: string
}

const SCM_OPTIONS = [
  { value: 'none', label: 'Disabled' },
  { value: 'github', label: 'GitHub' },
] as const

const INFRA_OPTIONS = [
  { value: 'none', label: 'Disabled' },
  { value: 'flyio', label: 'Fly.io' },
] as const

function emptyFlyApp(): FlyIOApp {
  return { app_name: '', environment_slug: '', service: '' }
}

function normalizeConfig(input?: ConfigResponse | null): IntegrationsDraft {
  return {
    scmProvider: input?.scm?.provider?.trim() || 'none',
    infrastructureProvider: input?.infrastructure?.provider?.trim() || 'none',
    flyioOrganizationSlug:
      input?.infrastructure?.flyio?.organization_slug?.trim() || '',
    flyioApps:
      input?.infrastructure?.flyio?.apps?.map((app) => ({
        app_name: app.app_name ?? '',
        environment_slug: app.environment_slug ?? '',
        service: app.service ?? '',
      })) ?? [],
    // Token is never returned from API — always start empty.
    // User types a new token or leaves blank to keep existing.
    flyioApiToken: '',
  }
}

function serializePayload(
  draft: IntegrationsDraft,
  credentialStatus: CredentialStatus,
): ConfigPayload {
  const scmProvider = draft.scmProvider.trim()
  const infrastructureProvider = draft.infrastructureProvider.trim()

  const cfg: ConfigPayload = {}
  if (scmProvider && scmProvider !== 'none') {
    cfg.scm = { provider: scmProvider }
  }
  if (infrastructureProvider && infrastructureProvider !== 'none') {
    cfg.infrastructure = {
      provider: infrastructureProvider,
    }
    if (infrastructureProvider === 'flyio') {
      cfg.infrastructure.flyio = {
        organization_slug: draft.flyioOrganizationSlug.trim() || undefined,
        apps: draft.flyioApps
          .map((app) => ({
            app_name: app.app_name.trim(),
            environment_slug: app.environment_slug.trim(),
            service: app.service?.trim() || undefined,
          }))
          .filter((app) => app.app_name || app.environment_slug || app.service),
      }
    }
  }

  // Only send credentials if user typed a new token.
  // If blank and a token is already configured, we send the masked value
  // to signal "keep existing" (backend detects '***' and preserves).
  const tokenInput = draft.flyioApiToken.trim()
  if (tokenInput) {
    cfg.credentials = { flyio_api_token: tokenInput }
  } else if (credentialStatus.flyio_configured && credentialStatus.flyio_masked) {
    cfg.credentials = { flyio_api_token: credentialStatus.flyio_masked }
  }
  return cfg
}

function fetchConfig(
  path: string,
  token: string | null,
): Promise<{ ok: true; data: ConfigResponse } | { ok: false; error: string }> {
  return fetch(path, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
    .then(async (response) => {
      const raw = (await response.json().catch(() => null)) as
        | Record<string, unknown>
        | null
      if (!response.ok) {
        return {
          ok: false as const,
          error:
            raw && typeof raw.error === 'string' ? raw.error : 'Request failed',
        }
      }
      return { ok: true as const, data: (raw ?? {}) as ConfigResponse }
    })
    .catch((error: unknown) => ({
      ok: false as const,
      error: error instanceof Error ? error.message : 'Request failed',
    }))
}

// --- Connection status badge ---

function ConnectionStatus({
  configured,
  masked,
}: {
  configured: boolean
  masked?: string
}) {
  if (configured) {
    return (
      <div className="flex items-center gap-1.5">
        <CheckCircle2 className="size-3.5 text-emerald-400" />
        <span className="text-xs text-emerald-400">Connected</span>
        {masked ? (
          <span className="font-mono text-xs text-muted-foreground">
            ({masked})
          </span>
        ) : null}
      </div>
    )
  }
  return (
    <div className="flex items-center gap-1.5">
      <XCircle className="size-3.5 text-muted-foreground" />
      <span className="text-xs text-muted-foreground">Not configured</span>
    </div>
  )
}

// --- Provider card wrapper for extensibility ---

function ProviderCard({
  icon: Icon,
  title,
  description,
  status,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  description: string
  status?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Icon className="size-4 text-muted-foreground" />
            <CardTitle className="text-sm">{title}</CardTitle>
          </div>
          {status}
        </div>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">{children}</CardContent>
    </Card>
  )
}

// --- Main component ---

export function ProjectIntegrationsSection({
  projectId,
  project,
  onSaved,
}: {
  projectId: string
  project: Project
  onSaved?: () => void
}) {
  const { client, token } = useAuth()
  const [loading, setLoading] = useState(true)
  const [draft, setDraft] = useState<IntegrationsDraft>(() => normalizeConfig())
  const [baseline, setBaseline] = useState<IntegrationsDraft>(() =>
    normalizeConfig(),
  )
  const [credentialStatus, setCredentialStatus] = useState<CredentialStatus>({})
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [showToken, setShowToken] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    Promise.all([
      fetchConfig(`/projects/${projectId}/integrations`, token),
      client.GET('/projects/{projectID}/environments', {
        params: { path: { projectID: projectId } },
      }),
    ])
      .then(([configResult, envResult]) => {
        if (cancelled) return
        if (!configResult.ok) {
          setError(configResult.error)
          setLoading(false)
          return
        }
        const normalized = normalizeConfig(configResult.data)
        setDraft(normalized)
        setBaseline(normalized)
        setCredentialStatus(configResult.data.credentials ?? {})

        if (!envResult.response.ok) {
          setError(formatApiError(envResult.error))
          setEnvironments([])
        } else {
          setEnvironments(envResult.data ?? [])
        }
        setLoading(false)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Request failed')
        setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [client, projectId, token])

  // Dirty check: compare config fields + whether user typed a new token
  const configChanged = useMemo(() => {
    const draftPayload = JSON.stringify(serializePayload(draft, {}))
    const basePayload = JSON.stringify(serializePayload(baseline, {}))
    return draftPayload !== basePayload
  }, [draft, baseline])

  const tokenChanged = draft.flyioApiToken.trim() !== ''
  const isDirty = configChanged || tokenChanged

  const environmentSlugs = useMemo(
    () => environments.map((env) => env.slug).filter(Boolean) as string[],
    [environments],
  )

  const updateFlyApp = useCallback(
    (index: number, patch: Partial<FlyIOApp>) => {
      setDraft((current) => {
        const flyioApps = [...current.flyioApps]
        flyioApps[index] = { ...flyioApps[index], ...patch }
        return { ...current, flyioApps }
      })
    },
    [],
  )

  const validationError = useMemo(() => {
    if (draft.infrastructureProvider !== 'flyio') return null
    for (const app of draft.flyioApps) {
      const hasAnyValue =
        app.app_name.trim() !== '' ||
        app.environment_slug.trim() !== '' ||
        (app.service?.trim() ?? '') !== ''
      if (!hasAnyValue) continue
      if (app.app_name.trim() === '' || app.environment_slug.trim() === '') {
        return 'Each Fly.io app mapping needs both an environment slug and app name.'
      }
    }
    return null
  }, [draft.flyioApps, draft.infrastructureProvider])

  const handleSave = useCallback(async () => {
    if (saving || validationError) return
    setSaving(true)
    setError(null)
    const payload = serializePayload(draft, credentialStatus)
    try {
      const response = await fetch(`/projects/${projectId}/integrations`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify(payload),
      })
      const raw = (await response.json().catch(() => null)) as
        | Record<string, unknown>
        | null
      if (!response.ok) {
        setError(
          raw && typeof raw.error === 'string' ? raw.error : 'Request failed',
        )
        setSaving(false)
        return
      }
      const configResp = (raw ?? {}) as ConfigResponse
      const normalized = normalizeConfig(configResp)
      setDraft(normalized)
      setBaseline(normalized)
      setCredentialStatus(configResp.credentials ?? {})
      setSavedAt(Date.now())
      setSaving(false)
      setShowToken(false)
      onSaved?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed')
      setSaving(false)
    }
  }, [credentialStatus, draft, onSaved, projectId, saving, token, validationError])

  if (loading) {
    return (
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-muted-foreground">
            Integrations
          </h2>
        </div>
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <Skeleton className="h-72 w-full rounded-xl" />
          <Skeleton className="h-72 w-full rounded-xl" />
        </div>
      </section>
    )
  }

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <h2 className="text-sm font-medium text-muted-foreground">
            Integrations
          </h2>
          <p className="text-xs text-muted-foreground">
            Project-scoped provider wiring for source control and infrastructure.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {savedAt && !isDirty ? (
            <span className="text-xs text-muted-foreground">Saved just now</span>
          ) : null}
          <Button
            type="button"
            size="sm"
            onClick={handleSave}
            disabled={saving || !isDirty || !!validationError}
          >
            <Save className="size-3.5" />
            {saving ? 'Saving\u2026' : 'Save integrations'}
          </Button>
        </div>
      </div>

      {error || validationError ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-3 text-sm text-destructive">
          {validationError ?? error}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {/* Source Control provider card */}
        <ProviderCard
          icon={GitBranch}
          title="Source Control"
          description="Pull request and repository data providers."
        >
          <Label>
            Provider
            <Select
              value={draft.scmProvider || 'none'}
              onValueChange={(value) =>
                setDraft((current) => ({ ...current, scmProvider: value }))
              }
            >
              <SelectTrigger className="w-full bg-white/5">
                <SelectValue placeholder="Choose provider" />
              </SelectTrigger>
              <SelectContent>
                {SCM_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>

          {draft.scmProvider === 'github' ? (
            <div className="space-y-3 rounded-lg border border-white/10 bg-white/5 px-3 py-3">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">github</Badge>
                <Badge variant="outline">env: GITHUB_TOKEN</Badge>
              </div>
              {project.repo_url ? (
                <div className="space-y-1">
                  <div className="text-xs text-muted-foreground">
                    Repository
                  </div>
                  <div className="break-all font-mono text-xs text-foreground">
                    {project.repo_url}
                  </div>
                </div>
              ) : (
                <div className="text-sm text-amber-300">
                  Set `repo_url` on the project so GitHub can resolve open pull
                  requests.
                </div>
              )}
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-white/10 px-3 py-4 text-sm text-muted-foreground">
              Source control integration is disabled.
            </div>
          )}
        </ProviderCard>

        {/* Infrastructure provider card */}
        <ProviderCard
          icon={Server}
          title="Infrastructure"
          description="Environment and deployment state providers."
          status={
            draft.infrastructureProvider === 'flyio' ? (
              <ConnectionStatus
                configured={credentialStatus.flyio_configured ?? false}
                masked={credentialStatus.flyio_masked}
              />
            ) : undefined
          }
        >
          <Label>
            Provider
            <Select
              value={draft.infrastructureProvider || 'none'}
              onValueChange={(value) =>
                setDraft((current) => ({
                  ...current,
                  infrastructureProvider: value,
                  flyioApps:
                    value === 'flyio'
                      ? current.flyioApps.length > 0
                        ? current.flyioApps
                        : [emptyFlyApp()]
                      : current.flyioApps,
                }))
              }
            >
              <SelectTrigger className="w-full bg-white/5">
                <SelectValue placeholder="Choose provider" />
              </SelectTrigger>
              <SelectContent>
                {INFRA_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>

          {draft.infrastructureProvider === 'flyio' ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">flyio</Badge>
              </div>

              {/* Fly.io API Token credential input */}
              <div className="space-y-2 rounded-lg border border-white/10 bg-white/5 px-3 py-3">
                <div className="flex items-center justify-between">
                  <Label className="text-xs font-medium">API Token</Label>
                  <ConnectionStatus
                    configured={
                      draft.flyioApiToken.trim() !== '' ||
                      (credentialStatus.flyio_configured ?? false)
                    }
                    masked={
                      draft.flyioApiToken.trim()
                        ? undefined
                        : credentialStatus.flyio_masked
                    }
                  />
                </div>
                <div className="relative">
                  <Input
                    type={showToken ? 'text' : 'password'}
                    value={draft.flyioApiToken}
                    onChange={(event) =>
                      setDraft((current) => ({
                        ...current,
                        flyioApiToken: event.target.value,
                      }))
                    }
                    placeholder={
                      credentialStatus.flyio_configured
                        ? 'Leave blank to keep existing token'
                        : 'fo1_...'
                    }
                    className="pr-10 font-mono text-xs"
                    autoComplete="off"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    onClick={() => setShowToken(!showToken)}
                    title={showToken ? 'Hide token' : 'Show token'}
                  >
                    {showToken ? (
                      <EyeOff className="size-3.5" />
                    ) : (
                      <Eye className="size-3.5" />
                    )}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  Per-project token overrides the server-wide FLY_API_TOKEN env
                  var. Never stored in plaintext in API responses.
                </p>
              </div>

              <Label>
                Organization Slug
                <Input
                  value={draft.flyioOrganizationSlug}
                  onChange={(event) =>
                    setDraft((current) => ({
                      ...current,
                      flyioOrganizationSlug: event.target.value,
                    }))
                  }
                  placeholder="optional"
                />
              </Label>

              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <div className="space-y-1">
                    <div className="text-sm font-medium">App Mappings</div>
                    <div className="text-xs text-muted-foreground">
                      Map Fly apps to Flywheel environment slugs and service
                      names.
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    onClick={() =>
                      setDraft((current) => ({
                        ...current,
                        flyioApps: [...current.flyioApps, emptyFlyApp()],
                      }))
                    }
                  >
                    <Plus className="size-3.5" />
                    Add app
                  </Button>
                </div>

                {environmentSlugs.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {environmentSlugs.map((slug) => (
                      <Badge key={slug} variant="outline">
                        {slug}
                      </Badge>
                    ))}
                  </div>
                ) : null}

                <div className="space-y-3">
                  {draft.flyioApps.map((app, index) => (
                    <div
                      key={`${index}:${app.environment_slug}:${app.app_name}`}
                      className="grid gap-3 rounded-lg border border-white/10 bg-white/5 px-3 py-3"
                    >
                      <div className="grid gap-3 md:grid-cols-3">
                        <Label>
                          Environment Slug
                          <Input
                            value={app.environment_slug}
                            onChange={(event) =>
                              updateFlyApp(index, {
                                environment_slug: event.target.value,
                              })
                            }
                            placeholder="prod"
                          />
                        </Label>
                        <Label>
                          Fly App
                          <Input
                            value={app.app_name}
                            onChange={(event) =>
                              updateFlyApp(index, {
                                app_name: event.target.value,
                              })
                            }
                            placeholder="myapp-prod"
                          />
                        </Label>
                        <Label>
                          Service
                          <Input
                            value={app.service ?? ''}
                            onChange={(event) =>
                              updateFlyApp(index, {
                                service: event.target.value,
                              })
                            }
                            placeholder="api"
                          />
                        </Label>
                      </div>
                      <div className="flex justify-end">
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          onClick={() =>
                            setDraft((current) => ({
                              ...current,
                              flyioApps: current.flyioApps.filter(
                                (_, currentIndex) => currentIndex !== index,
                              ),
                            }))
                          }
                        >
                          <Trash2 className="size-3.5" />
                          Remove
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-white/10 px-3 py-4 text-sm text-muted-foreground">
              Infrastructure integration is disabled.
            </div>
          )}
        </ProviderCard>
      </div>
    </section>
  )
}
