import { useCallback, useEffect, useState } from 'react'
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  ChevronDown,
  ChevronRight,
  GripVertical,
  Plus,
  Save,
  Trash2,
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
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import {
  getPhaseTypeMeta,
  PhaseConfigForm,
  PHASE_TYPE_META,
  type PhaseType,
} from './workflow-phase-config'

type WorkflowDefinition = components['schemas']['WorkflowDefinition']
type WorkflowPhase = components['schemas']['WorkflowPhase']

const PRIMARY_TYPES: PhaseType[] = ['agent', 'external', 'gate']

function newPhaseID(phases: WorkflowPhase[]): string {
  let idx = phases.length + 1
  const ids = new Set(phases.map((p) => p.id))
  while (ids.has(`step-${idx}`)) idx++
  return `step-${idx}`
}

function defaultPhase(phases: WorkflowPhase[], type: PhaseType): WorkflowPhase {
  const id = newPhaseID(phases)
  const meta = PHASE_TYPE_META[type]
  return {
    id,
    name: `New ${meta.label} Step`,
    type,
    description: '',
    config: type === 'agent' ? { role: 'executor' } : type === 'external' ? { mode: 'sync' } : {},
  }
}

function SortablePhaseNode({
  phase,
  index,
  totalPhases,
  allPhases,
  expanded,
  onToggle,
  onChange,
  onRemove,
}: {
  phase: WorkflowPhase
  index: number
  totalPhases: number
  allPhases: WorkflowPhase[]
  expanded: boolean
  onToggle: () => void
  onChange: (phase: WorkflowPhase) => void
  onRemove: () => void
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: phase.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 10 : undefined,
    opacity: isDragging ? 0.5 : 1,
  }

  const meta = getPhaseTypeMeta(phase.type)
  const Icon = meta.icon

  // On-failure dropdown options
  const failureOptions = allPhases.filter((p) => p.id !== phase.id)

  return (
    <div ref={setNodeRef} style={style}>
      {/* Timeline connector */}
      {index > 0 ? (
        <div className="ml-5 h-4 w-px bg-white/10" />
      ) : null}

      <div className={`rounded-xl border ${expanded ? meta.bgColor : 'border-white/10 bg-white/[0.03]'} transition-colors`}>
        {/* Header row */}
        <div className="flex items-center gap-2 px-3 py-2.5">
          <button
            type="button"
            className="cursor-grab touch-none text-muted-foreground hover:text-foreground"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="size-4" />
          </button>

          <button type="button" onClick={onToggle} className="text-muted-foreground hover:text-foreground">
            {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
          </button>

          <div className={`rounded-md p-1 ${meta.bgColor}`}>
            <Icon className={`size-3.5 ${meta.color}`} />
          </div>

          {expanded ? (
            <Input
              value={phase.name}
              onChange={(e) => onChange({ ...phase, name: e.target.value })}
              className="h-7 flex-1 border-none bg-transparent px-1 text-sm font-medium"
            />
          ) : (
            <span className="flex-1 truncate text-sm font-medium">{phase.name}</span>
          )}

          <Badge variant="outline" className={`text-[10px] ${meta.color}`}>
            {meta.label}
          </Badge>

          <span className="text-[10px] text-muted-foreground tabular-nums">
            {index + 1}/{totalPhases}
          </span>

          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onRemove}
            className="text-muted-foreground hover:text-destructive"
            title="Remove step"
          >
            <Trash2 className="size-3.5" />
          </Button>
        </div>

        {/* Expanded config */}
        {expanded ? (
          <div className="space-y-3 border-t border-white/10 px-3 py-3">
            <div className="grid gap-3 sm:grid-cols-3">
              <Label>
                Phase ID
                <Input
                  value={phase.id}
                  onChange={(e) => onChange({ ...phase, id: e.target.value })}
                  className="font-mono text-xs"
                />
              </Label>
              <Label>
                Type
                <Select
                  value={phase.type}
                  onValueChange={(v) => onChange({ ...phase, type: v as WorkflowPhase['type'], config: {} })}
                >
                  <SelectTrigger className="w-full bg-white/5">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PRIMARY_TYPES.map((t) => (
                      <SelectItem key={t} value={t}>
                        {PHASE_TYPE_META[t].label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Label>
              <Label>
                On failure
                <Select
                  value={phase.on_failure || '_none'}
                  onValueChange={(v) => onChange({ ...phase, on_failure: v === '_none' ? undefined : v })}
                >
                  <SelectTrigger className="w-full bg-white/5">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="_none">Fail ticket</SelectItem>
                    {failureOptions.map((p) => (
                      <SelectItem key={p.id} value={p.id}>Jump to: {p.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Label>
            </div>

            <Label>
              Description
              <Input
                value={phase.description || ''}
                onChange={(e) => onChange({ ...phase, description: e.target.value })}
                placeholder="What this step does"
              />
            </Label>

            <PhaseConfigForm
              phase={phase}
              allPhases={allPhases}
              onChange={onChange}
            />
          </div>
        ) : null}
      </div>

      {/* Connector after (only if not last) */}
      {index < totalPhases - 1 && !expanded ? (
        <div className="ml-5 h-4 w-px bg-white/10" />
      ) : null}
    </div>
  )
}

export function WorkflowTimelineEditor({
  projectId,
  orgId,
  scope = 'project',
}: {
  projectId?: string
  orgId?: string
  scope?: 'project' | 'org'
}) {
  const { client } = useAuth()
  const [definition, setDefinition] = useState<WorkflowDefinition | null>(null)
  const [phases, setPhases] = useState<WorkflowPhase[]>([])
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const [expandedId, setExpandedId] = useState<string | null>(null)
  const [isSuggested, setIsSuggested] = useState(false)
  const [source, setSource] = useState<string | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      const endpoint = scope === 'org'
        ? '/orgs/{orgID}/workflow'
        : '/projects/{projectID}/workflow'
      const pathParams = scope === 'org'
        ? { orgID: orgId }
        : { projectID: projectId }
      const { data, response } = await client.GET(
        endpoint as never,
        { params: { path: pathParams } } as never,
      )
      if (cancelled) return
      setLoading(false)
      const payload = data as { workflow?: WorkflowDefinition; suggested?: WorkflowDefinition; source?: string } | undefined
      const wf = payload?.workflow
      const suggested = payload?.suggested
      const wfSource = payload?.source
      if (response.ok && wf) {
        setDefinition(wf)
        setPhases(wf.phases ?? [])
        setName(wf.name ?? '')
        setDescription(wf.description ?? '')
        setSource(wfSource ?? scope)
        setIsSuggested(false)
      } else if (response.ok && suggested) {
        setPhases((suggested.phases ?? []) as WorkflowPhase[])
        setName(suggested.name ?? 'Delivery Pipeline')
        setDescription(suggested.description ?? '')
        setIsSuggested(true)
        setSource(null)
      }
    })()
    return () => { cancelled = true }
  }, [client, projectId, orgId, scope])

  const handleDragEnd = useCallback((event: DragEndEvent) => {
    const { active, over } = event
    if (over && active.id !== over.id) {
      setPhases((prev) => {
        const oldIndex = prev.findIndex((p) => p.id === active.id)
        const newIndex = prev.findIndex((p) => p.id === over.id)
        return arrayMove(prev, oldIndex, newIndex)
      })
    }
  }, [])

  const updatePhase = useCallback((index: number, updated: WorkflowPhase) => {
    setPhases((prev) => {
      const next = [...prev]
      next[index] = updated
      return next
    })
  }, [])

  const removePhase = useCallback((index: number) => {
    setPhases((prev) => prev.filter((_, i) => i !== index))
    setExpandedId(null)
  }, [])

  const addPhase = useCallback((type: PhaseType, afterIndex?: number) => {
    setPhases((prev) => {
      const p = defaultPhase(prev, type)
      const next = [...prev]
      const insertAt = afterIndex !== undefined ? afterIndex + 1 : next.length
      next.splice(insertAt, 0, p)
      setExpandedId(p.id)
      return next
    })
  }, [])

  const saveToScope = useCallback(async (targetScope: 'project' | 'org') => {
    setSaving(true)
    setError(null)
    setSavedAt(null)

    const body = {
      name: name || 'Delivery Pipeline',
      description,
      phases,
    }

    const endpoint = targetScope === 'org'
      ? '/orgs/{orgID}/workflow'
      : '/projects/{projectID}/workflow'
    const pathParams = targetScope === 'org'
      ? { orgID: orgId }
      : { projectID: projectId }

    const { data, error: apiError, response } = await client.PUT(
      endpoint as never,
      {
        params: { path: pathParams },
        body,
      } as never,
    )
    setSaving(false)
    if (!response.ok) {
      setError(formatApiError(apiError))
      return
    }
    setDefinition(data as unknown as WorkflowDefinition)
    setIsSuggested(false)
    setSource(targetScope)
    setSavedAt(Date.now())
  }, [client, projectId, orgId, name, description, phases])

  const save = useCallback(() => saveToScope(scope), [saveToScope, scope])

  if (loading) {
    return (
      <Card className="border-white/10 bg-white/5 backdrop-blur-md">
        <CardContent className="py-8 text-center">
          <span className="text-sm text-muted-foreground">Loading workflow…</span>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1.5">
          <CardTitle className="text-sm">
            {scope === 'org' ? 'Organization Default Pipeline' : 'Delivery Pipeline'}
          </CardTitle>
          <CardDescription>
            {scope === 'org'
              ? definition
                ? 'Default pipeline inherited by all projects in this organization.'
                : isSuggested
                  ? 'Pre-populated with the suggested default. Save to set as org default.'
                  : 'No org default configured. Add steps to create one.'
              : definition
                ? 'Define the stages tickets move through in this project.'
                : isSuggested
                  ? 'Pre-populated with the suggested default. Save to activate.'
                  : 'No pipeline configured. Add steps to create one.'}
          </CardDescription>
        </div>
        <div className="flex items-center gap-2">
          {savedAt ? <span className="text-xs text-emerald-400">Saved</span> : null}
          <Button size="xs" onClick={() => void save()} disabled={saving}>
            <Save className="size-3.5" />
            {saving ? 'Saving…' : scope === 'org' ? 'Save org default' : 'Save pipeline'}
          </Button>
          {scope === 'project' && orgId && phases.length > 0 ? (
            <Button
              size="xs"
              variant="outline"
              onClick={() => void saveToScope('org')}
              disabled={saving}
              title="Save this pipeline as the default for all projects in this organization"
            >
              <Save className="size-3.5" />
              Set as org default
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? <p className="text-sm text-destructive">{error}</p> : null}

        {isSuggested ? (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-300">
            This is the suggested default pipeline (Standard SDLC). Click <strong>{scope === 'org' ? 'Save org default' : 'Save pipeline'}</strong> to activate it.
          </div>
        ) : null}

        {!isSuggested && scope === 'project' && source && source !== 'project' ? (
          <div className="rounded-lg border border-blue-500/30 bg-blue-500/10 px-3 py-2 text-sm text-blue-300">
            Inherited from <strong>{source}</strong> scope. Save to override for this project.
          </div>
        ) : null}

        <div className="grid gap-3 sm:grid-cols-2">
          <Label>
            Pipeline name
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Delivery Pipeline"
            />
          </Label>
          <Label>
            Description
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="How tickets flow through this project"
            />
          </Label>
        </div>

        {/* Timeline */}
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={handleDragEnd}
        >
          <SortableContext
            items={phases.map((p) => p.id)}
            strategy={verticalListSortingStrategy}
          >
            <div className="space-y-0">
              {phases.map((phase, index) => (
                <SortablePhaseNode
                  key={phase.id}
                  phase={phase}
                  index={index}
                  totalPhases={phases.length}
                  allPhases={phases}
                  expanded={expandedId === phase.id}
                  onToggle={() => setExpandedId(expandedId === phase.id ? null : phase.id)}
                  onChange={(updated) => updatePhase(index, updated)}
                  onRemove={() => removePhase(index)}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>

        {/* Add step buttons */}
        <div className="flex items-center gap-2 pt-2">
          <span className="text-xs text-muted-foreground">Add step:</span>
          {PRIMARY_TYPES.map((type) => {
            const meta = PHASE_TYPE_META[type]
            const Icon = meta.icon
            return (
              <Button
                key={type}
                variant="outline"
                size="xs"
                onClick={() => addPhase(type)}
                className={meta.color}
              >
                <Icon className="size-3.5" />
                <Plus className="size-3" />
                {meta.label}
              </Button>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}
