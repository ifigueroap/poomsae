import { MarkerType, Position, type Edge, type Node } from '@xyflow/svelte'

import type {
  PoomsaeReachableGraphEdge,
  PoomsaeReachableGraphEdgeKind,
  PoomsaeReachableGraphResponse,
  PoomsaeReachableGraphUnresolvedRef,
  PoomsaeTaskSummary,
} from '../poomsae/types'

export interface PoomsaeGraphNodeData extends Record<string, unknown> {
  label: string
  title: string
  idLabel: string
  // The project's name, for its mark; projectLabel names it in text only
  // where the graph spans projects or two tasks share a short ID.
  projectName: string
  projectLabel: string
  qualifiedLabel: string
  accessibleLabel: string
  status: PoomsaeTaskSummary['status'] | 'uncached'
  closedReason?: PoomsaeTaskSummary['closed_reason'] | undefined
  priorityLabel: string | null
  isSource: boolean
  selectable: boolean
  isDepthContext: boolean
  onSelect?: ((uid: string) => void) | undefined
  layoutDirection: PoomsaeGraphLayoutDirection
}

export type PoomsaeGraphAdjacentRelation =
  | 'blocks'
  | 'blockedBy'
  | 'child'
  | 'parent'
  | 'related'
  | null
export type PoomsaeGraphDepthLimit = 'full' | '1' | '2' | '3'
export type PoomsaeGraphContextDepth = 'all' | '1' | '2' | '3'
export type PoomsaeGraphLayoutDirection = 'LR' | 'TB'
export type PoomsaeGraphNode = Node<PoomsaeGraphNodeData, 'poomsaeTask'>
export type PoomsaeGraphEdge = Edge<PoomsaeGraphEdgeData>

export interface PoomsaeGraphMissingRef {
  uid: string
  side: 'from' | 'to'
  kind: PoomsaeReachableGraphEdgeKind
  otherUID: string
}

interface PoomsaeGraphEdgeData extends Record<string, unknown> {
  kind: PoomsaeReachableGraphEdgeKind
  layout: boolean
  isDepthContext?: boolean | undefined
  isSelectedAdjacent?: boolean | undefined
}

export interface BuildKataReachableGraphInput {
  sourceUID: string
  graph: PoomsaeReachableGraphResponse | null | undefined
  contextDepth?: PoomsaeGraphContextDepth | undefined
  layoutDirection?: PoomsaeGraphLayoutDirection | undefined
}

// A node fits its two text rows, and the gaps between nodes follow the
// accepted graph: about a seventh of a node's width between layers and a
// narrow gap between siblings.
const POOMSAE_GRAPH_NODE_WIDTH = 250
const POOMSAE_GRAPH_NODE_HEIGHT = 62
const POOMSAE_GRAPH_X_SPACING = 285
const POOMSAE_GRAPH_Y_SPACING = 76

function priorityLabel(priority: number | undefined): string | null {
  if (priority === undefined) return null
  return `P${priority}`
}

function maxContextDepth(limit: PoomsaeGraphContextDepth | undefined): number {
  return limit === undefined || limit === 'all' ? Number.POSITIVE_INFINITY : Number(limit)
}

function missingRefKey(ref: PoomsaeReachableGraphUnresolvedRef): string {
  return `${ref.side}:${ref.kind}:${ref.uid}:${ref.other_uid}`
}

function missingRefsByUID(
  refs: readonly PoomsaeReachableGraphUnresolvedRef[],
): Map<string, PoomsaeGraphMissingRef> {
  const out = new Map<string, PoomsaeGraphMissingRef>()
  for (const ref of refs) {
    if (!ref.uid) continue
    out.set(ref.uid, {
      uid: ref.uid,
      side: ref.side,
      kind: ref.kind,
      otherUID: ref.other_uid,
    })
  }
  return out
}

function nodeClass(
  task: PoomsaeTaskSummary | undefined,
  sourceUID: string,
  isDepthContext: boolean,
): string {
  return [
    'poomsae-graph-node',
    task?.status === 'closed' ? 'poomsae-graph-node--closed' : 'poomsae-graph-node--open',
    task?.uid === sourceUID ? 'poomsae-graph-node--source' : '',
    task ? '' : 'poomsae-graph-node--uncached',
    isDepthContext ? 'poomsae-graph-node--depth-context' : '',
  ]
    .filter(Boolean)
    .join(' ')
}

