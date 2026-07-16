import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { DiffEditor, type DiffOnMount, type Monaco } from '@monaco-editor/react'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThreadFile,
  WorkspaceChangeComment,
  WorkspaceChangeCommentAnchor,
} from '../types'
import { InlineCommentThread } from './InlineCommentThread'
import {
  ReviewEditorAdapter,
  type ReviewDiffLayout,
  type ReviewZoneDescriptor,
  type ReviewZonePortalTarget,
} from './monaco/review-editor-adapter'
import { scheduleDetachedReviewModelDisposal } from './monaco/review-model-lifecycle'
import { Utf8OffsetIndex } from './monaco/utf8-offset-index'
import { logWorkspaceChangeError, workspaceChangeErrorMessage } from '../errors'
import './review-diff.css'

interface ReviewDiffEditorProps {
  threadID: string
  file: ReviewThreadFile
  comments: WorkspaceChangeComment[]
  layout: ReviewDiffLayout
  busy?: boolean
  onDraftChange?: (hasDraft: boolean) => void
  onCreateComment: (request: CreateWorkspaceChangeCommentRequest) => Promise<void>
  onUpdateComment: (comment: WorkspaceChangeComment, body: string) => Promise<void>
  onResolveComment: (comment: WorkspaceChangeComment, resolved: boolean) => Promise<void>
  onDeleteComment: (comment: WorkspaceChangeComment) => Promise<void>
}

interface ResolvedCommentThread extends ReviewZoneDescriptor {
  comments: WorkspaceChangeComment[]
}

interface CommentDraft {
  anchor: WorkspaceChangeCommentAnchor
  body: string
}

