import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, ExternalLink, FileDiff, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { lineDiffStats } from '../diff-stats'
import { logWorkspaceChangeError, workspaceChangeErrorMessage } from '../errors'
import {
  createWorkspaceChangeComment,
  deleteWorkspaceChangeComment,
  redoWorkspaceChangeGroup,
  resolveWorkspaceChangeComment,
  reviewWorkspaceChangeGroup,
  undoWorkspaceChangeGroup,
  updateWorkspaceChangeComment,
} from '../api'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThread,
  ReviewThreadFile,
  WorkspaceChangeComment,
  WorkspaceChangeGroupSummary,
  WorkspaceChangeMutationResult,
} from '../types'
import { invalidateWorkspaceChangeQueries, useWorkspaceChangeReviewThread } from '../use-change-review'
import { ReviewFileNavigator } from './ReviewFileNavigator'
import { ReviewToolbar } from './ReviewToolbar'
import { ReviewUtilityTab } from './ReviewUtilityTab'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { Utf8OffsetIndex } from './monaco/utf8-offset-index'

const ReviewDiffEditor = lazy(() => import('./ReviewDiffEditor').then((module) => ({ default: module.ReviewDiffEditor })))

const REVIEW_LAYOUT_STORAGE_KEY = 'nova:change-review-layout'

interface ChangeReviewWorkspaceProps {
  workspace: string
  threadID: string
  /** Prevents mutating a review thread while its Agent run is still appending changes. */
  disabled?: boolean
  selectedPath?: string | null
  onClose: () => void
  onOpenFile?: (path: string) => void | Promise<void>
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
  onFeedbackCommentsChange?: (threadID: string, comments: WorkspaceChangeComment[]) => void
}

type ReviewVariables = {
  workspace: string
  group: WorkspaceChangeGroupSummary
  decision: 'accept' | 'reject'
}

type HistoryVariables = {
  workspace: string
  group: WorkspaceChangeGroupSummary
  action: 'undo' | 'redo'
}

type CommentVariables =
  | { action: 'create'; workspace: string; request: CreateWorkspaceChangeCommentRequest }
  | { action: 'update'; workspace: string; comment: WorkspaceChangeComment; body: string }
  | { action: 'resolve'; workspace: string; comment: WorkspaceChangeComment; resolved: boolean }
  | { action: 'delete'; workspace: string; comment: WorkspaceChangeComment }

