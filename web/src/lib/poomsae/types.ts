import type {
  PoomsaeCommentLink,
  PoomsaeCreationAttributionWire,
  PoomsaeReplyKind,
} from '@ifigueroap/poomsae-ui'

export type PoomsaeTaskViewName = 'inbox' | 'today' | 'delegated' | 'scheduled' | 'all' | 'logbook'

export interface PoomsaeTaskChecklistItem {
  id: string
  text: string
  done: boolean
}

export interface PoomsaeTaskMetadata {
  scheduled_on?: string | undefined
  deadline_on?: string | undefined
  today_bucket?: 'day' | 'evening' | undefined
  checklist?: PoomsaeTaskChecklistItem[] | undefined
  area?: string | undefined
  [key: string]: unknown
}

export interface PoomsaeProjectMetadata {
  area?: string | undefined
  sidebar_order?: number | undefined
  icon?: string | undefined
  timezone?: string | undefined
  role?: string | undefined
  [key: string]: unknown
}

export interface PoomsaeProjectSummary {
  id: number
  uid: string
  name: string
  metadata: PoomsaeProjectMetadata
  revision?: number | undefined
  created_at?: string | undefined
  deleted_at?: string | undefined
  open_count: number
}

export interface PoomsaeLinkPeer {
  uid: string
  short_id: string
}

export interface PoomsaeOpenBlocker extends PoomsaeLinkPeer {
  qualified_id: string
}

export interface PoomsaeTaskLinkPeer extends PoomsaeLinkPeer {
  qualified_id: string
  status: 'open' | 'closed'
}

export interface PoomsaeTaskSummary extends PoomsaeCreationAttributionWire {
  id: number
  uid: string
  project_id: number
  short_id: string
  qualified_id: string
  title: string
  body?: string | undefined
  status: 'open' | 'closed'
  project_uid: string
  project_name: string
  scheduled_on_date?: string | undefined
  deadline_on_date?: string | undefined
  metadata: PoomsaeTaskMetadata
  revision: number
  content_revision?: number | undefined
  owner?: string | undefined
  assignment_expires_on?: string | undefined
  author: string
  priority?: number | undefined
  labels?: string[] | undefined
  parent?: PoomsaeLinkPeer | undefined
  parent_short_id?: string | undefined
  blocks?: PoomsaeLinkPeer[] | undefined
  blocked_by?: PoomsaeLinkPeer[] | undefined
  open_blocked_by?: PoomsaeOpenBlocker[] | undefined
  related?: PoomsaeLinkPeer[] | undefined
  child_counts?: { open: number; total: number } | undefined
  recurrence_id?: number | undefined
  occurrence_key?: string | undefined
  created_at: string
  updated_at: string
  closed_reason?: 'done' | 'wontfix' | 'duplicate' | 'superseded' | 'audit-no-change' | undefined
  closed_at?: string | undefined
  deleted_at?: string | undefined
}

export interface PoomsaeTaskGroup {
  id: string
  title: string
  issues: PoomsaeTaskSummary[]
  handoff?: { teammate: string; author: string }
}

export interface PoomsaeTaskViewResponse {
  view: PoomsaeTaskViewName
  groups: PoomsaeTaskGroup[]
  fetched_at: string
}

export type PoomsaeTaskStatusFilter = 'open' | 'ready' | 'closed' | 'all'
export type PoomsaeTaskSearchScope = { kind: 'all' } | { kind: 'project'; project_uid: string }

export interface PoomsaeTaskSearchFilters {
  scope: PoomsaeTaskSearchScope
  status: PoomsaeTaskStatusFilter
  owner: string
  label: string
  query: string
  relationships?: string[] | undefined
}

export type PoomsaeReachableGraphDepth = 'full' | '1' | '2' | '3'
export type PoomsaeReachableGraphEdgeKind = 'parent' | 'blocks' | 'related'

export interface PoomsaeReachableGraphEdge {
  from_uid: string
  to_uid: string
  kind: PoomsaeReachableGraphEdgeKind
  layout: boolean
}

