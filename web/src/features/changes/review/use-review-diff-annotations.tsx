import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import type { DiffLineAnnotation, SelectedLineRange } from '@pierre/diffs/react'
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineCommentThread } from '@/components/review/InlineCommentThread'
import type { CodeDiffSurfaceFile } from '@/features/diff/CodeDiffSurface'
import type { DiffFileDocument } from '@/features/diff/types'
import { logWorkspaceChangeError } from '../errors'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThreadFile,
  WorkspaceChangeComment,
  WorkspaceChangeCommentAnchor,
} from '../types'
import { Utf8OffsetIndex } from './utf8-offset-index'

const MAX_QUOTE_BYTES = 16 * 1024
const CONTEXT_BYTES = 256

interface ResolvedCommentThread {
  key: string
  side: 'before' | 'after'
  start: number
  end: number
  comments: WorkspaceChangeComment[]
}

interface CommentDraft {
  key: string
  path: string
  side: 'before' | 'after'
  lineNumber: number
  anchor: WorkspaceChangeCommentAnchor
}

export type ReviewDiffAnnotation =
  | { kind: 'thread'; thread: ResolvedCommentThread }
  | { kind: 'outdated'; key: string; comments: WorkspaceChangeComment[] }
  | { kind: 'draft'; key: string }
  | { kind: 'conflict'; key: string }

interface UseReviewDiffAnnotationsOptions {
  identity: string
  files: readonly ReviewThreadFile[]
  comments: readonly WorkspaceChangeComment[]
  busy: boolean
  onCreate: (request: CreateWorkspaceChangeCommentRequest) => Promise<void>
  onUpdate: (comment: WorkspaceChangeComment, body: string) => Promise<void>
  onDelete: (comment: WorkspaceChangeComment) => Promise<void>
}