/** Full-width, server-projected review surface rendered in the central editor region. */
export function ChangeReviewWorkspace({ workspace, threadID, disabled = false, selectedPath, onClose, onOpenFile, onWorkspaceChanged, onFeedbackCommentsChange }: ChangeReviewWorkspaceProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const threadQuery = useWorkspaceChangeReviewThread(workspace, threadID)
  const thread = threadQuery.data
  const [layout, setLayout] = useState<ReviewDiffLayout>(readReviewLayout)
  const [activePath, setActivePath] = useState('')
  const [selectedGroupID, setSelectedGroupID] = useState('')
  const [error, setError] = useState('')
  const [hasCommentDraft, setHasCommentDraft] = useState(false)
  const activeWorkspaceRef = useRef(workspace)
  const feedbackCallbackRef = useRef(onFeedbackCommentsChange)
  activeWorkspaceRef.current = workspace
  feedbackCallbackRef.current = onFeedbackCommentsChange

  useEffect(() => {
    try {
      window.localStorage.setItem(REVIEW_LAYOUT_STORAGE_KEY, layout)
    } catch {
      // Browser privacy settings may disable storage; the in-memory choice still works.
    }
  }, [layout])

  useEffect(() => {
    setActivePath('')
    setSelectedGroupID('')
    setError('')
    setHasCommentDraft(false)
  }, [threadID, workspace])

  useEffect(() => {
    const files = thread?.files ?? []
    if (!files.length) {
      setActivePath('')
      return
    }
    if (files.some((file) => file.path === activePath)) return
    const preferred = selectedPath && files.some((file) => file.path === selectedPath) ? selectedPath : files[0].path
    setActivePath(preferred)
  }, [activePath, selectedPath, thread?.files])

  useEffect(() => {
    const groups = thread?.groups ?? []
    if (!groups.length) {
      setSelectedGroupID('')
      return
    }
    if (groups.some((group) => group.id === selectedGroupID)) return
    const preferred = groups.find((group) => group.id === thread?.latest_group_id) ?? groups[groups.length - 1]
    setSelectedGroupID(preferred.id)
  }, [selectedGroupID, thread?.groups, thread?.latest_group_id])

  useEffect(() => {
    if (!activePath || !thread?.groups.length) return
    const current = thread.groups.find((group) => group.id === selectedGroupID)
    if (current?.paths?.includes(activePath)) return
    const matching = [...thread.groups].reverse().find((group) => group.paths?.includes(activePath))
    if (matching) setSelectedGroupID(matching.id)
  }, [activePath, selectedGroupID, thread?.groups])

  const selectedFile = useMemo(() => thread?.files.find((file) => file.path === activePath) ?? null, [activePath, thread?.files])
  const selectedFileStats = useMemo(() => selectedFile ? lineDiffStats(selectedFile.before_content, selectedFile.after_content) : null, [selectedFile])
  const selectedGroup = useMemo(() => thread?.groups.find((group) => group.id === selectedGroupID) ?? null, [selectedGroupID, thread?.groups])
  const selectedGroupOwnsFile = !selectedFile || Boolean(selectedGroup?.paths?.includes(selectedFile.path))
  const fileComments = useMemo(() => selectedFile && thread ? commentsForFile(selectedFile, thread.comments) : [], [selectedFile, thread])
  const feedbackComments = useMemo(() => thread ? deriveFeedbackComments(thread) : [], [thread])

  useEffect(() => {
    if (!thread) return
    feedbackCallbackRef.current?.(thread.id, feedbackComments)
  }, [feedbackComments, thread?.id])

  useEffect(() => {
    if (threadQuery.isError) logWorkspaceChangeError('中央变更审阅加载失败', threadQuery.error)
  }, [threadQuery.error, threadQuery.isError])

  const finishWorkspaceMutation = useCallback(async (
    result: WorkspaceChangeMutationResult,
    variables: ReviewVariables | HistoryVariables,
    workspaceMutated: boolean,
  ) => {
    await invalidateWorkspaceChangeQueries(queryClient, variables.workspace)
    if (activeWorkspaceRef.current !== variables.workspace) return
    setError('')
    if (!workspaceMutated || result.workspace !== variables.workspace) return
    const hasReceiptPaths = Object.prototype.hasOwnProperty.call(result, 'affected_paths')
      || Object.prototype.hasOwnProperty.call(result, 'paths')
      || Object.prototype.hasOwnProperty.call(result, 'path')
    const paths = Array.from(new Set(hasReceiptPaths ? [
      ...(result.affected_paths ?? []),
      ...(result.paths ?? []),
      ...(result.path ? [result.path] : []),
    ] : (variables.group.paths ?? [])))
    if (paths.length) await onWorkspaceChanged?.(paths)
  }, [onWorkspaceChanged, queryClient])

  const showError = useCallback((reason: unknown, expectedWorkspace: string) => {
    const message = workspaceChangeErrorMessage(t, reason)
    logWorkspaceChangeError('中央变更审阅请求失败', reason)
    if (activeWorkspaceRef.current !== expectedWorkspace) return
    setError(message)
    toast.error(t('changes.operationFailed'), { description: message })
  }, [t])

  const reviewMutation = useMutation({
    mutationFn: (variables: ReviewVariables) => reviewWorkspaceChangeGroup(variables.workspace, variables.group.id, { decision: variables.decision }),
    onSuccess: async (result, variables) => {
      if (activeWorkspaceRef.current === variables.workspace) {
        toast.success(t(variables.decision === 'accept' ? 'changes.accepted' : 'changes.rejected'))
      }
      await finishWorkspaceMutation(result, variables, variables.decision === 'reject')
    },
    onError: (reason, variables) => showError(reason, variables.workspace),
  })

  const historyMutation = useMutation({
    mutationFn: (variables: HistoryVariables) => variables.action === 'undo'
      ? undoWorkspaceChangeGroup(variables.workspace, variables.group.id)
      : redoWorkspaceChangeGroup(variables.workspace, variables.group.id),
    onSuccess: async (result, variables) => {
      if (activeWorkspaceRef.current === variables.workspace) {
        toast.success(t(variables.action === 'undo' ? 'changes.undoSuccess' : 'changes.redoSuccess'))
      }
      await finishWorkspaceMutation(result, variables, true)
    },
    onError: (reason, variables) => showError(reason, variables.workspace),
  })

  const commentMutation = useMutation({
    mutationFn: (variables: CommentVariables) => {
      switch (variables.action) {
        case 'create':
          return createWorkspaceChangeComment(variables.workspace, variables.request)
        case 'update':
          return updateWorkspaceChangeComment(variables.workspace, variables.comment.id, variables.body)
        case 'resolve':
          return resolveWorkspaceChangeComment(variables.workspace, variables.comment.id, variables.resolved)
        case 'delete':
          return deleteWorkspaceChangeComment(variables.workspace, variables.comment.id)
      }
    },
    onSuccess: async (_result, variables) => {
      await invalidateWorkspaceChangeQueries(queryClient, variables.workspace)
      if (activeWorkspaceRef.current === variables.workspace) setError('')
    },
    onError: (reason, variables) => showError(reason, variables.workspace),
  })

  const busy = disabled || reviewMutation.isPending || historyMutation.isPending || commentMutation.isPending
  const reviewLocked = busy || hasCommentDraft
  const conflict = selectedFile && (selectedFile.continuity !== 'continuous' || selectedFile.apply_state === 'conflicted')

  if (threadQuery.isLoading) {
    return <ReviewSurfaceState onClose={onClose} icon={<Loader2 className="h-5 w-5 animate-spin" />} label={t('changes.loading')} />
  }
  if (threadQuery.isError) {
    return (
      <ReviewSurfaceState
        onClose={onClose}
        icon={<AlertTriangle className="h-5 w-5 text-[var(--nova-danger)]" />}
        label={workspaceChangeErrorMessage(t, threadQuery.error, 'changes.loadFailed')}
        action={<Button type="button" size="sm" variant="outline" onClick={() => void threadQuery.refetch()}>{t('changes.retry')}</Button>}
      />
    )
  }
  if (!thread) return <ReviewSurfaceState onClose={onClose} icon={<FileDiff className="h-5 w-5" />} label={t('changes.noHistoryTitle')} />

  return (
    <section data-change-review-workspace={thread.id} className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-xs text-[var(--nova-text-muted)]" aria-label={t('changes.title')}>
      <ReviewToolbar
        thread={thread}
        selectedGroup={selectedGroup}
        layout={layout}
        busy={reviewLocked}
        refreshing={threadQuery.isFetching}
        actionScopeAvailable={selectedGroupOwnsFile}
        onLayoutChange={setLayout}
        onGroupChange={(groupID) => {
          setSelectedGroupID(groupID)
          const group = thread.groups.find((item) => item.id === groupID)
          if (!group?.paths?.includes(activePath)) {
            const nextPath = group?.paths?.find((path) => thread.files.some((file) => file.path === path))
            if (nextPath) setActivePath(nextPath)
          }
        }}
        onReview={(decision) => selectedGroup && reviewMutation.mutate({ workspace, group: selectedGroup, decision })}
        onHistory={(action) => selectedGroup && historyMutation.mutate({ workspace, group: selectedGroup, action })}
        onRefresh={() => void threadQuery.refetch()}
        onClose={onClose}
      />

      {error && <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">{error}</div>}
      {conflict && (
        <div role="status" className="flex shrink-0 items-start gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" />
          <span>{t('changes.applyState.conflictedDescription')}</span>
        </div>
      )}

      <div className="flex min-h-0 flex-1 max-lg:flex-col">
        <main className="flex min-h-0 min-w-0 flex-1 flex-col">
          {selectedFile ? (
            <>
              <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3">
                <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]" title={selectedFile.path}>{selectedFile.path}</span>
                <span className="font-mono text-[10px] text-[var(--nova-success)]">+{selectedFileStats?.additions ?? 0}</span>
                <span className="font-mono text-[10px] text-[var(--nova-danger)]">−{selectedFileStats?.deletions ?? 0}</span>
                {onOpenFile && (
                  <Button type="button" size="xs" variant="ghost" disabled={reviewLocked} onClick={() => void onOpenFile(selectedFile.path)}>
                    <ExternalLink />{t('changes.openFile')}
                  </Button>
                )}
              </div>
              <div className="min-h-0 flex-1">
                <Suspense fallback={<ReviewState icon={<Loader2 className="h-5 w-5 animate-spin" />} label={t('changes.loading')} />}>
                  <ReviewDiffEditor
                    threadID={thread.id}
                    file={selectedFile}
                    comments={fileComments}
                    layout={layout}
                    busy={busy}
                    onDraftChange={setHasCommentDraft}
                    onCreateComment={(request) => commentMutation.mutateAsync({ action: 'create', workspace, request }).then(() => undefined)}
                    onUpdateComment={(comment, body) => commentMutation.mutateAsync({ action: 'update', workspace, comment, body }).then(() => undefined)}
                    onResolveComment={(comment, resolved) => commentMutation.mutateAsync({ action: 'resolve', workspace, comment, resolved }).then(() => undefined)}
                    onDeleteComment={(comment) => commentMutation.mutateAsync({ action: 'delete', workspace, comment }).then(() => undefined)}
                  />
                </Suspense>
              </div>
            </>
          ) : (
            <ReviewState icon={<Check className="h-5 w-5 text-[var(--nova-success)]" />} label={t('changes.noPendingTitle')} />
          )}
        </main>
        <ReviewFileNavigator files={thread.files} selectedPath={activePath} disabled={reviewLocked} onSelect={setActivePath} />
      </div>
    </section>
  )
}

