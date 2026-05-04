import type { components } from '@/lib/api/v1'

type Org = components['schemas']['Org']

const PREFERRED_ORG_KEY = 'flywheel.preferredOrgId'

export type OrgWithID = Org & { id: string }

export function orgsWithIDs(orgs: readonly Org[] | null | undefined): OrgWithID[] {
  return (orgs ?? []).filter((org): org is OrgWithID => Boolean(org.id))
}

export function getPreferredOrgId(): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(PREFERRED_ORG_KEY)
  } catch {
    return null
  }
}

export function setPreferredOrgId(orgId: string): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(PREFERRED_ORG_KEY, orgId)
  } catch {
    // Ignore localStorage failures in private mode or locked-down browsers.
  }
}

export function resolvePreferredOrgId(orgs: readonly Org[] | null | undefined): string | null {
  const candidates = orgsWithIDs(orgs)
  if (candidates.length === 0) return null

  const preferred = getPreferredOrgId()
  if (preferred && candidates.some((org) => org.id === preferred)) {
    return preferred
  }

  return candidates[0].id
}

/** Returns the slug for the preferred org (falls back to ID). */
export function resolvePreferredOrgSlug(orgs: readonly Org[] | null | undefined): string | null {
  const id = resolvePreferredOrgId(orgs)
  if (!id) return null
  const match = orgsWithIDs(orgs).find((org) => org.id === id)
  return match?.slug ?? id
}
