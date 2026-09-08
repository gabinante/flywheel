import { useState } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { StyledSelect } from '@/components/ui/styled-select'
import { PromptLibrary } from '@/components/prompt-library'
import { modelChoices, effortChoices, workerOptions } from '@/lib/worker-options'
import { ROLE_OPTIONS } from '@/lib/dispatch-roles'
import type { components } from '@/lib/api/v1'

type Settings = components['schemas']['OperatorSettings']
type Worker = components['schemas']['DispatchWorkerProfile']
const isBuiltin = (id?: string) => ['builtin-dispatch', 'builtin-orchestrator', 'builtin-review', 'builtin-feedback'].includes(id || '')
function workerPromptIDs(worker: Worker, settings: Settings): string[] {
  const ids = new Set<string>()
  if (worker.id === 'builtin-review' || settings.review.worker_id === worker.id) ids.add('code_review')
  if (worker.id === 'builtin-feedback' || settings.feedback.worker_id === worker.id) ids.add('pr_feedback')
  if (worker.id === 'builtin-orchestrator') ids.add('orchestrator')
  if (worker.id === 'builtin-dispatch') { ids.add('dispatch_executor'); ids.add('dispatch_worker') }
  for (const [role, policy] of Object.entries(settings.workers.policies || {})) {
    if (!policy.worker_ids?.includes(worker.id!)) continue
    if (settings.review.role_id === role) ids.add('code_review')
    if (settings.feedback.role_id === role) ids.add('pr_feedback')
    if (role === 'orchestrator') ids.add('orchestrator')
    else if (role === 'conflict_resolver') ids.add('conflict_resolver')
    else if (role === 'validator') ids.add('ticket_reviewer')
    else if (['planner', 'executor', 'deployer', 'investigator', 'operator', 'decomposer', 'fast-executor'].includes(role)) ids.add(`dispatch_${role}`)
  }
  return [...ids]
}
const TASKS = [
  ...ROLE_OPTIONS.map((t) => (t.key === 'validator' ? { ...t, label: 'Workflow validation' } : t)),
  { key: 'operator', label: 'Operations', description: 'Triage and operational tasks.' },
  { key: 'decomposer', label: 'Decomposition', description: 'Break work into tickets.' },
  { key: 'fast-executor', label: 'Fast implementation', description: 'Small implementation tasks.' },
]
function supportsReviews(w: Worker) {
  return (
    w.enabled !== false &&
    (w.driver === 'codex' || w.driver === 'claude') &&
    !w.cli_path &&
    !w.args?.length &&
    !w.api_base_url &&
    !w.credential_env_var &&
    (!w.runner || w.runner === 'cli')
  )
}

export function WorkerAssignment({
  settings,
  task,
  onChange,
}: {
  settings: Settings
  task: 'review' | 'feedback'
  onChange: (settings: Settings) => void
}) {
  const current = settings[task]
  const defaultWorker = settings.workers.workers?.find(w => w.id === `builtin-${task}`)
  const options = [
    ...(!defaultWorker || current.role_id ? [{
      value: 'default',
      label: current.role_id
        ? `Existing routing (${current.role_id})`
        : settings.workers.workers?.find(w => w.id === `builtin-${task}`)?.name || `Default ${current.harness} worker`,
    }] : []),
    ...(settings.workers.workers ?? []).filter(supportsReviews).map((w) => ({ value: w.id!, label: w.name || w.id! })),
  ]
  if (current.worker_id && !options.some((o) => o.value === current.worker_id))
    options.push({ value: current.worker_id, label: `${current.worker_id} (unavailable)` })
  return (
    <Label>
      {task === 'review' ? 'Review worker' : 'Feedback worker'}
      <StyledSelect
        aria-label={task === 'review' ? 'Review worker' : 'Feedback worker'}
        id={`${task}-worker`}
        className="w-full min-w-0"
        value={current.worker_id || (current.role_id ? 'default' : defaultWorker?.id) || 'default'}
        options={options}
        onValueChange={(v) => onChange({ ...settings, [task]: { ...current, worker_id: v === 'default' ? '' : v } })}
      />
    </Label>
  )
}

