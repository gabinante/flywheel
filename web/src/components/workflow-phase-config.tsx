import { Bot, Cog, Plus, ShieldCheck, Trash2, Zap } from 'lucide-react'

import { Button } from '@/components/ui/button'
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

type WorkflowPhase = components['schemas']['WorkflowPhase']

const WORKER_ROLES = [
  { value: 'executor', label: 'Executor', description: 'Implements changes — code, config, or other deliverables' },
  { value: 'planner', label: 'Planner', description: 'Investigates context, produces structured implementation plans' },
  { value: 'validator', label: 'Validator', description: 'Reviews work adversarially, approves or rejects with feedback' },
  { value: 'deployer', label: 'Deployer', description: 'Executes deployments and verifies rollout success' },
  { value: 'investigator', label: 'Investigator', description: 'Read-only research and analysis' },
  { value: 'operator', label: 'Operator', description: 'Triage, analysis, and operational response' },
] as const

const EXTERNAL_MODES = [
  { value: 'sync', label: 'Sync' },
  { value: 'async', label: 'Async' },
  { value: 'poll', label: 'Poll' },
] as const

export const PHASE_TYPE_META = {
  agent: {
    label: 'Agent',
    color: 'text-blue-400',
    dotColor: 'bg-blue-500',
    bgColor: 'bg-blue-500/10 border-blue-500/30',
    icon: Bot,
  },
  external: {
    label: 'External',
    color: 'text-amber-400',
    dotColor: 'bg-amber-500',
    bgColor: 'bg-amber-500/10 border-amber-500/30',
    icon: Zap,
  },
  gate: {
    label: 'Gate',
    color: 'text-purple-400',
    dotColor: 'bg-purple-500',
    bgColor: 'bg-purple-500/10 border-purple-500/30',
    icon: ShieldCheck,
  },
  action: {
    label: 'Action',
    color: 'text-emerald-400',
    dotColor: 'bg-emerald-500',
    bgColor: 'bg-emerald-500/10 border-emerald-500/30',
    icon: Cog,
  },
} as const

export type PhaseType = keyof typeof PHASE_TYPE_META

export function getPhaseTypeMeta(type: WorkflowPhase['type']) {
  if (type in PHASE_TYPE_META) return PHASE_TYPE_META[type as PhaseType]
  // Legacy type fallback
  switch (type) {
    case 'manual':
      return PHASE_TYPE_META.gate
    case 'automated':
    case 'deploy':
    case 'observe':
      return PHASE_TYPE_META.external
    default:
      return PHASE_TYPE_META.agent
  }
}

type PhaseConfigProps = {
  phase: WorkflowPhase
  allPhases: WorkflowPhase[]
  onChange: (phase: WorkflowPhase) => void
}

function updateConfig(phase: WorkflowPhase, key: string, value: unknown): WorkflowPhase {
  return { ...phase, config: { ...phase.config, [key]: value } }
}