export interface PoomsaeReachableGraphUnresolvedRef {
  uid: string
  side: 'from' | 'to'
  kind: PoomsaeReachableGraphEdgeKind
  other_uid: string
}

export interface PoomsaeReachableGraphResponse {
  source_uid: string
  depth: PoomsaeReachableGraphDepth
  hide_done: boolean
  nodes: PoomsaeTaskSummary[]
  edges: PoomsaeReachableGraphEdge[]
  unresolved_refs: PoomsaeReachableGraphUnresolvedRef[]
  fetched_at: string
}

export interface PoomsaeCommentReplyIntent {
  replyTo: string
  kind: PoomsaeReplyKind
  force: boolean
}

export interface PoomsaeComment extends PoomsaeCreationAttributionWire {
  uid?: string
  handle?: string
  reply?: PoomsaeCommentLink
  backlinks_truncated?: boolean
  backlinks?: PoomsaeCommentLink[]
  edited_at?: string
  id: number
  issue_id: number
  author: string
  body: string
  created_at: string
}

export interface PoomsaeTaskLabel {
  issue_id: number
  label: string
  author: string
  created_at: string
}

export interface PoomsaeTaskLink {
  id: number
  project_id: number
  from: PoomsaeTaskLinkPeer
  to: PoomsaeTaskLinkPeer
  type: 'parent' | 'blocks' | 'related'
  author: string
  created_at: string
}

export interface PoomsaeTaskRef {
  uid: string
  short_id: string
  qualified_id: string
  title: string
  status: 'open' | 'closed'
}

export interface PoomsaeTaskDetail {
  issue: PoomsaeTaskSummary & {
    body: string
  }
  etag?: string | undefined
  comments: PoomsaeComment[]
  labels: PoomsaeTaskLabel[]
  links: PoomsaeTaskLink[]
  effective_close_event?: PoomsaeTaskEvent | undefined
  parent?: PoomsaeTaskRef | undefined
  children?: PoomsaeTaskSummary[] | undefined
  // Accepted snapshot enrichment carries the workspace action atomically
  // with selected task detail.
  workspace_target?: PoomsaeWorkspaceTarget | undefined
}

export type PoomsaeWorkspaceTarget = Record<string, unknown>

export interface PoomsaeTaskEvent {
  event_id: number
  event_uid: string
  origin_instance_uid: string
  type: string
  project_id: number
  project_uid: string
  project_name: string
  issue_id?: number | undefined
  issue_uid?: string | undefined
  issue_short_id?: string | undefined
  related_issue_id?: number | undefined
  related_issue_uid?: string | undefined
  related_issue_short_id?: string | undefined
  actor: string
  payload?: Record<string, unknown> | undefined
  created_at: string
}

export interface PoomsaeRecurrence {
  id: number
  uid: string
  project_id: number
  rrule: string
  dtstart: string
  timezone: string
  template_title: string
  template_body: string
  template_owner?: string | undefined
  template_priority?: number | undefined
  template_labels: string[]
  template_metadata: PoomsaeTaskMetadata
  next_occurrence_key?: string | undefined
  last_materialized_uid?: string | undefined
  author: string
  revision: number
  created_at: string
  updated_at: string
  deleted_at?: string | undefined
}

export interface PoomsaeRecurrencesResponse {
  recurrences: PoomsaeRecurrence[]
  fetched_at: string
}

export interface PoomsaeRecurrenceTemplateInput {
  title: string
  body?: string | undefined
  owner?: string | undefined
  priority?: number | undefined
  labels?: string[] | undefined
  metadata?: PoomsaeTaskMetadata | undefined
}

export interface PoomsaeCreateRecurrenceInput {
  actor: string
  initialIssueRef: string
  rrule: string
  dtstart: string
  timezone: string
  template: PoomsaeRecurrenceTemplateInput
}

export interface PoomsaeRecurrenceTemplateUpdateInput {
  title?: string | undefined
  body?: string | undefined
  owner?: string | undefined
  clearOwner?: boolean | undefined
  priority?: number | undefined
  clearPriority?: boolean | undefined
  labels?: string[] | undefined
  metadata?: PoomsaeTaskMetadata | undefined
}

