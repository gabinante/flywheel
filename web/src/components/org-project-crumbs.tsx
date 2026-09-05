import { Link } from 'react-router-dom'

type OrgProjectCrumbsProps = {
  orgId: string
  projectId: string
  projectLabel: string
}

/** `Organizations / Projects / {project}` — project links to the project overview. */
export function OrgProjectCrumbs({
  orgId,
  projectId,
  projectLabel,
}: OrgProjectCrumbsProps) {
  return (
    <>
      <Link to={orgId ? `/orgs/${orgId}/projects` : "/orgs"} className="hover:underline">
        Projects
      </Link>
      {projectId && <>
      <span className="px-1">/</span>
      <Link
        to={`/orgs/${orgId}/projects/${projectId}`}
        className="hover:underline"
      >
        {projectLabel}
      </Link>
      </>}
    </>
  )
}
