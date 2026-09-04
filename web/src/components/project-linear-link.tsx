import { useEffect, useState } from 'react'
import { ExternalLink } from 'lucide-react'

import { useAuth } from '@/contexts/use-auth'
import type { components } from '@/lib/api/v1'

type ProjectLinearLink = components['schemas']['ProjectLinearLink']

/** Small "Linear ↗" chip for project headers; renders nothing when the project is not linked. */
export function ProjectLinearLinkChip({ projectId }: { projectId: string }) {
  const { client } = useAuth()
  const [link, setLink] = useState<ProjectLinearLink | null>(null)

  useEffect(() => {
    let cancelled = false
    void client.GET('/projects/{projectID}/linear', { params: { path: { projectID: projectId } } }).then(({ data, response }) => {
      if (!cancelled && response.ok && data?.linked) setLink(data)
    })
    return () => {
      cancelled = true
    }
  }, [client, projectId])

  if (!link?.linear_project_url) return null
  return (
    <a
      href={link.linear_project_url}
      target="_blank"
      rel="noreferrer"
      className="inline-flex items-center gap-1 rounded-md border border-violet-500/30 bg-violet-500/10 px-2 py-0.5 text-xs text-violet-200 hover:bg-violet-500/20"
      title={link.linear_project_name ?? 'Open in Linear'}
    >
      Linear <ExternalLink className="size-3" />
    </a>
  )
}
