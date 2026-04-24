import { useEffect, useMemo, useState } from 'react'
import { ArrowLeftRight, Plus, Save, Trash2 } from 'lucide-react'

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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type Project = components['schemas']['Project']
type DispatchConfig = components['schemas']['DispatchConfig']
type DispatchWorkerProfile = components['schemas']['DispatchWorkerProfile']
type DispatchRolePolicy = components['schemas']['DispatchRolePolicy']

type DispatchWorkerDraft = NonNullable<DispatchConfig['workers']>[number]
type DispatchConfigDraft = {
  workers: DispatchWorkerDraft[]
  policies: Record<string, DispatchRolePolicy>
}

const RUNNER_OPTIONS = [
  { value: 'inherit', label: 'Inherit' },
  { value: 'cli', label: 'CLI' },
  { value: 'docker', label: 'Docker' },
  { value: 'openai-responses', label: 'OpenAI Responses' },
] as const

const DRIVER_OPTIONS = [
  { value: 'inherit', label: 'Inherit' },
  { value: 'claude', label: 'Claude' },
  { value: 'codex', label: 'Codex' },
  { value: 'generic', label: 'Generic' },
] as const

const SELECTION_OPTIONS = [
  { value: 'ordered', label: 'Ordered failover' },
  { value: 'any', label: 'Rotate across workers' },
] as const

const ROLE_OPTIONS = [
  {
    key: 'orchestrator',
    label: 'Command Center',
    description: 'Conversation planning and ticket creation in the orchestrator.',
  },
  {
    key: 'planner',
    label: 'Planning',
    description: 'Ticket planning and investigation before implementation starts.',
  },
  {
    key: 'executor',
    label: 'Implementation',
    description: 'Main code-writing workers for claimed and executing tickets.',
  },
  {
    key: 'validator',
    label: 'Review',
    description: 'PR review, validation, and acceptance decisions.',
  },
  {
    key: 'deployer',
    label: 'Deployment',
    description: 'Deployment and post-merge operational work.',
  },
  {
    key: 'investigator',
    label: 'Investigation',
    description: 'Read-only investigation subagents.',
  },
  {
    key: 'conflict_resolver',
    label: 'Conflict Resolution',
    description: 'Merge/rebase resolution when auto-merge hits conflicts.',
  },
] as const

function emptyPolicy(policy?: DispatchRolePolicy): DispatchRolePolicy {
  return {
    selection_mode: policy?.selection_mode === 'any' ? 'any' : 'ordered',
    worker_ids: [...(policy?.worker_ids ?? [])],
  }
}

function normalizeDispatchConfig(input?: DispatchConfig | null): DispatchConfigDraft {
  const workers = (input?.workers ?? []).map((worker, index) => ({
    id: worker.id?.trim() || `worker-${index + 1}`,
    name: worker.name?.trim() || worker.id?.trim() || `worker-${index + 1}`,
    enabled: worker.enabled !== false,
    runner: worker.runner ?? undefined,
    driver: worker.driver ?? undefined,
    cli_path: worker.cli_path ?? '',
    model: worker.model ?? '',
    reasoning_effort: worker.reasoning_effort ?? '',
    api_base_url: worker.api_base_url ?? '',
    credential_env_var: worker.credential_env_var ?? '',
    args: [...(worker.args ?? [])],
  }))

  const policies: Record<string, DispatchRolePolicy> = {}
  for (const role of ROLE_OPTIONS) {
    policies[role.key] = emptyPolicy(input?.policies?.[role.key])
  }
  for (const [key, value] of Object.entries(input?.policies ?? {})) {
    policies[key] = emptyPolicy(value)
  }

  return { workers, policies }
}

function serializeDispatchConfig(draft: DispatchConfigDraft): DispatchConfig {
  return {
    workers: draft.workers.map((worker) => ({
      id: worker.id?.trim() || undefined,
      name: worker.name?.trim() || worker.id?.trim() || undefined,
      enabled: worker.enabled !== false,
      runner: worker.runner || undefined,
      driver: worker.driver || undefined,
      cli_path: worker.cli_path?.trim() || undefined,
      model: worker.model?.trim() || undefined,
      reasoning_effort: worker.reasoning_effort?.trim() || undefined,
      api_base_url: worker.api_base_url?.trim() || undefined,
      credential_env_var: worker.credential_env_var?.trim() || undefined,
      args: (worker.args ?? []).map((value) => value.trim()).filter(Boolean),
    })),
    policies: Object.fromEntries(
      Object.entries(draft.policies).map(([key, policy]) => [
        key,
        {
          selection_mode: policy.selection_mode === 'any' ? 'any' : 'ordered',
          worker_ids: [...(policy.worker_ids ?? [])],
        },
      ]),
    ),
  }
}