function graphNodeSortKey(id: string, task: PoomsaeTaskSummary | undefined): string {
  if (!task) return `1:${id}`
  return `0:${task.project_name}:${task.short_id}:${task.title}:${task.uid}`
}

function compareGraphNodeEntries(
  [leftID, leftTask]: [string, PoomsaeTaskSummary | undefined],
  [rightID, rightTask]: [string, PoomsaeTaskSummary | undefined],
): number {
  return graphNodeSortKey(leftID, leftTask).localeCompare(
    graphNodeSortKey(rightID, rightTask),
    undefined,
    {
      numeric: true,
      sensitivity: 'base',
    },
  )
}

function makeEdge(edge: PoomsaeReachableGraphEdge): PoomsaeGraphEdge {
  return {
    id: `${edge.kind}:${edge.from_uid}:${edge.to_uid}`,
    source: edge.from_uid,
    target: edge.to_uid,
    type: 'smoothstep',
    class: `poomsae-graph-edge poomsae-graph-edge--${edge.kind}`,
    data: { kind: edge.kind, layout: edge.layout },
    ...(edge.kind === 'blocks' || edge.kind === 'parent'
      ? {
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color:
              edge.kind === 'blocks'
                ? 'var(--poomsae-graph-edge-blocks)'
                : 'var(--poomsae-graph-edge-ambient)',
            width: 18,
            height: 18,
          },
        }
      : {}),
    ariaLabel: `${edge.kind} relationship from ${edge.from_uid} to ${edge.to_uid}`,
    interactionWidth: 12,
    selectable: false,
    zIndex: 2,
  }
}

function edgeID(edge: PoomsaeGraphEdge): string {
  return edge.id
}

function dedupeEdges(edges: readonly PoomsaeGraphEdge[]): PoomsaeGraphEdge[] {
  const seen = new Set<string>()
  const out: PoomsaeGraphEdge[] = []
  for (const edge of edges) {
    const id = edgeID(edge)
    if (seen.has(id)) continue
    seen.add(id)
    out.push(edge)
  }
  return out
}

function nodeData(
  id: string,
  task: PoomsaeTaskSummary | undefined,
  sourceUID: string,
  adjacentRelation: PoomsaeGraphAdjacentRelation,
  projectLabel: string,
  missingRef: PoomsaeGraphMissingRef | undefined,
  layoutDirection: PoomsaeGraphLayoutDirection,
  isDepthContext: boolean,
): PoomsaeGraphNodeData {
  if (!task) {
    const label = missingRef?.uid.slice(-4) || id.slice(-4) || id
    return {
      label,
      title: label,
      idLabel: label,
      projectName: '',
      projectLabel,
      qualifiedLabel: label,
      accessibleLabel: `Uncached linked task ${label}`,
      status: 'uncached',
      priorityLabel: null,
      isSource: false,
      selectable: false,
      isDepthContext,
      layoutDirection,
    }
  }

  return {
    label: task.title,
    title: task.title,
    idLabel: task.short_id,
    projectName: task.project_name || task.project_uid,
    projectLabel,
    qualifiedLabel: task.qualified_id || task.short_id,
    accessibleLabel: [
      task.uid === sourceUID ? 'Source task' : 'Task',
      task.uid === sourceUID ? 'selected' : '',
      task.title,
      task.qualified_id || task.short_id,
      task.status === 'closed'
        ? `closed${task.closed_reason ? ` ${task.closed_reason}` : ''}`
        : 'open',
      isDepthContext ? 'outside depth context' : '',
      adjacentRelation ? `adjacent ${adjacentRelation}` : '',
    ]
      .filter(Boolean)
      .join(', '),
    status: task.status,
    closedReason: task.closed_reason,
    priorityLabel: priorityLabel(task.priority),
    isSource: task.uid === sourceUID,
    selectable: true,
    isDepthContext,
    layoutDirection,
  }
}

