import { useCallback, useEffect, useRef, useState } from 'react'
import type { TFunction } from 'i18next'
import { toast } from 'sonner'
import { createAgentCommandID, type InteractiveImage } from '@/lib/api'
import { agentCommandRetryKey, isKnownAgentCommandOutcome, rememberAgentCommandID } from '@/lib/agent-command'
import { generateInteractiveImage } from '../../api'
import type { Snapshot } from '../../types'

interface UseStoryImagesOptions {
  stageKey: string
  storyId: string
  branchId: string
  snapshot: Snapshot | null
  t: TFunction
  onDone: (options?: { silent?: boolean }) => void | Promise<Snapshot | void>
  setActivity: (content: string) => void
}

// Keeps optimistic image projection and generation lifecycle separate from the
// chat stream lifecycle. Image completion refreshes the persisted snapshot but
// never mutates the active agent operation.
export function useStoryImages({
  stageKey,
  storyId,
  branchId,
  snapshot,
  t,
  onDone,
  setActivity,
}: UseStoryImagesOptions) {
  const [optimisticImages, setOptimisticImages] = useState<Record<string, InteractiveImage[]>>({})
  const [generatingTurnId, setGeneratingTurnId] = useState<string | null>(null)
  const manualCommandIDsRef = useRef(new Map<string, string>())

  useEffect(() => setOptimisticImages({}), [stageKey])

  const rememberImage = useCallback((image?: InteractiveImage) => {
    if (!image?.turn_id || !image.image_path) return
    setOptimisticImages((current) => {
      const images = current[image.turn_id] || []
      if (images.some((item) => item.image_path === image.image_path)) return current
      return { ...current, [image.turn_id]: [...images, image] }
    })
  }, [])

  const generateForTurn = useCallback(async (
    turnId: string,
    source: 'manual' | 'auto' = 'manual',
    force = true,
  ) => {
    if (!turnId || !storyId) {
      console.error(`[use-story-images.ts] interactive image target is unavailable story_id=${storyId || '<empty>'} turn_id=${turnId || '<empty>'}`)
      toast.error(t('storyStage.interactiveImage.targetMissing'))
      return null
    }
    if (generatingTurnId) {
      toast.warning(t('storyStage.interactiveImage.alreadyGenerating'))
      return null
    }
    const targetBranchID = branchId || snapshot?.branch_id || ''
    const retryKey = agentCommandRetryKey(stageKey, 'interactive-image', {
      storyId,
      branchId: targetBranchID,
      turnId,
      source,
      force,
    })
    const commandID = source === 'auto'
      ? automaticInteractiveImageCommandID(storyId, targetBranchID, turnId)
      : rememberAgentCommandID(manualCommandIDsRef.current, retryKey, createAgentCommandID)
    setGeneratingTurnId(turnId)
    setActivity(t('storyStage.interactiveImage.generating'))
    try {
      const result = await generateInteractiveImage(storyId, {
        command_id: commandID,
        branch_id: targetBranchID,
        turn_id: turnId,
        source,
        force,
      })
      if (source === 'manual') manualCommandIDsRef.current.delete(retryKey)
      rememberImage(result.image)
      await onDone({ silent: true })
      return result
    } catch (error) {
      if (source === 'manual' && isKnownAgentCommandOutcome(error)) manualCommandIDsRef.current.delete(retryKey)
      const messageText = error instanceof Error ? error.message : t('storyStage.interactiveImage.generateFailed')
      console.error(`[use-story-images.ts] interactive image generation failed story_id=${storyId} turn_id=${turnId}`, error)
      toast.error(t('storyStage.interactiveImage.generateFailed'), { description: messageText })
      return null
    } finally {
      setGeneratingTurnId(null)
      setActivity('')
    }
  }, [branchId, generatingTurnId, onDone, rememberImage, setActivity, snapshot?.branch_id, stageKey, storyId, t])

  const maybeGenerateAutomatically = useCallback(async (nextSnapshot: Snapshot | void) => {
    const targetSnapshot = nextSnapshot || snapshot
    const turn = targetSnapshot?.turns?.[targetSnapshot.turns.length - 1]
    if (!turn || !storyId) return
    const targetBranchID = targetSnapshot.branch_id || branchId
    const result = await generateInteractiveImage(storyId, {
      command_id: automaticInteractiveImageCommandID(storyId, targetBranchID, turn.id),
      branch_id: targetBranchID,
      turn_id: turn.id,
      source: 'auto',
      force: false,
    }).catch((error) => {
      console.warn('[use-story-images.ts] automatic interactive image generation failed', error)
      return null
    })
    if (result && !result.skipped) {
      rememberImage(result.image)
      await onDone({ silent: true })
    }
  }, [branchId, onDone, rememberImage, snapshot, storyId])

  return {
    generateForTurn,
    generatingTurnId,
    maybeGenerateAutomatically,
    optimisticImages,
  }
}

// Automatic generation is one durable operation per persisted story turn.
// Story, branch, and turn IDs are server-generated bounded identifiers, so the
// encoded command stays deterministic across remounts without local storage.
export function automaticInteractiveImageCommandID(storyID: string, branchID: string, turnID: string) {
  return ['interactive-image', 'auto', storyID, branchID, turnID]
    .map((part) => encodeURIComponent(part))
    .join(':')
}

export type StoryImagesController = ReturnType<typeof useStoryImages>
