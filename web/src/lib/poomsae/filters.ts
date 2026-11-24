import type { PoomsaeTaskStatusFilter, PoomsaeTaskSummary } from './types'

export function poomsaeTaskStatusMatchesFilter(
  issue: Pick<PoomsaeTaskSummary, 'uid' | 'status'>,
  filter: PoomsaeTaskStatusFilter,
  readyIssueUIDs?: ReadonlySet<string>,
): boolean {
  if (filter === 'all') return true
  if (filter === 'ready') return readyIssueUIDs?.has(issue.uid) === true
  return issue.status === filter
}