function selectedAdjacentRelation(
  edge: PoomsaeGraphEdge,
  nodeID: string,
  selectedUID: string,
): PoomsaeGraphAdjacentRelation {
  if (edge.source !== selectedUID && edge.target !== selectedUID) return null
  if (nodeID !== edge.source && nodeID !== edge.target) return null
  if (nodeID === selectedUID) return null

  if (edge.data?.kind === 'parent') {
    return edge.source === selectedUID ? 'child' : 'parent'
  }
  if (edge.data?.kind === 'blocks') {
    return edge.source === selectedUID ? 'blocks' : 'blockedBy'
  }
  return 'related'
}

function selectedAdjacentRelations(
  edges: readonly PoomsaeGraphEdge[],
  selectedUID: string,
): Map<string, PoomsaeGraphAdjacentRelation> {
  const relations = new Map<string, PoomsaeGraphAdjacentRelation>()
  for (const edge of edges) {
    if (edge.source === selectedUID) {
      mergeAdjacentRelation(
        relations,
        edge.target,
        selectedAdjacentRelation(edge, edge.target, selectedUID),
      )
    } else if (edge.target === selectedUID) {
      mergeAdjacentRelation(
        relations,
        edge.source,
        selectedAdjacentRelation(edge, edge.source, selectedUID),
      )
    }
  }
  return relations
}

function adjacentRelationPriority(relation: PoomsaeGraphAdjacentRelation): number {
  if (relation === 'blockedBy') return 5
  if (relation === 'blocks') return 4
  if (relation === 'parent') return 3
  if (relation === 'child') return 2
  if (relation === 'related') return 1
  return 0
}

function mergeAdjacentRelation(
  relations: Map<string, PoomsaeGraphAdjacentRelation>,
  nodeID: string,
  relation: PoomsaeGraphAdjacentRelation,
): void {
  const current = relations.get(nodeID) ?? null
  if (adjacentRelationPriority(relation) > adjacentRelationPriority(current)) {
    relations.set(nodeID, relation)
  }
}

function layoutNode(
  id: string,
  task: PoomsaeTaskSummary | undefined,
  position: { x: number; y: number },
  sourceUID: string,
  adjacentRelation: PoomsaeGraphAdjacentRelation,
  projectLabel: string,
  missingRef: PoomsaeGraphMissingRef | undefined,
  layoutDirection: PoomsaeGraphLayoutDirection,
  isDepthContext: boolean,
): PoomsaeGraphNode {
  const sourcePosition = layoutDirection === 'TB' ? Position.Bottom : Position.Right
  const targetPosition = layoutDirection === 'TB' ? Position.Top : Position.Left
  return {
    id,
    type: 'poomsaeTask',
    position,
    data: nodeData(
      id,
      task,
      sourceUID,
      adjacentRelation,
      projectLabel,
      missingRef,
      layoutDirection,
      isDepthContext,
    ),
    class: nodeClass(task, sourceUID, isDepthContext),
    draggable: false,
    selectable: task !== undefined,
    sourcePosition,
    targetPosition,
    width: POOMSAE_GRAPH_NODE_WIDTH,
    height: POOMSAE_GRAPH_NODE_HEIGHT,
  }
}

