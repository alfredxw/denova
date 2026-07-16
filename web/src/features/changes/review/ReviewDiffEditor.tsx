import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { DiffEditor, Editor, type DiffOnMount, type Monaco, type OnMount } from '@monaco-editor/react'
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
import { fitReviewEditorToHost, type ReviewEditorLayoutTarget } from './monaco/review-editor-dom'
import {
  installReviewMonacoThemes,
  REVIEW_MONACO_THEME_DARK,
  REVIEW_MONACO_THEME_LIGHT,
} from './monaco/review-monaco-theme'
import { UnifiedReviewEditorAdapter } from './monaco/unified-review-editor-adapter'
import { buildUnifiedReviewProjection } from './monaco/unified-review-projection'
import { Utf8OffsetIndex } from './monaco/utf8-offset-index'
import { logWorkspaceChangeError, workspaceChangeErrorMessage } from '../errors'

interface ReviewDiffEditorProps {
  threadID: string
  file: ReviewThreadFile
  comments: WorkspaceChangeComment[]
  layout: ReviewDiffLayout
  busy?: boolean
  onDraftChange?: (hasDraft: boolean) => void
  initialHeight?: number
  onHeightChange?: (height: number) => void
  onCreateComment: (request: CreateWorkspaceChangeCommentRequest) => Promise<void>
  onUpdateComment: (comment: WorkspaceChangeComment, body: string) => Promise<void>
  onResolveComment: (comment: WorkspaceChangeComment, resolved: boolean) => Promise<void>
  onDeleteComment: (comment: WorkspaceChangeComment) => Promise<void>
}

interface ResolvedCommentThread extends ReviewZoneDescriptor {
  comments: WorkspaceChangeComment[]
}

interface CommentDraft {
  key: string
  anchor: WorkspaceChangeCommentAnchor
}