export function WorkerWorkspace({
  settings,
  onChange,
  connections,
  legacy,
  initialTab = 'workers',
}: {
  settings: Settings
  onChange: (settings: Settings) => void
  connections: React.ReactNode
  legacy: React.ReactNode
  initialTab?: string
}) {
  const [tab, setTab] = useState(initialTab)
  const [selected, setSelected] = useState<string | null>(null)
  const workers = settings.workers.workers ?? []
  const worker = workers.find((w) => w.id === selected)
  const promptIDs = worker ? workerPromptIDs(worker, settings) : []
  const update = (patch: Partial<Worker>) =>
    onChange({ ...settings, workers: { ...settings.workers, workers: workers.map((w) => (w.id === selected ? { ...w, ...patch } : w)) } })
  const add = () => {
    const id = `worker-${crypto.randomUUID().slice(0, 8)}`
    // Adding a definition must not implicitly assign it to unconfigured tasks.
    const policies = { ...settings.workers.policies }
    const previous = workers.filter((w) => w.enabled !== false && !isBuiltin(w.id)).map((w) => w.id!)
    for (const task of [...TASKS, ...(settings.workers.roles ?? []).map((r) => ({ key: r.id! }))]) {
      if (!policies[task.key]?.worker_ids?.length)
        policies[task.key] = {
          selection_mode: policies[task.key]?.selection_mode || 'ordered',
          worker_ids: previous.length ? previous : ['default'],
        }
    }
    onChange({
      ...settings,
      workers: { ...settings.workers, policies, workers: [...workers, { id, name: 'New worker', driver: 'codex', enabled: true }] },
    })
    setSelected(id)
  }
  return (
    <div className="space-y-5">
      <div>
        <p className="text-sm text-muted-foreground">
          Define a worker once, then choose it for a task or workflow step. Changes apply to new runs after saving.
        </p>
      </div>
      <div className="flex flex-wrap gap-2" role="tablist" aria-label="Worker settings">
        {[
          ['workers', 'Your workers'],
          ['assignments', 'Task assignments'],
          ['connections', 'Harness connections'],
        ].map(([id, label]) => (
          <Button key={id} role="tab" aria-selected={tab === id} variant={tab === id ? 'secondary' : 'ghost'} onClick={() => setTab(id)}>
            {label}
          </Button>
        ))}
      </div>
      {tab === 'workers' && (
        <>
          <div className="flex items-center justify-between">
            <p className="text-sm text-muted-foreground">{workers.length} saved or draft definitions</p>
            <Button onClick={add}>
              <Plus className="size-4" />
              Add worker
            </Button>
          </div>
          <div className="grid gap-4 xl:grid-cols-[220px_minmax(0,1fr)]">
            <div className="space-y-2">
              {workers.map((w) => (
                <button
                  key={w.id}
                  onClick={() => setSelected(w.id!)}
                  className={`w-full rounded-xl border p-3 text-left ${w.id === selected ? 'border-primary bg-primary/5' : 'border-border'}`}
                >
                  <span className="block font-medium">{w.name || 'Unnamed worker'}</span>
                  <span className="block text-xs text-muted-foreground">
                    {[w.driver || 'Default harness', w.model || 'Default model', w.enabled === false ? 'Disabled' : '']
                      .filter(Boolean)
                      .join(' · ')}
                  </span>
                </button>
              ))}
            </div>
            {worker ? (
              <Card>
                <CardHeader>
                  <CardTitle>{worker.name || 'New worker'}</CardTitle>
                  <CardDescription>Harness, model and standing instructions travel with this worker.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <Label>
                    Name
                    <Input value={worker.name || ''} onChange={(e) => update({ name: e.target.value })} />
                  </Label>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Label>
                      Harness
                      <StyledSelect
                        aria-label="Harness"
                        className="w-full min-w-0"
                        value={worker.driver || 'default'}
                        options={[
                          { value: 'default', label: 'Task default' },
                          { value: 'codex', label: 'Codex' },
                          { value: 'claude', label: 'Claude Code' },
                          { value: 'generic', label: 'Generic CLI' },
                        ]}
                        onValueChange={(v) => update({ driver: v === 'default' ? undefined : (v as Worker['driver']), model: '', reasoning_effort: '' })}
                      />
                    </Label>
                    <Label>
                      Model
                      <StyledSelect
                        aria-label="Model"
                        className="w-full min-w-0"
                        value={worker.model || '__harness_default__'}
                        options={workerOptions(modelChoices(worker.driver), worker.model,
                          workers.filter(w => w.driver === worker.driver).map(w => w.model || '').concat(
                            worker.driver === 'codex' || worker.driver === 'claude' ? settings.harnesses[worker.driver].model : []))}
                        onValueChange={(v) => {
                          const model = v === '__harness_default__' ? '' : v
                          update({ model, reasoning_effort: effortChoices(worker.driver, model || (worker.driver === 'codex' || worker.driver === 'claude' ? settings.harnesses[worker.driver].model : '')).includes(worker.reasoning_effort || '') ? worker.reasoning_effort : '' })
                        }}
                      />
                    </Label>
                  </div>
                  <Label>
                    Reasoning effort
                    <StyledSelect
                      aria-label="Reasoning effort"
                      className="w-full min-w-0"
                      value={worker.reasoning_effort || '__harness_default__'}
                      options={workerOptions(effortChoices(worker.driver, worker.model ||
                        (worker.driver === 'codex' || worker.driver === 'claude' ? settings.harnesses[worker.driver].model : '')), worker.reasoning_effort)}
                      onValueChange={(v) => update({ reasoning_effort: v === '__harness_default__' ? '' : v })}
                    />
                  </Label>
                  {worker && promptIDs.length > 0 && <PromptLibrary key={`${worker.id}:${promptIDs.join(',')}`} promptIDs={promptIDs} title="Instructions" />}
                  <Label>
                    Additional instructions
                    <Textarea
                      aria-label="Additional instructions"
                      rows={8}
                      value={worker.system_prompt || ''}
                      placeholder="Optional guidance specific to this worker"
                      onChange={(e) => update({ system_prompt: e.target.value })}
                    />
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    Additional instructions are prepended to the task instructions. Leaving this field empty keeps the task instructions in effect.
                  </p>
                  <details className="space-y-3">
                    <summary className="cursor-pointer text-sm">Advanced options</summary>
                    <p className="text-xs text-muted-foreground">
                      Worker ID: {worker.id}. Custom connection options apply to dispatch and orchestrator tasks; reviews use shared harness
                      connections.
                    </p>
                    <Label>
                      Enabled
                      <StyledSelect
                        className="w-full min-w-0"
                        value={worker.enabled === false ? 'no' : 'yes'}
                        options={[
                          { value: 'yes', label: 'Enabled' },
                          { value: 'no', label: 'Disabled' },
                        ]}
                        onValueChange={(v) => update({ enabled: v === 'yes' })}
                      />
                    </Label>
                    <Label>
                      CLI path
                      <Input value={worker.cli_path || ''} onChange={(e) => update({ cli_path: e.target.value })} />
                    </Label>
                    <Label>
                      Extra arguments
                      <Textarea
                        value={(worker.args || []).join('\n')}
                        onChange={(e) => update({ args: e.target.value.split('\n').filter(Boolean) })}
                      />
                    </Label>
                  </details>
                </CardContent>
              </Card>
            ) : (
              <Card>
                <CardContent className="py-10 text-sm text-muted-foreground">
                  {workers.length
                    ? 'Select a worker to edit its definition.'
                    : 'No custom workers yet. Existing task defaults are still active. Add a worker, then assign it to a task.'}
                </CardContent>
              </Card>
            )}
          </div>
        </>
      )}
      {tab === 'assignments' && (
        <>
          <Card>
            <CardHeader>
              <CardTitle>Choose who does the work</CardTitle>
              <CardDescription>Projects inherit these choices. A workflow step can select its own worker.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <WorkerAssignment settings={settings} task="review" onChange={onChange} />
              <WorkerAssignment settings={settings} task="feedback" onChange={onChange} />
              {TASKS.map((task) => {
                const policy = settings.workers.policies?.[task.key]
                const ids = policy?.worker_ids ?? []
                const simple = ids.length === 1 && (!policy?.selection_mode || policy.selection_mode === 'ordered')
                const fallback = workers.find(w => w.id === (task.key === 'orchestrator' ? 'builtin-orchestrator' : 'builtin-dispatch'))
                const hasImplicitWorkers = workers.some(w => w.enabled !== false && !isBuiltin(w.id))
                const value = simple ? (ids[0] === 'default' && fallback ? fallback.id! : ids[0]) : !ids.length && !hasImplicitWorkers && fallback ? fallback.id! : 'existing'
                const defaultName = workers.find(w => w.id === (task.key === 'orchestrator' ? 'builtin-orchestrator' : 'builtin-dispatch'))?.name || 'Default server worker'
                const options = [
                  ...(value === 'existing' ? [{
                    value: 'existing',
                    label: ids.length
                      ? `Existing routing: ${ids.join(', ')}`
                      : workers.some((w) => w.enabled !== false && !isBuiltin(w.id))
                        ? 'Existing routing: all enabled workers'
                        : defaultName,
                  }] : []),
                  ...(!fallback ? [{ value: 'default', label: defaultName }] : []),
                  ...workers.filter((w) => w.enabled !== false).map((w) => ({ value: w.id!, label: w.name || w.id! })),
                ]
                if (!options.some((o) => o.value === value)) options.push({ value, label: `${value} (unavailable)` })
                return (
                  <Label key={task.key}>
                    {task.label}
                    <StyledSelect
                      aria-label={task.label}
                      className="w-full min-w-0"
                      value={value}
                      options={options}
                      onValueChange={(id) => {
                        if (id !== 'existing')
                          onChange({
                            ...settings,
                            workers: {
                              ...settings.workers,
                              policies: { ...settings.workers.policies, [task.key]: { selection_mode: 'ordered', worker_ids: [id] } },
                            },
                          })
                      }}
                    />
                    <span className="text-xs font-normal text-muted-foreground">{task.description}</span>
                  </Label>
                )
              })}
            </CardContent>
          </Card>
          <PromptLibrary />
          <details>
            <summary className="cursor-pointer text-sm text-muted-foreground">Advanced routing and legacy roles</summary>
            <div className="mt-4">{legacy}</div>
          </details>
        </>
      )}
      {tab === 'connections' && connections}
    </div>
  )
}