function graphPositions(
  entries: readonly [string, PoomsaeTaskSummary | undefined][],
  edges: readonly PoomsaeGraphEdge[],
  layoutDirection: PoomsaeGraphLayoutDirection,
): Map<string, { x: number; y: number }> {
  const ids = entries.map(([id]) => id)
  const idSet = new Set(ids)
  const stableOrder = new Map(ids.map((id, index) => [id, index]))
  const outgoing = new Map<string, string[]>()
  const indegree = new Map<string, number>()
  for (const id of ids) {
    outgoing.set(id, [])
    indegree.set(id, 0)
  }

  for (const edge of edges) {
    if (!idSet.has(edge.source) || !idSet.has(edge.target)) continue
    outgoing.get(edge.source)!.push(edge.target)
    indegree.set(edge.target, (indegree.get(edge.target) ?? 0) + 1)
  }

  const compareByStableOrder = (left: string, right: string) =>
    (stableOrder.get(left) ?? 0) - (stableOrder.get(right) ?? 0)
  // A min-heap preserves stable ordering without sorting after every insertion.
  const ready = ids.filter((id) => (indegree.get(id) ?? 0) === 0).sort(compareByStableOrder)
  const pushReady = (id: string) => {
    let index = ready.length
    ready.push(id)
    while (index > 0) {
      const parent = Math.floor((index - 1) / 2)
      if (compareByStableOrder(ready[parent]!, id) <= 0) break
      ready[index] = ready[parent]!
      index = parent
    }
    ready[index] = id
  }
  const popReady = (): string => {
    const first = ready[0]!
    const last = ready.pop()!
    if (ready.length > 0) {
      let index = 0
      while (index * 2 + 1 < ready.length) {
        let child = index * 2 + 1
        if (child + 1 < ready.length && compareByStableOrder(ready[child + 1]!, ready[child]!) < 0)
          child++
        if (compareByStableOrder(last, ready[child]!) <= 0) break
        ready[index] = ready[child]!
        index = child
      }
      ready[index] = last
    }
    return first
  }
  const topoOrder: string[] = []

  while (ready.length > 0) {
    const id = popReady()
    topoOrder.push(id)
    for (const target of [...(outgoing.get(id) ?? [])].sort(compareByStableOrder)) {
      const nextIndegree = (indegree.get(target) ?? 0) - 1
      indegree.set(target, nextIndegree)
      if (nextIndegree === 0) {
        pushReady(target)
      }
    }
  }

  const emitted = new Set(topoOrder)
  for (const id of ids) {
    if (!emitted.has(id)) topoOrder.push(id)
  }

  const rankByID = new Map(ids.map((id) => [id, 0]))
  for (const id of topoOrder) {
    const nextRank = (rankByID.get(id) ?? 0) + 1
    for (const target of outgoing.get(id) ?? []) {
      rankByID.set(target, Math.max(rankByID.get(target) ?? 0, nextRank))
    }
  }

  const layers = new Map<number, string[]>()
  for (const id of topoOrder) {
    const rank = rankByID.get(id) ?? 0
    const layer = layers.get(rank)
    if (layer) layer.push(id)
    else layers.set(rank, [id])
  }

  const positions = new Map<string, { x: number; y: number }>()
  for (const [depth, layerIDs] of layers) {
    const layerOffset =
      layoutDirection === 'TB'
        ? -((layerIDs.length - 1) * POOMSAE_GRAPH_X_SPACING) / 2
        : -((layerIDs.length - 1) * POOMSAE_GRAPH_Y_SPACING) / 2
    layerIDs.forEach((id, index) => {
      if (layoutDirection === 'TB') {
        positions.set(id, {
          x: layerOffset + index * POOMSAE_GRAPH_X_SPACING,
          y: depth * POOMSAE_GRAPH_Y_SPACING,
        })
      } else {
        positions.set(id, {
          x: depth * POOMSAE_GRAPH_X_SPACING,
          y: layerOffset + index * POOMSAE_GRAPH_Y_SPACING,
        })
      }
    })
  }
  return positions
}

function visibleProjectLabels(
  entries: readonly [string, PoomsaeTaskSummary | undefined][],
): Map<string, string> {
  const projectUIDs = new Set<string>()
  const shortIDCounts = new Map<string, number>()
  for (const [, task] of entries) {
    if (!task) continue
    projectUIDs.add(task.project_uid)
    shortIDCounts.set(task.short_id, (shortIDCounts.get(task.short_id) ?? 0) + 1)
  }

  const labels = new Map<string, string>()
  for (const [id, task] of entries) {
    if (!task) continue
    if (projectUIDs.size <= 1 && (shortIDCounts.get(task.short_id) ?? 0) <= 1) continue
    labels.set(id, task.project_name || task.project_uid)
  }
  return labels
}

function missingProjectLabel(
  ref: PoomsaeGraphMissingRef | undefined,
  projectLabels: Map<string, string>,
): string {
  return projectLabels.size > 0 ? (ref?.uid.slice(-4) ?? '') : ''
}

