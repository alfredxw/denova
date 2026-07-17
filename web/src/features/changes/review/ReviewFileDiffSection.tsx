import { lazy, Suspense, useEffect, useMemo, useRef, useState, type RefCallback } from 'react'
import { AlertTriangle, ChevronDown, ChevronRight, ExternalLink, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { lineDiffStats } from '../diff-stats'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThreadFile,
  WorkspaceChangeComment,
} from '../types'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { estimateUnifiedReviewLineCount } from './monaco/unified-review-projection'

const ReviewDiffEditor = lazy(() => import('./ReviewDiffEditor').then((module) => ({ default: module.ReviewDiffEditor })))

interface ReviewFileDiffSectionProps {
  threadID: string
  file: ReviewThreadFile
  comments: WorkspaceChangeComment[]
  layout: ReviewDiffLayout
  active: boolean
  collapsed: boolean
  hasDraft: boolean
  mutationBusy: boolean
  navigationLocked: boolean
  sectionRef: RefCallback<HTMLElement>
  onToggle: () => void
  onOpenFile?: (path: string) => void | Promise<void>
  onDraftChange: (hasDraft: boolean) => void
  onCreateComment: (request: CreateWorkspaceChangeCommentRequest) => Promise<void>
  onUpdateComment: (comment: WorkspaceChangeComment, body: string) => Promise<void>
  onResolveComment: (comment: WorkspaceChangeComment, resolved: boolean) => Promise<void>
  onDeleteComment: (comment: WorkspaceChangeComment) => Promise<void>
}

/** One independently collapsible file in the thread-wide review scroll surface. */
export function ReviewFileDiffSection({
  threadID,
  file,
  comments,
  layout,
  active,
  collapsed,
  hasDraft,
  mutationBusy,
  navigationLocked,
  sectionRef,
  onToggle,
  onOpenFile,
  onDraftChange,
  onCreateComment,
  onUpdateComment,
  onResolveComment,
  onDeleteComment,
}: ReviewFileDiffSectionProps) {
  const { t } = useTranslation()
  const contentRef = useRef<HTMLDivElement | null>(null)
  const [nearViewport, setNearViewport] = useState(() => typeof window === 'undefined' || !('IntersectionObserver' in window))
  const stats = useMemo(() => lineDiffStats(file.before_content, file.after_content), [file.after_content, file.before_content])
  const estimatedHeight = useMemo(() => reviewEditorHeight(file, layout), [file, layout])
  const [measuredHeight, setMeasuredHeight] = useState(estimatedHeight)
  const measuredHeightRef = useRef(estimatedHeight)
  const conflicted = file.continuity !== 'continuous' || file.apply_state === 'conflicted'
  const renderEditor = hasDraft || (!collapsed && (comments.length > 0 || nearViewport || active))

  useEffect(() => {
    measuredHeightRef.current = estimatedHeight
    setMeasuredHeight(estimatedHeight)
  }, [estimatedHeight, file.base_revision, file.path, file.revision, layout])

  useEffect(() => {
    if (active) setNearViewport(true)
  }, [active])

  useEffect(() => {
    if (collapsed) {
      if (!hasDraft) setNearViewport(false)
      return
    }
    const node = contentRef.current
    if (!node || !('IntersectionObserver' in window)) {
      setNearViewport(true)
      return
    }
    const observer = new window.IntersectionObserver((entries) => {
      setNearViewport(entries.some((entry) => entry.isIntersecting))
    }, { rootMargin: '640px 0px' })
    observer.observe(node)
    return () => observer.disconnect()
  }, [collapsed, hasDraft])

  return (
    <section
      ref={sectionRef}
      data-review-file={file.path}
      className="scroll-mt-1 border-b border-[var(--nova-border)] bg-[var(--nova-bg)]"
      aria-label={file.path}
    >
      <div className={`sticky top-0 z-20 flex min-h-9 items-center border-l-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] pr-2 ${active ? 'border-l-[var(--nova-accent-blue)]' : 'border-l-transparent'}`}>
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={t(collapsed ? 'changes.expandFile' : 'changes.collapseFile', { path: file.path })}
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 self-stretch px-2 text-left hover:bg-[var(--nova-hover)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--nova-accent-blue)]"
        >
          {collapsed ? <ChevronRight className="h-3.5 w-3.5 shrink-0" /> : <ChevronDown className="h-3.5 w-3.5 shrink-0" />}
          <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]" title={file.path}>{file.path}</span>
        </button>
        {hasDraft && <span className="mr-2 hidden text-[10px] text-[var(--nova-accent-blue)] sm:inline">{t('changes.commentDraft')}</span>}
        {conflicted && <AlertTriangle className="mr-2 h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" aria-label={t('changes.applyState.conflicted')} />}
        <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-success)]">+{stats.additions}</span>
        <span className="mr-1.5 font-mono text-[10px] text-[var(--nova-danger)]">−{stats.deletions}</span>
        {onOpenFile && (
          <Button type="button" size="xs" variant="ghost" disabled={navigationLocked} onClick={() => void onOpenFile(file.path)}>
            <ExternalLink />{t('changes.openFile')}
          </Button>
        )}
      </div>

      <div ref={contentRef} data-review-file-content={file.path} hidden={collapsed}>
        {conflicted && (
          <div role="status" className="flex items-start gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" />
            <span>{t('changes.applyState.conflictedDescription')}</span>
          </div>
        )}
        <div style={{ height: measuredHeight }}>
          {renderEditor ? (
            <Suspense fallback={<ReviewEditorLoading label={t('changes.loading')} />}>
              <ReviewDiffEditor
                threadID={threadID}
                file={file}
                comments={comments}
                layout={layout}
                busy={mutationBusy}
                initialHeight={estimatedHeight}
                onHeightChange={(height) => {
                  if (height <= 0 || measuredHeightRef.current === height) return
                  measuredHeightRef.current = height
                  setMeasuredHeight(height)
                }}
                onDraftChange={onDraftChange}
                onCreateComment={onCreateComment}
                onUpdateComment={onUpdateComment}
                onResolveComment={onResolveComment}
                onDeleteComment={onDeleteComment}
              />
            </Suspense>
          ) : <ReviewEditorLoading label={t('changes.loading')} />}
        </div>
      </div>
    </section>
  )
}

/** Estimates the full editor height until Monaco reports its wrapped content height. */
export function reviewEditorHeight(file: Pick<ReviewThreadFile, 'before_content' | 'after_content'>, layout: ReviewDiffLayout = 'unified'): number {
  const lines = layout === 'unified'
    ? estimateUnifiedReviewLineCount(file.before_content, file.after_content)
    : Math.max(lineCount(file.before_content), lineCount(file.after_content))
  return Math.max(160, 28 + lines * 18)
}

function lineCount(value: string): number {
  if (!value) return 1
  return value.split('\n').length
}

function ReviewEditorLoading({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
      <Loader2 className="h-4 w-4 animate-spin" />{label}
    </div>
  )
}
