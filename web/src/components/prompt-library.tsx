import { useEffect, useState } from 'react'
import { RotateCcw, Save, ScrollText } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { useAPI } from '@/contexts/use-api'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'

type PromptDefinition = components['schemas']['PromptDefinition']

/** One editable built-in prompt. */
function PromptEditor({ p, onSaved }: { p: PromptDefinition; onSaved: (next: PromptDefinition) => void }) {
  const { client } = useAPI()
  const [text, setText] = useState(p.text)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)
  const dirty = text !== p.text

  const put = async (value: string) => {
    setBusy(true)
    setErr(null)
    const { data, error, response } = await client.PUT('/prompts/{promptID}', { params: { path: { promptID: p.id } }, body: { text: value } })
    setBusy(false)
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setText(data.text)
    setSavedAt(Date.now())
    onSaved(data)
  }

  return (
    <div className="space-y-2 rounded-xl border border-white/5 bg-white/[0.02] p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{p.name}</span>
        <Badge variant="outline" className="text-[10px] font-normal text-muted-foreground">
          {p.used_by}
        </Badge>
        {p.customized ? (
          <Badge variant="outline" className="border-amber-400/30 text-[10px] font-normal text-amber-300">
            customized
          </Badge>
        ) : (
          <Badge variant="outline" className="text-[10px] font-normal text-muted-foreground">
            default
          </Badge>
        )}
        <span className="ml-auto flex items-center gap-2">
          {err && <span className="text-xs text-destructive">{err}</span>}
          {savedAt && !dirty && !err && <span className="text-xs text-emerald-300/80">Saved</span>}
          {p.customized && (
            <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs" disabled={busy} onClick={() => put('')} title="Restore the built-in default">
              <RotateCcw className="mr-1 size-3" />
              Reset to default
            </Button>
          )}
          <Button type="button" size="sm" className="h-7 px-2 text-xs" disabled={busy || !dirty} onClick={() => put(text)}>
            <Save className="mr-1 size-3" />
            {busy ? 'Saving…' : 'Save'}
          </Button>
        </span>
      </div>
      <p className="text-xs text-muted-foreground">{p.description}</p>
      <Textarea aria-label={`${p.name} prompt`} value={text} onChange={(e) => setText(e.target.value)} rows={Math.min(18, Math.max(4, text.split('\n').length + 1))} className="font-mono text-xs leading-relaxed" />
      {p.customized && text !== p.default_text && (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer hover:text-foreground">Show the built-in default</summary>
          <pre className="mt-2 whitespace-pre-wrap rounded-lg bg-black/20 p-3 font-mono text-[11px] leading-relaxed">{p.default_text}</pre>
        </details>
      )}
    </div>
  )
}

/** Settings → Prompts: the base prompt of every default worker, editable live. */
export function PromptLibrary() {
  const { client } = useAPI()
  const [items, setItems] = useState<PromptDefinition[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void client.GET('/prompts').then(({ data, error, response }) => {
      if (cancelled) return
      if (!response.ok || !data) {
        setErr(formatApiError(error))
        return
      }
      setItems(data.items)
    })
    return () => {
      cancelled = true
    }
  }, [client])

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ScrollText className="size-4 text-muted-foreground" />
          Prompts
        </CardTitle>
        <CardDescription>
          The base prompt of every default worker. Edits apply to the next run, no restart. Structural parts (project context pack,
          ticket details, tool protocol, content-defense rules) are appended by Flywheel and are not editable here. Per-worker base
          prompts under Workers &amp; roles are prepended on top of these.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {err && <p className="text-sm text-destructive">{err}</p>}
        {!items && !err && <p className="text-sm text-muted-foreground">Loading…</p>}
        {items?.map((p) => (
          <PromptEditor key={p.id} p={p} onSaved={(next) => setItems((prev) => (prev ?? []).map((x) => (x.id === next.id ? next : x)))} />
        ))}
      </CardContent>
    </Card>
  )
}
