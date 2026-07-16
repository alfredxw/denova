import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Check, FileDiff, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
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
import { invalidateWorkspaceChangeQueries, useWorkspaceChangeGroup, useWorkspaceChangeReviewThread } from '../use-change-review'
import { ReviewFileDiffSection } from './ReviewFileDiffSection'
import { ReviewFileNavigator } from './ReviewFileNavigator'
import { ReviewToolbar } from './ReviewToolbar'
import { ReviewUtilityTab } from './ReviewUtilityTab'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { Utf8OffsetIndex } from './monaco/utf8-offset-index'
import { projectReviewGroupFiles } from './review-group-projection'
import type { ChangeReviewScopeRequest } from '../use-writing-change-review'
import './review-diff.css'

const REVIEW_LAYOUT_STORAGE_KEY = 'nova:change-review-layout'
const REVIEW_SCOPE_THREAD = 'thread'

interface ChangeReviewWorkspaceProps {
  workspace: string
  threadID: string
  scopeRequest?: ChangeReviewScopeRequest | null
  /** Prevents mutating a review thread while its Agent run is still appending changes. */
  disabled?: boolean
  selectedPath?: string | null
  agentVisible?: boolean
  onToggleAgent?: () => void
  onClose: () => void
  onOpenFile?: (path: string) => void | Promise<void>
  onWorkspaceChanged?: (paths: string[]) => void | Promise<void>
  onFeedbackCommentsChange?: (threadID: string, comments: WorkspaceChangeComment[]) => void
  hiddenCommentIDs?: ReadonlySet<string>
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
export function ChangeReviewWorkspace({ workspace, threadID, scopeRequest, disabled = false, selectedPath, agentVisible = false, onToggleAgent, onClose, onOpenFile, onWorkspaceChanged, onFeedbackCommentsChange, hiddenCommentIDs }: ChangeReviewWorkspaceProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const threadQuery = useWorkspaceChangeReviewThread(workspace, threadID)
  const [layout, setLayout] = useState<ReviewDiffLayout>(readReviewLayout)
  const [activePath, setActivePath] = useState('')
  const [selectedScopeID, setSelectedScopeID] = useState(REVIEW_SCOPE_THREAD)
  const [error, setError] = useState('')
  const [commentDraftPaths, setCommentDraftPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [collapsedPaths, setCollapsedPaths] = useState<ReadonlySet<string>>(() => new Set())
  const [navigatorVisible, setNavigatorVisible] = useState(true)
  const freezeProjection = commentDraftPaths.size > 0
  const historicalGroupID = selectedScopeID === REVIEW_SCOPE_THREAD ? '' : selectedScopeID
  const historicalGroupQuery = useWorkspaceChangeGroup(workspace, historicalGroupID)
  const thread = useFrozenReviewValue(`${workspace}:${threadID}`, threadQuery.data, freezeProjection)
  const historicalGroup = useFrozenReviewValue(`${workspace}:${historicalGroupID}`, historicalGroupQuery.data, freezeProjection)
  const reviewFiles = useMemo(() => selectedScopeID === REVIEW_SCOPE_THREAD
    ? (thread?.files ?? [])
    : projectReviewGroupFiles(historicalGroup), [historicalGroup, selectedScopeID, thread?.files])
  const reviewComments = (selectedScopeID === REVIEW_SCOPE_THREAD ? (thread?.comments ?? []) : (historicalGroup?.comments ?? []))
    .filter((comment) => !hiddenCommentIDs?.has(comment.id))
  const activeWorkspaceRef = useRef(workspace)
  const feedbackCallbackRef = useRef(onFeedbackCommentsChange)
  const reviewScrollRef = useRef<HTMLDivElement | null>(null)
  const fileSectionRefs = useRef(new Map<string, HTMLElement>())
  const scrollFrameRef = useRef<number | null>(null)
  const jumpFrameRef = useRef<number | null>(null)
  const pendingJumpPathRef = useRef('')
  const appliedScopeRequestRef = useRef(0)
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
    setSelectedScopeID(REVIEW_SCOPE_THREAD)
    setError('')
    setCommentDraftPaths(new Set())
    setCollapsedPaths(new Set())
    setNavigatorVisible(true)
    appliedScopeRequestRef.current = 0
    fileSectionRefs.current.clear()
  }, [threadID, workspace])

  useEffect(() => {
    if (!scopeRequest || scopeRequest.threadID !== threadID || appliedScopeRequestRef.current === scopeRequest.id) return
    if (scopeRequest.groupID && !thread?.groups.some((group) => group.id === scopeRequest.groupID)) return
    appliedScopeRequestRef.current = scopeRequest.id
    setSelectedScopeID(scopeRequest.groupID || REVIEW_SCOPE_THREAD)
  }, [scopeRequest, thread?.groups, threadID])