function nextWorkerID(workers: DispatchWorkerDraft[]) {
  const max = workers.reduce((highest, worker) => {
    const match = /^worker-(\d+)$/.exec(worker.id ?? '')
    if (!match) return highest
    return Math.max(highest, Number(match[1]))
  }, 0)
  return `worker-${max + 1}`
}

function parseRunnerValue(
  value: string,
): DispatchWorkerProfile['runner'] | undefined {
  switch (value) {
    case 'cli':
    case 'docker':
    case 'openai-responses':
      return value
    default:
      return undefined
  }
}

function parseDriverValue(
  value: string,
): DispatchWorkerProfile['driver'] | undefined {
  switch (value) {
    case 'claude':
    case 'codex':
    case 'generic':
      return value
    default:
      return undefined
  }
}

type WorkerOption = {
  id: string
  name: string
  enabled: boolean
  description: string
}

export function ProjectDispatchRoutingCard({
  projectId,
  project,
  onProjectChange,
}: {
  projectId: string
  project: Project
  onProjectChange: (project: Project) => void
}) {
  const { client } = useAuth()
  const [draft, setDraft] = useState<DispatchConfigDraft>(() =>
    normalizeDispatchConfig(project.dispatch_config),
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)

  useEffect(() => {
    setDraft(normalizeDispatchConfig(project.dispatch_config))
  }, [project.dispatch_config])

  const workerOptions = useMemo<WorkerOption[]>(
    () => [
      {
        id: 'default',
        name: 'Default server worker',
        enabled: true,
        description: 'Uses the DISPATCH_* or ORCHESTRATOR_* server defaults.',
      },
      ...draft.workers.map((worker) => ({
        id: worker.id ?? '',
        name: worker.name ?? worker.id ?? 'Unnamed worker',
        enabled: worker.enabled !== false,
        description:
          [worker.runner, worker.driver, worker.model]
            .filter(Boolean)
            .join(' / ') || 'Custom worker profile',
      })),
    ],
    [draft.workers],
  )

  function updateWorker(index: number, patch: Partial<DispatchWorkerProfile>) {
    setDraft((current) => {
      const workers = [...current.workers]
      workers[index] = { ...workers[index], ...patch }
      return { ...current, workers }
    })
  }

  function removeWorker(workerID: string) {
    setDraft((current) => ({
      workers: current.workers.filter((worker) => worker.id !== workerID),
      policies: Object.fromEntries(
        Object.entries(current.policies).map(([role, policy]) => [
          role,
          {
            ...emptyPolicy(policy),
            worker_ids: (policy.worker_ids ?? []).filter((id) => id !== workerID),
          },
        ]),
      ),
    }))
  }

  function addWorker() {
    setDraft((current) => ({
      ...current,
      workers: [
        ...current.workers,
        {
          id: nextWorkerID(current.workers),
          name: `Worker ${current.workers.length + 1}`,
          enabled: true,
          runner: undefined,
          driver: undefined,
          cli_path: '',
          model: '',
          reasoning_effort: '',
          api_base_url: '',
          credential_env_var: '',
          args: [],
        },
      ],
    }))
  }

  function setPolicy(role: string, patch: Partial<DispatchRolePolicy>) {
    setDraft((current) => ({
      ...current,
      policies: {
        ...current.policies,
        [role]: {
          ...emptyPolicy(current.policies[role]),
          ...patch,
        },
      },
    }))
  }

  function addWorkerToPolicy(role: string, workerID: string) {
    const policy = emptyPolicy(draft.policies[role])
    if (policy.worker_ids?.includes(workerID)) return
    setPolicy(role, { worker_ids: [...(policy.worker_ids ?? []), workerID] })
  }

  function moveWorker(role: string, index: number, direction: -1 | 1) {
    const policy = emptyPolicy(draft.policies[role])
    const workerIDs = [...(policy.worker_ids ?? [])]
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= workerIDs.length) return
    const [workerID] = workerIDs.splice(index, 1)
    workerIDs.splice(nextIndex, 0, workerID)
    setPolicy(role, { worker_ids: workerIDs })
  }

  async function saveConfig() {
    setSaving(true)
    setError(null)
    setSavedAt(null)
    const { data, error: apiError, response } = await client.PATCH(
      '/projects/{projectID}',
      {
        params: { path: { projectID: projectId } },
        body: { dispatch_config: serializeDispatchConfig(draft) },
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
      <CardHeader className="gap-3">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">Worker routing</CardTitle>
            <CardDescription>
              Configure reusable Claude, Codex, or API-native worker profiles
              and decide which roles can use them.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            {savedAt ? (
              <span className="text-xs text-emerald-400">Saved</span>
            ) : null}
            <Button variant="outline" size="xs" onClick={addWorker}>
              <Plus className="size-3.5" />
              Add worker
            </Button>
            <Button size="xs" onClick={() => void saveConfig()} disabled={saving}>
              <Save className="size-3.5" />
              {saving ? 'Saving…' : 'Save routing'}
            </Button>
          </div>
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">
          `credential_env_var` is the name of an environment variable on the
          Flywheel server. Secrets stay in the server environment, not in the
          project record.
        </p>
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </CardHeader>
      <CardContent className="space-y-6">
        <section className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium text-foreground">Worker profiles</h3>
              <p className="text-xs text-muted-foreground">
                Profiles can target Claude, Codex, Docker, or API-native runners.
              </p>
            </div>
            <Badge variant="outline">
              {draft.workers.length} custom worker{draft.workers.length === 1 ? '' : 's'}
            </Badge>
          </div>

          {draft.workers.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] px-4 py-5 text-sm text-muted-foreground">
              No custom workers yet. Policies can still use the default server
              worker, or you can add explicit Claude/Codex profiles here.
            </div>
          ) : (
            <div className="space-y-4">
              {draft.workers.map((worker, index) => (
                <div
                  key={worker.id ?? `worker-${index}`}
                  className="rounded-2xl border border-white/10 bg-black/10 p-4"
                >
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                    <div className="grid flex-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
                      <Label>
                        Name
                        <Input
                          value={worker.name ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { name: event.target.value })
                          }
                          placeholder="Claude implementation"
                        />
                      </Label>
                      <Label>
                        Worker ID
                        <Input
                          value={worker.id ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { id: event.target.value })
                          }
                          placeholder="claude-impl"
                        />
                      </Label>
                      <Label>
                        Runner
                        <Select
                          value={worker.runner ?? 'inherit'}
                          onValueChange={(value) =>
                            updateWorker(index, {
                              runner: parseRunnerValue(value),
                            })
                          }
                        >
                          <SelectTrigger className="w-full bg-white/5">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {RUNNER_OPTIONS.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Label>
                      <Label>
                        Driver
                        <Select
                          value={worker.driver ?? 'inherit'}
                          onValueChange={(value) =>
                            updateWorker(index, {
                              driver: parseDriverValue(value),
                            })
                          }
                        >
                          <SelectTrigger className="w-full bg-white/5">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {DRIVER_OPTIONS.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Label>
                      <Label>
                        CLI path
                        <Input
                          value={worker.cli_path ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { cli_path: event.target.value })
                          }
                          placeholder="claude or codex"
                        />
                      </Label>
                      <Label>
                        Model
                        <Input
                          value={worker.model ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { model: event.target.value })
                          }
                          placeholder="gpt-5.2-codex"
                        />
                      </Label>
                      <Label>
                        Reasoning effort
                        <Input
                          value={worker.reasoning_effort ?? ''}
                          onChange={(event) =>
                            updateWorker(index, {
                              reasoning_effort: event.target.value,
                            })
                          }
                          placeholder="medium or xhigh"
                        />
                      </Label>
                      <Label>
                        API base URL
                        <Input
                          value={worker.api_base_url ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { api_base_url: event.target.value })
                          }
                          placeholder="https://api.openai.com/v1"
                        />
                      </Label>
                      <Label className="md:col-span-2 xl:col-span-2">
                        Credential env var
                        <Input
                          value={worker.credential_env_var ?? ''}
                          onChange={(event) =>
                            updateWorker(index, {
                              credential_env_var: event.target.value,
                            })
                          }
                          placeholder="OPENAI_API_KEY or CLAUDE_FALLBACK_API_KEY"
                        />
                      </Label>
                    </div>
                    <div className="flex items-center gap-3">
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={worker.enabled !== false}
                          onCheckedChange={(enabled) =>
                            updateWorker(index, { enabled })
                          }
                        />
                        <span className="text-xs text-muted-foreground">
                          {worker.enabled !== false ? 'Enabled' : 'Disabled'}
                        </span>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => removeWorker(worker.id ?? '')}
                        title="Remove worker"
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    </div>
                  </div>
                  <div className="mt-4">
                    <Label>
                      Extra args
                      <Textarea
                        value={(worker.args ?? []).join('\n')}
                        onChange={(event) =>
                          updateWorker(index, {
                            args: event.target.value
                              .split('\n')
                              .map((line) => line.trim())
                              .filter(Boolean),
                          })
                        }
                        rows={3}
                        placeholder="One CLI argument per line"
                      />
                    </Label>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-medium text-foreground">Role policies</h3>
            <p className="text-xs text-muted-foreground">
              Leave a role empty to use every enabled worker for that role.
              Ordered mode fails over in sequence. Rotate mode spreads initial
              attempts across the selected workers and still fails over.
            </p>
          </div>

          <div className="space-y-3">
            {ROLE_OPTIONS.map((role) => {
              const policy = emptyPolicy(draft.policies[role.key])
              const selectedWorkerIDs = policy.worker_ids ?? []
              const selectedWorkers = selectedWorkerIDs.map(
                (workerID) =>
                  workerOptions.find((worker) => worker.id === workerID) ?? {
                    id: workerID,
                    name: workerID,
                    enabled: false,
                    description: 'Missing worker profile',
                  },
              )
              const available = workerOptions.filter(
                (worker) => !selectedWorkerIDs.includes(worker.id),
              )

              return (
                <div
                  key={role.key}
                  className="rounded-2xl border border-white/10 bg-black/10 p-4"
                >
                  <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-medium text-foreground">
                          {role.label}
                        </p>
                        <Badge variant="outline">{role.key}</Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {role.description}
                      </p>
                    </div>
                    <Label className="xl:w-52">
                      Selection mode
                      <Select
                        value={policy.selection_mode === 'any' ? 'any' : 'ordered'}
                        onValueChange={(value) =>
                          setPolicy(role.key, {
                            selection_mode: value === 'any' ? 'any' : 'ordered',
                          })
                        }
                      >
                        <SelectTrigger className="w-full bg-white/5">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {SELECTION_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Label>
                  </div>

                  <div className="mt-4 space-y-3">
                    <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
                      <ArrowLeftRight className="size-3.5" />
                      Selected workers
                    </div>
                    {selectedWorkers.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        Uses all enabled workers for this role.
                      </p>
                    ) : (
                      <div className="flex flex-wrap gap-2">
                        {selectedWorkers.map((worker, index) => (
                          <div
                            key={`${role.key}-${worker.id}`}
                            className="flex items-center gap-1 rounded-lg border border-white/10 bg-white/[0.04] px-2 py-1"
                          >
                            <div className="flex flex-col">
                              <span className="text-xs font-medium text-foreground">
                                {worker.name}
                              </span>
                              <span className="text-[11px] text-muted-foreground">
                                {worker.description}
                                {worker.enabled ? '' : ' (disabled)'}
                              </span>
                            </div>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => moveWorker(role.key, index, -1)}
                              disabled={index === 0}
                              title="Move earlier"
                            >
                              ←
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => moveWorker(role.key, index, 1)}
                              disabled={index === selectedWorkers.length - 1}
                              title="Move later"
                            >
                              →
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() =>
                                setPolicy(role.key, {
                                  worker_ids: selectedWorkerIDs.filter(
                                    (id) => id !== worker.id,
                                  ),
                                })
                              }
                              title="Remove"
                            >
                              ×
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}

                    <div className="flex flex-wrap gap-2">
                      {available.map((worker) => (
                        <Button
                          key={`${role.key}-add-${worker.id}`}
                          variant="outline"
                          size="xs"
                          onClick={() => addWorkerToPolicy(role.key, worker.id)}
                        >
                          <Plus className="size-3.5" />
                          {worker.name}
                        </Button>
                      ))}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </section>
      </CardContent>
    </Card>
  )
}
