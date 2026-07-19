export interface WorkspaceFileConflictSnapshot {
  workspace: string
  content: string
  revision: string
}

/**
 * Carries the canonical file snapshot loaded after a workspace CAS failure.
 * The editor adapter can then rebase even when the affected file is no longer active.
 */
export class WorkspaceFileRevisionConflictError extends Error {
  readonly latest: WorkspaceFileConflictSnapshot
  readonly originalError: unknown

  constructor(originalError: unknown, latest: WorkspaceFileConflictSnapshot) {
    super(originalError instanceof Error ? originalError.message : 'Workspace file revision conflict')
    this.name = 'WorkspaceFileRevisionConflictError'
    this.latest = latest
    this.originalError = originalError
  }
}
