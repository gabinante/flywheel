import { HashRouter, Navigate, Outlet, Route, Routes, useParams } from 'react-router-dom'

import { AppShell } from '@/components/app-shell'
import { AuthProvider } from '@/contexts/auth-provider'
import { useAuth } from '@/contexts/use-auth'
import { CommandCenterPage } from '@/pages/command-center-page'
import { HomePage } from '@/pages/home-page'
import { InfrastructurePage } from '@/pages/infrastructure-page'
import { OrgsPage } from '@/pages/orgs-page'
import { PolicyHealthPage } from '@/pages/policy-health-page'
import { ProjectPage } from '@/pages/project-page'
import { ProjectsPage } from '@/pages/projects-page'
import { ReviewsPage } from '@/pages/reviews-page'
import { TicketDetailPage } from '@/pages/ticket-detail-page'
import { TicketsPage } from '@/pages/tickets-page'
import { UsagePage } from '@/pages/usage-page'
import { WorkStreamCreatePage } from '@/pages/work-stream-create-page'
import { WorkStreamEditPage } from '@/pages/work-stream-edit-page'
import { WorkStreamsPage } from '@/pages/work-streams-page'

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
    <AuthProvider>
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
                element={<CommandCenterPage />}
              />
              <Route
                path="/orgs/:orgId/projects/:projectId/settings"
                element={<ProjectPage />}
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
                element={<InfrastructurePage />}
              />
              <Route
                path="/orgs/:orgId/projects/:projectId/policies"
                element={<PolicyHealthPage />}
              />
              <Route
                path="/orgs/:orgId/projects/:projectId/usage"
                element={<UsagePage />}
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
    </AuthProvider>
  )
}
