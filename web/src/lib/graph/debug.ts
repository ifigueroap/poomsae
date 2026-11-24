export type PoomsaeGraphDebugEventKind =
  | 'detail-load-abort'
  | 'detail-load-complete'
  | 'detail-load-stale'
  | 'detail-load-start'
  | 'events-load-error'
  | 'graph-load-complete'
  | 'graph-load-error'
  | 'graph-load-start'
  | 'graph-layout-complete'
  | 'graph-layout-error'
  | 'graph-layout-start'
  | 'graph-render'
  | 'selection-start'

export interface PoomsaeGraphDebugEvent {
  id: number
  at: number
  kind: PoomsaeGraphDebugEventKind
  detail?: Record<string, unknown> | undefined
}

export interface PoomsaeGraphDebugGraphSnapshot {
  sourceUID: string
  hideDone: boolean
  graphLoading?: boolean | undefined
  graphError?: string | null | undefined
  contextDepth: string
  depthLimit: string
  layoutMode: string
  layoutDirection: string
  layoutReady: boolean
  nodeIds: string[]
  edges: Array<{
    id: string
    source: string
    target: string
    kind: string | null
    isDepthContext: boolean
  }>
  nodePositions: Array<{ id: string; x: number; y: number }>
  disabledNodeIds: string[]
  missingRefKeys: string[]
  nodeCount: number
  edgeCount: number
  layoutEdgeCount: number
  layoutBounds: { width: number; height: number; aspectRatio: number }
}

export interface PoomsaeGraphDebugStoreSnapshot {
  queueKeys: string[]
  graphLoadActive: boolean
  issueRefreshActive: boolean
  pendingSelectionUID: string | null
  selectedIssueUID: string | null
  cachedTaskCount: number
}

export interface PoomsaeGraphDebugSnapshot {
  events: PoomsaeGraphDebugEvent[]
  latestGraph?: PoomsaeGraphDebugGraphSnapshot | undefined
  store?: PoomsaeGraphDebugStoreSnapshot | undefined
}

export interface PoomsaeGraphDebugAPI {
  snapshot: () => PoomsaeGraphDebugSnapshot
  reset: () => void
}

const maxEvents = 200
let nextEventID = 1
let events: PoomsaeGraphDebugEvent[] = []
let latestGraph: PoomsaeGraphDebugGraphSnapshot | undefined
let store: PoomsaeGraphDebugStoreSnapshot | undefined

function now(): number {
  if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
    return performance.now()
  }
  return Date.now()
}

function cloneSnapshot(): PoomsaeGraphDebugSnapshot {
  return {
    events: events.map((event) => ({
      ...event,
      detail: event.detail ? { ...event.detail } : undefined,
    })),
    latestGraph: latestGraph
      ? {
          ...latestGraph,
          nodeIds: [...latestGraph.nodeIds],
          edges: latestGraph.edges.map((edge) => ({ ...edge })),
          nodePositions: latestGraph.nodePositions.map((position) => ({ ...position })),
          disabledNodeIds: [...latestGraph.disabledNodeIds],
          missingRefKeys: [...latestGraph.missingRefKeys],
          layoutBounds: { ...latestGraph.layoutBounds },
        }
      : undefined,
    store: store ? { ...store, queueKeys: [...store.queueKeys] } : undefined,
  }
}

function installKataGraphDebugHook(): void {
  if (typeof window === 'undefined') return
  ;(window as Window & { __kata_graph_debug?: PoomsaeGraphDebugAPI }).__kata_graph_debug =
    poomsaeGraphDebug
}

export function recordKataGraphDebugEvent(
  kind: PoomsaeGraphDebugEventKind,
  detail?: Record<string, unknown>,
): void {
  installKataGraphDebugHook()
  events = [...events, { id: nextEventID++, at: now(), kind, detail }].slice(-maxEvents)
}

export function setKataGraphDebugGraph(snapshot: PoomsaeGraphDebugGraphSnapshot): void {
  installKataGraphDebugHook()
  latestGraph = {
    ...snapshot,
    nodeIds: [...snapshot.nodeIds],
    edges: snapshot.edges.map((edge) => ({ ...edge })),
    nodePositions: snapshot.nodePositions.map((position) => ({ ...position })),
    disabledNodeIds: [...snapshot.disabledNodeIds],
    missingRefKeys: [...snapshot.missingRefKeys],
    layoutBounds: { ...snapshot.layoutBounds },
  }
}

export function setKataGraphDebugStore(snapshot: PoomsaeGraphDebugStoreSnapshot): void {
  installKataGraphDebugHook()
  store = { ...snapshot, queueKeys: [...snapshot.queueKeys] }
}

export function getKataGraphDebugSnapshot(): PoomsaeGraphDebugSnapshot {
  installKataGraphDebugHook()
  return cloneSnapshot()
}

export function resetKataGraphDebug(): void {
  installKataGraphDebugHook()
  nextEventID = 1
  events = []
  latestGraph = undefined
  store = undefined
}

export const poomsaeGraphDebug: PoomsaeGraphDebugAPI = {
  snapshot: getKataGraphDebugSnapshot,
  reset: resetKataGraphDebug,
}

installKataGraphDebugHook()
