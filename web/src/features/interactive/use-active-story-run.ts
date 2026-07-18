import { useEffect, useRef } from 'react'
import { getActiveInteractiveChat, type ActiveInteractiveChat } from './api'

type ResumeActiveStoryRun = (
  active: ActiveInteractiveChat,
  controller: AbortController,
  isDisposed: () => boolean,
) => Promise<void>

interface ActiveStoryRunRecoveryOptions {
  stageKey: string
  storyId: string
  branchId: string
  isStreaming: () => boolean
  onResume: ResumeActiveStoryRun
  onDetach: () => void
}

const stageAbortControllers = new Map<string, AbortController>()
const stageResumeClaims = new Map<string, symbol>()

// useActiveStoryRunRecovery owns only the view subscription. The backend task
// remains alive when this component unmounts, so a later mount can reconnect
// to the same buffered event stream without resubmitting the player's action.
export function useActiveStoryRunRecovery({ stageKey, storyId, branchId, isStreaming, onResume, onDetach }: ActiveStoryRunRecoveryOptions) {
  const isStreamingRef = useRef(isStreaming)
  const onResumeRef = useRef(onResume)
  const onDetachRef = useRef(onDetach)
  isStreamingRef.current = isStreaming
  onResumeRef.current = onResume
  onDetachRef.current = onDetach

  useEffect(() => {
    const checkStreaming = isStreamingRef.current
    if (!storyId || checkStreaming() || stageResumeClaims.has(stageKey)) return
    const claim = Symbol(stageKey)
    stageResumeClaims.set(stageKey, claim)
    const resume = onResumeRef.current
    const detach = onDetachRef.current
    let disposed = false
    let abortController: AbortController | null = null

    // Deferring one microtask lets React Strict Mode finish its setup/cleanup
    // probe before this effect claims a real SSE subscription.
    void Promise.resolve().then(async () => {
      if (disposed) return
      const active = await getActiveInteractiveChat(storyId, branchId)
      if (disposed || !active.active || !active.message?.trim() || checkStreaming()) return
      abortController = new AbortController()
      registerStoryRunAbortController(stageKey, abortController)
      await resume(active, abortController, () => disposed)
    }).catch((error) => {
      if (!disposed) console.error('[interactive-stage] 恢复游戏模式流失败', error)
    }).finally(() => {
      if (stageResumeClaims.get(stageKey) === claim) stageResumeClaims.delete(stageKey)
    })

    return () => {
      disposed = true
      abortController?.abort()
      if (abortController && clearStoryRunAbortController(stageKey, abortController)) {
        detach()
      }
      if (stageResumeClaims.get(stageKey) === claim) stageResumeClaims.delete(stageKey)
    }
  }, [branchId, stageKey, storyId])
}

export function registerStoryRunAbortController(stageKey: string, controller: AbortController) {
  stageAbortControllers.set(stageKey, controller)
}

export function clearStoryRunAbortController(stageKey: string, controller: AbortController) {
  if (stageAbortControllers.get(stageKey) !== controller) return false
  stageAbortControllers.delete(stageKey)
  return true
}

export function abortStoryRunStream(stageKey: string) {
  stageAbortControllers.get(stageKey)?.abort()
}
