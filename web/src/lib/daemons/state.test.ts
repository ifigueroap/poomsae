import { beforeEach, describe, expect, test } from 'vitest'

import { loadDaemonRoute, saveDaemonRoute } from './state'

describe('daemon workspace state', () => {
  beforeEach(() => localStorage.clear())

  test('restores independent canonical routes per daemon', () => {
    saveDaemonRoute('example-local', '/poomsae?scope=01J00000000000000000000001')
    saveDaemonRoute('example-remote', '/poomsae?view=today')

    expect(loadDaemonRoute('example-local')).toBe('/poomsae?scope=01J00000000000000000000001')
    expect(loadDaemonRoute('example-remote')).toBe('/poomsae?view=today')
  })

  test('ignores malformed stored routes without disturbing other daemon state', () => {
    localStorage.setItem(
      'poomsae.web.workspace-state.v1',
      JSON.stringify({
        version: 1,
        daemons: { 'example-local': '/wrong', 'example-remote': '/poomsae?view=all-open' },
      }),
    )

    expect(loadDaemonRoute('example-local')).toBeUndefined()
    expect(loadDaemonRoute('example-remote')).toBe('/poomsae?view=all-open')
  })
})