/** Owns durable comment anchors and local draft state independently of virtualized rows. */
export function useReviewDiffAnnotations({ identity, files, comments, busy, onCreate, onUpdate, onDelete }: UseReviewDiffAnnotationsOptions) {
  const { t } = useTranslation()
  const [drafts, setDrafts] = useState<CommentDraft[]>([])
  const [draftBodies, setDraftBodies] = useState<Readonly<Record<string, string>>>(() => ({}))
  const [submittingDraftKeys, setSubmittingDraftKeys] = useState<ReadonlySet<string>>(() => new Set())
  const [editingThreadKeys, setEditingThreadKeys] = useState<ReadonlySet<string>>(() => new Set())
  const draftsRef = useRef(drafts)
  const draftBodiesRef = useRef(draftBodies)
  const busyRef = useRef(busy)
  draftsRef.current = drafts
  draftBodiesRef.current = draftBodies
  busyRef.current = busy

  useEffect(() => {
    setDrafts([])
    setDraftBodies({})
    setSubmittingDraftKeys(new Set())
    setEditingThreadKeys(new Set())
  }, [identity])

  const filesByPath = useMemo(() => new Map(files.map((file) => [file.path, file])), [files])
  const resolvedByPath = useMemo(() => new Map(files.map((file) => [
    file.path,
    resolveCommentThreads(file, commentsForFile(file, comments)),
  ])), [comments, files])

  const annotationsByPath = useMemo(() => {
    const result = new Map<string, readonly DiffLineAnnotation<ReviewDiffAnnotation>[]>()
    for (const file of files) {
      const annotations: DiffLineAnnotation<ReviewDiffAnnotation>[] = []
      const resolved = resolvedByPath.get(file.path)
      for (const thread of resolved?.threads ?? []) {
        const index = new Utf8OffsetIndex(thread.side === 'before' ? file.before_content : file.after_content)
        annotations.push({
          side: thread.side === 'before' ? 'deletions' : 'additions',
          lineNumber: index.positionAtByteOffset(thread.end || thread.start).lineNumber,
          metadata: { kind: 'thread', thread },
        })
      }
      if (resolved?.outdated.length) {
        annotations.push({
          side: 'additions',
          lineNumber: 0,
          metadata: { kind: 'outdated', key: `outdated:${file.path}`, comments: resolved.outdated },
        })
      }
      if (file.continuity !== 'continuous' || file.apply_state === 'conflicted') {
        annotations.push({
          side: 'additions',
          lineNumber: 0,
          metadata: { kind: 'conflict', key: `conflict:${file.path}` },
        })
      }
      for (const draft of drafts) {
        if (draft.path !== file.path) continue
        annotations.push({
          side: draft.side === 'before' ? 'deletions' : 'additions',
          lineNumber: draft.lineNumber,
          metadata: { kind: 'draft', key: draft.key },
        })
      }
      if (annotations.length) result.set(file.path, annotations)
    }
    return result
  }, [drafts, files, resolvedByPath])

  const annotationRevisionByPath = useMemo(() => {
    const result = new Map<string, string>()
    for (const [path, annotations] of annotationsByPath) {
      result.set(path, annotations.map((annotation) => {
        const metadata = annotation.metadata
        if (metadata.kind === 'thread') {
          return `${annotation.side}:${annotation.lineNumber}:${metadata.thread.key}:${metadata.thread.comments.map((comment) => `${comment.id}:${comment.updated_at ?? comment.body}`).join(',')}`
        }
        if (metadata.kind === 'outdated') return `${metadata.key}:${metadata.comments.map((comment) => `${comment.id}:${comment.updated_at ?? comment.body}`).join(',')}`
        return `${metadata.kind}:${metadata.key}:${annotation.side}:${annotation.lineNumber}`
      }).join('|'))
    }
    return result
  }, [annotationsByPath])

  const removeDraft = useCallback((key: string) => {
    setDrafts((current) => current.filter((draft) => draft.key !== key))
    setDraftBodies((current) => {
      if (!Object.prototype.hasOwnProperty.call(current, key)) return current
      const next = { ...current }
      delete next[key]
      return next
    })
    setSubmittingDraftKeys((current) => {
      if (!current.has(key)) return current
      const next = new Set(current)
      next.delete(key)
      return next
    })
  }, [])

  const submitDraft = useCallback(async (key: string) => {
    const draft = draftsRef.current.find((candidate) => candidate.key === key)
    const body = (draftBodiesRef.current[key] ?? '').trim()
    const file = draft ? filesByPath.get(draft.path) : undefined
    if (!draft || !file || !body || busyRef.current) return
    setSubmittingDraftKeys((current) => new Set(current).add(key))
    try {
      await onCreate({ ...reviewCommentTarget(file, draft.side), body, anchor: draft.anchor })
      removeDraft(key)
    } catch (reason) {
        logWorkspaceChangeError('Failed to create a review comment', reason)
    } finally {
      setSubmittingDraftKeys((current) => {
        if (!current.has(key)) return current
        const next = new Set(current)
        next.delete(key)
        return next
      })
    }
  }, [filesByPath, onCreate, removeDraft])

  const handleSelection = useCallback((surfaceFile: CodeDiffSurfaceFile, range: SelectedLineRange) => {
    if (busyRef.current) return
    const file = filesByPath.get(surfaceFile.path)
    if (!file) return
    const selection = reviewAnchorFromLineSelection(file, range)
    const { anchor, lineNumber, side: selectedSide } = selection
    const start = anchor.start ?? 0
    const end = anchor.end ?? start
    const key = `new-comment-draft:${file.path}:${selectedSide}:${start}:${end}`
    if (draftsRef.current.some((draft) => draft.key === key)) return
    setDrafts((current) => current.some((draft) => draft.key === key)
      ? current
      : [...current, { key, path: file.path, side: selectedSide, lineNumber, anchor }])
    setDraftBodies((current) => Object.prototype.hasOwnProperty.call(current, key)
      ? current
      : { ...current, [key]: '' })
  }, [filesByPath])

  const handleEditingChange = useCallback((key: string, editing: boolean) => {
    setEditingThreadKeys((current) => {
      if (editing === current.has(key)) return current
      const next = new Set(current)
      if (editing) next.add(key)
      else next.delete(key)
      return next
    })
  }, [])

  const renderAnnotation = useCallback((annotation: DiffLineAnnotation<ReviewDiffAnnotation>, file: CodeDiffSurfaceFile): ReactNode => {
    const metadata = annotation.metadata
    if (metadata.kind === 'conflict') {
      return (
        <div role="status" className="m-1 flex items-start gap-2 rounded-md border border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-[var(--nova-warning)]" />
          <span>{t('changes.applyState.conflictedDescription')}</span>
        </div>
      )
    }
    if (metadata.kind === 'outdated') {
      return (
        <div className="m-1 rounded-md border border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] py-1">
          <div className="flex items-center gap-2 px-3 py-1 text-[11px] font-medium text-[var(--nova-warning)]">
            <AlertTriangle className="size-3.5" />{t('changes.comments.outdated')} · {metadata.comments.length}
          </div>
          <InlineCommentThread
            comments={metadata.comments}
            anchorLabel={t('changes.comments.outdatedDescription')}
            disabled={busy}
            onEditingChange={(editing) => handleEditingChange(metadata.key, editing)}
            onUpdate={onUpdate}
            onDelete={onDelete}
          />
        </div>
      )
    }
    if (metadata.kind === 'thread') {
      const thread = metadata.thread
      const index = new Utf8OffsetIndex(thread.side === 'before' ? file.before_content : file.after_content)
      return (
        <InlineCommentThread
          comments={thread.comments}
          anchorLabel={anchorLabel(file.path, index, thread.start)}
          quote={index.sliceBytes(thread.start, thread.end)}
          disabled={busy}
          onEditingChange={(editing) => handleEditingChange(thread.key, editing)}
          onUpdate={onUpdate}
          onDelete={onDelete}
        />
      )
    }
    const draft = draftsRef.current.find((candidate) => candidate.key === metadata.key)
    if (!draft) return null
    const index = new Utf8OffsetIndex(draft.side === 'before' ? file.before_content : file.after_content)
    return (
      <InlineCommentThread
        anchorLabel={anchorLabel(file.path, index, draft.anchor.start ?? 0)}
        quote={draft.anchor.quote}
        disabled={busy}
        draft={{
          body: draftBodies[metadata.key] ?? '',
          submitting: submittingDraftKeys.has(metadata.key),
          onChange: (body) => setDraftBodies((current) => ({ ...current, [metadata.key]: body })),
          onSubmit: () => void submitDraft(metadata.key),
          onCancel: () => removeDraft(metadata.key),
        }}
      />
    )
  }, [busy, draftBodies, handleEditingChange, onDelete, onUpdate, removeDraft, submitDraft, submittingDraftKeys, t])

  return {
    annotationsByPath,
    annotationRevisionByPath,
    draftPaths: new Set(drafts.map((draft) => draft.path)),
    hasDraft: drafts.length > 0 || editingThreadKeys.size > 0,
    onLineSelectionEnd: handleSelection,
    renderAnnotation,
  }
}

