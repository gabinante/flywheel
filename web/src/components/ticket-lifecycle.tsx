import { cn } from '@/lib/utils'
import type { components } from '@/lib/api/v1'
import { getPhaseTypeMeta } from './workflow-phase-config'

/**
 * Ticket lifecycle states and their visual configuration.
 * The state machine flows: pending → claimed → executing → awaiting_review → done
 * With branch states: blocked, needs_human, failed
 *
 * When a workflow is present, renders dynamic workflow phases instead.
 */

type WorkflowPhase = components['schemas']['WorkflowPhase']

type LifecycleState = {
  key: string
  label: string
  dotColor: string
  activeColor: string
  activeBg: string
}

const MAIN_STATES: LifecycleState[] = [
  {
    key: 'pending',
    label: 'Pending',
    dotColor: 'bg-zinc-500',
    activeColor: 'text-zinc-300',
    activeBg: 'bg-zinc-500/15 border-zinc-500/40',
  },
  {
    key: 'claimed',
    label: 'Claimed',
    dotColor: 'bg-blue-500',
    activeColor: 'text-blue-300',
    activeBg: 'bg-blue-500/15 border-blue-500/40',
  },
  {
    key: 'executing',
    label: 'Executing',
    dotColor: 'bg-amber-500',
    activeColor: 'text-amber-300',
    activeBg: 'bg-amber-500/15 border-amber-500/40',
  },
  {
    key: 'awaiting_review',
    label: 'Review',
    dotColor: 'bg-purple-500',
    activeColor: 'text-purple-300',
    activeBg: 'bg-purple-500/15 border-purple-500/40',
  },
  {
    key: 'done',
    label: 'Done',
    dotColor: 'bg-emerald-500',
    activeColor: 'text-emerald-300',
    activeBg: 'bg-emerald-500/15 border-emerald-500/40',
  },
]

const BRANCH_STATES: LifecycleState[] = [
  {
    key: 'blocked',
    label: 'Blocked',
    dotColor: 'bg-orange-500',
    activeColor: 'text-orange-300',
    activeBg: 'bg-orange-500/15 border-orange-500/40',
  },
  {
    key: 'needs_human',
    label: 'Needs Human',
    dotColor: 'bg-yellow-500',
    activeColor: 'text-yellow-300',
    activeBg: 'bg-yellow-500/15 border-yellow-500/40',
  },
  {
    key: 'failed',
    label: 'Failed',
    dotColor: 'bg-red-500',
    activeColor: 'text-red-300',
    activeBg: 'bg-red-500/15 border-red-500/40',
  },
]

function getMainIndex(state: string): number {
  return MAIN_STATES.findIndex((s) => s.key === state)
}

type TicketLifecycleProps = {
  currentState?: string
  className?: string
  workflowPhases?: WorkflowPhase[]
  currentPhaseId?: string
}