  useEffect(() => {
    setActivePath('')
    setCollapsedPaths(new Set())
    fileSectionRefs.current.clear()
  }, [selectedScopeID])

  useEffect(() => {
    const files = reviewFiles
    if (!files.length) {
      setActivePath('')
      return
    }
    if (files.some((file) => file.path === activePath)) return
    const preferred = selectedPath && files.some((file) => file.path === selectedPath) ? selectedPath : files[0].path
    setActivePath(preferred)
  }, [activePath, reviewFiles, selectedPath])

  useEffect(() => {
    const available = new Set(reviewFiles.map((file) => file.path))
    setCollapsedPaths((current) => {
      const next = new Set([...current].filter((path) => available.has(path)))
      return next.size === current.size ? current : next
    })
    setCommentDraftPaths((current) => {
      const next = new Set([...current].filter((path) => available.has(path)))
      return next.size === current.size ? current : next
    })
  }, [reviewFiles])

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
  const reviewLocked = busy || commentDraftPaths.size > 0
  const allDiffsCollapsed = reviewFiles.length > 0 && reviewFiles.every((file) => collapsedPaths.has(file.path))
  const scopeLoading = selectedScopeID !== REVIEW_SCOPE_THREAD && historicalGroupQuery.isLoading
  const scopeError = selectedScopeID !== REVIEW_SCOPE_THREAD && historicalGroupQuery.isError

  const handleDraftChange = useCallback((path: string, hasDraft: boolean) => {
    setCommentDraftPaths((current) => {
      if (hasDraft === current.has(path)) return current
      const next = new Set(current)
      if (hasDraft) next.add(path)
      else next.delete(path)
      return next
    })
  }, [])

  const registerFileSection = useCallback((path: string, node: HTMLElement | null) => {
    if (node) fileSectionRefs.current.set(path, node)
    else fileSectionRefs.current.delete(path)
  }, [])

