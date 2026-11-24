import { describe, expect, it } from 'vitest'

import { parseRoute, serializeRoute, screenNameForRoute } from './router'

const issueUID = '01HZNQ7VFPK1XGD8R5MABCD4EX'

describe('canonical Poomsae routes', () => {
  it.each([
    ['issue=example%23abc4', { issue: 'example#abc4' }],
    ['scope=example&issue=abc4', { scope: 'example', issue: 'abc4' }],
    [`scope=${issueUID}&issue=abc4`, { scope: issueUID, issue: 'abc4' }],
    [`scope=example&issue=${issueUID}`, { scope: 'example', issue: issueUID }],
    ['scope=%20example%20&issue=abc4', { scope: ' example ', issue: 'abc4' }],
    [`issue=${issueUID}%23abc4`, { issue: `${issueUID}#abc4` }],
  ])('preserves link values in %s', (query, link) => {
    const route = parseRoute(new URL(`https://daemon.example/poomsae?${query}&graph=1&label=ready`))
    expect(route).toMatchObject({ kind: 'poomsae', link, graph: true })
    if (route.kind !== 'poomsae') throw new Error('Expected a Poomsae route')
    expect(route.link).toEqual(link)
    expect(
      parseRoute(new URL(serializeRoute({ ...route, issueUID }), 'https://daemon.example')),
    ).toEqual(route)
  })
  it('keeps Forge view, scope, and issue state independent', () => {
    const projectUID = '01HZNQ7VFPK1XGD8R5MABCD4EW'
    const route = parseRoute(
      new URL(
        `https://daemon.example/poomsae?view=all-open&scope=${projectUID}&issue=${issueUID}&label=ready`,
      ),
    )

    expect(route).toMatchObject({
      kind: 'poomsae',
      view: 'all-open',
      projectUID,
      issueUID,
      graph: false,
    })
    expect(serializeRoute(route)).toBe(
      `/poomsae?view=all-open&scope=${projectUID}&issue=${issueUID}&label=ready`,
    )
  })

  it('parses only the canonical route families', () => {
    expect(
      parseRoute(new URL(`https://daemon.example/poomsae?view=today&label=ready`)),
    ).toMatchObject({
      kind: 'poomsae',
      view: 'today',
    })
    expect(parseRoute(new URL(`https://daemon.example/poomsae?scope=${issueUID}`))).toMatchObject({
      kind: 'poomsae',
      projectUID: issueUID,
    })
    expect(
      parseRoute(new URL(`https://daemon.example/poomsae?issue=${issueUID}&graph=1`)),
    ).toMatchObject({
      kind: 'poomsae',
      issueUID,
      graph: true,
    })
  })

  it('parses and serializes the Delegated view', () => {
    const route = parseRoute(new URL('https://daemon.example/poomsae?view=delegated'))

    expect(route).toMatchObject({ kind: 'poomsae', view: 'delegated' })
    expect(serializeRoute(route)).toBe('/poomsae?view=delegated')
  })

  it('parses and serializes the Scheduled view', () => {
    const route = parseRoute(new URL('https://daemon.example/poomsae?view=scheduled'))

    expect(route).toMatchObject({ kind: 'poomsae', view: 'scheduled' })
    expect(serializeRoute(route)).toBe('/poomsae?view=scheduled')
  })

  it.each(['credentials', 'settings'])(
    'parses %s as a daemon route without carrying issue workspace state',
    (view) => {
      const route = parseRoute(
        new URL(`https://daemon.example/poomsae?view=${view}&issue=${issueUID}&label=ready`),
      )
      expect(route).toEqual({
        kind: 'poomsae',
        view,
        graph: false,
        filters: { status: [], owner: [], label: [], relationship: [] },
      })
      expect(serializeRoute(route)).toBe(`/poomsae?view=${view}`)
    },
  )

  it.each(['appearance', 'search', 'access'])(
    'preserves the %s settings section address',
    (section) => {
      const route = parseRoute(
        new URL(`https://daemon.example/poomsae?view=settings&section=${section}`),
      )
      expect(route).toMatchObject({ kind: 'poomsae', view: 'settings', section })
      expect(serializeRoute(route)).toBe(`/poomsae?view=settings&section=${section}`)
    },
  )

  it('drops unknown settings sections and sections on other views', () => {
    for (const query of ['view=settings&section=unknown', 'view=credentials&section=access']) {
      const route = parseRoute(new URL(`https://daemon.example/poomsae?${query}`))
      expect(route).not.toHaveProperty('section')
      expect(serializeRoute(route)).not.toContain('section=')
    }
  })

  it('keeps invalid issue UIDs routed and gives short refs one search action', () => {
    expect(parseRoute(new URL('https://daemon.example/poomsae?issue=abc4'))).toEqual({
      kind: 'route-error',
      path: '/poomsae?issue=abc4',
      reason: 'issue_uid',
      searchRef: 'abc4',
    })
    expect(
      parseRoute(new URL('https://daemon.example/poomsae?issue=not-a-valid-full-uid-value')),
    ).toEqual({
      kind: 'route-error',
      path: '/poomsae?issue=not-a-valid-full-uid-value',
      reason: 'issue_uid',
    })
  })

  it('serializes shareable filters deterministically', () => {
    const route = parseRoute(
      new URL(
        `https://daemon.example/poomsae?scope=${issueUID}&label=urgent&status=open&label=backend&owner=user-a`,
      ),
    )
    expect(serializeRoute(route)).toBe(
      `/poomsae?scope=${issueUID}&label=backend&label=urgent&owner=user-a&status=open`,
    )
  })

  it('parses and serializes a browser application mounted below a path', () => {
    const routePath = '/tools/tasks/'
    const route = parseRoute(
      new URL(`https://daemon.example${routePath}?view=today&label=ready`),
      routePath,
    )

    expect(route).toMatchObject({ kind: 'poomsae', view: 'today' })
    expect(serializeRoute(route, routePath)).toBe('/tools/tasks/?view=today&label=ready')
  })
})

describe('screen names', () => {
  it.each([
    ['', 'inbox'],
    ['?view=all-open', 'issues'],
    ['?view=today', 'today'],
    ['?view=delegated', 'delegated'],
    ['?view=scheduled', 'scheduled'],
    ['?view=logbook', 'logbook'],
    [`?scope=${issueUID}`, 'issues'],
    [`?issue=${issueUID}`, 'issue'],
    [`?issue=${issueUID}&graph=1`, 'graph'],
    [`?view=credentials&issue=${issueUID}`, 'credentials'],
    [`?view=settings&issue=${issueUID}`, 'settings'],
    ['?view=unknown', undefined],
  ])('maps %s to %s', (query, screen) => {
    expect(screenNameForRoute(parseRoute(new URL(`https://daemon.example/poomsae${query}`)))).toBe(
      screen,
    )
  })
})
