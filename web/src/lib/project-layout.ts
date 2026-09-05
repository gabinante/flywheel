import { useEffect, useMemo, useRef, useState } from 'react'

import { useAPI } from '@/contexts/use-api'
import type { components } from '@/lib/api/v1'

export type Layout = components['schemas']['Layout']
export type ProjectSection = components['schemas']['ProjectSection']

export const UNSECTIONED = '__unsectioned__'

type Item = { id: string }

/** Arrange items into the saved sections; anything not placed lands in a trailing implicit section. */
export function arrange<T extends Item>(items: T[], sections: ProjectSection[]): Array<{ section: ProjectSection; items: T[] }> {
  const byId = new Map(items.map((i) => [i.id, i]))
  const placed = new Set<string>()
  const out: Array<{ section: ProjectSection; items: T[] }> = []
  for (const s of sections) {
    const list: T[] = []
    for (const id of s.project_ids) {
      const it = byId.get(id)
      if (it && !placed.has(id)) {
        list.push(it)
        placed.add(id)
      }
    }
    out.push({ section: s, items: list })
  }
  const rest = items.filter((i) => !placed.has(i.id))
  if (rest.length > 0 || sections.length === 0) {
    out.push({ section: { id: UNSECTIONED, name: sections.length === 0 ? '' : 'Other projects', project_ids: rest.map((r) => r.id), collapsed: false }, items: rest })
  }
  return out
}

/** Loads and persists the operator's project-page layout. */
export function useProjectLayout() {
  const { client } = useAPI()
  const [layout, setLayout] = useState<Layout | null>(null)
  const timer = useRef<number | null>(null)

  useEffect(() => {
    let cancelled = false
    void client.GET('/me/layout').then(({ data, response }) => {
      if (!cancelled) setLayout(response.ok && data ? data : { project_sections: [] })
    })
    return () => {
      cancelled = true
    }
  }, [client])

  const update = (next: Layout) => {
    setLayout(next)
    if (timer.current) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      void client.PUT('/me/layout', { body: next })
    }, 400)
  }

  return { layout, update }
}
function newId() {
  return `sec_${Math.random().toString(36).slice(2, 8)}`
}

/** Pure layout edits. */
export const layoutOps = {
  addSection(l: Layout, name: string): Layout {
    return { project_sections: [...l.project_sections, { id: newId(), name, project_ids: [], collapsed: false }] }
  },
  patchSection(l: Layout, id: string, patch: Partial<ProjectSection>): Layout {
    return { project_sections: l.project_sections.map((s) => (s.id === id ? { ...s, ...patch } : s)) }
  },
  deleteSection(l: Layout, id: string): Layout {
    return { project_sections: l.project_sections.filter((s) => s.id !== id) }
  },
  moveSection(l: Layout, id: string, dir: -1 | 1): Layout {
    const arr = [...l.project_sections]
    const i = arr.findIndex((s) => s.id === id)
    const j = i + dir
    if (i < 0 || j < 0 || j >= arr.length) return l
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
    return { project_sections: arr }
  },
  /** Move a project into a section (or out of all sections) at the end. */
  moveTo(l: Layout, projectId: string, sectionId: string, orderedUnsectioned: string[]): Layout {
    const sections = l.project_sections.map((s) => ({ ...s, project_ids: s.project_ids.filter((p) => p !== projectId) }))
    if (sectionId !== UNSECTIONED) {
      const s = sections.find((x) => x.id === sectionId)
      if (s) s.project_ids = [...s.project_ids, projectId]
    }
    void orderedUnsectioned
    return { project_sections: sections }
  },
  /** Reorder within a section. For the implicit section, materialize it as a real section so the order can be saved. */
  moveWithin(l: Layout, sectionId: string, ids: string[], index: number, dir: -1 | 1): Layout {
    const j = index + dir
    if (j < 0 || j >= ids.length) return l
    const next = [...ids]
    ;[next[index], next[j]] = [next[j], next[index]]
    if (sectionId === UNSECTIONED) {
      return { project_sections: [...l.project_sections, { id: newId(), name: 'Other projects', project_ids: next, collapsed: false }] }
    }
    return { project_sections: l.project_sections.map((s) => (s.id === sectionId ? { ...s, project_ids: next } : s)) }
  },
}

/** Keeps saved sections in sync when projects disappear (merged/deleted): drop unknown ids on read. */
export function pruneLayout(l: Layout, knownIds: Set<string>): Layout {
  return { project_sections: l.project_sections.map((s) => ({ ...s, project_ids: s.project_ids.filter((id) => knownIds.has(id)) })) }
}

export function useArranged<T extends Item>(items: T[], layout: Layout | null) {
  return useMemo(() => arrange(items, layout?.project_sections ?? []), [items, layout])
}