  const toggleFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      const next = new Set(current)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }, [])

  const toggleAllDiffs = useCallback(() => {
    setCollapsedPaths(allDiffsCollapsed ? new Set() : new Set(reviewFiles.map((file) => file.path)))
  }, [allDiffsCollapsed, reviewFiles])

  const cancelPendingJump = useCallback(() => {
    pendingJumpPathRef.current = ''
    if (jumpFrameRef.current !== null) {
      window.cancelAnimationFrame(jumpFrameRef.current)
      jumpFrameRef.current = null
    }
  }, [])

  const scheduleFileNavigation = useCallback((path: string) => {
    pendingJumpPathRef.current = path
    if (jumpFrameRef.current !== null) window.cancelAnimationFrame(jumpFrameRef.current)
    jumpFrameRef.current = window.requestAnimationFrame(() => {
      jumpFrameRef.current = null
      const pendingPath = pendingJumpPathRef.current
      pendingJumpPathRef.current = ''
      const section = fileSectionRefs.current.get(pendingPath)
      if (typeof section?.scrollIntoView === 'function') {
        section.scrollIntoView({ behavior: 'auto', block: 'start', inline: 'nearest' })
      }
    })
  }, [])

  const jumpToFile = useCallback((path: string) => {
    setActivePath(path)
    setCollapsedPaths((current) => {
      if (!current.has(path)) return current
      const next = new Set(current)
      next.delete(path)
      return next
    })
    scheduleFileNavigation(path)
  }, [scheduleFileNavigation])

  const syncActivePathFromScroll = useCallback(() => {
    scrollFrameRef.current = null
    const scroll = reviewScrollRef.current
    if (!scroll || reviewFiles.length === 0) return
    if (pendingJumpPathRef.current) return
    const activationLine = scroll.getBoundingClientRect().top + 48
    let nextPath = reviewFiles[0].path
    for (const file of reviewFiles) {
      const section = fileSectionRefs.current.get(file.path)
      if (!section || section.getBoundingClientRect().top > activationLine) break
      nextPath = file.path
    }
    if (scroll.scrollHeight - scroll.scrollTop - scroll.clientHeight <= 2) {
      nextPath = reviewFiles[reviewFiles.length - 1].path
    }
    setActivePath((current) => current === nextPath ? current : nextPath)
  }, [reviewFiles])

  const handleReviewScroll = useCallback(() => {
    cancelPendingJump()
    if (scrollFrameRef.current !== null) return
    scrollFrameRef.current = window.requestAnimationFrame(syncActivePathFromScroll)
  }, [cancelPendingJump, syncActivePathFromScroll])

  useEffect(() => () => {
    if (scrollFrameRef.current !== null) window.cancelAnimationFrame(scrollFrameRef.current)
    cancelPendingJump()
  }, [cancelPendingJump])

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
        layout={layout}
        busy={reviewLocked}
        refreshing={threadQuery.isFetching || historicalGroupQuery.isFetching}
        allDiffsCollapsed={allDiffsCollapsed}
        navigatorVisible={navigatorVisible}
        agentVisible={agentVisible}
        onLayoutChange={setLayout}
        onScopeChange={setSelectedScopeID}
        onReview={(decision) => selectedGroup && reviewMutation.mutate({ workspace, group: selectedGroup, decision })}
        onHistory={(action) => selectedGroup && historyMutation.mutate({ workspace, group: selectedGroup, action })}
        onRefresh={() => {
          void threadQuery.refetch()
          if (selectedScopeID !== REVIEW_SCOPE_THREAD) void historicalGroupQuery.refetch()
        }}
        onToggleAllDiffs={toggleAllDiffs}
        onToggleNavigator={() => setNavigatorVisible((visible) => !visible)}
        onToggleAgent={onToggleAgent}
        onClose={onClose}
      />

      {error && <div role="alert" className="shrink-0 border-b border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">{error}</div>}

      <div className="nova-review-container min-h-0 flex-1">
        <div className="nova-review-layout flex h-full min-h-0">
          <ScrollArea
            type="always"
            data-review-scroll-root="true"
            className="min-h-0 min-w-0 flex-1 overflow-hidden"
            viewportRef={reviewScrollRef}
            viewportProps={{
              role: 'main',
              tabIndex: 0,
              'aria-label': t('changes.viewDiff'),
              onScroll: handleReviewScroll,
              onWheelCapture: cancelPendingJump,
              onPointerDownCapture: cancelPendingJump,
              onKeyDownCapture: cancelPendingJump,
              onTouchStartCapture: cancelPendingJump,
              className: 'nova-review-scrollbar overscroll-contain focus-visible:ring-0 focus-visible:outline focus-visible:outline-1 focus-visible:outline-offset-[-1px] focus-visible:outline-[var(--nova-accent-blue)]',
            }}
          >
          {scopeLoading ? (
            <ReviewState icon={<Loader2 className="h-5 w-5 animate-spin" />} label={t('changes.loading')} />
          ) : scopeError ? (
            <ReviewState
              icon={<AlertTriangle className="h-5 w-5 text-[var(--nova-danger)]" />}
              label={workspaceChangeErrorMessage(t, historicalGroupQuery.error, 'changes.loadFailed')}
              action={<Button type="button" size="sm" variant="outline" onClick={() => void historicalGroupQuery.refetch()}>{t('changes.retry')}</Button>}
            />
          ) : reviewFiles.length > 0 ? (
            reviewFiles.map((file) => (
              <ReviewFileDiffSection
                key={file.path}
                threadID={thread.id}
                file={file}
                comments={commentsForFile(file, reviewComments)}
                layout={layout}
                active={file.path === activePath}
                collapsed={collapsedPaths.has(file.path)}
                hasDraft={commentDraftPaths.has(file.path)}
                mutationBusy={busy}
                navigationLocked={reviewLocked}
                sectionRef={(node) => registerFileSection(file.path, node)}
                onToggle={() => toggleFile(file.path)}
                onOpenFile={onOpenFile}
                onDraftChange={(hasDraft) => handleDraftChange(file.path, hasDraft)}
                onCreateComment={(request) => commentMutation.mutateAsync({ action: 'create', workspace, request }).then(() => undefined)}
                onUpdateComment={(comment, body) => commentMutation.mutateAsync({ action: 'update', workspace, comment, body }).then(() => undefined)}
                onResolveComment={(comment, resolved) => commentMutation.mutateAsync({ action: 'resolve', workspace, comment, resolved }).then(() => undefined)}
                onDeleteComment={(comment) => commentMutation.mutateAsync({ action: 'delete', workspace, comment }).then(() => undefined)}
              />
            ))
          ) : (
            <ReviewState icon={<Check className="h-5 w-5 text-[var(--nova-success)]" />} label={t('changes.noPendingTitle')} />
          )}
          </ScrollArea>
          {navigatorVisible && (
            <ReviewFileNavigator
              files={reviewFiles}
              selectedPath={activePath}
              onSelect={jumpToFile}
              onCollapse={() => setNavigatorVisible(false)}
            />
          )}
        </div>
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

/** Keeps Monaco's source snapshots stable while a local inline comment is being composed. */
function useFrozenReviewValue<T>(identity: string, liveValue: T | undefined, frozen: boolean): T | undefined {
  const snapshotRef = useRef<{ identity: string; value: T | undefined }>({ identity, value: liveValue })
  if (snapshotRef.current.identity !== identity) {
    snapshotRef.current = { identity, value: liveValue }
  } else if (!frozen) {
    snapshotRef.current.value = liveValue
  }
  return frozen ? snapshotRef.current.value : liveValue
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
