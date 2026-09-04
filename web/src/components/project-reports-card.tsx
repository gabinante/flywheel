import { useEffect, useState } from 'react'
import { ExternalLink, FileText, RefreshCw, Send } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { StyledSelect } from '@/components/ui/styled-select'
import { Textarea } from '@/components/ui/textarea'
import { useAuth } from '@/contexts/use-auth'
import { formatApiError } from '@/lib/api/client'
import type { components } from '@/lib/api/v1'
import { relativeTime } from '@/lib/sessions-format'

type Report = components['schemas']['Report']

const HEALTH_OPTIONS = [
  { value: 'onTrack', label: 'On track' },
  { value: 'atRisk', label: 'At risk' },
  { value: 'offTrack', label: 'Off track' },
]

function ReportEditor({
  title,
  description,
  preview,
  onPreview,
  onPost,
  showHealth,
}: {
  title: string
  description: string
  preview: Report | null
  onPreview: () => Promise<void>
  onPost: (body: string, health: string) => Promise<string | null>
  showHealth: boolean
}) {
  // The parent remounts this editor (via key) whenever a new preview arrives, so the
  // initial state below is the preview itself.
  const [body, setBody] = useState(preview?.body ?? '')
  const [health, setHealth] = useState(preview?.health ?? 'onTrack')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)

  const post = async () => {
    setBusy(true)
    const err = await onPost(body, health)
    setBusy(false)
    setMsg(err ? `Failed: ${err}` : 'Posted to Linear.')
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <FileText className="size-4 text-muted-foreground" /> {title}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" variant="ghost" onClick={() => void onPreview()} disabled={busy}>
            <RefreshCw className="size-4" /> Preview
          </Button>
          {showHealth && <StyledSelect value={health} onValueChange={setHealth} options={HEALTH_OPTIONS} aria-label="Health" className="min-w-[9rem]" />}
          <Button size="sm" onClick={post} disabled={busy || !body}>
            <Send className="size-4" /> Post to Linear
          </Button>
          {preview && (
            <span className="text-xs text-muted-foreground">
              window {new Date(preview.window_start).toLocaleDateString()} → {new Date(preview.window_end).toLocaleDateString()}
            </span>
          )}
          {msg && <span className="text-xs text-muted-foreground">{msg}</span>}
        </div>
        <Textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder="Click Preview to render from Flywheel data (tickets, merged PRs, reviews, sessions)." className="min-h-[14rem] font-mono text-xs" />
      </CardContent>
    </Card>
  )
}

export function ProjectReportsCard({ projectId }: { projectId: string }) {
  const { client } = useAuth()
  const [update, setUpdate] = useState<Report | null>(null)
  const [weekly, setWeekly] = useState<Report | null>(null)
  const [history, setHistory] = useState<Report[]>([])
  const [err, setErr] = useState<string | null>(null)

  const loadHistory = async () => {
    const { data } = await client.GET('/reports', { params: { query: { limit: 20 } } })
    if (data) setHistory(data.reports)
  }

  useEffect(() => {
    let cancelled = false
    void client.GET('/reports', { params: { query: { limit: 20 } } }).then(({ data }) => {
      if (!cancelled && data) setHistory(data.reports)
    })
    return () => {
      cancelled = true
    }
  }, [client])

  const previewUpdate = async () => {
    const { data, error, response } = await client.GET('/projects/{projectID}/reports/status-update/preview', { params: { path: { projectID: projectId } } })
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setUpdate(data)
  }
  const postUpdate = async (body: string, health: string) => {
    const { error, response } = await client.POST('/projects/{projectID}/reports/status-update', {
      params: { path: { projectID: projectId } },
      body: { body, health: health as 'onTrack' | 'atRisk' | 'offTrack' },
    })
    await loadHistory()
    return response.ok ? null : formatApiError(error)
  }
  const previewWeekly = async () => {
    const { data, error, response } = await client.GET('/reports/weekly/preview')
    if (!response.ok || !data) {
      setErr(formatApiError(error))
      return
    }
    setErr(null)
    setWeekly(data)
  }
  const postWeekly = async (body: string) => {
    const { error, response } = await client.POST('/reports/weekly', { body: { body } })
    await loadHistory()
    return response.ok ? null : formatApiError(error)
  }

  return (
    <div className="flex flex-col gap-4">
      {err && <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-200">{err}</div>}
      <ReportEditor
        key={`update-${update?.window_end ?? 'none'}`}
        title="Project status update"
        description="Delta since the last posted update: done, merged, in review, in progress, blocked, review feedback awaiting a response, agent activity. Posts as a Linear project update."
        preview={update}
        onPreview={previewUpdate}
        onPost={postUpdate}
        showHealth
      />
      <ReportEditor
        key={`weekly-${weekly?.window_end ?? 'none'}`}
        title="Weekly roundup"
        description="This week's merged PRs, per-project ticket table, and agent activity across all linked projects. Prepended to the rolling Linear roundup document."
        preview={weekly}
        onPreview={previewWeekly}
        onPost={(body) => postWeekly(body)}
        showHealth={false}
      />
      {history.length > 0 && (
        <Card className="border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="py-3">
            <CardTitle className="text-sm">Posted reports</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-1.5 pt-0 text-xs">
            {history.map((r) => (
              <div key={r.id} className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-1.5">
                <span className="rounded border border-white/10 px-1 text-[10px] uppercase text-muted-foreground">{r.kind.replace('_', ' ')}</span>
                <span>
                  {new Date(r.window_start).toLocaleDateString()} → {new Date(r.window_end).toLocaleDateString()}
                </span>
                {r.health && <span className="text-muted-foreground">{r.health}</span>}
                <span className="ml-auto text-muted-foreground">{relativeTime(r.created_at)}</span>
                {r.url && (
                  <a href={r.url} target="_blank" rel="noreferrer" className="text-muted-foreground hover:underline">
                    <ExternalLink className="size-3" />
                  </a>
                )}
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