function WorkflowLifecycle({
  phases,
  currentPhaseId,
  className,
}: {
  phases: WorkflowPhase[]
  currentPhaseId?: string
  className?: string
}) {
  const currentIdx = phases.findIndex((p) => p.id === currentPhaseId)
  const isDone = currentPhaseId === '' || (currentPhaseId === undefined && phases.length > 0)

  return (
    <div className={cn('flex items-center gap-0 flex-wrap', className)}>
      {phases.map((phase, i) => {
        const meta = getPhaseTypeMeta(phase.type)
        const isActive = phase.id === currentPhaseId
        const isPast = currentIdx >= 0 ? i < currentIdx : isDone
        const isFuture = currentIdx >= 0 ? i > currentIdx : false

        return (
          <div key={phase.id} className="flex items-center">
            <div
              className={cn(
                'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition-all duration-200',
                isActive
                  ? `${meta.bgColor} ${meta.color}`
                  : isPast
                    ? 'border-white/10 bg-white/[0.04] text-muted-foreground'
                    : isFuture
                      ? 'border-transparent bg-transparent text-muted-foreground/40'
                      : 'border-transparent bg-transparent text-muted-foreground/40',
              )}
            >
              <span
                className={cn(
                  'size-1.5 rounded-full transition-colors duration-200',
                  isActive
                    ? meta.dotColor
                    : isPast
                      ? 'bg-muted-foreground/50'
                      : 'bg-muted-foreground/20',
                )}
              />
              {phase.name}
            </div>

            {i < phases.length - 1 ? (
              <div
                className={cn(
                  'h-px w-4 transition-colors duration-200',
                  isPast
                    ? 'bg-muted-foreground/30'
                    : 'bg-muted-foreground/10',
                )}
              />
            ) : null}
          </div>
        )
      })}

      {/* Done node */}
      <div className="flex items-center">
        <div
          className={cn(
            'h-px w-4 transition-colors duration-200',
            isDone ? 'bg-muted-foreground/30' : 'bg-muted-foreground/10',
          )}
        />
        <div
          className={cn(
            'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition-all duration-200',
            isDone
              ? 'bg-emerald-500/15 border-emerald-500/40 text-emerald-300'
              : 'border-transparent bg-transparent text-muted-foreground/40',
          )}
        >
          <span
            className={cn(
              'size-1.5 rounded-full transition-colors duration-200',
              isDone ? 'bg-emerald-500' : 'bg-muted-foreground/20',
            )}
          />
          Done
        </div>
      </div>
    </div>
  )
}

export function TicketLifecycle({
  currentState,
  className,
  workflowPhases,
  currentPhaseId,
}: TicketLifecycleProps) {
  // If workflow phases are provided, render the workflow-aware lifecycle
  if (workflowPhases && workflowPhases.length > 0) {
    return (
      <WorkflowLifecycle
        phases={workflowPhases}
        currentPhaseId={currentPhaseId}
        className={className}
      />
    )
  }

  // Legacy hardcoded state machine
  const mainIdx = getMainIndex(currentState ?? '')
  const isBranch = mainIdx === -1 && currentState !== undefined
  const activeBranch = isBranch
    ? BRANCH_STATES.find((s) => s.key === currentState)
    : null

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      {/* Main flow */}
      <div className="flex items-center gap-0">
        {MAIN_STATES.map((state, i) => {
          const isActive = state.key === currentState
          const isPast = mainIdx >= 0 && i < mainIdx
          const isFuture = mainIdx >= 0 && i > mainIdx

          return (
            <div key={state.key} className="flex items-center">
              {/* State node */}
              <div
                className={cn(
                  'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition-all duration-200',
                  isActive
                    ? `${state.activeBg} ${state.activeColor}`
                    : isPast
                      ? 'border-white/10 bg-white/[0.04] text-muted-foreground'
                      : isFuture
                        ? 'border-transparent bg-transparent text-muted-foreground/40'
                        : 'border-transparent bg-transparent text-muted-foreground/40',
                )}
              >
                <span
                  className={cn(
                    'size-1.5 rounded-full transition-colors duration-200',
                    isActive
                      ? state.dotColor
                      : isPast
                        ? 'bg-muted-foreground/50'
                        : 'bg-muted-foreground/20',
                  )}
                />
                {state.label}
              </div>

              {/* Connector line between nodes */}
              {i < MAIN_STATES.length - 1 ? (
                <div
                  className={cn(
                    'h-px w-4 transition-colors duration-200',
                    isPast
                      ? 'bg-muted-foreground/30'
                      : 'bg-muted-foreground/10',
                  )}
                />
              ) : null}
            </div>
          )
        })}
      </div>

      {/* Branch states - only shown when active */}
      {activeBranch ? (
        <div className="flex items-center gap-2 pl-1">
          <div className="h-3 w-px bg-muted-foreground/20" />
          <div
            className={cn(
              'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium',
              activeBranch.activeBg,
              activeBranch.activeColor,
            )}
          >
            <span
              className={cn('size-1.5 rounded-full', activeBranch.dotColor)}
            />
            {activeBranch.label}
          </div>
        </div>
      ) : null}
    </div>
  )
}
