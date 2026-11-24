import type { PoomsaeTaskLink, PoomsaeTaskStatusFilter, PoomsaeTaskSummary } from './types'

export const POOMSAE_LINK_RELATIONS = [
  'parent',
  'child',
  'blocks',
  'blocked_by',
  'related',
] as const

export type PoomsaeLinkRelation = (typeof POOMSAE_LINK_RELATIONS)[number]

export interface PoomsaeLinkFilters {
  statuses: Record<PoomsaeTaskSummary['status'], boolean>
  relations: Record<PoomsaeLinkRelation, boolean>
}

export type PoomsaeLinkPeerResolution =
  | { kind: 'pending' }
  | { kind: 'failed' }
  | {
      kind: 'resolved'
      peer: { status: PoomsaeTaskSummary['status']; title?: string | undefined }
    }

function statusesForScope(scope: PoomsaeTaskStatusFilter): PoomsaeLinkFilters['statuses'] {
  return {
    open: scope !== 'closed',
    closed: scope !== 'open',
  }
}

export function createKataLinkFilters(scope: PoomsaeTaskStatusFilter): PoomsaeLinkFilters {
  return {
    statuses: statusesForScope(scope),
    relations: {
      parent: true,
      child: true,
      blocks: true,
      blocked_by: true,
      related: true,
    },
  }
}

export function applyKataLinkStatusScope(
  current: PoomsaeLinkFilters,
  scope: PoomsaeTaskStatusFilter,
): PoomsaeLinkFilters {
  return {
    statuses: statusesForScope(scope),
    relations: { ...current.relations },
  }
}

export function relationForKataLink(
  link: PoomsaeTaskLink,
  selectedUID: string,
): PoomsaeLinkRelation {
  if (link.type === 'parent') return link.from.uid === selectedUID ? 'parent' : 'child'
  if (link.type === 'blocks') return link.to.uid === selectedUID ? 'blocked_by' : 'blocks'
  return 'related'
}

export function poomsaeLinkCouldAffectVisibleResults(
  link: PoomsaeTaskLink,
  selectedUID: string,
  filters: PoomsaeLinkFilters,
): boolean {
  return (
    filters.relations[relationForKataLink(link, selectedUID)] &&
    (filters.statuses.open || filters.statuses.closed)
  )
}

export function poomsaeLinkMatchesFilters(
  link: PoomsaeTaskLink,
  selectedUID: string,
  resolution: PoomsaeLinkPeerResolution,
  filters: PoomsaeLinkFilters,
): boolean {
  if (!poomsaeLinkCouldAffectVisibleResults(link, selectedUID, filters)) return false
  if (resolution.kind === 'failed') return true
  if (resolution.kind === 'pending') return filters.statuses.open && filters.statuses.closed
  return filters.statuses[resolution.peer.status]
}
