import { useState } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { StyledSelect } from '@/components/ui/styled-select'
import { PromptLibrary } from '@/components/prompt-library'
import { ROLE_OPTIONS } from '@/lib/dispatch-roles'
import type { components } from '@/lib/api/v1'

type Settings = components['schemas']['OperatorSettings']
type Worker = components['schemas']['DispatchWorkerProfile']
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
  const options = [
    {
      value: 'default',
      label: current.role_id
        ? `Existing routing (${current.role_id})`
        : `Default ${current.harness} worker${current.model ? ` · ${current.model}` : ''}`,
    },
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
        value={current.worker_id || 'default'}
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
  const update = (patch: Partial<Worker>) =>
    onChange({ ...settings, workers: { ...settings.workers, workers: workers.map((w) => (w.id === selected ? { ...w, ...patch } : w)) } })
  const add = () => {
    const id = `worker-${crypto.randomUUID().slice(0, 8)}`
    // Adding a definition must not implicitly assign it to unconfigured tasks.
    const policies = { ...settings.workers.policies }
    const previous = workers.filter((w) => w.enabled !== false).map((w) => w.id!)
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
                        className="w-full min-w-0"
                        value={worker.driver || 'default'}
                        options={[
                          { value: 'default', label: 'Task default' },
                          { value: 'codex', label: 'Codex' },
                          { value: 'claude', label: 'Claude Code' },
                          { value: 'generic', label: 'Generic CLI' },
                        ]}
                        onValueChange={(v) => update({ driver: v === 'default' ? undefined : (v as Worker['driver']) })}
                      />
                    </Label>
                    <Label>
                      Model
                      <Input value={worker.model || ''} placeholder="Harness default" onChange={(e) => update({ model: e.target.value })} />
                    </Label>
                  </div>
                  <Label>
                    Reasoning effort
                    <Input
                      value={worker.reasoning_effort || ''}
                      placeholder="Harness default"
                      onChange={(e) => update({ reasoning_effort: e.target.value })}
                    />
                  </Label>
                  <Label>
                    Instructions
                    <Textarea
                      aria-label="Instructions"
                      rows={8}
                      value={worker.system_prompt || ''}
                      placeholder="How should this worker approach its tasks?"
                      onChange={(e) => update({ system_prompt: e.target.value })}
                    />
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    These instructions supplement the task instructions. Edit shared task instructions under Task assignments.
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
                const value = simple ? ids[0] : 'existing'
                const options = [
                  {
                    value: 'existing',
                    label: ids.length
                      ? `Existing routing: ${ids.join(', ')}`
                      : workers.some((w) => w.enabled !== false)
                        ? 'Existing routing: all enabled workers'
                        : 'Default server worker',
                  },
                  { value: 'default', label: 'Default server worker' },
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
