import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Check, Copy, Settings, Trash2, UserPlus, Users } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ListPageSkeleton } from '@/components/ui/skeleton'
import { WorkflowTimelineEditor } from '@/components/workflow-timeline-editor'
import { useAuth } from '@/contexts/use-auth'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'

interface Member {
  org_id: string
  user_id: string
  role: string
}

interface Invite {
  id: string
  org_id: string
  code: string
  role: string
  created_by: string
  expires_at: string
  max_uses: number
  use_count: number
  revoked: boolean
  created_at: string
}

interface Org {
  id: string
  name: string
  slug: string
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      variant="ghost"
      size="xs"
      onClick={() => {
        navigator.clipboard.writeText(text)
        setCopied(true)
        setTimeout(() => setCopied(false), 2000)
      }}
    >
      {copied ? <Check className="size-3.5 text-emerald-400" /> : <Copy className="size-3.5" />}
      {copied ? 'Copied' : 'Copy'}
    </Button>
  )
}

function MembersCard({ orgId }: { orgId: string }) {
  const { client } = useAuth()
  const [members, setMembers] = useState<Member[] | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      const resp = await fetch(`/orgs/${orgId}/members`, {
        headers: { Authorization: `Bearer ${sessionStorage.getItem('flywheel_jwt') ?? ''}` },
      })
      if (cancelled) return
      if (resp.ok) {
        setMembers(await resp.json())
      }
    })()
    return () => { cancelled = true }
  }, [client, orgId])

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <div className="flex items-center gap-2">
          <Users className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Members</CardTitle>
        </div>
        <CardDescription>Current members of this organization.</CardDescription>
      </CardHeader>
      <CardContent>
        {members === null ? (
          <div className="h-8 w-48 animate-pulse rounded bg-white/[0.06]" />
        ) : members.length === 0 ? (
          <p className="text-xs text-muted-foreground">No members yet.</p>
        ) : (
          <div className="space-y-2">
            {members.map((m) => (
              <div
                key={m.user_id}
                className="flex items-center justify-between rounded-lg border border-white/10 bg-black/10 px-3 py-2"
              >
                <span className="truncate font-mono text-xs text-muted-foreground">
                  {m.user_id}
                </span>
                <Badge variant="outline" className="text-xs">
                  {m.role}
                </Badge>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function InvitesCard({ orgId }: { orgId: string }) {
  const [invites, setInvites] = useState<Invite[] | null>(null)
  const [creating, setCreating] = useState(false)
  const [role, setRole] = useState<'member' | 'admin'>('member')
  const [maxUses, setMaxUses] = useState(10)
  const [err, setErr] = useState<string | null>(null)

  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${sessionStorage.getItem('flywheel_jwt') ?? ''}`,
  }

  async function loadInvites() {
    const resp = await fetch(`/orgs/${orgId}/invites`, { headers })
    if (resp.ok) {
      setInvites(await resp.json())
    }
  }

  useEffect(() => {
    loadInvites()
  }, [orgId])

  async function createInvite() {
    setCreating(true)
    setErr(null)
    const resp = await fetch(`/orgs/${orgId}/invites`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ role, max_uses: maxUses, expire_in: '168h' }),
    })
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}))
      setErr(data.error || 'Failed to create invite')
      setCreating(false)
      return
    }
    setCreating(false)
    loadInvites()
  }

  async function revokeInvite(inviteId: string) {
    await fetch(`/orgs/${orgId}/invites/${inviteId}`, {
      method: 'DELETE',
      headers,
    })
    loadInvites()
  }

  function inviteLink(code: string) {
    return `${window.location.origin}${window.location.pathname}#/invite/${code}`
  }

  return (
    <Card className="border-white/10 bg-white/5 backdrop-blur-md">
      <CardHeader>
        <div className="flex items-center gap-2">
          <UserPlus className="size-4 text-muted-foreground" />
          <CardTitle className="text-sm">Invite links</CardTitle>
        </div>
        <CardDescription>
          Generate shareable links to invite people to this organization.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Role</span>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value as 'member' | 'admin')}
              className="block rounded-md border border-white/10 bg-black/20 px-3 py-1.5 text-sm text-foreground"
            >
              <option value="member">Member</option>
              <option value="admin">Admin</option>
            </select>
          </label>
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Max uses</span>
            <Input
              type="number"
              min={1}
              max={100}
              value={maxUses}
              onChange={(e) => setMaxUses(parseInt(e.target.value) || 1)}
              className="w-20"
            />
          </label>
          <Button size="sm" onClick={() => void createInvite()} disabled={creating}>
            <UserPlus className="size-3.5" />
            {creating ? 'Creating...' : 'Generate link'}
          </Button>
        </div>
        {err ? <p className="text-xs text-destructive">{err}</p> : null}

        {invites === null ? (
          <div className="h-8 w-48 animate-pulse rounded bg-white/[0.06]" />
        ) : invites.length === 0 ? (
          <p className="text-xs text-muted-foreground">No active invites.</p>
        ) : (
          <div className="space-y-2">
            {invites.map((inv) => {
              const expired = new Date(inv.expires_at) < new Date()
              const exhausted = inv.use_count >= inv.max_uses
              return (
                <div
                  key={inv.id}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-white/10 bg-black/10 px-3 py-2"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <code className="text-xs text-muted-foreground">{inv.code.slice(0, 12)}...</code>
                    <Badge variant="outline" className="text-xs">{inv.role}</Badge>
                    <span className="text-xs text-muted-foreground">
                      {inv.use_count}/{inv.max_uses} used
                    </span>
                    {expired ? (
                      <Badge variant="muted" className="text-xs">Expired</Badge>
                    ) : exhausted ? (
                      <Badge variant="secondary" className="text-xs">Exhausted</Badge>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-1">
                    {!expired && !exhausted ? (
                      <CopyButton text={inviteLink(inv.code)} />
                    ) : null}
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => void revokeInvite(inv.id)}
                    >
                      <Trash2 className="size-3.5 text-destructive" />
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function OrgSettingsPage() {
  const { orgId, orgParam } = useResolvedRouteParams()
  const { client } = useAuth()
  const [org, setOrg] = useState<Org | null | undefined>(undefined)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    ;(async () => {
      const resp = await fetch(`/orgs/${orgId}`, {
        headers: { Authorization: `Bearer ${sessionStorage.getItem('flywheel_jwt') ?? ''}` },
      })
      if (cancelled) return
      if (!resp.ok) {
        setErr('Failed to load organization')
        setOrg(null)
        return
      }
      setOrg(await resp.json())
    })()
    return () => { cancelled = true }
  }, [client, orgId])

  if (!orgId) return <p className="text-sm text-destructive">Missing org ID.</p>
  if (err) return <p className="text-sm text-destructive">{err}</p>
  if (org === undefined) return <ListPageSkeleton />
  if (!org) return <p className="text-sm text-muted-foreground">Organization not found.</p>

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1.5">
        <p className="text-xs text-muted-foreground">
          <Link to="/orgs" className="hover:underline">Orgs</Link>
          <span className="px-1">/</span>
          <Link to={`/orgs/${orgParam}/projects`} className="hover:underline">{org.name}</Link>
          <span className="px-1">/</span>
          <span>Settings</span>
        </p>
        <div className="flex items-center gap-3">
          <Settings className="size-5 text-muted-foreground" />
          <h1 className="text-2xl font-semibold tracking-tight">
            Organization settings
          </h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Manage members, invite links, and default workflow for {org.name}.
        </p>
      </div>
      <MembersCard orgId={orgId} />
      <InvitesCard orgId={orgId} />
      <WorkflowTimelineEditor orgId={orgId} scope="org" />
    </div>
  )
}
