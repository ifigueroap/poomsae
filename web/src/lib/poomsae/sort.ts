import type { PoomsaeTaskSummary } from './types'
import { deadlineDate } from './view'

export type PoomsaeTaskSortKey = 'priority' | 'title' | 'updated' | 'owner' | 'id' | 'due'
export type PoomsaeTaskSortDirection = 'asc' | 'desc'

export interface PoomsaeTaskSort {
  key: PoomsaeTaskSortKey
  direction: PoomsaeTaskSortDirection
}

export const POOMSAE_TASK_NATURAL_DIRECTION: Record<PoomsaeTaskSortKey, PoomsaeTaskSortDirection> =
  {
    priority: 'asc',
    title: 'asc',
    updated: 'desc',
    owner: 'asc',
    id: 'asc',
    due: 'asc',
  }

export const DEFAULT_POOMSAE_TASK_SORT: PoomsaeTaskSort = {
  key: 'updated',
  direction: POOMSAE_TASK_NATURAL_DIRECTION.updated,
}

export function togglePoomsaeTaskSort(
  current: PoomsaeTaskSort,
  key: PoomsaeTaskSortKey,
): PoomsaeTaskSort {
  if (current.key === key) {
    return { key, direction: current.direction === 'asc' ? 'desc' : 'asc' }
  }
  return { key, direction: POOMSAE_TASK_NATURAL_DIRECTION[key] }
}

export function sortPoomsaeTasks(
  issues: readonly PoomsaeTaskSummary[],
  sort: PoomsaeTaskSort,
  childrenOf?: (issue: PoomsaeTaskSummary) => readonly PoomsaeTaskSummary[],
  dateOf: (issue: PoomsaeTaskSummary) => string | undefined = deadlineDate,
): PoomsaeTaskSummary[] {
  const dueDate = dueDateLookup(childrenOf, dateOf)
  return [...issues].sort((a, b) => compareWithDueDate(a, b, sort, dueDate))
}

function dueDateLookup(
  childrenOf?: (issue: PoomsaeTaskSummary) => readonly PoomsaeTaskSummary[],
  dateOf: (issue: PoomsaeTaskSummary) => string | undefined = deadlineDate,
) {
  const dates = new Map<PoomsaeTaskSummary, string | undefined>()
  return (issue: PoomsaeTaskSummary): string | undefined => {
    if (dates.has(issue)) return dates.get(issue)
    const seen = new Set<string>()
    const visit = (item: PoomsaeTaskSummary): string | undefined => {
      if (seen.has(item.uid)) return undefined
      seen.add(item.uid)
      let earliest = dateOf(item)
      for (const child of childrenOf?.(item) ?? []) {
        const date = visit(child)
        if (date && (!earliest || date < earliest)) earliest = date
      }
      return earliest
    }
    const date = visit(issue)
    dates.set(issue, date)
    return date
  }
}

export function comparePoomsaeTasks(
  a: PoomsaeTaskSummary,
  b: PoomsaeTaskSummary,
  sort: PoomsaeTaskSort,
  childrenOf?: (issue: PoomsaeTaskSummary) => readonly PoomsaeTaskSummary[],
): number {
  return compareWithDueDate(a, b, sort, dueDateLookup(childrenOf))
}

function compareWithDueDate(
  a: PoomsaeTaskSummary,
  b: PoomsaeTaskSummary,
  sort: PoomsaeTaskSort,
  dueDate: (issue: PoomsaeTaskSummary) => string | undefined,
): number {
  const missing = compareMissing(a, b, sort.key, dueDate)
  if (missing !== 0) return missing
  const primary =
    sort.key === 'due'
      ? (dueDate(a) ?? '').localeCompare(dueDate(b) ?? '')
      : compareKey(a, b, sort.key)
  const ordered = sort.direction === 'asc' ? primary : -primary
  if (ordered !== 0) return ordered
  if (sort.key !== 'updated') {
    const updated = compareKey(a, b, 'updated')
    if (updated !== 0) return -updated
  }
  return a.uid.localeCompare(b.uid)
}

function compareMissing(
  a: PoomsaeTaskSummary,
  b: PoomsaeTaskSummary,
  key: PoomsaeTaskSortKey,
  dueDate: (issue: PoomsaeTaskSummary) => string | undefined,
): number {
  if (key === 'priority') {
    const am = a.priority === undefined
    const bm = b.priority === undefined
    if (am && bm) return 0
    if (am) return 1
    if (bm) return -1
  } else if (key === 'owner' || key === 'due') {
    const am = key === 'owner' ? !a.owner : !dueDate(a)
    const bm = key === 'owner' ? !b.owner : !dueDate(b)
    if (am && bm) return 0
    if (am) return 1
    if (bm) return -1
  }
  return 0
}

function compareKey(a: PoomsaeTaskSummary, b: PoomsaeTaskSummary, key: PoomsaeTaskSortKey): number {
  switch (key) {
    case 'priority':
      return (a.priority ?? 0) - (b.priority ?? 0)
    case 'title':
      return a.title.localeCompare(b.title, undefined, { sensitivity: 'base' })
    case 'updated': {
      const at = Date.parse(a.updated_at)
      const bt = Date.parse(b.updated_at)
      const av = Number.isFinite(at) ? at : Number.NEGATIVE_INFINITY
      const bv = Number.isFinite(bt) ? bt : Number.NEGATIVE_INFINITY
      if (av === bv) return 0
      return av < bv ? -1 : 1
    }
    case 'owner':
      return (a.owner ?? '').localeCompare(b.owner ?? '', undefined, { sensitivity: 'base' })
    case 'id':
      return a.qualified_id.localeCompare(b.qualified_id, undefined, { numeric: true })
    case 'due':
      return (deadlineDate(a) ?? '').localeCompare(deadlineDate(b) ?? '')
  }
}
