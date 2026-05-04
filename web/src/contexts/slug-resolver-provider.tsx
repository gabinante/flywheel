import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'

import { useAuth } from '@/contexts/use-auth'

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

type SlugMap = {
  slugToId: Map<string, string>
  idToSlug: Map<string, string>
}

type SlugResolverContextValue = {
  resolveOrgId: (slugOrUUID: string | undefined) => string | undefined
  resolveProjectId: (orgParam: string | undefined, projectParam: string | undefined) => string | undefined
  orgSlug: (id: string) => string | undefined
  projectSlug: (id: string) => string | undefined
  ready: boolean
}

const SlugResolverContext = createContext<SlugResolverContextValue>({
  resolveOrgId: (v) => v,
  resolveProjectId: (_o, v) => v,
  orgSlug: () => undefined,
  projectSlug: () => undefined,
  ready: false,
})

export function useSlugResolver() {
  return useContext(SlugResolverContext)
}

export function SlugResolverProvider({ children }: { children: React.ReactNode }) {
  const { token, client } = useAuth()
  const [orgMap, setOrgMap] = useState<SlugMap>({ slugToId: new Map(), idToSlug: new Map() })
  const [projectMaps, setProjectMaps] = useState<Map<string, SlugMap>>(new Map())
  const [orgsReady, setOrgsReady] = useState(false)
  const loadedOrgIds = useRef<Set<string>>(new Set())

  // Fetch orgs on mount / token change
  useEffect(() => {
    if (!token) {
      setOrgMap({ slugToId: new Map(), idToSlug: new Map() })
      setOrgsReady(false)
      return
    }
    let cancelled = false
    ;(async () => {
      const { data, response } = await client.GET('/orgs', {})
      if (cancelled) return
      if (!response.ok || !data) {
        setOrgsReady(true)
        return
      }
      const slugToId = new Map<string, string>()
      const idToSlug = new Map<string, string>()
      for (const org of data) {
        if (org.id && org.slug) {
          slugToId.set(org.slug, org.id)
          idToSlug.set(org.id, org.slug)
        }
      }
      setOrgMap({ slugToId, idToSlug })
      setOrgsReady(true)
    })()
    return () => { cancelled = true }
  }, [client, token])

  const resolveOrgId = useCallback(
    (slugOrUUID: string | undefined): string | undefined => {
      if (!slugOrUUID) return undefined
      if (UUID_RE.test(slugOrUUID)) return slugOrUUID
      return orgMap.slugToId.get(slugOrUUID) ?? slugOrUUID
    },
    [orgMap],
  )

  const loadProjectsForOrg = useCallback(
    (orgId: string) => {
      if (!token || loadedOrgIds.current.has(orgId)) return
      loadedOrgIds.current.add(orgId)
      ;(async () => {
        const { data, response } = await client.GET('/orgs/{orgID}/projects', {
          params: { path: { orgID: orgId }, query: { status: 'all' } },
        })
        if (!response.ok || !data) return
        const slugToId = new Map<string, string>()
        const idToSlug = new Map<string, string>()
        for (const p of data) {
          if (p.id && p.slug) {
            slugToId.set(p.slug, p.id)
            idToSlug.set(p.id, p.slug)
          }
        }
        setProjectMaps((prev) => {
          const next = new Map(prev)
          next.set(orgId, { slugToId, idToSlug })
          return next
        })
      })()
    },
    [client, token],
  )

  const resolveProjectId = useCallback(
    (orgParam: string | undefined, projectParam: string | undefined): string | undefined => {
      if (!projectParam) return undefined
      if (UUID_RE.test(projectParam)) return projectParam

      // Resolve org first to find the right project map
      const orgId = resolveOrgId(orgParam)
      if (!orgId) return projectParam

      // Ensure projects are loaded for this org
      loadProjectsForOrg(orgId)

      const pMap = projectMaps.get(orgId)
      return pMap?.slugToId.get(projectParam) ?? projectParam
    },
    [resolveOrgId, projectMaps, loadProjectsForOrg],
  )

  const getOrgSlug = useCallback(
    (id: string): string | undefined => orgMap.idToSlug.get(id),
    [orgMap],
  )

  const getProjectSlug = useCallback(
    (id: string): string | undefined => {
      for (const pMap of projectMaps.values()) {
        const slug = pMap.idToSlug.get(id)
        if (slug) return slug
      }
      return undefined
    },
    [projectMaps],
  )

  const value = useMemo<SlugResolverContextValue>(
    () => ({
      resolveOrgId,
      resolveProjectId,
      orgSlug: getOrgSlug,
      projectSlug: getProjectSlug,
      ready: orgsReady,
    }),
    [resolveOrgId, resolveProjectId, getOrgSlug, getProjectSlug, orgsReady],
  )

  return (
    <SlugResolverContext.Provider value={value}>
      {children}
    </SlugResolverContext.Provider>
  )
}
