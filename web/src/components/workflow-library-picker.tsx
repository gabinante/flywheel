import { useCallback, useEffect, useMemo, useState } from 'react'
import { BookOpen, Search, Trash2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useAPI } from '@/contexts/use-api'
import type { components } from '@/lib/api/v1'

type WorkflowPhase = components['schemas']['WorkflowPhase']

interface LibraryEntry {
  id: string
  name: string
  description?: string
  phases: WorkflowPhase[]
  phase_count: number
  source: 'builtin' | 'library'
}

interface Props {
  orgId: string
  onSelect: (def: { name: string; description: string; phases: WorkflowPhase[] }) => void
}

export function WorkflowLibraryPicker({ orgId, onSelect }: Props) {
  const { client } = useAPI()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [entries, setEntries] = useState<LibraryEntry[]>([])
  const [loading, setLoading] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  const fetchEntries = useCallback(async () => {
    setLoading(true)
    const { data } = await client.GET(
      '/orgs/{orgID}/workflow-library' as never,
      { params: { path: { orgID: orgId } } } as never,
    )
    const payload = data as { entries?: LibraryEntry[] } | undefined
    setEntries(payload?.entries ?? [])
    setLoading(false)
  }, [client, orgId])

  useEffect(() => {
    if (open) {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- Begin an external request and reset its loading state.
      void fetchEntries()
      setSearch('')
      setConfirmDelete(null)
    }
  }, [open, fetchEntries])

  const filtered = useMemo(() => {
    if (!search) return entries
    const q = search.toLowerCase()
    return entries.filter(
      (e) =>
        e.name.toLowerCase().includes(q) ||
        (e.description ?? '').toLowerCase().includes(q),
    )
  }, [entries, search])

  const builtins = useMemo(() => filtered.filter((e) => e.source === 'builtin'), [filtered])
  const library = useMemo(() => filtered.filter((e) => e.source === 'library'), [filtered])

  const handleSelect = (entry: LibraryEntry) => {
    onSelect({
      name: entry.name,
      description: entry.description ?? '',
      phases: entry.phases,
    })
    setOpen(false)
  }

  const handleDelete = async (id: string) => {
    await client.DELETE(
      '/orgs/{orgID}/workflow-library/{id}' as never,
      { params: { path: { orgID: orgId, id } } } as never,
    )
    setConfirmDelete(null)
    void fetchEntries()
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="xs">
          <BookOpen className="size-3.5" />
          Select from library
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="end">
        <div className="border-b border-white/10 p-2">
          <div className="relative">
            <Search className="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search templates..."
              className="h-7 pl-7 text-xs"
              autoFocus
            />
          </div>
        </div>
        <div className="max-h-72 overflow-y-auto p-1">
          {loading ? (
            <p className="px-2 py-3 text-center text-xs text-muted-foreground">Loading...</p>
          ) : filtered.length === 0 ? (
            <p className="px-2 py-3 text-center text-xs text-muted-foreground">No templates found</p>
          ) : (
            <>
              {builtins.length > 0 && (
                <>
                  <p className="px-2 py-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                    Built-in
                  </p>
                  {builtins.map((entry) => (
                    <button
                      key={entry.id}
                      type="button"
                      onClick={() => handleSelect(entry)}
                      className="flex w-full items-start gap-2 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-white/10"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="truncate text-xs font-medium">{entry.name}</span>
                          <Badge variant="outline" className="text-[9px] text-muted-foreground">
                            built-in
                          </Badge>
                        </div>
                        {entry.description && (
                          <p className="truncate text-[10px] text-muted-foreground">
                            {entry.description}
                          </p>
                        )}
                      </div>
                      <Badge variant="outline" className="shrink-0 text-[9px]">
                        {entry.phase_count}
                      </Badge>
                    </button>
                  ))}
                </>
              )}
              {library.length > 0 && (
                <>
                  {builtins.length > 0 && (
                    <div className="my-1 h-px bg-white/10" />
                  )}
                  <p className="px-2 py-1 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                    Organization library
                  </p>
                  {library.map((entry) => (
                    <div
                      key={entry.id}
                      className="group flex items-start gap-2 rounded-lg px-2 py-1.5 transition-colors hover:bg-white/10"
                    >
                      <button
                        type="button"
                        onClick={() => handleSelect(entry)}
                        className="min-w-0 flex-1 text-left"
                      >
                        <span className="truncate text-xs font-medium">{entry.name}</span>
                        {entry.description && (
                          <p className="truncate text-[10px] text-muted-foreground">
                            {entry.description}
                          </p>
                        )}
                      </button>
                      <Badge variant="outline" className="shrink-0 text-[9px]">
                        {entry.phase_count}
                      </Badge>
                      {confirmDelete === entry.id ? (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => void handleDelete(entry.id)}
                          className="shrink-0 text-destructive"
                          title="Confirm delete"
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      ) : (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => setConfirmDelete(entry.id)}
                          className="shrink-0 text-muted-foreground opacity-0 group-hover:opacity-100"
                          title="Delete from library"
                        >
                          <Trash2 className="size-3" />
                        </Button>
                      )}
                    </div>
                  ))}
                </>
              )}
            </>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