/** Maps each cumulative snapshot side back to the ChangeSet that owns it. */
export function reviewCommentTarget(file: ReviewThreadFile, side: 'before' | 'after'): Pick<CreateWorkspaceChangeCommentRequest, 'group_id' | 'change_set_id'> {
  return side === 'before'
    ? { group_id: file.base_group_id, change_set_id: file.base_change_set_id }
    : { group_id: file.latest_group_id, change_set_id: file.latest_change_set_id }
}

/** Converts CodeView's whole-line selection to the ledger's UTF-8 byte anchor. */
export function reviewAnchorFromLineSelection(file: DiffFileDocument, range: SelectedLineRange): { side: 'before' | 'after'; lineNumber: number; anchor: WorkspaceChangeCommentAnchor } {
  const rangeSide = range.side
  const endSide = range.endSide ?? rangeSide
  const side = rangeSide === 'deletions' ? 'before' : 'after'
  const crossesSides = rangeSide !== undefined && endSide !== undefined && rangeSide !== endSide
  const startLine = Math.max(1, Math.min(range.start, crossesSides ? range.start : range.end))
  const endLine = Math.max(startLine, crossesSides ? startLine : Math.max(range.start, range.end))
  const text = side === 'before' ? file.before_content : file.after_content
  const revision = side === 'before' ? file.base_revision : file.revision
  const index = new Utf8OffsetIndex(text)
  const start = index.byteOffsetAtPosition({ lineNumber: startLine, column: 1 })
  const rawEnd = index.byteOffsetAtPosition({ lineNumber: endLine, column: Number.MAX_SAFE_INTEGER })
  const cappedEnd = Math.min(Math.max(start, rawEnd), start + MAX_QUOTE_BYTES)
  const safeEnd = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(cappedEnd))
  const prefixStart = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(Math.max(0, start - CONTEXT_BYTES)))
  const suffixEnd = index.byteOffsetAtUtf16Offset(index.utf16OffsetAtByteOffset(Math.min(index.byteLength, safeEnd + CONTEXT_BYTES)))
  return {
    side,
    lineNumber: endLine,
    anchor: {
      kind: 'text-range',
      side,
      encoding: 'utf8-bytes-v1',
      revision,
      start,
      end: safeEnd,
      quote: index.sliceBytes(start, safeEnd),
      prefix: index.sliceBytes(prefixStart, start),
      suffix: index.sliceBytes(safeEnd, suffixEnd),
    },
  }
}

