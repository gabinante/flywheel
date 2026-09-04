import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ArrowLeft, Library, Plus, Trash2, Workflow as WorkflowIcon } from 'lucide-react'

import { WorkflowTimelineEditor } from '@/components/workflow-timeline-editor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/use-auth'
import { resolvePreferredOrgId } from '@/lib/org-preferences'
import type { components } from '@/lib/api/v1'

type WorkflowPhase = components['schemas']['WorkflowPhase']

type LibraryEntry = {
  id: string
  name: string
  description?: string
  phases: WorkflowPhase[]
  phase_count: number
  source: 'builtin' | 'library'
}

function useOrgId() {
  const { client } = useAuth()
  const [orgId, setOrgId] = useState<string | null>(null)
  useEffect(() => {
    let cancelled = false
    void client.GET('/orgs', {}).then(({ data }) => {
      if (cancelled || !data || data.length === 0) return
      setOrgId(resolvePreferredOrgId(data) ?? data[0].id ?? null)
    })
    return () => {
      cancelled = true
    }
  }, [client])
  return orgId
}

/** The workflow library: built-in templates, saved library workflows, and the org default. */
export function WorkflowsPage() {
  const { client } = useAuth()
  const orgId = useOrgId()
  const [entries, setEntries] = useState<LibraryEntry[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    void client
      .GET('/orgs/{orgID}/workflow-library' as never, { params: { path: { orgID: orgId } } } as never)
      .then(({ data, response }) => {
        if (cancelled) return
        if (!response.ok || !data) {
          setErr('Could not load the workflow library.')
          return
        }
        setEntries(((data as { entries?: LibraryEntry[] }).entries ?? []) as LibraryEntry[])
      })
    return () => {
      cancelled = true
    }
  }, [client, orgId])

  const remove = async (e: LibraryEntry) => {
    if (!orgId || e.source !== 'library') return
    if (!window.confirm(`Delete “${e.name}” from the library? Projects using a copy keep theirs.`)) return
    const { response } = await client.DELETE('/orgs/{orgID}/workflow-library/{id}' as never, { params: { path: { orgID: orgId, id: e.id } } } as never)
    if (response.ok) setEntries((prev) => (prev ?? []).filter((x) => x.id !== e.id))
  }

  const builtins = (entries ?? []).filter((e) => e.source === 'builtin')
  const saved = (entries ?? []).filter((e) => e.source === 'library')

  return (
    <div className="mx-auto w-full max-w-[1500px] space-y-6 p-6 xl:px-10">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold">
            <Library className="size-5 text-muted-foreground" />
            Workflows
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            The full library. Saved workflows are yours to edit; built-in templates open in the editor and save as a new library entry. The
            org default is what projects use unless they set their own under Project settings → Workflow stages.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link to="/workflows/org">Edit org default</Link>
        </Button>
      </div>

      {err && <p className="text-sm text-destructive">{err}</p>}
      {!entries && !err && <p className="text-sm text-muted-foreground">Loading…</p>}

      {entries && (
        <>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Saved · {saved.length}</h2>
            {saved.length === 0 && (
              <p className="text-sm text-muted-foreground">Nothing saved yet — open a template below and save it, or save one from a project’s Workflow stages.</p>
            )}
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {saved.map((e) => (
                <EntryCard key={e.id} e={e} href={`/workflows/${e.id}`} onDelete={() => remove(e)} />
              ))}
            </div>
          </section>
          <section className="space-y-2">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Built-in templates · {builtins.length}</h2>
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {builtins.map((e) => (
                <EntryCard key={e.id} e={e} href={`/workflows/new?template=${encodeURIComponent(e.id)}`} />
              ))}
            </div>
          </section>
        </>
      )}
    </div>
  )
}

function EntryCard({ e, href, onDelete }: { e: LibraryEntry; href: string; onDelete?: () => void }) {
  return (
    <div className="group relative rounded-xl border border-white/10 bg-white/[0.03] p-4 transition-colors hover:bg-white/[0.06]">
      <Link to={href} className="block">
        <div className="flex items-center gap-2">
          <WorkflowIcon className="size-4 text-muted-foreground" />
          <span className="text-sm font-medium">{e.name}</span>
          <Badge variant="outline" className="text-[10px] font-normal text-muted-foreground">
            {e.source === 'builtin' ? 'template' : 'library'}
          </Badge>
        </div>
        {e.description && <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{e.description}</p>}
        <p className="mt-2 text-xs text-muted-foreground">
          {e.phase_count} phase{e.phase_count === 1 ? '' : 's'}: {e.phases.map((p) => p.name).join(' → ')}
        </p>
      </Link>
      {onDelete && (
        <button
          type="button"
          onClick={onDelete}
          className="absolute right-2 top-2 rounded-md p-1 text-muted-foreground/50 opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
          title="Delete from library"
        >
          <Trash2 className="size-3.5" />
        </button>
      )}
    </div>
  )
}

/** Edit one workflow: a library entry by id, the org default ("org"), or a new one from a template ("new?template=…"). */
export function WorkflowEditorPage() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { client } = useAuth()
  const orgId = useOrgId()
  const templateId = params.get('template')
  const [template, setTemplate] = useState<{ name: string; description?: string; phases: WorkflowPhase[] } | null>(null)

  useEffect(() => {
    if (id !== 'new' || !templateId || !orgId) return
    let cancelled = false
    void client
      .GET('/orgs/{orgID}/workflow-library' as never, { params: { path: { orgID: orgId } } } as never)
      .then(({ data }) => {
        if (cancelled) return
        const entry = ((data as unknown as { entries?: LibraryEntry[] } | undefined)?.entries ?? []).find((e) => e.id === templateId)
        if (entry) setTemplate({ name: entry.name, description: entry.description, phases: entry.phases })
      })
    return () => {
      cancelled = true
    }
  }, [client, id, templateId, orgId])

  if (!orgId) return <p className="p-6 text-sm text-muted-foreground">Loading…</p>
  if (id === 'new' && !template) return <p className="p-6 text-sm text-muted-foreground">Loading template…</p>

  return (
    <div className="mx-auto w-full max-w-[1500px] space-y-4 p-6 xl:px-10">
      <Button asChild variant="ghost" size="sm">
        <Link to="/workflows">
          <ArrowLeft className="mr-1.5 size-3.5" />
          Workflow library
        </Link>
      </Button>
      {id === 'org' ? (
        <WorkflowTimelineEditor orgId={orgId} scope="org" />
      ) : id === 'new' && template ? (
        <WorkflowTimelineEditor orgId={orgId} scope="org" template={template} onCreated={(newId) => navigate(`/workflows/${newId}`, { replace: true })} />
      ) : (
        <WorkflowTimelineEditor orgId={orgId} scope="org" definitionId={id} />
      )}
      {id === 'new' && (
        <p className="text-xs text-muted-foreground">
          <Plus className="mr-1 inline size-3" />
          Saving creates a new library entry from this template.
        </p>
      )}
    </div>
  )
}