export function AgentPhaseConfig({ phase, onChange }: PhaseConfigProps) {
  const config = phase.config ?? {}
  const selectedRole = (config.role as string) || 'executor'
  const roleInfo = WORKER_ROLES.find((r) => r.value === selectedRole)
  const autoAdvance = config.auto_advance !== false
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label>
            Worker role
            <Select
              value={selectedRole}
              onValueChange={(v) => onChange(updateConfig(phase, 'role', v))}
            >
              <SelectTrigger className="w-full bg-white/5">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WORKER_ROLES.map((r) => (
                  <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>
          {roleInfo && (
            <p className="text-[11px] text-muted-foreground">{roleInfo.description}</p>
          )}
        </div>
        <Label>
          Goal
          <Textarea
            value={(config.goal as string) || ''}
            onChange={(e) => onChange(updateConfig(phase, 'goal', e.target.value))}
            placeholder="Objective for the agent session"
            rows={2}
          />
        </Label>
      </div>
      <Label>
        Custom instructions
        <Textarea
          value={(config.prompt as string) || ''}
          onChange={(e) => onChange(updateConfig(phase, 'prompt', e.target.value))}
          placeholder="Additional system prompt instructions for this phase's agent"
          rows={3}
        />
      </Label>
      <div className="flex items-center gap-6">
        <Label>
          Max iterations (0 = unlimited)
          <Input
            type="number"
            min={0}
            value={(config.max_iterations as number) ?? 0}
            onChange={(e) => onChange(updateConfig(phase, 'max_iterations', parseInt(e.target.value) || 0))}
            className="w-32"
          />
        </Label>
        <div className="flex items-center gap-2 pt-5">
          <Switch
            id={`auto-advance-${phase.id}`}
            checked={autoAdvance}
            onCheckedChange={(checked) => onChange(updateConfig(phase, 'auto_advance', checked))}
          />
          <label htmlFor={`auto-advance-${phase.id}`} className="text-xs text-muted-foreground cursor-pointer">
            Auto-advance on success
          </label>
        </div>
      </div>
    </div>
  )
}

export function ExternalPhaseConfig({ phase, onChange }: PhaseConfigProps) {
  const config = phase.config ?? {}
  const mode = (config.mode as string) || 'sync'
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-3">
        <Label>
          Mode
          <Select value={mode} onValueChange={(v) => onChange(updateConfig(phase, 'mode', v))}>
            <SelectTrigger className="w-full bg-white/5">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {EXTERNAL_MODES.map((m) => (
                <SelectItem key={m.value} value={m.value}>{m.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Label>
        <Label>
          Method
          <Select
            value={(config.method as string) || 'POST'}
            onValueChange={(v) => onChange(updateConfig(phase, 'method', v))}
          >
            <SelectTrigger className="w-full bg-white/5">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="GET">GET</SelectItem>
              <SelectItem value="POST">POST</SelectItem>
            </SelectContent>
          </Select>
        </Label>
        <Label>
          URL
          <Input
            value={(config.url as string) || ''}
            onChange={(e) => onChange(updateConfig(phase, 'url', e.target.value))}
            placeholder="https://api.example.com/webhook"
          />
        </Label>
      </div>
      {mode === 'poll' ? (
        <div className="grid gap-3 sm:grid-cols-3">
          <Label>
            Poll URL
            <Input
              value={(config.poll_url as string) || ''}
              onChange={(e) => onChange(updateConfig(phase, 'poll_url', e.target.value))}
              placeholder="URL to poll for status"
            />
          </Label>
          <Label>
            Interval
            <Input
              value={(config.poll_interval as string) || '30s'}
              onChange={(e) => onChange(updateConfig(phase, 'poll_interval', e.target.value))}
              placeholder="30s"
            />
          </Label>
          <Label>
            Timeout
            <Input
              value={(config.poll_timeout as string) || '30m'}
              onChange={(e) => onChange(updateConfig(phase, 'poll_timeout', e.target.value))}
              placeholder="30m"
            />
          </Label>
        </div>
      ) : null}
      <Label>
        Body template
        <Textarea
          value={(config.body_template as string) || ''}
          onChange={(e) => onChange(updateConfig(phase, 'body_template', e.target.value))}
          placeholder={'Variables: {{ticket_id}}, {{phase_id}}, {{workflow_id}}, {{callback_url}}'}
          rows={2}
        />
      </Label>
    </div>
  )
}

type GateCondition = { type: string; config?: Record<string, unknown> }

const GATE_CONDITION_TYPES = [
  { value: 'github_checks', label: 'GitHub Checks' },
  { value: 'human_approval', label: 'Human Approval' },
  { value: 'webhook', label: 'Webhook' },
  { value: 'http_check', label: 'HTTP Check' },
] as const

function conditionLabel(type: string): string {
  return GATE_CONDITION_TYPES.find((t) => t.value === type)?.label ?? type
}

/** Convert legacy requirements array to conditions array */
function effectiveConditions(config: Record<string, unknown>): GateCondition[] {
  if (Array.isArray(config.conditions) && config.conditions.length > 0) {
    return config.conditions as GateCondition[]
  }
  if (Array.isArray(config.requirements)) {
    return (config.requirements as string[]).map((r) => ({ type: r }))
  }
  return []
}

export function GatePhaseConfig({ phase, onChange }: PhaseConfigProps) {
  const config = phase.config ?? {}
  const conditions = effectiveConditions(config)

  const setConditions = (next: GateCondition[]) => {
    // Write conditions and clear legacy fields
    const updated = { ...phase, config: { ...config, conditions: next } }
    delete updated.config.requirements
    delete updated.config.prompt
    onChange(updated)
  }

  const addCondition = (type: string) => {
    setConditions([...conditions, { type }])
  }

  const removeCondition = (index: number) => {
    setConditions(conditions.filter((_, i) => i !== index))
  }

  const updateConditionConfig = (index: number, key: string, value: unknown) => {
    const next = [...conditions]
    next[index] = { ...next[index], config: { ...next[index].config, [key]: value } }
    setConditions(next)
  }

  return (
    <div className="space-y-3">
      <Label>
        Required role (optional)
        <Input
          value={(config.required_role as string) || ''}
          onChange={(e) => onChange(updateConfig(phase, 'required_role', e.target.value))}
          placeholder="e.g. admin"
        />
      </Label>
      <div className="space-y-2">
        <span className="text-xs font-medium text-muted-foreground">Conditions</span>
        {conditions.map((cond, i) => (
          <div key={i} className="flex items-start gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
            <span className="mt-0.5 rounded-full border border-purple-500/40 bg-purple-500/15 px-2 py-0.5 text-[11px] font-medium text-purple-300">
              {conditionLabel(cond.type)}
            </span>
            <div className="flex-1 space-y-2">
              {cond.type === 'http_check' && (
                <div className="grid gap-2 sm:grid-cols-3">
                  <Input
                    value={(cond.config?.url as string) || ''}
                    onChange={(e) => updateConditionConfig(i, 'url', e.target.value)}
                    placeholder="https://api.example.com/health"
                    className="text-xs"
                  />
                  <Select
                    value={(cond.config?.method as string) || 'GET'}
                    onValueChange={(v) => updateConditionConfig(i, 'method', v)}
                  >
                    <SelectTrigger className="w-full bg-white/5 text-xs">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="GET">GET</SelectItem>
                      <SelectItem value="HEAD">HEAD</SelectItem>
                    </SelectContent>
                  </Select>
                  <Input
                    type="number"
                    value={(cond.config?.expected_status as number) ?? 200}
                    onChange={(e) => updateConditionConfig(i, 'expected_status', parseInt(e.target.value) || 200)}
                    placeholder="200"
                    className="text-xs"
                  />
                </div>
              )}
              {cond.type === 'webhook' && (
                <Input
                  value={(cond.config?.notification_url as string) || ''}
                  onChange={(e) => updateConditionConfig(i, 'notification_url', e.target.value)}
                  placeholder="Notification URL (optional)"
                  className="text-xs"
                />
              )}
            </div>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => removeCondition(i)}
              className="mt-0.5 text-muted-foreground hover:text-destructive"
            >
              <Trash2 className="size-3" />
            </Button>
          </div>
        ))}
        <Select onValueChange={(v) => addCondition(v)} value="">
          <SelectTrigger className="w-48 bg-white/5 text-xs">
            <span className="flex items-center gap-1 text-muted-foreground">
              <Plus className="size-3" /> Add condition
            </span>
          </SelectTrigger>
          <SelectContent>
            {GATE_CONDITION_TYPES.map((t) => (
              <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}

export function ActionPhaseConfigForm({ phase, onChange }: PhaseConfigProps) {
  const config = phase.config ?? {}
  return (
    <div className="space-y-3">
      <Label>
        Action name
        <Input
          value={(config.action as string) || ''}
          onChange={(e) => onChange(updateConfig(phase, 'action', e.target.value))}
          placeholder="registered_handler_name"
          className="font-mono"
        />
      </Label>
      <Label>
        Parameters (JSON)
        <Textarea
          value={config.params ? JSON.stringify(config.params, null, 2) : ''}
          onChange={(e) => {
            try {
              const parsed = e.target.value.trim() ? JSON.parse(e.target.value) : undefined
              onChange(updateConfig(phase, 'params', parsed))
            } catch {
              // Allow partial editing — don't update until valid JSON
            }
          }}
          placeholder='{"key": "value"}'
          rows={3}
          className="font-mono text-xs"
        />
      </Label>
    </div>
  )
}

export function PhaseConfigForm(props: PhaseConfigProps) {
  const meta = getPhaseTypeMeta(props.phase.type)
  if (meta === PHASE_TYPE_META.agent || props.phase.type === 'agent') {
    return <AgentPhaseConfig {...props} />
  }
  if (props.phase.type === 'gate' || props.phase.type === 'manual') {
    return <GatePhaseConfig {...props} />
  }
  if (props.phase.type === 'action') {
    return <ActionPhaseConfigForm {...props} />
  }
  return <ExternalPhaseConfig {...props} />
}
