export interface PoomsaeAttachment {
  name: string
  media_type: string
  size: number
  docbank_version: string
  origin?: string | undefined
}

export interface PoomsaeChecklistItem {
  id: string
  text: string
  done: boolean
}

/** Server-derived creation projection; source labels alone are not proof. */
export interface PoomsaeCreationAttributionWire {
  verification?: string | undefined
  accountable_actor?: string | undefined
  source_actor?: string | undefined
  teammate?: string | undefined
  authority_uid?: string | undefined
}

export interface PoomsaeCreationAttributionModel {
  verification: 'verified' | 'pending' | 'legacy'
  sourceActor: string
  teammate?: string
  accountableActor?: string
}

export interface PoomsaeIssueWire extends PoomsaeCreationAttributionWire {
  uid: string
  project_uid?: string | undefined
  project_name?: string | undefined
  short_id?: string | undefined
  qualified_id?: string | undefined
  title: string
  author?: string | undefined
  body?: string | undefined
  status: string
  owner?: string | undefined
  priority?: number | undefined
  metadata?:
    | {
        scheduled_on?: string | undefined
        deadline_on?: string | undefined
        checklist?: PoomsaeChecklistItem[] | undefined
        [key: string]: unknown
      }
    | undefined
  labels?: string[] | null | undefined
  updated_at?: string | undefined
}

export interface PoomsaeIssueReferenceWire {
  uid: string
  short_id?: string | undefined
  qualified_id?: string | undefined
  title?: string | undefined
  status?: string | undefined
}

export type PoomsaeReplyKind = 'reply' | 'confirm' | 'refute' | 'supersede'
export interface PoomsaeCommentLink {
  uid?: string | undefined
  handle?: string | undefined
  kind: string
  author?: string | undefined
  teammate?: string | undefined
  issue_uid?: string | undefined
  issue_short_id?: string | undefined
  project_uid?: string | undefined
  project_id?: number | undefined
  status?: string | undefined
  target_edited?: boolean | undefined
  body?: string | undefined
  created_at?: string | undefined
  edited_at?: string | undefined
}

export interface PoomsaeIssueDetailWire {
  issue: PoomsaeIssueWire
  comments?:
    | Array<
        PoomsaeCreationAttributionWire & {
          id: number
          uid?: string | undefined
          handle?: string | undefined
          reply?: PoomsaeCommentLink | undefined
          backlinks?: PoomsaeCommentLink[] | undefined
          backlinks_truncated?: boolean | undefined
          edited_at?: string | undefined
          author: string
          teammate?: string | undefined
          body: string
          created_at: string
        }
      >
    | null
    | undefined
  labels?: Array<{ label: string }> | null | undefined
  links?:
    | Array<{
        id: number
        from: PoomsaeIssueReferenceWire
        to: PoomsaeIssueReferenceWire
        type: string
      }>
    | null
    | undefined
  parent?: PoomsaeIssueReferenceWire | null | undefined
  children?: PoomsaeIssueWire[] | null | undefined
  claim?:
    | {
        holder: string
        claim_kind?: string | undefined
        purpose?: string | undefined
      }
    | null
    | undefined
  pending_claims?:
    | Array<{
        holder: string
        claim_kind?: string | undefined
        purpose?: string | undefined
      }>
    | null
    | undefined
}

export interface PoomsaeIssueReferenceModel {
  uid: string
  reference: string
  title: string
  status: string
}

export interface PoomsaeIssueDetailModel {
  issue: {
    uid: string
    projectUID: string
    projectName: string
    reference: string
    title: string
    body: string
    status: string
    owner?: string
    priority?: number
    scheduledOn?: string
    deadlineOn?: string
    attachments?: Record<string, PoomsaeAttachment>
    checklist: PoomsaeChecklistItem[]
    labels: string[]
    updatedAt?: string
    creation?: PoomsaeCreationAttributionModel
  }
  comments: Array<{
    id: string
    uid?: string
    handle?: string
    reply?: PoomsaeCommentLink
    backlinksPartial?: boolean
    backlinks?: PoomsaeCommentLink[]
    editedAt?: string
    author: string
    teammate?: string
    body: string
    createdAt: string
    creation?: PoomsaeCreationAttributionModel
  }>
  links: Array<{
    id: string
    relation: string
    peerUID: string
    peerReference: string
    peerStatus: string
  }>
  parent?: PoomsaeIssueReferenceModel
  children: PoomsaeIssueReferenceModel[]
  claim?: { holder: string; kind: string; purpose: string }
  pendingClaims: Array<{ holder: string; kind: string; purpose: string }>
}

export interface PoomsaeIssueHostAction {
  id: string
  label: string
  disabled?: boolean
  busy?: boolean
  invoke: () => void | Promise<void>
}

export interface PoomsaeIssueDetailProps {
  attachmentBaseURL?: string | undefined
  fetchAttachment?: ((url: string) => Promise<Blob>) | undefined
  uploadAttachment?: ((file: File) => Promise<void>) | undefined
  attachmentsEnabled?: boolean | undefined
  actionsDisabled?: boolean | undefined
  detail: PoomsaeIssueDetailModel
  actions?: readonly PoomsaeIssueHostAction[]
  /** Host navigation for parent, child, and linked issue references. */
  onOpenIssue?: (uid: string) => void
  onOpenComment?: (
    issueUID: string,
    commentUID: string,
    source?: { issueUID: string; commentUID: string },
  ) => void
  returnComment?: { issueUID: string; commentUID: string } | undefined
  onReplyComment?: (commentUID: string, kind: PoomsaeReplyKind) => void
  commentActionsDisabled?: boolean
  selectedCommentUID?: string | undefined
  /** True while the host shows a summary and loads the comments. */
  pending?: boolean | undefined
}
