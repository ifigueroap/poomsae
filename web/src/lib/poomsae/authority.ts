import type {
  PoomsaeProjectSummary,
  PoomsaeTaskSearchFilters,
  PoomsaeTaskSummary,
  PoomsaeTaskViewName,
  PoomsaeTaskViewResponse,
} from './types'
import { localDateString } from './dates'
import { buildPoomsaeTaskView, poomsaeViewIncludesDates } from './view'

export interface PoomsaeAreaSummary {
  name: string
  projects: PoomsaeProjectSummary[]
}

export interface PoomsaeCurrentView {
  name: PoomsaeTaskViewName
  groups: PoomsaeTaskViewResponse['groups']
  fetched_at?: string | undefined
}

export interface ProjectPoomsaeWorkspaceViewOptions {
  view: PoomsaeTaskViewName
  filters: PoomsaeTaskSearchFilters
  snapshot: {
    projects: readonly PoomsaeProjectSummary[]
    fetched_at: string
  }
  issues: readonly PoomsaeTaskSummary[]
  today?: string | undefined
}

function defaultStatusForView(view: PoomsaeTaskViewName): PoomsaeTaskSearchFilters['status'] {
  return view === 'logbook' ? 'closed' : 'open'
}

function statusChangesViewMembership(
  view: PoomsaeTaskViewName,
  filters: PoomsaeTaskSearchFilters,
): boolean {
  return (
    filters.status !== defaultStatusForView(view) &&
    !(filters.status === 'ready' && view !== 'logbook')
  )
}

export function defaultPoomsaeTaskSearchFilters(
  view: PoomsaeTaskViewName = 'all',
): PoomsaeTaskSearchFilters {
  return {
    scope: { kind: 'all' },
    status: defaultStatusForView(view),
    owner: '',
    label: '',
    query: '',
    relationships: [],
  }
}

const defaultArea = 'Projects'

// Projects without an area (or with the retired "Unfiled" label) share one
// default group.
function projectArea(project: PoomsaeProjectSummary): string {
  const area = project.metadata.area?.trim()
  return area && area !== 'Unfiled' ? area : defaultArea
}

function compareProjectOrder(a: PoomsaeProjectSummary, b: PoomsaeProjectSummary): number {
  const leftOrder = a.metadata.sidebar_order ?? Number.MAX_SAFE_INTEGER
  const rightOrder = b.metadata.sidebar_order ?? Number.MAX_SAFE_INTEGER
  if (leftOrder !== rightOrder) return leftOrder - rightOrder
  return a.name.localeCompare(b.name)
}

export function derivePoomsaeAreas(
  projects: readonly PoomsaeProjectSummary[],
): PoomsaeAreaSummary[] {
  const groups = new Map<string, PoomsaeProjectSummary[]>()
  for (const project of projects) {
    const area = projectArea(project)
    groups.set(area, [...(groups.get(area) ?? []), project])
  }

  const preferred = ['Personal', 'Work', defaultArea]
  return [...groups.entries()]
    .sort(([left], [right]) => {
      const leftIndex = preferred.indexOf(left)
      const rightIndex = preferred.indexOf(right)
      if (leftIndex !== -1 || rightIndex !== -1) {
        return (
          (leftIndex === -1 ? Number.MAX_SAFE_INTEGER : leftIndex) -
          (rightIndex === -1 ? Number.MAX_SAFE_INTEGER : rightIndex)
        )
      }
      return left.localeCompare(right)
    })
    .map(([name, areaProjects]) => ({
      name,
      projects: [...areaProjects].sort(compareProjectOrder),
    }))
}

export function projectPoomsaeWorkspaceView(
  options: ProjectPoomsaeWorkspaceViewOptions,
): PoomsaeTaskViewResponse {
  const today = options.today ?? localDateString()
  const issues = options.issues
    .filter(
      (issue) =>
        options.filters.scope.kind !== 'project' ||
        issue.project_uid === options.filters.scope.project_uid,
    )
    .filter((issue) => poomsaeViewIncludesDates(options.view, issue, today))
    .map((issue) => ({ ...issue }))
  if (statusChangesViewMembership(options.view, options.filters)) {
    return {
      view: options.view,
      groups: issues.length > 0 ? [{ id: 'search-results', title: 'Results', issues }] : [],
      fetched_at: options.snapshot.fetched_at,
    }
  }

  return buildPoomsaeTaskView({
    view: options.view,
    issues,
    projects: options.snapshot.projects.map((project) => ({ ...project })),
    today,
    fetched_at: options.snapshot.fetched_at,
  })
}
