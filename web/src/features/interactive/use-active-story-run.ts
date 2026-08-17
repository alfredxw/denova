import { useEffect, useRef } from 'react'
import { isKnownAgentCommandOutcome } from '@/lib/agent-command'
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
const STORY_RECOVERY_READY_EVENT = 'nova:interactive-agent-recovery-ready'

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
    const abortController = new AbortController()
    let observationRegistered = false
    let lastImmediateRetry = ''

    // Deferring one microtask lets React Strict Mode finish its setup/cleanup
    // probe before this effect claims a real SSE subscription.
    void Promise.resolve().then(async () => {
      if (disposed || checkStreaming()) return
      while (!disposed && !abortController.signal.aborted) {
        let projectionFingerprint = ''
        try {
          const active = await getActiveInteractiveChat(storyId, branchId)
          projectionFingerprint = interactiveRecoveryProjectionFingerprint(active)
          if (disposed || !isObservableInteractiveRuntime(active)) return
          if (!observationRegistered) {
            observationRegistered = true
            registerStoryRunAbortController(stageKey, abortController)
          }
          await resume(active, abortController, () => disposed)
          return
        } catch (error) {
          if (disposed || abortController.signal.aborted) return
          console.error('[use-active-story-run.ts] failed to recover game Agent observation', error)
          if (isRecoveryProjectionRefreshError(error) && projectionFingerprint && projectionFingerprint !== lastImmediateRetry) {
            lastImmediateRetry = projectionFingerprint
            continue
          }
          if (isKnownAgentCommandOutcome(error)) return
          await waitForStoryRecoveryOpportunity(stageKey, abortController.signal)
        }
      }
    }).catch((error) => {
      if (!disposed && !abortController.signal.aborted) console.error('[use-active-story-run.ts] game Agent recovery loop failed', error)
    }).finally(() => {
      if (stageResumeClaims.get(stageKey) === claim) stageResumeClaims.delete(stageKey)
    })

    return () => {
      disposed = true
      abortController.abort()
      if (observationRegistered && clearStoryRunAbortController(stageKey, abortController)) {
        detach()
      }
      if (stageResumeClaims.get(stageKey) === claim) stageResumeClaims.delete(stageKey)
    }
  }, [branchId, stageKey, storyId])
}

function isObservableInteractiveRuntime(active: ActiveInteractiveChat) {
  if (active.active && active.task_id?.trim()) return true
  // The active endpoint retains the last settled display Task for replay. Once
  // its canonical turn is persisted, that Task is observable only when the
  // durable runtime still exposes an explicit recovery boundary.
  if (active.stream_attached && active.runtime_recoverable && active.task_id?.trim()) return true
  return Boolean(active.runtime_recoverable && active.recovery_actions?.length)
}

function interactiveRecoveryProjectionFingerprint(active: ActiveInteractiveChat) {
  return JSON.stringify([
    active.task_id || '',
    active.active_operation_id || '',
    active.recovery_paused === true,
    active.recovery_actions || [],
  ])
}

function isRecoveryProjectionRefreshError(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const code = (error as { code?: unknown }).code
  return code === 'agent_runtime.rehydrate_required' ||
    code === 'agent_runtime.recovery_changed' ||
    code === 'agent_runtime.stream_attached'
}

function waitForStoryRecoveryOpportunity(stageKey: string, signal: AbortSignal) {
  if (signal.aborted) return Promise.reject(signal.reason || new DOMException('Aborted', 'AbortError'))
  return new Promise<void>((resolve, reject) => {
    const cleanup = () => {
      window.removeEventListener('online', resume)
      window.removeEventListener('focus', resume)
      document.removeEventListener('visibilitychange', resumeWhenVisible)
      window.removeEventListener(STORY_RECOVERY_READY_EVENT, resumeWhenRuntimeReady)
      signal.removeEventListener('abort', abort)
    }
    const resume = () => {
      cleanup()
      resolve()
    }
    const resumeWhenVisible = () => {
      if (document.visibilityState === 'visible') resume()
    }
    const resumeWhenRuntimeReady = (event: Event) => {
      if ((event as CustomEvent<{ stageKey?: string }>).detail?.stageKey === stageKey) resume()
    }
    const abort = () => {
      cleanup()
      reject(signal.reason || new DOMException('Aborted', 'AbortError'))
    }
    window.addEventListener('online', resume, { once: true })
    window.addEventListener('focus', resume, { once: true })
    document.addEventListener('visibilitychange', resumeWhenVisible)
    window.addEventListener(STORY_RECOVERY_READY_EVENT, resumeWhenRuntimeReady)
    signal.addEventListener('abort', abort, { once: true })
  })
}

/** Wake the current projection loop after an explicit recovery action creates a task. */
export function wakeStoryRunRecovery(stageKey: string) {
  window.dispatchEvent(new CustomEvent(STORY_RECOVERY_READY_EVENT, { detail: { stageKey } }))
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
