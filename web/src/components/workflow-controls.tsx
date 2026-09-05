import { useActivityVersion } from '@/contexts/use-activity'
import { useCallback, useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

type Phase = { id: string; name: string; type: string; config?: { auto_advance?: boolean; conditions?: { type: string }[] } }
type Position = { callback_url?: string; status: string; entered_at: string; position?: { current_phase?: Phase } }

export function WorkflowControls({ ticketId, onChanged }: { ticketId: string; onChanged: () => void }) {
  const activityVersion = useActivityVersion('tickets', 'workflows')
  const [data, setData] = useState<Position | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const res = await fetch(`/tickets/${ticketId}/workflow`, { signal })
      if (!res.ok) throw new Error('Could not load the current workflow phase')
      setData(await res.json())
    } catch (e) { if (!signal?.aborted) setError(e instanceof Error ? e.message : String(e)) }
  }, [ticketId])
  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    return () => { controller.abort() }
  }, [load, activityVersion])
  const phase = data?.position?.current_phase
  if (!phase || !data) return error ? <p role="alert">{error}</p> : null
  const human = phase.type === 'manual' || phase.config?.conditions?.some((c) => c.type === 'human_approval')
  const action = data.status === 'failed' ? 'retry' : human ? 'approve' : phase.type === 'agent' && phase.config?.auto_advance === false && data.status === 'blocked' ? 'continue' : null
  const decide = async () => {
    setBusy(true); setError('')
    try {
      const res = await fetch(`/tickets/${ticketId}/workflow/decision`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, phase_id: phase.id, entered_at: data.entered_at }),
      })
      if (!res.ok) { const body = await res.json().catch(() => null); throw new Error(typeof body?.error === 'string' ? body.error : body?.message ?? body?.error?.message ?? 'Could not save the workflow decision.') }
      await load(); onChanged()
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) } finally { setBusy(false) }
  }
  return <section aria-label="Workflow controls" className="rounded-xl border border-white/10 bg-card/60 p-4 space-y-3">
    <div className="flex items-center gap-2"><span className="text-sm font-medium">{phase.name}</span><Badge variant="outline">{data.status}</Badge></div>
    {data.status === 'failed' && <p className="text-sm text-muted-foreground">This phase failed. Review the trace and fix the cause before retrying.</p>}
    {data.callback_url && <label className="block text-xs text-muted-foreground">Webhook callback URL<input aria-label="Webhook callback URL" readOnly value={data.callback_url} className="mt-1 block w-full rounded border border-white/10 bg-transparent p-2 font-mono" /></label>}
    {action && <Button disabled={busy} onClick={() => void decide()}>{busy ? 'Saving…' : action === 'retry' ? 'Retry phase' : action === 'approve' ? 'Approve phase' : 'Continue workflow'}</Button>}
    {error && <p role="alert" className="text-sm text-red-300">{error}</p>}
  </section>
}