export function ReviewDiffEditor({ threadID, file, comments, layout, busy = false, onDraftChange, onCreateComment, onUpdateComment, onResolveComment, onDeleteComment }: ReviewDiffEditorProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const adapterRef = useRef<ReviewEditorAdapter | null>(null)
  const monacoRef = useRef<Monaco | null>(null)
  const [portalTargets, setPortalTargets] = useState<ReviewZonePortalTarget[]>([])
  const [draft, setDraft] = useState<CommentDraft | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [commentError, setCommentError] = useState('')
  const hasDraft = Boolean(draft)
  const draftRef = useRef<CommentDraft | null>(draft)
  const busyRef = useRef(busy)
  draftRef.current = draft
  busyRef.current = busy
  const beforeIndex = useMemo(() => new Utf8OffsetIndex(file.before_content), [file.before_content])
  const afterIndex = useMemo(() => new Utf8OffsetIndex(file.after_content), [file.after_content])
  const originalModelPath = reviewModelPath(threadID, file, 'before')
  const modifiedModelPath = reviewModelPath(threadID, file, 'after')
  const language = languageForPath(file.path)

  const { threads, outdated } = useMemo(() => resolveCommentThreads(file, comments), [comments, file])
  const draftDescriptor = useMemo<ReviewZoneDescriptor | null>(() => {
    const anchor = draft?.anchor
    if (!anchor?.side) return null
    return {
      key: 'new-comment-draft',
      side: anchor.side,
      start: anchor.start ?? 0,
      end: anchor.end ?? anchor.start ?? 0,
    }
  }, [draft?.anchor])
  const zoneDescriptors = useMemo(() => (
    draftDescriptor ? [...threads, draftDescriptor] : threads
  ), [draftDescriptor, threads])
  const commentingDisabled = busy || Boolean(draft)
  const latestConfigRef = useRef({ file, layout, zones: zoneDescriptors, commentingDisabled })
  latestConfigRef.current = { file, layout, zones: zoneDescriptors, commentingDisabled }

  useEffect(() => {
    setDraft(null)
    setPortalTargets([])
    setCommentError('')
  }, [file.base_revision, file.path, file.revision])

  useEffect(() => {
    onDraftChange?.(hasDraft)
  }, [hasDraft, onDraftChange])

  useEffect(() => () => onDraftChange?.(false), [onDraftChange])

  useEffect(() => {
    adapterRef.current?.update(file, layout, zoneDescriptors, commentingDisabled)
  }, [commentingDisabled, file, layout, zoneDescriptors])

  useEffect(() => () => {
    adapterRef.current?.dispose()
    adapterRef.current = null
  }, [])

  useEffect(() => {
    const modelPaths = [originalModelPath, modifiedModelPath]
    return () => {
      // @monaco-editor/react disposes DiffEditor models before the widget. Monaco
      // 0.55 treats that ordering as an error, so retain them through widget
      // disposal and release detached models on the following microtask instead.
      const monaco = monacoRef.current
      if (monaco) scheduleDetachedReviewModelDisposal(monaco, modelPaths)
    }
  }, [modifiedModelPath, originalModelPath])

  const handleMount = useCallback<DiffOnMount>((editor, monaco) => {
    monacoRef.current = monaco
    adapterRef.current?.dispose()
    const adapter = new ReviewEditorAdapter(editor, monaco, {
      commentLabel: t('changes.comment'),
      beforeLabel: t('changes.diff.before'),
      afterLabel: t('changes.diff.after'),
      onCommentRequest: (anchor) => {
        if (busyRef.current || draftRef.current) return
        setCommentError('')
        const nextDraft = { anchor, body: '' }
        draftRef.current = nextDraft
        setDraft(nextDraft)
      },
      onPortalTargetsChange: setPortalTargets,
    })
    adapterRef.current = adapter
    const config = latestConfigRef.current
    adapter.update(config.file, config.layout, config.zones, config.commentingDisabled)
  }, [t])

  const submitDraft = async () => {
    if (!draft?.body.trim()) return
    setSubmitting(true)
    setCommentError('')
    try {
      await onCreateComment({
        ...reviewCommentTarget(file, draft.anchor.side ?? 'after'),
        body: draft.body.trim(),
        anchor: draft.anchor,
      })
      setDraft(null)
    } catch (reason) {
      logWorkspaceChangeError('中央变更审阅添加评论失败', reason)
      setCommentError(workspaceChangeErrorMessage(t, reason))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      {outdated.length > 0 && (
        <div role="status" className="max-h-36 shrink-0 overflow-y-auto border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-xs text-[var(--nova-text-muted)]">
          <div className="mb-1.5 flex items-center gap-2 font-medium text-[var(--nova-warning)]">
            <AlertTriangle className="h-3.5 w-3.5" />
            {t('changes.comments.outdated')} · {outdated.length}
          </div>
          <InlineCommentThread
            comments={outdated}
            anchorLabel={t('changes.comments.outdatedDescription')}
            disabled={busy}
            onUpdate={onUpdateComment}
            onResolve={onResolveComment}
            onDelete={onDeleteComment}
          />
        </div>
      )}
      {commentError && <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{commentError}</div>}
      <div className="min-h-0 flex-1 overflow-hidden">
        <DiffEditor
          height="100%"
          theme={resolvedTheme === 'light' ? 'light' : 'vs-dark'}
          language={language}
          original={file.before_content}
          modified={file.after_content}
          originalModelPath={originalModelPath}
          modifiedModelPath={modifiedModelPath}
          keepCurrentOriginalModel
          keepCurrentModifiedModel
          onMount={handleMount}
          loading={<div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]"><Loader2 className="h-4 w-4 animate-spin" />{t('changes.loading')}</div>}
          options={{
            readOnly: true,
            domReadOnly: true,
            originalEditable: false,
            automaticLayout: true,
            renderSideBySide: layout === 'split',
            // Split is an explicit review choice. Monaco's responsive fallback would
            // otherwise silently render the unified view in the focused center pane.
            useInlineViewWhenSpaceIsLimited: layout !== 'split',
            diffAlgorithm: 'advanced',
            diffWordWrap: 'on',
            renderIndicators: true,
            renderMarginRevertIcon: false,
            renderGutterMenu: false,
            renderOverviewRuler: false,
            glyphMargin: true,
            minimap: { enabled: false },
            folding: false,
            scrollBeyondLastLine: false,
            lineNumbersMinChars: 3,
            padding: { top: 12, bottom: 16 },
            hideUnchangedRegions: {
              enabled: true,
              contextLineCount: 3,
              minimumLineCount: 8,
              revealLineCount: 20,
            },
            originalAriaLabel: `${file.path} · ${t('versions.diffSnapshot', { defaultValue: 'Before' })}`,
            modifiedAriaLabel: `${file.path} · ${t('versions.diffWorkspace', { defaultValue: 'After' })}`,
            accessibilityVerbose: true,
            scrollbar: {
              verticalScrollbarSize: 10,
              horizontalScrollbarSize: 10,
            },
          }}
        />
      </div>

      {portalTargets.map((target) => {
        if (target.key === draftDescriptor?.key && draft) {
          const index = draft.anchor.side === 'before' ? beforeIndex : afterIndex
          return createPortal(
            <InlineCommentThread
              anchorLabel={anchorLabel(file.path, index, draft.anchor.start ?? 0)}
              disabled={busy}
              draft={{
                body: draft.body,
                submitting,
                onChange: (body) => setDraft((current) => current ? { ...current, body } : current),
                onSubmit: () => void submitDraft(),
                onCancel: () => setDraft(null),
              }}
            />,
            target.domNode,
            target.key,
          )
        }
        const thread = threads.find((item) => item.key === target.key)
        if (!thread) return null
        const index = thread.side === 'before' ? beforeIndex : afterIndex
        return createPortal(
          <InlineCommentThread
            comments={thread.comments}
            anchorLabel={anchorLabel(file.path, index, thread.start)}
            disabled={busy}
            onUpdate={onUpdateComment}
            onResolve={onResolveComment}
            onDelete={onDeleteComment}
          />,
          target.domNode,
          target.key,
        )
      })}
    </div>
  )
}

/** Maps each cumulative snapshot side back to the ChangeSet that owns it. */
export function reviewCommentTarget(file: ReviewThreadFile, side: 'before' | 'after'): Pick<CreateWorkspaceChangeCommentRequest, 'group_id' | 'change_set_id'> {
  return side === 'before'
    ? { group_id: file.base_group_id, change_set_id: file.base_change_set_id }
    : { group_id: file.latest_group_id, change_set_id: file.latest_change_set_id }
}

export function resolveCommentThreads(file: ReviewThreadFile, comments: WorkspaceChangeComment[]): { threads: ResolvedCommentThread[]; outdated: WorkspaceChangeComment[] } {
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

function resolveCommentAnchor(file: ReviewThreadFile, comment: WorkspaceChangeComment): Omit<ReviewZoneDescriptor, 'key'> | null {
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

function anchorLabel(path: string, index: Utf8OffsetIndex, byteOffset: number): string {
  return `${path}:L${index.positionAtByteOffset(byteOffset).lineNumber}`
}

function reviewModelPath(threadID: string, file: ReviewThreadFile, side: 'before' | 'after'): string {
  const revision = side === 'before' ? file.base_revision : file.revision
  return `denova-review://thread/${encodeURIComponent(threadID)}/${encodeURIComponent(file.path)}?side=${side}&revision=${encodeURIComponent(revision)}`
}

function languageForPath(path: string): string {
  const extension = path.split('.').pop()?.toLowerCase()
  const languages: Record<string, string> = {
    c: 'c',
    cpp: 'cpp',
    css: 'css',
    go: 'go',
    html: 'html',
    java: 'java',
    js: 'javascript',
    json: 'json',
    jsx: 'javascript',
    md: 'markdown',
    markdown: 'markdown',
    py: 'python',
    rs: 'rust',
    sh: 'shell',
    ts: 'typescript',
    tsx: 'typescript',
    yaml: 'yaml',
    yml: 'yaml',
  }
  return extension ? languages[extension] || 'plaintext' : 'plaintext'
}
