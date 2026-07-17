export type ChangeReviewDecision = 'accept' | 'reject'
export type ChangeReviewStatus = 'pending' | 'accepted' | 'rejected' | 'mixed'
export type ChangeApplyState = 'prepared' | 'applied' | 'reverted' | 'conflicted'

export interface WorkspaceChangeHunk {
  id: string
  before_start?: number
  before_end?: number
  after_start?: number
  after_end?: number
  before?: string
  after?: string
  old_string?: string
  new_string?: string
  review_status?: ChangeReviewStatus
}

export interface WorkspaceChangeEdit {
  id: string
  old_string?: string
  new_string?: string
  replacements?: number
  replace_all?: boolean
  review_status?: ChangeReviewStatus
  hunks?: WorkspaceChangeHunk[]
}

export interface WorkspaceChangeSet {
  id: string
  sequence?: number
  group_id: string
  path: string
  base_revision?: string
  revision?: string
  before_blob?: string
  after_blob?: string
  before_exists?: boolean
  after_exists?: boolean
  before_content?: string
  after_content?: string
  edits?: WorkspaceChangeEdit[]
  review_status: ChangeReviewStatus
  apply_state: ChangeApplyState
  reverts_id?: string
  replays_id?: string
  created_at: string
  origin?: string
  review_thread_id?: string
  run_id?: string
  session_id?: string
  tool_call_id?: string
}

export interface WorkspaceChangeCommentAnchor {
  kind?: string
  /** Snapshot side that owns the byte range. */
  side?: 'before' | 'after'
  /** Start/end are UTF-8 byte offsets, never JavaScript UTF-16 offsets. */
  encoding?: 'utf8-bytes-v1'
  revision?: string
  start?: number
  end?: number
  quote?: string
  prefix?: string
  suffix?: string
}

export interface WorkspaceChangeComment {
  /** Canonical workspace returned by the comment mutation envelope. */
  workspace?: string
  id: string
  group_id: string
  change_set_id?: string
  edit_id?: string
  hunk_id?: string
  body: string
  author?: string
  resolved?: boolean
  deleted?: boolean
  anchor?: WorkspaceChangeCommentAnchor
  /** Derived UI metadata for Agent feedback chips; not required in the ledger. */
  review_path?: string
  /** One-based line derived from the authoritative UTF-8 anchor. */
  review_line?: number
  created_at?: string
  updated_at?: string
}

export interface WorkspaceChangeGroup {
  id: string
  origin?: string
  review_thread_id?: string
  run_id?: string
  session_id?: string
  created_at: string
  review_status: ChangeReviewStatus
  apply_state: ChangeApplyState
  change_sets: WorkspaceChangeSet[]
  comments?: WorkspaceChangeComment[]
  can_undo?: boolean
  can_redo?: boolean
  pending_edit_count?: number
  unresolved_comment_count?: number
}

export interface WorkspaceChangeGroupSummary {
  id: string
  origin?: string
  review_thread_id?: string
  run_id?: string
  session_id?: string
  created_at: string
  review_status: ChangeReviewStatus
  apply_state: ChangeApplyState
  change_sets?: WorkspaceChangeSet[]
  change_set_count?: number
  paths?: string[]
  unresolved_comment_count?: number
  can_undo?: boolean
  can_redo?: boolean
  pending_edit_count?: number
}

export type ReviewThreadContinuity = 'continuous' | 'discontinuous' | 'conflicted'

/** Server-composed cumulative file projection for a review thread. */
export interface ReviewThreadFile {
  path: string
  before_content: string
  after_content: string
  base_revision: string
  revision: string
  base_group_id: string
  base_change_set_id: string
  latest_group_id: string
  latest_change_set_id: string
  group_ids: string[]
  change_set_ids: string[]
  pending_edit_ids: string[]
  review_status: ChangeReviewStatus
  apply_state: ChangeApplyState
  continuity: ReviewThreadContinuity
  /** Earlier, non-contiguous changes excluded from the displayed snapshot. */
  omitted_iteration_count?: number
  before_exists?: boolean
  after_exists?: boolean
  additions?: number
  deletions?: number
}

/** Durable review identity spanning one or more independent Agent runs. */
export interface ReviewThread {
  id: string
  latest_group_id: string
  groups: WorkspaceChangeGroupSummary[]
  comments: WorkspaceChangeComment[]
  files: ReviewThreadFile[]
  created_at?: string
  updated_at?: string
  review_status?: ChangeReviewStatus
  apply_state?: ChangeApplyState
  pending_edit_count?: number
  unresolved_comment_count?: number
}

export interface WorkspaceChangeEvent {
  /** Canonical workspace identity emitted by the backend. */
  workspace?: string
  change_group_id?: string
  group_id?: string
  change_set_id?: string
  path?: string
  paths?: string[]
  affected_paths?: string[]
  action?: string
}

export function isWorkspaceChangeForWorkspace(event: Pick<WorkspaceChangeEvent, 'workspace'> | null | undefined, workspace: string): boolean {
  // Once a workspace is active, identity-less events are unsafe: they may be a
  // late receipt from the previously active workspace.
  return workspace ? event?.workspace === workspace : !event?.workspace
}

export interface ReviewWorkspaceChangeRequest {
  decision: ChangeReviewDecision
  change_set_id?: string
  edit_ids?: string[]
  base_revision?: string
}

export interface CreateWorkspaceChangeCommentRequest {
  group_id: string
  change_set_id?: string
  edit_id?: string
  hunk_id?: string
  body: string
  anchor?: WorkspaceChangeCommentAnchor
}

export interface WorkspaceChangeMutationResult {
  /** Canonical workspace that held the server-side mutation lease. */
  workspace?: string
  group?: WorkspaceChangeGroup
  change_group?: WorkspaceChangeGroup
  affected_paths?: string[]
  paths?: string[]
  path?: string
  message?: string
}

export function groupPaths(group?: Pick<WorkspaceChangeGroup, 'change_sets'> | null): string[] {
  if (!group) return []
  return Array.from(new Set(group.change_sets.map((changeSet) => changeSet.path).filter(Boolean)))
}
