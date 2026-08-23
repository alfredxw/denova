import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, ExternalLink, FileDiff, Loader2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { CodeDiffSurface } from '@/features/diff/CodeDiffSurface'
import type { DiffLayout } from '@/features/diff/types'
import { useMultiFileDiffNavigation } from '@/features/diff/use-multi-file-diff-navigation'
import { lineDiffStats } from '../diff-stats'
import { logWorkspaceChangeError, workspaceChangeErrorMessage } from '../errors'
import {
  createProjectChangeComment,
  deleteProjectChangeComment,
  redoProjectChangeGroup,
  reviewProjectChangeGroup,
  undoProjectChangeGroup,
  updateProjectChangeComment,
} from '../api'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThread,
  WorkspaceChangeComment,
  WorkspaceChangeGroupSummary,
  WorkspaceChangeMutationResult,
} from '../types'
import { invalidateProjectChangeQueries, useProjectChangeGroup, useProjectChangeReviewThread } from '../use-change-review'
import { reviewFileNavigationItems } from './ReviewFileNavigator'
import { ReviewToolbar } from './ReviewToolbar'
import { Utf8OffsetIndex } from './utf8-offset-index'
import { projectReviewGroupFiles } from './review-group-projection'
import { useReviewDiffAnnotations } from './use-review-diff-annotations'
import type { ChangeReviewScopeRequest } from '../use-writing-change-review'

const REVIEW_LAYOUT_STORAGE_KEY = 'nova:change-review-layout'
const REVIEW_SCOPE_THREAD = 'thread'

interface ChangeReviewWorkspaceProps {
  projectId: string
  threadID: string
  scopeRequest?: ChangeReviewScopeRequest | null
  /** Prevents mutating a review thread while its Agent run is still appending changes. */
  disabled?: boolean
  selectedPath?: string | null
  agentVisible?: boolean
  onToggleAgent?: () => void
  /** Standalone Writing review closes from its toolbar; workbench review closes from its real tab. */
  onClose?: () => void
  onOpenFile?: (path: string) => void | Promise<void>
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
  onFeedbackCommentsChange?: (threadID: string, comments: WorkspaceChangeComment[]) => void
  hiddenCommentIDs?: ReadonlySet<string>
}

type ReviewVariables = {
  projectId: string
  group: WorkspaceChangeGroupSummary
  decision: 'accept' | 'reject'
}

type HistoryVariables = {
  projectId: string
  group: WorkspaceChangeGroupSummary
  action: 'undo' | 'redo'
}

type CommentVariables =
  | { action: 'create'; projectId: string; request: CreateWorkspaceChangeCommentRequest }
  | { action: 'update'; projectId: string; comment: WorkspaceChangeComment; body: string }
  | { action: 'delete'; projectId: string; comment: WorkspaceChangeComment }