export function ReviewDiffEditor({ threadID, file, comments, layout, busy = false, onDraftChange, initialHeight = 240, onHeightChange, onCreateComment, onUpdateComment, onResolveComment, onDeleteComment }: ReviewDiffEditorProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const splitAdapterRef = useRef<ReviewEditorAdapter | null>(null)
  const unifiedAdapterRef = useRef<UnifiedReviewEditorAdapter | null>(null)
  const monacoRef = useRef<Monaco | null>(null)
  const rootRef = useRef<HTMLDivElement | null>(null)
  const codeHostRef = useRef<HTMLDivElement | null>(null)
  const heightDisposablesRef = useRef<Array<{ dispose: () => void }>>([])
  const editorLayoutDisposableRef = useRef<{ dispose: () => void } | null>(null)
  const heightFrameRef = useRef<number | null>(null)
  const [portalTargets, setPortalTargets] = useState<ReviewZonePortalTarget[]>([])
  const [drafts, setDrafts] = useState<CommentDraft[]>([])
  const [draftBodies, setDraftBodies] = useState<Readonly<Record<string, string>>>(() => ({}))
  const [editingThreadKeys, setEditingThreadKeys] = useState<ReadonlySet<string>>(() => new Set())
  const [submittingDraftKeys, setSubmittingDraftKeys] = useState<ReadonlySet<string>>(() => new Set())
  const [commentError, setCommentError] = useState('')
  const [expandedRegionIDs, setExpandedRegionIDs] = useState<ReadonlySet<string>>(() => new Set())
  const [contentHeight, setContentHeight] = useState(initialHeight)
  const hasDraft = drafts.length > 0 || editingThreadKeys.size > 0
  const draftsRef = useRef<CommentDraft[]>(drafts)
  const draftBodiesRef = useRef(draftBodies)
  const busyRef = useRef(busy)
  const onDraftChangeRef = useRef(onDraftChange)
  draftsRef.current = drafts
  draftBodiesRef.current = draftBodies
  busyRef.current = busy
  onDraftChangeRef.current = onDraftChange
  const beforeIndex = useMemo(() => new Utf8OffsetIndex(file.before_content), [file.before_content])
  const afterIndex = useMemo(() => new Utf8OffsetIndex(file.after_content), [file.after_content])
  const originalModelPath = reviewModelPath(threadID, file, 'before')
  const modifiedModelPath = reviewModelPath(threadID, file, 'after')
  const unifiedModelPath = reviewModelPath(threadID, file, 'unified')
  const language = languageForPath(file.path)
  const lightTheme = resolvedTheme === 'light'
  const reviewTheme = lightTheme ? REVIEW_MONACO_THEME_LIGHT : REVIEW_MONACO_THEME_DARK

  const { threads, outdated } = useMemo(() => resolveCommentThreads(file, comments), [comments, file])
  const draftDescriptors = useMemo<ReviewZoneDescriptor[]>(() => drafts.flatMap((draft) => {
    const { anchor } = draft
    if (!anchor.side) return []
    return [{
      key: draft.key,
      side: anchor.side,
      start: anchor.start ?? 0,
      end: anchor.end ?? anchor.start ?? 0,
    }]
  }), [drafts])
  const zoneDescriptors = useMemo(() => (
    draftDescriptors.length > 0 ? [...threads, ...draftDescriptors] : threads
  ), [draftDescriptors, threads])
  const visibleSourceLines = useMemo(() => new Set(zoneDescriptors.map((descriptor) => {
    const index = descriptor.side === 'before' ? beforeIndex : afterIndex
    return `${descriptor.side}:${index.positionAtByteOffset(descriptor.end || descriptor.start).lineNumber}`
  })), [afterIndex, beforeIndex, zoneDescriptors])
  const unifiedProjection = useMemo(() => buildUnifiedReviewProjection(
    file.before_content,
    file.after_content,
    {
      collapsedLabel: (count) => t('changes.diff.unmodifiedLines', { count }),
      expandedRegionIDs,
      visibleSourceLines,
    },
  ), [expandedRegionIDs, file.after_content, file.before_content, t, visibleSourceLines])
  const commentingDisabled = busy
  const latestConfigRef = useRef({ file, layout, zones: zoneDescriptors, commentingDisabled, unifiedProjection })
  latestConfigRef.current = { file, layout, zones: zoneDescriptors, commentingDisabled, unifiedProjection }

  useEffect(() => {
    setDrafts([])
    setDraftBodies({})
    setSubmittingDraftKeys(new Set())
    setEditingThreadKeys(new Set())
    setPortalTargets([])
    setCommentError('')
    setExpandedRegionIDs(new Set())
  }, [file.base_revision, file.path, file.revision])

  useEffect(() => setContentHeight(initialHeight), [file.base_revision, file.path, file.revision, initialHeight, layout])

  useEffect(() => {
    onDraftChangeRef.current?.(hasDraft)
  }, [hasDraft])

  useEffect(() => () => onDraftChangeRef.current?.(false), [])

  useEffect(() => {
    if (layout === 'unified') {
      unifiedAdapterRef.current?.update(file, unifiedProjection, zoneDescriptors, commentingDisabled)
    } else {
      splitAdapterRef.current?.update(file, 'split', zoneDescriptors, commentingDisabled)
    }
  }, [commentingDisabled, file, layout, unifiedProjection, zoneDescriptors])

  useEffect(() => () => {
    splitAdapterRef.current?.dispose()
    splitAdapterRef.current = null
    unifiedAdapterRef.current?.dispose()
    unifiedAdapterRef.current = null
    for (const disposable of heightDisposablesRef.current.splice(0)) disposable.dispose()
    editorLayoutDisposableRef.current?.dispose()
    editorLayoutDisposableRef.current = null
    if (heightFrameRef.current !== null) window.cancelAnimationFrame(heightFrameRef.current)
  }, [])

  useEffect(() => {
    const modelPaths = [originalModelPath, modifiedModelPath, unifiedModelPath]
    return () => {
      // @monaco-editor/react disposes DiffEditor models before the widget. Monaco
      // 0.55 treats that ordering as an error, so retain them through widget
      // disposal and release detached models on the following microtask instead.
      const monaco = monacoRef.current
      if (monaco) scheduleDetachedReviewModelDisposal(monaco, modelPaths)
    }
  }, [modifiedModelPath, originalModelPath, unifiedModelPath])

  useLayoutEffect(() => {
    const root = rootRef.current
    const codeHost = codeHostRef.current
    if (!root || !codeHost || contentHeight <= 0) return
    const chromeHeight = Math.max(0, root.getBoundingClientRect().height - codeHost.getBoundingClientRect().height)
    onHeightChange?.(Math.ceil(contentHeight + chromeHeight))
  }, [commentError, contentHeight, onHeightChange, outdated.length])

  const installHeightTracking = useCallback((editors: Array<{
    getContentHeight: () => number
    onDidContentSizeChange: (listener: () => void) => { dispose: () => void }
  }>) => {
    for (const disposable of heightDisposablesRef.current.splice(0)) disposable.dispose()
    const measure = () => {
      if (heightFrameRef.current !== null) window.cancelAnimationFrame(heightFrameRef.current)
      heightFrameRef.current = window.requestAnimationFrame(() => {
        heightFrameRef.current = null
        const nextHeight = Math.max(120, Math.ceil(Math.max(...editors.map((editor) => editor.getContentHeight()))) + 1)
        setContentHeight((current) => current === nextHeight ? current : nextHeight)
      })
    }
    heightDisposablesRef.current = editors.map((editor) => editor.onDidContentSizeChange(measure))
    measure()
  }, [])

  const installEditorLayout = useCallback((target: ReviewEditorLayoutTarget) => {
    editorLayoutDisposableRef.current?.dispose()
    const host = codeHostRef.current
    editorLayoutDisposableRef.current = host ? fitReviewEditorToHost(target, host) : null
  }, [])

  const handleCommentRequest = useCallback((anchor: WorkspaceChangeCommentAnchor) => {
    if (busyRef.current) return
    setCommentError('')
    const key = commentDraftKey(anchor)
    if (draftsRef.current.some((draft) => draft.key === key)) return
    const nextDrafts = [...draftsRef.current, { key, anchor }]
    draftsRef.current = nextDrafts
    onDraftChangeRef.current?.(true)
    setDrafts(nextDrafts)
    setDraftBodies((current) => {
      if (Object.prototype.hasOwnProperty.call(current, key)) return current
      const next = { ...current, [key]: '' }
      draftBodiesRef.current = next
      return next
    })
  }, [])

  const handleThreadEditingChange = useCallback((key: string, editing: boolean) => {
    if (editing) onDraftChangeRef.current?.(true)
    setEditingThreadKeys((current) => {
      if (editing === current.has(key)) return current
      const next = new Set(current)
      if (editing) next.add(key)
      else next.delete(key)
      return next
    })
  }, [])

  const handleSplitMount = useCallback<DiffOnMount>((editor, monaco) => {
    monacoRef.current = monaco
    unifiedAdapterRef.current?.dispose()
    unifiedAdapterRef.current = null
    splitAdapterRef.current?.dispose()
    const adapter = new ReviewEditorAdapter(editor, monaco, {
      commentLabel: t('changes.comment'),
      beforeLabel: t('changes.diff.before'),
      afterLabel: t('changes.diff.after'),
      onCommentRequest: handleCommentRequest,
      onPortalTargetsChange: setPortalTargets,
    })
    splitAdapterRef.current = adapter
    const config = latestConfigRef.current
    adapter.update(config.file, 'split', config.zones, config.commentingDisabled)
    installEditorLayout(editor)
    installHeightTracking([editor.getOriginalEditor(), editor.getModifiedEditor()])
  }, [handleCommentRequest, installEditorLayout, installHeightTracking, t])

  const handleUnifiedMount = useCallback<OnMount>((editor, monaco) => {
    monacoRef.current = monaco
    splitAdapterRef.current?.dispose()
    splitAdapterRef.current = null
    unifiedAdapterRef.current?.dispose()
    const adapter = new UnifiedReviewEditorAdapter(editor, monaco, {
      commentLabel: t('changes.comment'),
      beforeLabel: t('changes.diff.before'),
      afterLabel: t('changes.diff.after'),
      onCommentRequest: handleCommentRequest,
      onPortalTargetsChange: setPortalTargets,
      onExpandRegion: (collapseID) => setExpandedRegionIDs((current) => new Set([...current, collapseID])),
    })
    unifiedAdapterRef.current = adapter
    const config = latestConfigRef.current
    adapter.update(config.file, config.unifiedProjection, config.zones, config.commentingDisabled)
    installEditorLayout(editor)
    installHeightTracking([editor])
  }, [handleCommentRequest, installEditorLayout, installHeightTracking, t])

  const removeDraft = useCallback((key: string) => {
    const nextDrafts = draftsRef.current.filter((draft) => draft.key !== key)
    draftsRef.current = nextDrafts
    setDrafts(nextDrafts)
    setDraftBodies((current) => {
      if (!Object.prototype.hasOwnProperty.call(current, key)) return current
      const next = { ...current }
      delete next[key]
      draftBodiesRef.current = next
      return next
    })
    setSubmittingDraftKeys((current) => {
      if (!current.has(key)) return current
      const next = new Set(current)
      next.delete(key)
      return next
    })
  }, [])

  const submitDraft = async (key: string) => {
    const draft = draftsRef.current.find((candidate) => candidate.key === key)
    const body = (draftBodiesRef.current[key] ?? '').trim()
    if (!draft || !body) return
    setSubmittingDraftKeys((current) => new Set(current).add(key))
    setCommentError('')
    try {
      await onCreateComment({
        ...reviewCommentTarget(file, draft.anchor.side ?? 'after'),
        body,
        anchor: draft.anchor,
      })
      removeDraft(key)
    } catch (reason) {
      logWorkspaceChangeError('中央变更审阅添加评论失败', reason)
      setCommentError(workspaceChangeErrorMessage(t, reason))
    } finally {
      setSubmittingDraftKeys((current) => {
        if (!current.has(key)) return current
        const next = new Set(current)
        next.delete(key)
        return next
      })
    }
  }

  return (
    <div ref={rootRef} data-review-theme={lightTheme ? 'light' : 'dark'} className="nova-review-editor flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
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
            onEditingChange={(editing) => handleThreadEditingChange('outdated', editing)}
            onUpdate={onUpdateComment}
            onResolve={onResolveComment}
            onDelete={onDeleteComment}
          />
        </div>
      )}
      {commentError && <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">{commentError}</div>}
      <div ref={codeHostRef} className="min-h-0 flex-1 overflow-hidden">
        {layout === 'unified' ? (
          <Editor
            height="100%"
            theme={reviewTheme}
            language={language}
            path={unifiedModelPath}
            value={unifiedProjection.value}
            keepCurrentModel
            beforeMount={installReviewMonacoThemes}
            onMount={handleUnifiedMount}
            loading={<ReviewEditorLoading label={t('changes.loading')} />}
            options={{
              readOnly: true,
              domReadOnly: true,
              automaticLayout: true,
              ariaLabel: `${file.path} · ${t('changes.diff.unified')}`,
              glyphMargin: true,
              lineDecorationsWidth: 6,
              lineNumbers: (lineNumber) => unifiedProjection.lines[lineNumber - 1]?.lineNumberLabel ?? '',
              lineNumbersMinChars: 3,
              minimap: { enabled: false },
              folding: false,
              overviewRulerLanes: 0,
              renderLineHighlight: 'none',
              scrollBeyondLastLine: false,
              wordWrap: 'on',
              wrappingStrategy: 'advanced',
              unicodeHighlight: {
                ambiguousCharacters: false,
                nonBasicASCII: false,
                invisibleCharacters: true,
              },
              padding: { top: 12, bottom: 16 },
              stickyScroll: { enabled: false },
              scrollbar: {
                vertical: 'hidden',
                verticalScrollbarSize: 0,
                horizontalScrollbarSize: 10,
                handleMouseWheel: false,
                alwaysConsumeMouseWheel: false,
              },
            }}
          />
        ) : (
          <DiffEditor
            height="100%"
            theme={reviewTheme}
            language={language}
            original={file.before_content}
            modified={file.after_content}
            originalModelPath={originalModelPath}
            modifiedModelPath={modifiedModelPath}
            keepCurrentOriginalModel
            keepCurrentModifiedModel
            beforeMount={installReviewMonacoThemes}
            onMount={handleSplitMount}
            loading={<ReviewEditorLoading label={t('changes.loading')} />}
            options={{
              readOnly: true,
              domReadOnly: true,
              originalEditable: false,
              automaticLayout: true,
              renderSideBySide: true,
              useInlineViewWhenSpaceIsLimited: false,
              diffAlgorithm: 'advanced',
              diffWordWrap: 'on',
              maxComputationTime: 0,
              renderIndicators: true,
              renderMarginRevertIcon: false,
              renderGutterMenu: false,
              renderOverviewRuler: false,
              glyphMargin: true,
              minimap: { enabled: false },
              folding: false,
              scrollBeyondLastLine: false,
              lineNumbersMinChars: 3,
              unicodeHighlight: {
                ambiguousCharacters: false,
                nonBasicASCII: false,
                invisibleCharacters: true,
              },
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
                vertical: 'hidden',
                verticalScrollbarSize: 0,
                horizontalScrollbarSize: 10,
                handleMouseWheel: false,
                alwaysConsumeMouseWheel: false,
              },
            }}
          />
        )}
      </div>

      {portalTargets.map((target) => {
        const draft = drafts.find((candidate) => candidate.key === target.key)
        if (draft) {
          const index = draft.anchor.side === 'before' ? beforeIndex : afterIndex
          const body = draftBodies[draft.key] ?? ''
          return createPortal(
            <InlineCommentThread
              anchorLabel={anchorLabel(file.path, index, draft.anchor.start ?? 0)}
              disabled={busy}
              draft={{
                body,
                submitting: submittingDraftKeys.has(draft.key),
                onChange: (nextBody) => setDraftBodies((current) => {
                  const next = { ...current, [draft.key]: nextBody }
                  draftBodiesRef.current = next
                  return next
                }),
                onSubmit: () => void submitDraft(draft.key),
                onCancel: () => removeDraft(draft.key),
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
            onEditingChange={(editing) => handleThreadEditingChange(thread.key, editing)}
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

function commentDraftKey(anchor: WorkspaceChangeCommentAnchor): string {
  return `new-comment-draft:${anchor.side ?? 'after'}:${anchor.start ?? 0}:${anchor.end ?? anchor.start ?? 0}`
}

function reviewModelPath(threadID: string, file: ReviewThreadFile, side: 'before' | 'after' | 'unified'): string {
  const revision = side === 'before'
    ? file.base_revision
    : side === 'after'
      ? file.revision
      : `${file.base_revision}:${file.revision}`
  return `denova-review://thread/${encodeURIComponent(threadID)}/${encodeURIComponent(file.path)}?side=${side}&revision=${encodeURIComponent(revision)}`
}

function ReviewEditorLoading({ label }: { label: string }) {
  return <div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]"><Loader2 className="h-4 w-4 animate-spin" />{label}</div>
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
