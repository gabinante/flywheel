import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Building2, LogIn, UserPlus } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { useAuth } from '@/contexts/use-auth'

const INVITE_CODE_KEY = 'flywheel_pending_invite'

export function InviteAcceptPage() {
  const { code } = useParams<{ code: string }>()
  const { token, refreshTokenFromStorage } = useAuth()
  const navigate = useNavigate()
  const [accepting, setAccepting] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [accepted, setAccepted] = useState<{ org: { id: string; name: string; slug?: string } } | null>(null)

  // If we returned from OAuth, check for pending invite code
  useEffect(() => {
    if (token && !code) {
      const pending = sessionStorage.getItem(INVITE_CODE_KEY)
      if (pending) {
        sessionStorage.removeItem(INVITE_CODE_KEY)
        navigate(`/invite/${pending}`, { replace: true })
      }
    }
  }, [token, code, navigate])

  // If user just completed OAuth and landed back here, refresh token
  useEffect(() => {
    refreshTokenFromStorage()
  }, [refreshTokenFromStorage])

  if (!code) {
    return <p className="text-sm text-destructive">Invalid invite link.</p>
  }

  // Not signed in — prompt to sign in, storing the invite code
  if (!token) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Card className="w-full max-w-md border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04]">
              <UserPlus className="size-7 text-emerald-400" />
            </div>
            <CardTitle>You've been invited</CardTitle>
            <CardDescription>
              Sign in with GitHub to accept this invitation and join the organization.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <Button
              onClick={() => {
                sessionStorage.setItem(INVITE_CODE_KEY, code)
                window.location.href = '/auth/github'
              }}
            >
              <LogIn className="size-4" />
              Sign in with GitHub
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Already accepted
  if (accepted) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Card className="w-full max-w-md border-white/10 bg-white/5 backdrop-blur-md">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-14 w-14 items-center justify-center rounded-2xl border border-emerald-500/20 bg-emerald-500/10">
              <Building2 className="size-7 text-emerald-400" />
            </div>
            <CardTitle>Welcome to {accepted.org.name}</CardTitle>
            <CardDescription>
              You've joined the organization. Head to your projects to get started.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center">
            <Button onClick={() => navigate(`/orgs/${accepted.org.slug ?? accepted.org.id}/projects`)}>
              Go to projects
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  async function acceptInvite() {
    setAccepting(true)
    setErr(null)
    const resp = await fetch(`/invites/${code}/accept`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${sessionStorage.getItem('flywheel_jwt') ?? ''}`,
      },
    })
    if (!resp.ok) {
      const data = await resp.json().catch(() => ({}))
      setErr(data.error || 'Failed to accept invite')
      setAccepting(false)
      return
    }
    const result = await resp.json()
    setAccepted(result)
    setAccepting(false)
  }

  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Card className="w-full max-w-md border-white/10 bg-white/5 backdrop-blur-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-2 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/10 bg-white/[0.04]">
            <UserPlus className="size-7 text-emerald-400" />
          </div>
          <CardTitle>Accept invitation</CardTitle>
          <CardDescription>
            Click below to join the organization.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col items-center gap-3">
          <Button onClick={() => void acceptInvite()} disabled={accepting}>
            <UserPlus className="size-4" />
            {accepting ? 'Joining...' : 'Accept and join'}
          </Button>
          {err ? <p className="text-xs text-destructive">{err}</p> : null}
        </CardContent>
      </Card>
    </div>
  )
}