/** Full-width, server-projected review surface rendered in the central editor region. */
export function ChangeReviewWorkspace({ projectId, threadID, scopeRequest, disabled = false, selectedPath, agentVisible = false, onToggleAgent, onClose, onOpenFile, onWorkspaceChanged, onFeedbackCommentsChange, hiddenCommentIDs }: ChangeReviewWorkspaceProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const threadQuery = useProjectChangeReviewThread(projectId, threadID)
  const [layout, setLayout] = useState<DiffLayout>(readReviewLayout)
  const [selectedScopeID, setSelectedScopeID] = useState(REVIEW_SCOPE_THREAD)
  const [error, setError] = useState('')
  const historicalGroupID = selectedScopeID === REVIEW_SCOPE_THREAD ? '' : selectedScopeID
  const historicalGroupQuery = useProjectChangeGroup(projectId, historicalGroupID)
  const thread = threadQuery.data
  const historicalGroup = historicalGroupQuery.data
  const reviewFiles = useMemo(() => selectedScopeID === REVIEW_SCOPE_THREAD
    ? (thread?.files ?? [])
    : projectReviewGroupFiles(historicalGroup), [historicalGroup, selectedScopeID, thread?.files])
  const reviewPaths = useMemo(() => reviewFiles.map((file) => file.path), [reviewFiles])
  const navigatorFiles = useMemo(() => reviewFileNavigationItems(reviewFiles), [reviewFiles])
  const navigation = useMultiFileDiffNavigation({
    identity: `${projectId}:${threadID}:${selectedScopeID}`,
    paths: reviewPaths,
    preferredPath: selectedPath,
  })
  const reviewTotals = useMemo(() => reviewFiles.reduce((totals, file) => {
    const stats = file.additions === undefined || file.deletions === undefined
      ? lineDiffStats(file.before_content, file.after_content)
      : { additions: file.additions, deletions: file.deletions }
    totals.additions += stats.additions
    totals.deletions += stats.deletions
    return totals
  }, { additions: 0, deletions: 0 }), [reviewFiles])
  const reviewComments = (selectedScopeID === REVIEW_SCOPE_THREAD ? (thread?.comments ?? []) : (historicalGroup?.comments ?? []))
    .filter((comment) => !hiddenCommentIDs?.has(comment.id))
  const activeProjectRef = useRef(projectId)
  const feedbackCallbackRef = useRef(onFeedbackCommentsChange)
  const appliedScopeRequestRef = useRef(0)
  activeProjectRef.current = projectId
  feedbackCallbackRef.current = onFeedbackCommentsChange

  useEffect(() => {
    try {
      window.localStorage.setItem(REVIEW_LAYOUT_STORAGE_KEY, layout)
    } catch {
      // Browser privacy settings may disable storage; the in-memory choice still works.
    }
  }, [layout])

  useEffect(() => {
    setSelectedScopeID(REVIEW_SCOPE_THREAD)
    setError('')
    appliedScopeRequestRef.current = 0
  }, [projectId, threadID])

  useEffect(() => {
    if (!scopeRequest || scopeRequest.threadID !== threadID || appliedScopeRequestRef.current === scopeRequest.id) return
    if (scopeRequest.groupID && !thread?.groups.some((group) => group.id === scopeRequest.groupID)) return
    appliedScopeRequestRef.current = scopeRequest.id
    setSelectedScopeID(scopeRequest.groupID || REVIEW_SCOPE_THREAD)
  }, [scopeRequest, thread?.groups, threadID])

  useEffect(() => {
    if (selectedScopeID === REVIEW_SCOPE_THREAD) return
    if (thread?.groups.some((group) => group.id === selectedScopeID)) return
    setSelectedScopeID(REVIEW_SCOPE_THREAD)
  }, [selectedScopeID, thread?.groups])

  const selectedGroupID = selectedScopeID === REVIEW_SCOPE_THREAD ? thread?.latest_group_id : selectedScopeID
  const selectedGroup = useMemo(() => thread?.groups.find((group) => group.id === selectedGroupID) ?? null, [selectedGroupID, thread?.groups])
  const feedbackComments = useMemo(() => thread ? deriveFeedbackComments(thread) : [], [thread])

  useEffect(() => {
    if (!thread) return
    feedbackCallbackRef.current?.(thread.id, feedbackComments)
  }, [feedbackComments, thread?.id])

  useEffect(() => {
    if (threadQuery.isError) logWorkspaceChangeError('中央变更审阅加载失败', threadQuery.error)
  }, [threadQuery.error, threadQuery.isError])

  useEffect(() => {
    if (historicalGroupQuery.isError) logWorkspaceChangeError('历史变更审阅加载失败', historicalGroupQuery.error)
  }, [historicalGroupQuery.error, historicalGroupQuery.isError])

  const finishWorkspaceMutation = useCallback(async (
    result: WorkspaceChangeMutationResult,
    variables: ReviewVariables | HistoryVariables,
    workspaceMutated: boolean,
  ) => {
    await invalidateProjectChangeQueries(queryClient, variables.projectId)
    if (activeProjectRef.current !== variables.projectId) return
    setError('')
    if (!workspaceMutated || result.project_id !== variables.projectId) return
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

  const showError = useCallback((reason: unknown, expectedProjectId: string) => {
    const message = workspaceChangeErrorMessage(t, reason)
    logWorkspaceChangeError('中央变更审阅请求失败', reason)
    if (activeProjectRef.current !== expectedProjectId) return
    setError(message)
    toast.error(t('changes.operationFailed'), { description: message })
  }, [t])

  const reviewMutation = useMutation({
    mutationFn: (variables: ReviewVariables) => reviewProjectChangeGroup(variables.projectId, variables.group.id, { decision: variables.decision }),
    onSuccess: async (result, variables) => {
      if (activeProjectRef.current === variables.projectId) {
        toast.success(t(variables.decision === 'accept' ? 'changes.accepted' : 'changes.rejected'))
      }
      await finishWorkspaceMutation(result, variables, variables.decision === 'reject')
    },
    onError: (reason, variables) => showError(reason, variables.projectId),
  })

  const historyMutation = useMutation({
    mutationFn: (variables: HistoryVariables) => variables.action === 'undo'
      ? undoProjectChangeGroup(variables.projectId, variables.group.id)
      : redoProjectChangeGroup(variables.projectId, variables.group.id),
    onSuccess: async (result, variables) => {
      if (activeProjectRef.current === variables.projectId) {
        toast.success(t(variables.action === 'undo' ? 'changes.undoSuccess' : 'changes.redoSuccess'))
      }
      await finishWorkspaceMutation(result, variables, true)
    },
    onError: (reason, variables) => showError(reason, variables.projectId),
  })

  const commentMutation = useMutation({
    mutationFn: (variables: CommentVariables) => {
      switch (variables.action) {
        case 'create':
          return createProjectChangeComment(variables.projectId, variables.request)
        case 'update':
          return updateProjectChangeComment(variables.projectId, variables.comment.id, variables.body)
        case 'delete':
          return deleteProjectChangeComment(variables.projectId, variables.comment.id)
      }
    },
    onSuccess: async (_result, variables) => {
      await invalidateProjectChangeQueries(queryClient, variables.projectId)
      if (activeProjectRef.current === variables.projectId) setError('')
    },
    onError: (reason, variables) => showError(reason, variables.projectId),
  })

  const busy = disabled || reviewMutation.isPending || historyMutation.isPending || commentMutation.isPending
  const reviewAnnotations = useReviewDiffAnnotations({
    identity: `${projectId}:${threadID}:${selectedScopeID}`,
    files: reviewFiles,
    comments: reviewComments,
    busy,
    onCreate: (request) => commentMutation.mutateAsync({ action: 'create', projectId, request }).then(() => undefined),
    onUpdate: (comment, body) => commentMutation.mutateAsync({ action: 'update', projectId, comment, body }).then(() => undefined),
    onDelete: (comment) => commentMutation.mutateAsync({ action: 'delete', projectId, comment }).then(() => undefined),
  })
  const reviewLocked = busy || reviewAnnotations.hasDraft
  const scopeLoading = selectedScopeID !== REVIEW_SCOPE_THREAD && historicalGroupQuery.isLoading
  const scopeError = selectedScopeID !== REVIEW_SCOPE_THREAD && historicalGroupQuery.isError

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
        selectedScopeID={selectedScopeID}
        fileCount={reviewFiles.length}
        additions={reviewTotals.additions}
        deletions={reviewTotals.deletions}
        layout={layout}
        busy={reviewLocked}
        refreshing={threadQuery.isFetching || historicalGroupQuery.isFetching}
        allDiffsCollapsed={navigation.allDiffsCollapsed}
        navigatorVisible={navigation.navigatorVisible}
        agentVisible={agentVisible}
        onLayoutChange={setLayout}
        onScopeChange={setSelectedScopeID}
        onReview={(decision) => selectedGroup && reviewMutation.mutate({ projectId, group: selectedGroup, decision })}
        onHistory={(action) => selectedGroup && historyMutation.mutate({ projectId, group: selectedGroup, action })}
        onRefresh={() => {
          void threadQuery.refetch()
          if (selectedScopeID !== REVIEW_SCOPE_THREAD) void historicalGroupQuery.refetch()
        }}
        onToggleAllDiffs={navigation.toggleAllDiffs}
        onToggleNavigator={() => navigation.setNavigatorVisible((visible) => !visible)}
        onToggleAgent={onToggleAgent}
        onClose={onClose}
      />

      {error && <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">{error}</div>}

      <CodeDiffSurface
        files={scopeLoading || scopeError ? [] : reviewFiles}
        navigatorFiles={scopeLoading || scopeError ? [] : navigatorFiles}
        navigation={navigation}
        layout={layout}
        ariaLabel={t('changes.viewDiff')}
        annotationsByPath={reviewAnnotations.annotationsByPath}
        annotationRevisionByPath={reviewAnnotations.annotationRevisionByPath}
        renderAnnotation={reviewAnnotations.renderAnnotation}
        onLineSelectionEnd={reviewAnnotations.onLineSelectionEnd}
        renderHeaderMeta={(file) => {
          const reviewFile = reviewFiles.find((candidate) => candidate.path === file.path)
          const conflicted = reviewFile && (reviewFile.continuity !== 'continuous' || reviewFile.apply_state === 'conflicted')
          return (
            <>
              {reviewAnnotations.draftPaths.has(file.path) && <span className="mr-2 hidden text-[10px] text-[var(--nova-accent-blue)] sm:inline">{t('changes.commentDraft')}</span>}
              {conflicted && <AlertTriangle className="mr-2 size-3.5 shrink-0 text-[var(--nova-warning)]" aria-label={t('changes.applyState.conflicted')} />}
            </>
          )
        }}
        renderHeaderAction={(file) => onOpenFile && file.after_exists !== false ? (
          <Button type="button" size="icon-xs" variant="ghost" disabled={reviewLocked} onClick={() => void onOpenFile(file.path)} aria-label={t('changes.openFile')}>
            <ExternalLink />
          </Button>
        ) : null}
        empty={scopeLoading ? (
            <ReviewState icon={<Loader2 className="h-5 w-5 animate-spin" />} label={t('changes.loading')} />
          ) : scopeError ? (
            <ReviewState
              icon={<AlertTriangle className="h-5 w-5 text-[var(--nova-danger)]" />}
              label={workspaceChangeErrorMessage(t, historicalGroupQuery.error, 'changes.loadFailed')}
              action={<Button type="button" size="sm" variant="outline" onClick={() => void historicalGroupQuery.refetch()}>{t('changes.retry')}</Button>}
            />
          ) : (
            <ReviewState icon={<Check className="h-5 w-5 text-[var(--nova-success)]" />} label={t('changes.noPendingTitle')} />
          )}
      />
    </section>
  )
}

/** Derives UI-only path/line metadata while preserving the ledger comment as source of truth. */
export function deriveFeedbackComments(thread: ReviewThread): WorkspaceChangeComment[] {
  return thread.comments
    .filter((comment) => !comment.deleted)
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

function readReviewLayout(): DiffLayout {
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

function ReviewSurfaceState({ onClose, ...state }: { onClose?: () => void; icon: React.ReactNode; label: string; action?: React.ReactNode }) {
  const { t } = useTranslation()
  return (
    <section className="relative flex h-full min-h-0 flex-col bg-[var(--nova-bg)] text-xs text-[var(--nova-text-muted)]">
      {onClose ? (
        <Button type="button" size="icon-xs" variant="ghost" onClick={onClose} className="absolute right-2 top-2 z-10" aria-label={t('common.close')}><X /></Button>
      ) : null}
      <ReviewState {...state} />
    </section>
  )
}
