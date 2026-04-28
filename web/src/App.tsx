import { lazy, Suspense } from 'react'
import { HashRouter, Navigate, Outlet, Route, Routes, useParams } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'

import { AppShell } from '@/components/app-shell'
import { ErrorBoundary } from '@/components/error-boundary'
import { AuthProvider } from '@/contexts/auth-provider'
import { useAuth } from '@/contexts/use-auth'
import { queryClient } from '@/lib/query-client'
import { HomePage } from '@/pages/home-page'
import { OrgsPage } from '@/pages/orgs-page'
import { ProjectsPage } from '@/pages/projects-page'
import { TicketsPage } from '@/pages/tickets-page'
import { TicketDetailPage } from '@/pages/ticket-detail-page'
import { ReviewsPage } from '@/pages/reviews-page'
import { ProjectSettingsPage } from '@/pages/project-settings-page'
import { WorkStreamsPage } from '@/pages/work-streams-page'
import { WorkStreamCreatePage } from '@/pages/work-stream-create-page'
import { WorkStreamEditPage } from '@/pages/work-stream-edit-page'

// Lazy-load heavy pages that aren't needed on initial render
const CommandCenterPage = lazy(() =>
  import('@/pages/command-center-page').then((m) => ({ default: m.CommandCenterPage })),
)
const InfrastructurePage = lazy(() =>
  import('@/pages/infrastructure-page').then((m) => ({ default: m.InfrastructurePage })),
)
const EntityDetailPage = lazy(() =>
  import('@/pages/entity-detail-page').then((m) => ({ default: m.EntityDetailPage })),
)
const PolicyHealthPage = lazy(() =>
  import('@/pages/policy-health-page').then((m) => ({ default: m.PolicyHealthPage })),
)
const UsagePage = lazy(() =>
  import('@/pages/usage-page').then((m) => ({ default: m.UsagePage })),
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

/** Redirect project root to the command center */
function ProjectRedirect() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>()
  return <Navigate to={`/orgs/${orgId}/projects/${projectId}/command`} replace />
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ErrorBoundary>
          <HashRouter>
            <Routes>
              {/* Home/landing page renders full-width, outside AppShell constraints */}
              <Route path="/" element={<HomeRoute />} />
              <Route element={<AppShell />}>
                <Route element={<RequireAuthLayout />}>
                  <Route path="/orgs" element={<OrgsPage />} />
                  <Route
                    path="/orgs/:orgId/projects"
                    element={<ProjectsPage />}
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
                    path="/orgs/:orgId/projects/:projectId/infrastructure"
                    element={
                      <ErrorBoundary>
                        <PageSuspense>
                          <InfrastructurePage />
                        </PageSuspense>
                      </ErrorBoundary>
                    }
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/infrastructure/entities/:entityId"
                    element={
                      <ErrorBoundary>
                        <PageSuspense>
                          <EntityDetailPage />
                        </PageSuspense>
                      </ErrorBoundary>
                    }
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/policies"
                    element={
                      <ErrorBoundary>
                        <PageSuspense>
                          <PolicyHealthPage />
                        </PageSuspense>
                      </ErrorBoundary>
                    }
                  />
                  <Route
                    path="/orgs/:orgId/projects/:projectId/usage"
                    element={
                      <ErrorBoundary>
                        <PageSuspense>
                          <UsagePage />
                        </PageSuspense>
                      </ErrorBoundary>
                    }
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
            </Routes>
          </HashRouter>
        </ErrorBoundary>
      </AuthProvider>
    </QueryClientProvider>
  )
}