function graphEdgeData(
  edge: PoomsaeGraphEdge,
  kind: PoomsaeReachableGraphEdgeKind,
  overrides: Partial<Pick<PoomsaeGraphEdgeData, 'isDepthContext' | 'isSelectedAdjacent'>>,
): PoomsaeGraphEdgeData {
  return {
    kind,
    layout: edge.data?.layout ?? true,
    ...(edge.data?.isDepthContext !== undefined
      ? { isDepthContext: edge.data.isDepthContext }
      : {}),
    ...(edge.data?.isSelectedAdjacent !== undefined
      ? { isSelectedAdjacent: edge.data.isSelectedAdjacent }
      : {}),
    ...overrides,
  }
}

function depthContextEdge(edge: PoomsaeGraphEdge): PoomsaeGraphEdge {
  const kind = edge.data?.kind
  if (!kind) return edge
  const className = typeof edge.class === 'string' ? edge.class : ''
  const next: PoomsaeGraphEdge = {
    ...edge,
    class: `${className} poomsae-graph-edge--depth-context`.trim(),
    data: graphEdgeData(edge, kind, { isDepthContext: true }),
    zIndex: 0,
  }
  if (!edge.markerEnd || typeof edge.markerEnd !== 'object') return next
  return {
    ...next,
    markerEnd: { ...edge.markerEnd, color: 'var(--poomsae-graph-edge-context)' },
  } as PoomsaeGraphEdge
}

function ambientActiveEdge(edge: PoomsaeGraphEdge): PoomsaeGraphEdge {
  const kind = edge.data?.kind
  if (!kind) return edge
  const className = typeof edge.class === 'string' ? edge.class : ''
  return {
    ...edge,
    class: `${className} poomsae-graph-edge--ambient`.trim(),
    data: graphEdgeData(edge, kind, { isSelectedAdjacent: false }),
    zIndex: 1,
  }
}

function selectedActiveEdge(edge: PoomsaeGraphEdge): PoomsaeGraphEdge {
  const kind = edge.data?.kind
  if (!kind) return edge
  const className = typeof edge.class === 'string' ? edge.class : ''
  const next: PoomsaeGraphEdge = {
    ...edge,
    class: `${className} poomsae-graph-edge--selected-adjacent`.trim(),
    data: graphEdgeData(edge, kind, { isSelectedAdjacent: true }),
    zIndex: 4,
  }
  if (!edge.markerEnd || typeof edge.markerEnd !== 'object') return next
  return {
    ...next,
    markerEnd: {
      ...edge.markerEnd,
      color:
        kind === 'blocks'
          ? 'var(--poomsae-graph-edge-selected-blocks)'
          : 'var(--poomsae-graph-edge-selected)',
    },
  } as PoomsaeGraphEdge
}

function activeEdge(edge: PoomsaeGraphEdge, selectedUID: string | null): PoomsaeGraphEdge {
  if (selectedUID && (edge.source === selectedUID || edge.target === selectedUID)) {
    return selectedActiveEdge(edge)
  }
  return ambientActiveEdge(edge)
}

function visibleContextIDs(
  rootID: string,
  edges: readonly PoomsaeGraphEdge[],
  maxDepth: number,
): { nodeIDs: Set<string>; edgeIDs: Set<string> } {
  const nodeIDs = new Set<string>([rootID])
  const edgeIDs = new Set<string>()
  const distances = new Map<string, number>([[rootID, 0]])
  const adjacency = new Map<string, PoomsaeGraphEdge[]>()
  for (const edge of edges) {
    const sourceEdges = adjacency.get(edge.source)
    if (sourceEdges) sourceEdges.push(edge)
    else adjacency.set(edge.source, [edge])
    const targetEdges = adjacency.get(edge.target)
    if (targetEdges) targetEdges.push(edge)
    else adjacency.set(edge.target, [edge])
  }

  const queued = [rootID]
  for (let head = 0; head < queued.length; head++) {
    const id = queued[head]!
    const distance = distances.get(id) ?? 0
    if (distance >= maxDepth) continue
    for (const edge of adjacency.get(id) ?? []) {
      const nextID = edge.source === id ? edge.target : edge.source
      edgeIDs.add(edge.id)
      if (distances.has(nextID)) continue
      distances.set(nextID, distance + 1)
      nodeIDs.add(nextID)
      queued.push(nextID)
    }
  }

  return { nodeIDs, edgeIDs }
}

