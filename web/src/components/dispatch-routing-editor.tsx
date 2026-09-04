import { useMemo, useState } from 'react'
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
import type { components } from '@/lib/api/v1'
import { ROLE_OPTIONS } from '@/lib/dispatch-roles'

type DispatchConfig = components['schemas']['DispatchConfig']
type DispatchWorkerRole = components['schemas']['DispatchWorkerRole']
type DispatchWorkerProfile = components['schemas']['DispatchWorkerProfile']
type DispatchRolePolicy = components['schemas']['DispatchRolePolicy']

type DispatchWorkerRoleDraft = {
  id: string
  name: string
  description: string
  base_type: NonNullable<DispatchWorkerRole['base_type']>
}
type DispatchWorkerDraft = NonNullable<DispatchConfig['workers']>[number]
type DispatchConfigDraft = {
  max_active_workers: number
  roles: DispatchWorkerRoleDraft[]
  workers: DispatchWorkerDraft[]
  policies: Record<string, DispatchRolePolicy>
}

const RUNNER_OPTIONS = [
  { value: 'inherit', label: 'Inherit' },
  { value: 'cli', label: 'CLI (local harness)' },
] as const

const DRIVER_OPTIONS = [
  { value: 'inherit', label: 'Inherit (dispatch default)' },
  { value: 'claude', label: 'Claude Code' },
  { value: 'codex', label: 'Codex' },
  { value: 'generic', label: 'Generic CLI' },
] as const

const SELECTION_OPTIONS = [
  { value: 'ordered', label: 'Ordered failover' },
  { value: 'any', label: 'Rotate across workers' },
] as const

const BASE_TYPE_OPTIONS = [
  { value: 'executor', label: 'Implementation' },
  { value: 'validator', label: 'Review' },
  { value: 'planner', label: 'Planning' },
  { value: 'deployer', label: 'Deployment' },
  { value: 'investigator', label: 'Investigation' },
] as const


type RoleOption = {
  key: string
  label: string
  description: string
  builtIn: boolean
  customIndex?: number
  baseType?: DispatchWorkerRoleDraft['base_type']
}

function emptyPolicy(policy?: DispatchRolePolicy): DispatchRolePolicy {
  return {
    selection_mode: policy?.selection_mode === 'any' ? 'any' : 'ordered',
    worker_ids: [...(policy?.worker_ids ?? [])],
  }
}

function normalizeRoleKey(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
}

