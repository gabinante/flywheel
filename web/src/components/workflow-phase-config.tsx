import { Bot, Cog, ShieldCheck, Zap } from 'lucide-react'

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

const GATE_REQUIREMENT_OPTIONS = [
  { value: 'github_checks', label: 'GitHub Checks' },
  { value: 'human_approval', label: 'Human Approval' },
] as const

export function GatePhaseConfig({ phase, onChange }: PhaseConfigProps) {
  const config = phase.config ?? {}
  const requirements = (config.requirements as string[]) ?? []

  const toggleRequirement = (req: string) => {
    const next = requirements.includes(req)
      ? requirements.filter((r) => r !== req)
      : [...requirements, req]
    onChange(updateConfig(phase, 'requirements', next))
  }

  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Label>
          Prompt
          <Textarea
            value={(config.prompt as string) || ''}
            onChange={(e) => onChange(updateConfig(phase, 'prompt', e.target.value))}
            placeholder="What should the reviewer check before advancing?"
            rows={2}
          />
        </Label>
        <Label>
          Required role (optional)
          <Input
            value={(config.required_role as string) || ''}
            onChange={(e) => onChange(updateConfig(phase, 'required_role', e.target.value))}
            placeholder="e.g. admin"
          />
        </Label>
      </div>
      <Label>
        Requirements
        <div className="flex flex-wrap gap-2 pt-1">
          {GATE_REQUIREMENT_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => toggleRequirement(opt.value)}
              className={`rounded-full border px-2.5 py-1 text-[11px] font-medium transition-colors ${
                requirements.includes(opt.value)
                  ? 'border-purple-500/40 bg-purple-500/15 text-purple-300'
                  : 'border-white/10 bg-white/5 text-muted-foreground hover:border-white/20'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </Label>
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
