import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Bot, Settings as SettingsIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { useAPI } from '@/contexts/use-api'
import { patchSettings, type OperatorSettings } from '@/lib/settings-api'
import type { components } from '@/lib/api/v1'

type CodeReviewStatus = components['schemas']['CodeReviewStatus']

/**
 * The auto-review controls that matter on My Reviews: whether the review service is
 * running, whether PRs requesting your review are picked up automatically, and whether
 * the results are posted to GitHub or kept as dry runs.
 */
export function AutoReviewToggle() {
  const { client } = useAPI()
  const [settings, setSettings] = useState<OperatorSettings | null>(null)
  const [status, setStatus] = useState<CodeReviewStatus | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void client.GET('/settings').then(({ data }) => {
      if (!cancelled && data) setSettings(data)
    })
    void client.GET('/code-reviews/status').then(({ data }) => {
      if (!cancelled && data) setStatus(data)
    })
    return () => {
      cancelled = true
    }
  }, [client])

  const set = async (key: 'enabled' | 'watch_requested' | 'publish', value: boolean) => {
    setBusy(key)
    setErr(null)
    const res = await patchSettings(client, (s) => ({ ...s, review: { ...s.review, [key]: value } }))
    setBusy(null)
    if (res.error) {
      setErr(res.error)
      return
    }
    if (res.data) setSettings(res.data)
    void client.GET('/code-reviews/status').then(({ data }) => data && setStatus(data))
  }

  if (!settings) return null
  const r = settings.review
  const autoOn = r.enabled && r.watch_requested

  return (
    <div className="flex flex-col gap-3 rounded-xl border border-white/10 bg-white/[0.03] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        <Bot className={`mt-0.5 size-5 shrink-0 ${autoOn ? 'text-emerald-300' : 'text-muted-foreground'}`} />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">Auto-review PRs that request my review</span>
            <Badge variant="outline" className={`text-[11px] font-normal ${autoOn ? 'border-emerald-400/30 text-emerald-300' : 'text-muted-foreground'}`}>
              {autoOn ? 'on' : r.enabled ? 'watcher off' : 'review service paused'}
            </Badge>
            {status && (
              <span className="text-xs text-muted-foreground">
                {status.queued} queued · {status.active} active · {status.watching} watching
              </span>
            )}
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {r.harness === 'codex' ? 'Codex' : 'Claude Code'} reviews each PR in a detached worktree and{' '}
            {r.publish ? (
              <span className="text-amber-300/90">posts the review to GitHub</span>
            ) : (
              <span>records a dry run in Flywheel only</span>
            )}
            . Picks up {r.watch_scope === 'direct' ? 'only PRs that request you directly' : 'PRs that request you or a team you belong to'}; watched PRs are
            re-reviewed on new commits without reposting earlier findings.{' '}
            <Link to="/settings?section=review" className="inline-flex items-center gap-1 underline">
              <SettingsIcon className="size-3" />
              all review settings
            </Link>
          </p>
          {err && <p className="mt-1 text-xs text-destructive">{err}</p>}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-5">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          Service
          <Switch checked={r.enabled} disabled={busy !== null} onCheckedChange={(v) => set('enabled', v)} />
        </label>
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          Auto-review
          <Switch checked={r.watch_requested} disabled={busy !== null} onCheckedChange={(v) => set('watch_requested', v)} />
        </label>
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          Publish
          <Switch checked={r.publish} disabled={busy !== null} onCheckedChange={(v) => set('publish', v)} />
        </label>
      </div>
    </div>
  )
}
