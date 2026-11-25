import { describe, expect, it } from 'vitest'
import type { PoomsaeTaskLink, PoomsaeTaskSummary } from './types'
import {
  applyPoomsaeLinkStatusScope,
  createPoomsaeLinkFilters,
  poomsaeLinkCouldAffectVisibleResults,
  poomsaeLinkMatchesFilters,
  relationForPoomsaeLink,
} from './linkFilters'

const selectedUID = 'issue-selected'

function link(overrides: Partial<PoomsaeTaskLink> = {}): PoomsaeTaskLink {
  return {
    id: 1,
    project_id: 1,
    from: {
      uid: selectedUID,
      short_id: 'selected',
      qualified_id: 'Inbox#selected',
      status: 'open',
    },
    to: { uid: 'issue-peer', short_id: 'peer', qualified_id: 'Inbox#peer', status: 'open' },
    type: 'related',
    author: 'user-a',
    created_at: '2026-07-22T12:00:00Z',
    ...overrides,
  }
}

function peer(status: PoomsaeTaskSummary['status']): PoomsaeTaskSummary {
  return {
    id: 2,
    uid: 'issue-peer',
    project_id: 1,
    project_uid: 'project-1',
    project_name: 'Inbox',
    short_id: 'peer',
    qualified_id: 'Inbox#peer',
    title: 'Peer task',
    status,
    metadata: {},
    revision: 1,
    author: 'maintainer',
    created_at: '2026-07-22T12:00:00Z',
    updated_at: '2026-07-22T12:00:00Z',
  }
}

describe('poomsae link filters', () => {
  it.each([
    ['open', { open: true, closed: false }],
    ['closed', { open: false, closed: true }],
    ['all', { open: true, closed: true }],
  ] as const)('defaults task states from the %s scope', (scope, statuses) => {
    expect(createPoomsaeLinkFilters(scope).statuses).toEqual(statuses)
  })

  it('classifies relationship direction from the selected task', () => {
    expect(relationForPoomsaeLink(link({ type: 'parent' }), selectedUID)).toBe('parent')
    expect(
      relationForPoomsaeLink(
        link({
          type: 'parent',
          from: {
            uid: 'issue-parent',
            short_id: 'parent',
            qualified_id: 'Inbox#parent',
            status: 'open',
          },
          to: {
            uid: selectedUID,
            short_id: 'selected',
            qualified_id: 'Inbox#selected',
            status: 'open',
          },
        }),
        selectedUID,
      ),
    ).toBe('child')
    expect(relationForPoomsaeLink(link({ type: 'blocks' }), selectedUID)).toBe('blocks')
    expect(
      relationForPoomsaeLink(
        link({
          type: 'blocks',
          from: {
            uid: 'issue-blocker',
            short_id: 'blocker',
            qualified_id: 'Inbox#blocker',
            status: 'open',
          },
          to: {
            uid: selectedUID,
            short_id: 'selected',
            qualified_id: 'Inbox#selected',
            status: 'open',
          },
        }),
        selectedUID,
      ),
    ).toBe('blocked_by')
  })

  it('resets task states without changing relationship choices', () => {
    const current = createPoomsaeLinkFilters('all')
    current.relations.related = false

    expect(applyPoomsaeLinkStatusScope(current, 'closed')).toEqual({
      statuses: { open: false, closed: true },
      relations: { ...current.relations, related: false },
    })
  })

  it('matches resolved, pending, and failed peers without silently hiding failures', () => {
    const openOnly = createPoomsaeLinkFilters('open')
    const mixed = createPoomsaeLinkFilters('all')

    expect(
      poomsaeLinkMatchesFilters(
        link(),
        selectedUID,
        { kind: 'resolved', peer: peer('open') },
        openOnly,
      ),
    ).toBe(true)
    expect(
      poomsaeLinkMatchesFilters(
        link(),
        selectedUID,
        { kind: 'resolved', peer: peer('closed') },
        openOnly,
      ),
    ).toBe(false)
    expect(poomsaeLinkMatchesFilters(link(), selectedUID, { kind: 'pending' }, openOnly)).toBe(
      false,
    )
    expect(poomsaeLinkMatchesFilters(link(), selectedUID, { kind: 'pending' }, mixed)).toBe(true)
    expect(poomsaeLinkMatchesFilters(link(), selectedUID, { kind: 'failed' }, openOnly)).toBe(true)
  })

  it('ignores pending work that disabled filters cannot reveal', () => {
    const filters = createPoomsaeLinkFilters('open')
    filters.relations.related = false
    expect(poomsaeLinkCouldAffectVisibleResults(link(), selectedUID, filters)).toBe(false)

    filters.relations.related = true
    filters.statuses.open = false
    expect(poomsaeLinkCouldAffectVisibleResults(link(), selectedUID, filters)).toBe(false)
  })
})