function humanizeRoleKey(value: string): string {
  return normalizeRoleKey(value)
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function isBuiltInRoleKey(value: string): boolean {
  const key = normalizeRoleKey(value)
  return ROLE_OPTIONS.some((role) => role.key === key)
}

function parseRoleBaseType(
  value?: string,
): DispatchWorkerRoleDraft['base_type'] {
  switch (normalizeRoleKey(value ?? '')) {
    case 'planner':
    case 'planning':
      return 'planner'
    case 'validator':
    case 'review':
    case 'validation':
      return 'validator'
    case 'deployer':
    case 'deploy':
    case 'deployment':
      return 'deployer'
    case 'investigator':
    case 'investigation':
      return 'investigator'
    default:
      return 'executor'
  }
}

function uniqueRoleKey(base: string, roles: DispatchWorkerRoleDraft[], skipIndex = -1) {
  const root = normalizeRoleKey(base) || 'custom_role'
  const used = new Set(
    roles
      .filter((_, index) => index !== skipIndex)
      .map((role) => role.id)
      .filter(Boolean),
  )
  for (const role of ROLE_OPTIONS) {
    used.add(role.key)
  }
  if (!used.has(root)) return root
  for (let index = 2; ; index += 1) {
    const candidate = `${root}_${index}`
    if (!used.has(candidate)) return candidate
  }
}

function normalizeDispatchConfig(input?: DispatchConfig | null): DispatchConfigDraft {
  const maxActiveWorkers =
    typeof input?.max_active_workers === 'number' && input.max_active_workers > 0
      ? input.max_active_workers
      : 0
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

  const roles: DispatchWorkerRoleDraft[] = []
  for (const role of input?.roles ?? []) {
    const requestedID = normalizeRoleKey(
      role.id || role.name || `custom_role_${roles.length + 1}`,
    )
    if (isBuiltInRoleKey(requestedID)) continue
    const id = uniqueRoleKey(requestedID, roles)
    roles.push({
      id,
      name: role.name?.trim() || humanizeRoleKey(id) || id,
      description: role.description?.trim() ?? '',
      base_type: parseRoleBaseType(role.base_type),
    })
  }

  const policies: Record<string, DispatchRolePolicy> = {}
  for (const role of ROLE_OPTIONS) {
    policies[role.key] = emptyPolicy(input?.policies?.[role.key])
  }
  for (const [key, value] of Object.entries(input?.policies ?? {})) {
    const roleKey = normalizeRoleKey(key)
    if (!roleKey) continue
    policies[roleKey] = emptyPolicy(value)
    if (
      !isBuiltInRoleKey(roleKey) &&
      !roles.some((role) => role.id === roleKey)
    ) {
      roles.push({
        id: roleKey,
        name: humanizeRoleKey(roleKey) || roleKey,
        description: '',
        base_type: 'executor',
      })
    }
  }

  return { max_active_workers: maxActiveWorkers, roles, workers, policies }
}

function serializeDispatchConfig(draft: DispatchConfigDraft): DispatchConfig {
  return {
    max_active_workers:
      Number.isFinite(draft.max_active_workers) && draft.max_active_workers > 0
        ? Math.floor(draft.max_active_workers)
        : undefined,
    roles: draft.roles.map((role) => ({
      id: normalizeRoleKey(role.id),
      name: role.name.trim() || humanizeRoleKey(role.id) || role.id,
      description: role.description.trim() || undefined,
      base_type: parseRoleBaseType(role.base_type),
    })),
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
    case 'openai-compatible':
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

/**
 * Worker profiles, roles and routing policies. Bound to the shared library in
 * operator settings (Settings → Workers & roles); projects overlay it.
 */
export function DispatchRoutingEditor({
  initial,
  onSave,
  title = 'Workers & roles',
  description = 'Worker profiles (harness, model, effort, base prompt), the roles the dispatcher and reviewers fill, and which workers each role routes to.',
}: {
  initial: DispatchConfig | undefined
  onSave: (config: DispatchConfig) => Promise<string | null>
  title?: string
  description?: string
}) {
  const [draft, setDraft] = useState<DispatchConfigDraft>(() => normalizeDispatchConfig(initial))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const initialKey = JSON.stringify(initial ?? null)
  const [seenKey, setSeenKey] = useState(initialKey)
  if (seenKey !== initialKey) {
    // Parent handed us a new config (e.g. after a save elsewhere): adopt it.
    setSeenKey(initialKey)
    setDraft(normalizeDispatchConfig(initial))
  }

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

  const roleOptions = useMemo<RoleOption[]>(
    () => [
      ...ROLE_OPTIONS.map((role) => ({
        key: role.key,
        label: role.label,
        description: role.description,
        builtIn: true,
      })),
      ...draft.roles.map((role, index) => ({
        key: role.id,
        label: role.name || humanizeRoleKey(role.id) || role.id,
        description:
          role.description ||
          `Custom role using ${humanizeRoleKey(role.base_type).toLowerCase()} behavior.`,
        builtIn: false,
        customIndex: index,
        baseType: role.base_type,
      })),
    ],
    [draft.roles],
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
      ...current,
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

  function addRole() {
    setDraft((current) => {
      const id = uniqueRoleKey(`custom_role_${current.roles.length + 1}`, current.roles)
      return {
        ...current,
        roles: [
          ...current.roles,
          {
            id,
            name: humanizeRoleKey(id),
            description: '',
            base_type: 'executor',
          },
        ],
        policies: {
          ...current.policies,
          [id]: emptyPolicy(),
        },
      }
    })
  }

  function updateRole(index: number, patch: Partial<DispatchWorkerRoleDraft>) {
    setDraft((current) => {
      const roles = [...current.roles]
      const previous = roles[index]
      if (!previous) return current

      const nextID =
        patch.id !== undefined
          ? uniqueRoleKey(patch.id, current.roles, index)
          : previous.id
      roles[index] = {
        ...previous,
        ...patch,
        id: nextID,
        base_type: parseRoleBaseType(patch.base_type ?? previous.base_type),
      }

      if (nextID === previous.id) {
        return { ...current, roles }
      }

      const policies = { ...current.policies }
      policies[nextID] = emptyPolicy(policies[previous.id])
      delete policies[previous.id]
      return { ...current, roles, policies }
    })
  }

  function removeRole(index: number) {
    setDraft((current) => {
      const role = current.roles[index]
      if (!role) return current
      const roles = current.roles.filter((_, roleIndex) => roleIndex !== index)
      const policies = { ...current.policies }
      delete policies[role.id]
      return { ...current, roles, policies }
    })
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
    const err = await onSave(serializeDispatchConfig(draft))
    if (err) {
      setError(err)
      setSaving(false)
      return
    }
    setSavedAt(Date.now())
    setSaving(false)
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="gap-3">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="space-y-1.5">
            <CardTitle className="text-sm">{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
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
              {saving ? 'Saving…' : 'Save settings'}
            </Button>
          </div>
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">
          Worker profile secrets are referenced by environment variable name.
          Secret values stay in the Flywheel server environment.
        </p>
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </CardHeader>
      <CardContent className="space-y-6">
        <section className="space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium text-foreground">Worker profiles</h3>
              <p className="text-xs text-muted-foreground">
                Optional runtime overrides. Leave empty to use the server default
                worker.
              </p>
            </div>
            <Badge variant="outline">
              {draft.workers.length} custom worker{draft.workers.length === 1 ? '' : 's'}
            </Badge>
          </div>

          {draft.workers.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] px-4 py-5 text-sm text-muted-foreground">
              No custom workers yet. Policies can still use the default server
              worker, or you can add explicit Claude profiles here.
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
                          placeholder={worker.driver === 'codex' ? 'codex (from Models & harnesses)' : worker.driver === 'generic' ? '/path/to/agent' : 'claude (from Models & harnesses)'}
                        />
                      </Label>
                      <Label>
                        Model
                        <Input
                          value={worker.model ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { model: event.target.value })
                          }
                          placeholder={worker.driver === 'codex' ? 'blank = Codex default from Models & harnesses' : 'blank = harness default from Models & harnesses'}
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
                      <Label className="md:col-span-2 xl:col-span-4">
                        Base prompt
                        <Textarea
                          value={worker.system_prompt ?? ''}
                          onChange={(event) => updateWorker(index, { system_prompt: event.target.value })}
                          placeholder="Standing instructions for this worker, prepended to every role prompt (review recipe, house style, what to never do…)"
                          rows={3}
                        />
                      </Label>
                      {worker.driver === 'generic' ? (
                        <>
                      <Label>
                        API base URL
                        <Input
                          value={worker.api_base_url ?? ''}
                          onChange={(event) =>
                            updateWorker(index, { api_base_url: event.target.value })
                          }
                          placeholder="https://api.openai.com/v1 or http://localhost:4000/v1"
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
                        </>
                      ) : null}
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
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h3 className="text-sm font-medium text-foreground">Role policies</h3>
              <p className="text-xs text-muted-foreground">
                Leave a role empty to use every enabled worker for that role.
                Ordered mode fails over in sequence. Rotate mode spreads initial
                attempts across the selected workers and still fails over.
              </p>
            </div>
            <Button variant="outline" size="xs" onClick={addRole}>
              <Plus className="size-3.5" />
              Add role
            </Button>
          </div>

          <div className="space-y-3">
            {roleOptions.map((role) => {
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
              const customRole =
                role.customIndex !== undefined
                  ? draft.roles[role.customIndex]
                  : undefined

              return (
                <div
                  key={role.key}
                  className="rounded-2xl border border-white/10 bg-black/10 p-4"
                >
                  <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
                    <div className="flex-1 space-y-3">
                      <div className="flex items-center gap-2">
                        <p className="text-sm font-medium text-foreground">
                          {role.label}
                        </p>
                        <Badge variant="outline">{role.key}</Badge>
                        <Badge variant="outline">
                          {role.builtIn ? 'Built-in' : 'Custom'}
                        </Badge>
                        {role.baseType ? (
                          <Badge variant="outline">
                            {humanizeRoleKey(role.baseType)} base
                          </Badge>
                        ) : null}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {role.description}
                      </p>
                      {customRole ? (
                        <div className="grid gap-3 md:grid-cols-3">
                          <Label>
                            Role name
                            <Input
                              value={customRole.name}
                              onChange={(event) =>
                                updateRole(role.customIndex ?? -1, {
                                  name: event.target.value,
                                })
                              }
                              placeholder="Security review"
                            />
                          </Label>
                          <Label>
                            Role key
                            <Input
                              value={customRole.id}
                              onChange={(event) =>
                                updateRole(role.customIndex ?? -1, {
                                  id: event.target.value,
                                })
                              }
                              placeholder="security_review"
                            />
                          </Label>
                          <Label>
                            Base behavior
                            <Select
                              value={customRole.base_type}
                              onValueChange={(value) =>
                                updateRole(role.customIndex ?? -1, {
                                  base_type: parseRoleBaseType(value),
                                })
                              }
                            >
                              <SelectTrigger className="w-full bg-white/5">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {BASE_TYPE_OPTIONS.map((option) => (
                                  <SelectItem key={option.value} value={option.value}>
                                    {option.label}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </Label>
                          <Label className="md:col-span-3">
                            Role-specific instructions
                            <Textarea
                              value={customRole.description}
                              onChange={(event) =>
                                updateRole(role.customIndex ?? -1, {
                                  description: event.target.value,
                                })
                              }
                              rows={2}
                              placeholder="Instructions appended after the base prompt for this role. Use for role-specific behavioral rules."
                            />
                          </Label>
                        </div>
                      ) : null}
                    </div>
                    <div className="flex flex-col gap-3 xl:w-52">
                      <Label>
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
                      {role.customIndex !== undefined ? (
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => removeRole(role.customIndex ?? -1)}
                        >
                          <Trash2 className="size-3.5" />
                          Remove role
                        </Button>
                      ) : null}
                    </div>
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
