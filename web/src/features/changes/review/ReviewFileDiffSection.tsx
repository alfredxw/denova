import { lazy, Suspense, type RefCallback } from 'react'
import { AlertTriangle, ExternalLink, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DiffFileSection } from '@/features/diff/DiffFileSection'
import type {
  CreateWorkspaceChangeCommentRequest,
  ReviewThreadFile,
  WorkspaceChangeComment,
} from '../types'
import type { ReviewDiffLayout } from './monaco/review-editor-adapter'
import { loadReviewDiffEditor } from './review-editor-loader'

const ReviewDiffEditor = lazy(() => loadReviewDiffEditor().then((module) => ({ default: module.ReviewDiffEditor })))

interface ReviewFileDiffSectionProps {
  threadID: string
  file: ReviewThreadFile
  comments: WorkspaceChangeComment[]
  layout: ReviewDiffLayout
  active: boolean
  preRender?: boolean
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
  onDeleteComment: (comment: WorkspaceChangeComment) => Promise<void>
}

/** Review-specific capabilities layered onto the shared collapsible file Diff section. */
export function ReviewFileDiffSection({ threadID, file, comments, layout, active, preRender = false, collapsed, hasDraft, mutationBusy, navigationLocked, sectionRef, onToggle, onOpenFile, onDraftChange, onCreateComment, onUpdateComment, onDeleteComment }: ReviewFileDiffSectionProps) {
  const { t } = useTranslation()
  const conflicted = file.continuity !== 'continuous' || file.apply_state === 'conflicted'
  const deleted = file.after_exists === false

  return (
    <DiffFileSection
      file={file}
      layout={layout}
      active={active}
      preRender={preRender}
      collapsed={collapsed}
      pinned={hasDraft || comments.length > 0}
      sectionRef={sectionRef}
      onToggle={onToggle}
      headerMeta={(
        <>
          {hasDraft && <span className="mr-2 hidden text-[10px] text-[var(--nova-accent-blue)] sm:inline">{t('changes.commentDraft')}</span>}
          {conflicted && <AlertTriangle className="mr-2 h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" aria-label={t('changes.applyState.conflicted')} />}
        </>
      )}
      action={onOpenFile && !deleted ? (
        <Button type="button" size="xs" variant="ghost" disabled={navigationLocked} onClick={() => void onOpenFile(file.path)}>
          <ExternalLink />{t('changes.openFile')}
        </Button>
      ) : null}
      banner={conflicted ? (
        <div role="status" className="flex items-start gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" />
          <span>{t('changes.applyState.conflictedDescription')}</span>
        </div>
      ) : null}
      renderContent={({ initialHeight, onHeightChange }) => (
        <Suspense fallback={<ReviewEditorLoading label={t('changes.loading')} />}>
          <ReviewDiffEditor
            threadID={threadID}
            file={file}
            comments={comments}
            layout={layout}
            busy={mutationBusy}
            initialHeight={initialHeight}
            onHeightChange={onHeightChange}
            onDraftChange={onDraftChange}
            onCreateComment={onCreateComment}
            onUpdateComment={onUpdateComment}
            onDeleteComment={onDeleteComment}
          />
        </Suspense>
      )}
    />
  )
}

function ReviewEditorLoading({ label }: { label: string }) {
  return <div className="flex h-full items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]"><Loader2 className="h-4 w-4 animate-spin" />{label}</div>
}
