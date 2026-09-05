import type { components } from '@/lib/api/v1'

export type AgentSession = components['schemas']['AgentSession']
export type SessionLink = components['schemas']['SessionLink']

export const HARNESS_LABEL: Record<string, string> = {
  claude_code: 'Claude Code',
  codex: 'Codex',
}

export const HARNESS_CLASS: Record<string, string> = {
  claude_code: 'border-amber-500/30 bg-amber-500/10 text-amber-300',
  codex: 'border-sky-500/30 bg-sky-500/10 text-sky-300',
}

export const STATUS_DOT: Record<string, string> = {
  active: 'bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.8)]',
  idle: 'bg-amber-400',
  ended: 'bg-slate-500',
}

export function relativeTime(iso: string | undefined, now = Date.now()): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 60) return `${s}s ago`
  const m = Math.round(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.round(m / 60)
  if (h < 48) return `${h}h ago`
  const d = Math.round(h / 24)
  if (d < 14) return `${d}d ago`
  return new Date(iso).toLocaleDateString()
}

export function duration(startIso: string | undefined, endIso: string | undefined): string {
  if (!startIso || !endIso) return '—'
  const ms = new Date(endIso).getTime() - new Date(startIso).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export function compact(n: number | undefined): string {
  if (!n) return '0'
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`
  return `${(n / 1_000_000).toFixed(1)}M`
}

/** owner/repo#123 → GitHub PR URL; other refs have no canonical URL yet. */
export function linkHref(link: Pick<SessionLink, 'kind' | 'ref'>): string | null {
  if (link.kind === 'pr') {
    const m = /^([^/#]+)\/([^/#]+)#(\d+)$/.exec(link.ref)
    if (m) return `https://github.com/${m[1]}/${m[2]}/pull/${m[3]}`
  }
  return null
}

export function shortRepo(repo: string | undefined): string {
  if (!repo) return ''
  const i = repo.indexOf('/')
  return i >= 0 ? repo.slice(i + 1) : repo
}
