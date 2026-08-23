/** Minimal immutable document pair rendered by the shared multi-file Diff surface. */
export interface DiffFileDocument {
  path: string
  before_content: string
  after_content: string
  base_revision: string
  revision: string
  before_exists?: boolean
  after_exists?: boolean
  additions?: number
  deletions?: number
  binary?: boolean
}

export type DiffFileKind = 'added' | 'modified' | 'deleted'
export type DiffLayout = 'unified' | 'split'

export interface DiffFileNavigationItem {
  path: string
  kind: DiffFileKind
  conflicted?: boolean
  accepted?: boolean
}

export function diffFileKind(file: Pick<DiffFileDocument, 'before_exists' | 'after_exists'>): DiffFileKind {
  if (file.after_exists === false) return 'deleted'
  if (file.before_exists === false) return 'added'
  return 'modified'
}