/** Derives UI-only path/line metadata while preserving the ledger comment as source of truth. */
export function deriveFeedbackComments(thread: ReviewThread): WorkspaceChangeComment[] {
  return thread.comments
    .filter((comment) => !comment.deleted && !comment.resolved)
    .map((comment) => {
      const candidates = thread.files.filter((file) => comment.change_set_id
        ? file.change_set_ids.includes(comment.change_set_id)
        : Boolean(comment.anchor?.revision) && (comment.anchor?.revision === file.base_revision || comment.anchor?.revision === file.revision))
      if (candidates.length !== 1) return comment
      const file = candidates[0]
      const anchor = comment.anchor
      if (!anchor) return { ...comment, review_path: file.path }
      const side = anchor.side ?? (anchor.revision === file.base_revision ? 'before' : 'after')
      const text = side === 'before' ? file.before_content : file.after_content
      const revision = side === 'before' ? file.base_revision : file.revision
      const index = new Utf8OffsetIndex(text)
      const anchoredStart = anchor.start ?? 0
      const anchoredEnd = anchor.end ?? anchoredStart
      let start: number | undefined
      if ((!anchor.encoding || anchor.encoding === 'utf8-bytes-v1')
        && anchor.revision === revision
        && (!anchor.quote || index.sliceBytes(anchoredStart, anchoredEnd) === anchor.quote)) {
        start = anchoredStart
      } else if (anchor.quote) {
        const first = text.indexOf(anchor.quote)
        if (first >= 0 && text.lastIndexOf(anchor.quote) === first) {
          start = index.byteOffsetAtUtf16Offset(first)
        }
      }
      if (start === undefined) {
        return { ...comment, review_path: file.path, review_line: undefined }
      }
      return {
        ...comment,
        review_path: file.path,
        review_line: index.positionAtByteOffset(start).lineNumber,
      }
    })
}

