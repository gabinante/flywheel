import { useState } from 'react'
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, Plus, SlidersHorizontal, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { UNSECTIONED, type ProjectSection } from '@/lib/project-layout'

/** Header controls for one section while customizing. */
export function SectionHeader({
  section,
  index,
  total,
  customizing,
  count,
  onChange,
  onMove,
  onDelete,
}: {
  section: ProjectSection
  index: number
  total: number
  customizing: boolean
  count: number
  onChange: (patch: Partial<ProjectSection>) => void
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
}) {
  const implicit = section.id === UNSECTIONED
  if (!section.name && !customizing) return null
  const Chevron = section.collapsed ? ChevronRight : ChevronDown
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-widest text-muted-foreground hover:text-foreground"
        onClick={() => onChange({ collapsed: !section.collapsed })}
        aria-expanded={!section.collapsed}
      >
        <Chevron className="size-3.5" />
        {customizing && !implicit ? null : <span>{section.name || 'Projects'}</span>}
        <span className="font-normal normal-case tracking-normal">{count}</span>
      </button>
      {customizing && !implicit && (
        <>
          <Input
            value={section.name}
            onChange={(e) => onChange({ name: e.target.value })}
            placeholder="Section name"
            className="h-7 max-w-xs text-xs"
          />
          <Button variant="ghost" size="sm" className="h-7 px-1.5" disabled={index === 0} onClick={() => onMove(-1)} title="Move section up">
            <ArrowUp className="size-3.5" />
          </Button>
          <Button variant="ghost" size="sm" className="h-7 px-1.5" disabled={index >= total - 1} onClick={() => onMove(1)} title="Move section down">
            <ArrowDown className="size-3.5" />
          </Button>
          <Button variant="ghost" size="sm" className="h-7 px-1.5 text-destructive" onClick={onDelete} title="Remove section (projects stay)">
            <Trash2 className="size-3.5" />
          </Button>
        </>
      )}
    </div>
  )
}

/** Per-project controls while customizing: reorder within the section, move to another section. */
export function ItemControls({
  sections,
  currentSectionId,
  index,
  total,
  onMoveWithin,
  onMoveTo,
}: {
  sections: ProjectSection[]
  currentSectionId: string
  index: number
  total: number
  onMoveWithin: (dir: -1 | 1) => void
  onMoveTo: (sectionId: string) => void
}) {
  return (
    <div className="flex items-center gap-1 rounded-lg border border-white/10 bg-black/20 px-1 py-0.5">
      <Button variant="ghost" size="sm" className="h-6 px-1" disabled={index === 0} onClick={() => onMoveWithin(-1)} title="Move up">
        <ArrowUp className="size-3" />
      </Button>
      <Button variant="ghost" size="sm" className="h-6 px-1" disabled={index >= total - 1} onClick={() => onMoveWithin(1)} title="Move down">
        <ArrowDown className="size-3" />
      </Button>
      <select
        className="h-6 rounded-md border border-white/10 bg-transparent px-1 text-[11px] text-muted-foreground"
        value={currentSectionId}
        onChange={(e) => onMoveTo(e.target.value)}
        aria-label="Move to section"
      >
        {sections.map((s) => (
          <option key={s.id} value={s.id} className="bg-neutral-900">
            {s.name || 'Unnamed section'}
          </option>
        ))}
        <option value={UNSECTIONED} className="bg-neutral-900">
          Other projects
        </option>
      </select>
    </div>
  )
}

/** Toolbar: customize toggle + add section. */
export function LayoutToolbar({ customizing, onToggle, onAdd }: { customizing: boolean; onToggle: () => void; onAdd: (name: string) => void }) {
  const [name, setName] = useState('')
  return (
    <div className="flex items-center gap-2">
      {customizing && (
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            if (!name.trim()) return
            onAdd(name.trim())
            setName('')
          }}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="New section…" className="h-8 w-40 text-xs" />
          <Button type="submit" variant="outline" size="sm" className="h-8" disabled={!name.trim()}>
            <Plus className="mr-1 size-3.5" />
            Add
          </Button>
        </form>
      )}
      <Button variant={customizing ? 'default' : 'outline'} size="sm" className={cn('h-8')} onClick={onToggle}>
        <SlidersHorizontal className="mr-1.5 size-3.5" />
        {customizing ? 'Done' : 'Customize'}
      </Button>
    </div>
  )
}