function compareGraphEdges(left: PoomsaeGraphEdge, right: PoomsaeGraphEdge): number {
  const leftDepthContext = left.data?.isDepthContext ? 0 : 1
  const rightDepthContext = right.data?.isDepthContext ? 0 : 1
  return (
    leftDepthContext - rightDepthContext ||
    left.id.localeCompare(right.id, undefined, { numeric: true, sensitivity: 'base' })
  )
}

function nodeEntriesFromGraph(
  graph: PoomsaeReachableGraphResponse,
  edges: readonly PoomsaeGraphEdge[],
): [string, PoomsaeTaskSummary | undefined][] {
  const nodeTasks = new Map<string, PoomsaeTaskSummary | undefined>()
  for (const task of graph.nodes) {
    nodeTasks.set(task.uid, task)
  }
  for (const edge of edges) {
    if (!nodeTasks.has(edge.source)) nodeTasks.set(edge.source, undefined)
    if (!nodeTasks.has(edge.target)) nodeTasks.set(edge.target, undefined)
  }
  return [...nodeTasks.entries()].sort(compareGraphNodeEntries)
}

export function buildKataReachableGraph(input: BuildKataReachableGraphInput): {
  nodes: PoomsaeGraphNode[]
  edges: PoomsaeGraphEdge[]
  layoutEdges: PoomsaeGraphEdge[]
  missingRefs: PoomsaeGraphMissingRef[]
} {
  const graph = input.graph
  if (!graph) return { nodes: [], edges: [], layoutEdges: [], missingRefs: [] }

  const layoutDirection = input.layoutDirection ?? 'LR'
  const sourceUID = graph.source_uid || input.sourceUID
  const rawEdges = dedupeEdges(graph.edges.map(makeEdge))
  const visibleNodeEntries = nodeEntriesFromGraph(graph, rawEdges)
  const visibleIDs = new Set(visibleNodeEntries.map(([id]) => id))
  const visibleBaseEdges = rawEdges.filter(
    (edge) => visibleIDs.has(edge.source) && visibleIDs.has(edge.target),
  )
  const hasDepthContext = input.contextDepth !== undefined && input.contextDepth !== 'all'
  const visibleContext = hasDepthContext
    ? visibleContextIDs(sourceUID, visibleBaseEdges, maxContextDepth(input.contextDepth))
    : { nodeIDs: visibleIDs, edgeIDs: new Set(visibleBaseEdges.map((edge) => edge.id)) }
  const visibleEdges = visibleBaseEdges
    .map((edge) =>
      hasDepthContext &&
      (!visibleContext.edgeIDs.has(edge.id) ||
        !visibleContext.nodeIDs.has(edge.source) ||
        !visibleContext.nodeIDs.has(edge.target))
        ? depthContextEdge(edge)
        : activeEdge(edge, sourceUID),
    )
    .sort(compareGraphEdges)
  const layoutEdges = visibleBaseEdges
    .filter((edge) => edge.data?.layout !== false)
    .sort(compareGraphEdges)
  const positions = graphPositions(visibleNodeEntries, layoutEdges, layoutDirection)
  const projectLabels = visibleProjectLabels(visibleNodeEntries)
  const adjacentRelations = selectedAdjacentRelations(visibleEdges, sourceUID)
  const missingRefs = missingRefsByUID(graph.unresolved_refs)
  const nodes = visibleNodeEntries.map(([id, task]) =>
    layoutNode(
      id,
      task,
      positions.get(id) ?? { x: 0, y: 0 },
      sourceUID,
      adjacentRelations.get(id) ?? null,
      task
        ? (projectLabels.get(id) ?? '')
        : missingProjectLabel(missingRefs.get(id), projectLabels),
      missingRefs.get(id),
      layoutDirection,
      hasDepthContext && !visibleContext.nodeIDs.has(id),
    ),
  )

  return {
    nodes,
    edges: visibleEdges,
    layoutEdges,
    missingRefs: [...graph.unresolved_refs]
      .sort((left, right) =>
        missingRefKey(left).localeCompare(missingRefKey(right), undefined, { numeric: true }),
      )
      .map((ref) => ({
        uid: ref.uid,
        side: ref.side,
        kind: ref.kind,
        otherUID: ref.other_uid,
      })),
  }
}
