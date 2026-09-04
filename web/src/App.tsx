import { lazy, Suspense, useEffect, useState } from 'react'
import { HashRouter, Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'

import { AppShell } from '@/components/app-shell'
import { ErrorBoundary } from '@/components/error-boundary'
import { AuthProvider } from '@/contexts/auth-provider'
import { SlugResolverProvider, useSlugResolver } from '@/contexts/slug-resolver-provider'
import { useAuth } from '@/contexts/use-auth'
import { useResolvedRouteParams } from '@/hooks/use-resolved-route-params'
import { resolvePreferredOrgId, resolvePreferredOrgSlug, setPreferredOrgId } from '@/lib/org-preferences'
import { queryClient } from '@/lib/query-client'
import { HomePage } from '@/pages/home-page'
import { OrgsPage } from '@/pages/orgs-page'
import { OperatorSettingsPage } from '@/pages/operator-settings-page'
import { MyPRsPage } from '@/pages/my-prs-page'
import { MyReviewsPage } from '@/pages/my-reviews-page'
import { SchedulePage } from '@/pages/schedule-page'
import { WorkflowsPage, WorkflowEditorPage } from '@/pages/workflows-page'
import { ProjectsPage } from '@/pages/projects-page'
import { TicketsPage } from '@/pages/tickets-page'
import { TicketDetailPage } from '@/pages/ticket-detail-page'
import { ReviewsPage } from '@/pages/reviews-page'
import { SessionsPage } from '@/pages/sessions-page'
import { CodeReviewsPage } from '@/pages/code-reviews-page'
import { CodeReviewDetailPage } from '@/pages/code-review-detail-page'
import { SessionDetailPage } from '@/pages/session-detail-page'
import { ProjectSettingsPage } from '@/pages/project-settings-page'
import { WorkStreamsPage } from '@/pages/work-streams-page'
import { ProjectCreatePage } from '@/pages/project-create-page'
import { WorkStreamCreatePage } from '@/pages/work-stream-create-page'
import { WorkStreamEditPage } from '@/pages/work-stream-edit-page'

// Lazy-load heavy pages that aren't needed on initial render
const CommandCenterPage = lazy(() =>
  import('@/pages/command-center-page').then((m) => ({ default: m.CommandCenterPage })),
)

function PageSuspense({ children }: { children: React.ReactNode }) {
  return (
    <Suspense
      fallback={
        <div className="flex h-64 items-center justify-center">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-white/20 border-t-white/60" />
        </div>
      }
    >
      {children}
    </Suspense>
  )
}

function RequireAuthLayout() {
  const { token } = useAuth()
  if (!token) return <Navigate to="/" replace />
  return <Outlet />
}

function HomeRoute() {
  const { token } = useAuth()
  return <HomePage key={token ?? 'anon'} />
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/** Redirect UUID-based URLs to slug-based URLs when slugs are known. */
function SlugRedirect() {
  const location = useLocation()
  const { orgId, orgParam, projectId, projectParam } = useResolvedRouteParams()
  const { orgSlug, projectSlug } = useSlugResolver()

  if (orgParam && orgId && UUID_RE.test(orgParam)) {
    const slug = orgSlug(orgId)
    if (slug) {
      const newPath = location.pathname.replace(`/orgs/${orgParam}`, `/orgs/${slug}`)
      return <Navigate to={newPath + location.search} replace />
    }
  }

  if (projectParam && projectId && UUID_RE.test(projectParam)) {
    const slug = projectSlug(projectId)
    if (slug) {
      const newPath = location.pathname.replace(
        `/projects/${projectParam}`,
        `/projects/${slug}`,
      )
      return <Navigate to={newPath + location.search} replace />
    }
  }

  return <Outlet />
}

/** The single organization is implicit: forward /orgs to its projects; show the orgs page only when none exists. */
function OrgsGate() {
  const { client } = useAuth()
  const [target, setTarget] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    let cancelled = false
    void client.GET('/orgs', {}).then(({ data, response }) => {
      if (cancelled) return
      if (!response.ok || !data || data.length === 0) {
        setTarget(null)
        return
      }
      const id = resolvePreferredOrgId(data) ?? data[0].id ?? ''
      setPreferredOrgId(id)
      const slug = resolvePreferredOrgSlug(data) ?? data[0].slug ?? id
      setTarget(`/orgs/${slug}/projects`)
    })
    return () => {
      cancelled = true
    }
  }, [client])
  if (target === undefined) return null
  if (target === null) return <OrgsPage />
  return <Navigate to={target} replace />
}

/** Redirect project root to the command center */
function ProjectRedirect() {
  const { orgParam, projectParam } = useResolvedRouteParams()
  return <Navigate to={`/orgs/${orgParam}/projects/${projectParam}/command`} replace />
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ErrorBoundary>
          <HashRouter>
            <SlugResolverProvider>
            <Routes>
              {/* Home/landing page renders full-width, outside AppShell constraints */}
              <Route path="/" element={<HomeRoute />} />
              <Route element={<AppShell />}>
                <Route element={<RequireAuthLayout />}>
                  {/* Auto-redirect UUID URLs to slug URLs */}
                  <Route element={<SlugRedirect />}>
                  <Route path="/orgs" element={<OrgsGate />} />
                  <Route path="/settings" element={<OperatorSettingsPage />} />
                  <Route path="/my/prs" element={<MyPRsPage />} />
                  <Route path="/my/reviews" element={<MyReviewsPage />} />
                  <Route path="/schedule" element={<SchedulePage />} />
                  <Route path="/workflows" element={<WorkflowsPage />} />
                  <Route path="/workflows/:id" element={<WorkflowEditorPage />} />
                  <Route
                    path="/orgs/:orgId/projects"
                    element={<ProjectsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/new"
                    element={<ProjectCreatePage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId"
                    element={<ProjectRedirect />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/command"
                    element={
                      <ErrorBoundary>
                        <PageSuspense>
                          <CommandCenterPage />
                        </PageSuspense>
                      </ErrorBoundary>
                    }
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/settings"
                    element={<ProjectSettingsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/settings/:section"
                    element={<ProjectSettingsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/tickets"
                    element={<TicketsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/tickets/:ticketId"
                    element={<TicketDetailPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/reviews"
                    element={<ReviewsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/code-reviews"
                    element={<CodeReviewsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/code-reviews/:reviewId"
                    element={<CodeReviewDetailPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/sessions"
                    element={<SessionsPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/sessions/:sessionId"
                    element={<SessionDetailPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/work-streams/new"
                    element={<WorkStreamCreatePage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/work-streams/:workStreamId"
                    element={<WorkStreamEditPage />}
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/work-streams"
                    element={<WorkStreamsPage />}
                  />
                </Route>
              </Route>
              </Route>
            </Routes>
            </SlugResolverProvider>
          </HashRouter>
        </ErrorBoundary>
      </AuthProvider>
    </QueryClientProvider>
  )
}