function commentsForFile(file: ReviewThreadFile, comments: WorkspaceChangeComment[]): WorkspaceChangeComment[] {
  return comments.filter((comment) => {
    if (comment.change_set_id) return file.change_set_ids.includes(comment.change_set_id)
    const revision = comment.anchor?.revision
    return Boolean(revision && (revision === file.base_revision || revision === file.revision))
  })
}

function readReviewLayout(): ReviewDiffLayout {
  try {
    const stored = window.localStorage.getItem(REVIEW_LAYOUT_STORAGE_KEY)
    return stored === 'split' || stored === 'unified' ? stored : 'unified'
  } catch {
    return 'unified'
  }
}

function ReviewState({ icon, label, action }: { icon: React.ReactNode; label: string; action?: React.ReactNode }) {
  return (
    <div className="flex h-full min-h-40 flex-1 flex-col items-center justify-center gap-3 bg-[var(--nova-bg)] px-6 text-center text-xs text-[var(--nova-text-faint)]">
      {icon}
      <p className="max-w-md">{label}</p>
      {action}
    </div>
  )
}

function ReviewSurfaceState({ onClose, ...state }: { onClose: () => void; icon: React.ReactNode; label: string; action?: React.ReactNode }) {
  return (
    <section className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-xs text-[var(--nova-text-muted)]">
      <ReviewUtilityTab onClose={onClose} />
      <ReviewState {...state} />
    </section>
  )
}