export function resolveCommentThreads(file: DiffFileDocument, comments: readonly WorkspaceChangeComment[]): { threads: ResolvedCommentThread[]; outdated: WorkspaceChangeComment[] } {
  const grouped = new Map<string, ResolvedCommentThread>()
  const outdated: WorkspaceChangeComment[] = []
  for (const comment of comments) {
    if (comment.deleted) continue
    const resolved = resolveCommentAnchor(file, comment)
    if (!resolved) {
      outdated.push(comment)
      continue
    }
    const key = `comment:${resolved.side}:${resolved.start}:${resolved.end}`
    const existing = grouped.get(key)
    if (existing) existing.comments.push(comment)
    else grouped.set(key, { key, ...resolved, comments: [comment] })
  }
  return { threads: Array.from(grouped.values()), outdated }
}

function resolveCommentAnchor(file: DiffFileDocument, comment: WorkspaceChangeComment): Omit<ResolvedCommentThread, 'key' | 'comments'> | null {
  const anchor = comment.anchor
  if (!anchor || (anchor.encoding && anchor.encoding !== 'utf8-bytes-v1')) return null
  const side = anchor.side ?? (anchor.revision === file.base_revision ? 'before' : 'after')
  const text = side === 'before' ? file.before_content : file.after_content
  const revision = side === 'before' ? file.base_revision : file.revision
  const index = new Utf8OffsetIndex(text)
  const start = Math.max(0, Math.min(index.byteLength, anchor.start ?? 0))
  const end = Math.max(start, Math.min(index.byteLength, anchor.end ?? start))
  if (anchor.revision === revision && (!anchor.quote || index.sliceBytes(start, end) === anchor.quote)) {
    return { side, start, end }
  }
  if (!anchor.quote) return null
  const first = text.indexOf(anchor.quote)
  if (first < 0 || text.lastIndexOf(anchor.quote) !== first) return null
  return {
    side,
    start: index.byteOffsetAtUtf16Offset(first),
    end: index.byteOffsetAtUtf16Offset(first + anchor.quote.length),
  }
}

function commentsForFile(file: ReviewThreadFile, comments: readonly WorkspaceChangeComment[]): WorkspaceChangeComment[] {
  return comments.filter((comment) => {
    if (comment.change_set_id) return file.change_set_ids.includes(comment.change_set_id)
    const revision = comment.anchor?.revision
    return Boolean(revision && (revision === file.base_revision || revision === file.revision))
  })
}

function anchorLabel(path: string, index: Utf8OffsetIndex, byteOffset: number): string {
  return `${path}:L${index.positionAtByteOffset(byteOffset).lineNumber}`
}
