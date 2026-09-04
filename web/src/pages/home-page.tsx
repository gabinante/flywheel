import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'

import { useAuth } from '@/contexts/use-auth'
import { resolvePreferredOrgId, resolvePreferredOrgSlug, setPreferredOrgId } from '@/lib/org-preferences'

/* ------------------------------------------------------------------ */
/* SVG icon helpers (inline to avoid extra deps)                      */
/* ------------------------------------------------------------------ */


function FeatureIcon({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-400">
      {children}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Feature data                                                       */
/* ------------------------------------------------------------------ */

const features = [
  {
    title: 'Ticket-driven orchestration',
    description:
      'Every unit of work is a ticket with dependencies, success criteria, and a lease. Agents claim, execute, and submit — no thrashing.',
    icon: (
      <svg className="size-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 0 0 2.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 0 0-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 0 0 .75-.75 2.25 2.25 0 0 0-.1-.664m-5.8 0A2.251 2.251 0 0 1 13.5 2.25H15a2.25 2.25 0 0 1 2.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25ZM6.75 12h.008v.008H6.75V12Zm0 3h.008v.008H6.75V15Zm0 3h.008v.008H6.75V18Z" />
      </svg>
    ),
  },
  {
    title: 'Shared context, zero drift',
    description:
      'Agents and humans see the same plan, the same state machine, the same review queue. No context lost between handoffs.',
    icon: (
      <svg className="size-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 21 3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5" />
      </svg>
    ),
  },
  {
    title: 'Policy gates & risk model',
    description:
      'Define what auto-advances and what needs a human. Blast-radius analysis, approval gates, and audit trails baked in.',
    icon: (
      <svg className="size-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75m-3-7.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285Z" />
      </svg>
    ),
  },
  {
    title: 'Integrate, don\'t replace',
    description:
      'Plugs into your GitHub, CI/CD, observability, and secrets stack. Flywheel is the orchestration layer, not another silo.',
    icon: (
      <svg className="size-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 0 1 1.242 7.244l-4.5 4.5a4.5 4.5 0 0 1-6.364-6.364l1.757-1.757m13.35-.622 1.757-1.757a4.5 4.5 0 0 0-6.364-6.364l-4.5 4.5a4.5 4.5 0 0 0 1.242 7.244" />
      </svg>
    ),
  },
]

/* ------------------------------------------------------------------ */
/* Animated background – floating orbs                                */
/* ------------------------------------------------------------------ */

function AnimatedBackground() {
  return (
    <div className="pointer-events-none fixed inset-0 overflow-hidden" aria-hidden="true">
      {/* Gradient base */}
      <div className="absolute inset-0 bg-gradient-to-b from-emerald-950/30 via-background to-background" />

      {/* Floating orbs */}
      <div className="landing-orb landing-orb-1 absolute -left-32 -top-32 size-96 rounded-full bg-emerald-500/8 blur-3xl" />
      <div className="landing-orb landing-orb-2 absolute -right-24 top-1/4 size-80 rounded-full bg-emerald-600/6 blur-3xl" />
      <div className="landing-orb landing-orb-3 absolute -bottom-40 left-1/3 size-[28rem] rounded-full bg-emerald-400/5 blur-3xl" />

      {/* Grid overlay */}
      <div
        className="absolute inset-0 opacity-[0.03]"
        style={{
          backgroundImage:
            'linear-gradient(rgba(255,255,255,0.1) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.1) 1px, transparent 1px)',
          backgroundSize: '64px 64px',
        }}
      />
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Main component                                                     */
/* ------------------------------------------------------------------ */

export function HomePage() {
  const { token, client, signOut } = useAuth()
  const [verified, setVerified] = useState<boolean | null>(null)
  const [authenticatedTarget, setAuthenticatedTarget] = useState('/orgs')

  useEffect(() => {
    if (!token) return
    let cancelled = false
    ;(async () => {
      const { data, error, response } = await client.GET('/orgs', {})
      if (cancelled) return
      if (response.status === 401) {
        signOut()
        setVerified(false)
        return
      }
      if (error) {
        setVerified(false)
        return
      }
      const orgId = resolvePreferredOrgId(data)
      if (orgId) {
        setPreferredOrgId(orgId)
        const slug = resolvePreferredOrgSlug(data) ?? orgId
        setAuthenticatedTarget(`/orgs/${slug}/projects`)
      } else {
        setAuthenticatedTarget('/orgs')
      }
      setVerified(Array.isArray(data))
    })()
    return () => {
      cancelled = true
    }
  }, [token, client, signOut])

  /* Redirect authenticated users */
  if (token && verified === true) {
    return <Navigate to={authenticatedTarget} replace />
  }

  if (token && verified === null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <p className="text-muted-foreground text-sm animate-pulse">
          Verifying session&hellip;
        </p>
      </div>
    )
  }

  return (
    <div className="relative flex min-h-screen flex-col bg-background text-foreground">
      <AnimatedBackground />

      {/* ---- Top nav (minimal) ---- */}
      <header className="relative z-10 flex h-14 items-center justify-between px-6">
        <span className="text-sm font-semibold tracking-tight text-foreground">
          Flywheel
        </span>
        <a
          href="/auth/login"
          className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-white/5 px-3 py-1.5 text-xs font-medium text-foreground/80 backdrop-blur transition-colors hover:bg-white/10 hover:text-foreground"
        >
          Sign in
        </a>
      </header>

      {/* ---- Hero ---- */}
      <main className="relative z-10 flex flex-1 flex-col items-center justify-center px-6 pb-24">
        <div className="mx-auto flex w-full max-w-3xl flex-col items-center text-center">
          {/* Pill badge */}
          <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-xs font-medium text-emerald-400">
            <span className="relative flex size-1.5">
              <span className="absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex size-1.5 rounded-full bg-emerald-500" />
            </span>
            Open-source agent orchestration
          </div>

          {/* Headline */}
          <h1 className="landing-hero-heading text-4xl font-bold tracking-tight sm:text-5xl md:text-6xl">
            <span className="text-foreground">The nervous system</span>
            <br />
            <span className="bg-gradient-to-r from-emerald-400 via-emerald-300 to-teal-400 bg-clip-text text-transparent">
              for your business
            </span>
          </h1>

          {/* Sub-heading */}
          <p className="mt-5 max-w-xl text-base leading-relaxed text-muted-foreground sm:text-lg">
            Agents without structure thrash. Agents with structure ship.
            Flywheel is the work queue, state machine, and shared context layer
            that turns your AI coding agents into a disciplined engineering team.
          </p>

          {/* CTA */}
          <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row">
            <a
              href="/auth/login"
              className="group relative inline-flex items-center gap-2.5 overflow-hidden rounded-xl border border-white/10 bg-white/5 px-6 py-3 text-sm font-medium text-foreground backdrop-blur-md transition-all duration-200 hover:border-emerald-500/30 hover:bg-white/10 hover:shadow-[0_0_30px_-5px_rgba(16,185,129,0.2)]"
            >
              <span>Sign in</span>
              <span className="absolute inset-0 rounded-xl ring-1 ring-inset ring-white/10 transition-all group-hover:ring-emerald-500/20" />
            </a>
          </div>

          {/* Session-invalid message */}
          {token && verified === false && (
            <p className="mt-4 text-sm text-destructive">
              Session invalid. Please sign in again.
            </p>
          )}
        </div>

        {/* ---- Feature highlights ---- */}
        <section className="mx-auto mt-24 grid w-full max-w-4xl grid-cols-1 gap-4 sm:grid-cols-2">
          {features.map((feature) => (
            <div
              key={feature.title}
              className="group rounded-xl border border-white/[0.06] bg-white/[0.02] p-5 backdrop-blur-sm transition-all duration-200 hover:border-white/10 hover:bg-white/[0.04]"
            >
              <div className="flex items-start gap-4">
                <FeatureIcon>{feature.icon}</FeatureIcon>
                <div className="min-w-0">
                  <h3 className="text-sm font-medium text-foreground">
                    {feature.title}
                  </h3>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                    {feature.description}
                  </p>
                </div>
              </div>
            </div>
          ))}
        </section>
      </main>

      {/* ---- Footer ---- */}
      <footer className="relative z-10 border-t border-white/[0.06] px-6 py-6">
        <div className="mx-auto flex max-w-4xl items-center justify-between text-xs text-muted-foreground">
          <span>&copy; {new Date().getFullYear()} Flywheel</span>
          <span className="opacity-60">
            The unified substrate for AI-driven engineering.
          </span>
        </div>
      </footer>
    </div>
  )
}
