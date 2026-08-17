import { useCallback, useEffect, useRef, useState } from 'react'
import type {
  ReviewFeedbackComment,
  ReviewFeedbackSelection,
} from '@/features/changes/agent/ReviewFeedbackTray'
import type { DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import type { DocumentReviewTarget } from '@/features/document-review/types'

export type ReviewFeedbackNavigationTarget = DocumentReviewNavigationIntent & {
  target: DocumentReviewTarget
}

interface UseReviewFeedbackNavigationOptions {
  workspace: string
  selectedFile: string | null
  onSelectFile: (path: string) => boolean | void | Promise<boolean | void>
  onOpenLoreTab: () => Promise<boolean>
  onOpenChangeReview: (reviewThreadID: string, groupID: string) => unknown
}

/** Coordinates feedback-tray navigation without coupling either editor to workbench routing. */
export function useReviewFeedbackNavigation({
  workspace,
  selectedFile,
  onSelectFile,
  onOpenLoreTab,
  onOpenChangeReview,
}: UseReviewFeedbackNavigationOptions) {
  const [target, setTarget] = useState<ReviewFeedbackNavigationTarget | null>(
    null,
  )
  const requestRef = useRef(0)
  const nonceRef = useRef(0)

  useEffect(() => {
    requestRef.current += 1
    setTarget(null)
  }, [workspace])

  const open = useCallback(
    (selection: ReviewFeedbackSelection, comment: ReviewFeedbackComment) => {
      if (selection.source !== 'document') {
        void onOpenChangeReview(
          selection.reviewThreadId,
          comment.group_id || '',
        )
        return
      }
      const resource = comment.target
      if (
        !resource?.id ||
        (resource.kind !== 'workspace_file' && resource.kind !== 'lore_item')
      )
        return

      const requestID = ++requestRef.current
      const reveal = () => {
        if (requestRef.current !== requestID) return
        nonceRef.current += 1
        setTarget({
          target:
            resource.kind === 'lore_item'
              ? { kind: 'lore_item', id: resource.id, field: 'content' }
              : { kind: 'workspace_file', id: resource.id },
          commentID: comment.id,
          nonce: nonceRef.current,
        })
      }
      if (resource.kind === 'lore_item') {
        void onOpenLoreTab()
          .then((navigated) => {
            if (navigated) reveal()
          })
          .catch((error) => {
            console.error(
              '[useReviewFeedbackNavigation] failed to open lore review target',
              { commentID: comment.id, resource, error },
            )
          })
        return
      }
      if (selectedFile === resource.id) {
        reveal()
        return
      }
      void Promise.resolve(onSelectFile(resource.id))
        .then((navigated) => {
          if (navigated !== false) reveal()
        })
        .catch((error) => {
          console.error(
            '[useReviewFeedbackNavigation] failed to open file review target',
            { commentID: comment.id, resource, error },
          )
        })
    },
    [onOpenChangeReview, onOpenLoreTab, onSelectFile, selectedFile],
  )

  return { target, open }
}