export interface PoomsaePatchRecurrenceInput {
  actor: string
  rrule?: string | undefined
  dtstart?: string | undefined
  timezone?: string | undefined
  template?: PoomsaeRecurrenceTemplateUpdateInput | undefined
}

export interface PoomsaeRecurrenceResponse {
  recurrence: PoomsaeRecurrence
  etag?: string | undefined
  changed?: boolean | undefined
}

export interface PoomsaeTaskMutationTarget {
  project_id: number
  ref: string
}

export interface PoomsaeTaskCreateDraft {
  title: string
  body?: string | undefined
  owner?: string | undefined
  priority?: number | undefined
  labels?: string[] | undefined
  metadata?: PoomsaeTaskMetadata | undefined
  force_new?: boolean | undefined
}

export interface PoomsaeTaskEditPatch {
  title?: string | undefined
  body?: string | undefined
  expected_content_revision?: number | undefined
  links_delta?: PoomsaeTaskLinkDelta | undefined
}

export interface PoomsaeTaskLinkDelta {
  add_related?: string[] | undefined
  add_blocks?: string[] | undefined
  add_blocked_by?: string[] | undefined
  set_parent?: string | null | undefined
  remove?: string[] | undefined
}

export type PoomsaeTaskMetadataPatch = Record<string, unknown>

export interface PoomsaeTaskCloseOptions {
  reason?: 'done' | 'wontfix' | 'duplicate' | 'superseded' | 'audit-no-change' | undefined
  message?: string | undefined
  source?: string | undefined
  evidence?: import('../api/generated').Evidence[] | undefined
}

export interface PoomsaeTaskCloseRequest {
  reason: 'done' | 'wontfix' | 'duplicate' | 'superseded'
  message: string
  evidence: import('../api/generated').Evidence[]
}

export interface PoomsaeTaskMutationResponse {
  changed: boolean
}

export interface PoomsaePinnedDaemonOptions {
  daemonId: string
}

export interface PoomsaePinnedDaemonRequestOptions extends PoomsaePinnedDaemonOptions {
  signal?: AbortSignal | undefined
}

export interface PoomsaeTaskAPI {
  createProject(
    name: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  createIssue(
    projectID: number,
    actor: string,
    draft: PoomsaeTaskCreateDraft,
    options: PoomsaePinnedDaemonOptions,
    idempotencyKey?: string | undefined,
  ): Promise<PoomsaeTaskMutationResponse>
  addComment(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    body: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  addLabel(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    label: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  removeLabel(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    label: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  assignOwner(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    owner: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  unassignOwner(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  setPriority(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    priority: number | null,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  closeIssue(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    close: PoomsaeTaskCloseOptions,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  reopenIssue(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  editIssue(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    patch: PoomsaeTaskEditPatch,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  patchIssueMetadata(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    patch: PoomsaeTaskMetadataPatch,
    ifMatch: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  moveIssue(
    target: PoomsaeTaskMutationTarget,
    actor: string,
    toProjectUID: string,
    ifMatch: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeTaskMutationResponse>
  recurrences(
    projectID: number,
    options: PoomsaePinnedDaemonRequestOptions,
  ): Promise<PoomsaeRecurrencesResponse>
  createRecurrence(
    projectID: number,
    input: PoomsaeCreateRecurrenceInput,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeRecurrenceResponse>
  showRecurrence(
    projectID: number,
    recurrenceUID: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeRecurrenceResponse>
  patchRecurrence(
    projectID: number,
    recurrenceUID: string,
    patch: PoomsaePatchRecurrenceInput,
    ifMatch: string,
    options: PoomsaePinnedDaemonOptions,
  ): Promise<PoomsaeRecurrenceResponse>
  deleteRecurrence(
    projectID: number,
    recurrenceUID: string,
    actor: string,
    options: PoomsaePinnedDaemonOptions,
    ifMatch?: string,
  ): Promise<void>
}
