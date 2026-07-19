import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from 'react'
import {
  createSaveLane,
  type SaveLane,
  type SaveLaneRequest,
  type SaveLaneSnapshot,
} from '@/lib/autosave/save-lane'

interface UseSaveLaneOptions<T, TResult> {
  scopeKey: string
  delayMs: number
  save: (request: SaveLaneRequest<T>) => Promise<TResult>
  onSaved?: (request: SaveLaneRequest<T>, result: TResult) => void
  onError?: (request: SaveLaneRequest<T>, error: unknown) => void
}

export interface SaveLaneBinding<T, TResult> extends SaveLaneSnapshot {
  edit: SaveLane<T, TResult>['edit']
  reload: SaveLane<T, TResult>['reload']
  cancel: SaveLane<T, TResult>['cancel']
  flush: SaveLane<T, TResult>['flush']
  block: SaveLane<T, TResult>['block']
  unblock: SaveLane<T, TResult>['unblock']
  reset: SaveLane<T, TResult>['reset']
  hasWork: SaveLane<T, TResult>['hasWork']
  getSnapshot: SaveLane<T, TResult>['getSnapshot']
}

/** React binding for the framework-agnostic autosave lane. Domain hooks remain thin adapters. */
export function useSaveLane<T, TResult = void>({
  scopeKey,
  delayMs,
  save,
  onSaved,
  onError,
}: UseSaveLaneOptions<T, TResult>): SaveLaneBinding<T, TResult> {
  const saveRef = useRef(save)
  const onSavedRef = useRef(onSaved)
  const onErrorRef = useRef(onError)
  const activeRef = useRef(true)
  saveRef.current = save
  onSavedRef.current = onSaved
  onErrorRef.current = onError

  const laneRef = useRef<SaveLane<T, TResult> | null>(null)
  if (laneRef.current === null) {
    laneRef.current = createSaveLane<T, TResult>({
      delayMs,
      save: request => saveRef.current(request),
      onSaved: (request, result) => {
        if (activeRef.current) onSavedRef.current?.(request, result)
      },
      onError: (request, error) => {
        if (activeRef.current) onErrorRef.current?.(request, error)
      },
    })
  }
  const lane = laneRef.current

  useLayoutEffect(() => {
    lane.setDelayMs(delayMs)
  }, [delayMs, lane])

  useLayoutEffect(() => {
    lane.reset(scopeKey)
  }, [lane, scopeKey])

  useEffect(() => {
    activeRef.current = true
    return () => {
      activeRef.current = false
      // Keep the object reusable for React StrictMode's effect replay while
      // invalidating any request that belongs to the disappearing binding.
      lane.reset('__inactive__')
    }
  }, [lane])

  const snapshot = useSyncExternalStore(lane.subscribe, lane.getSnapshot, lane.getSnapshot)
  return {
    ...snapshot,
    edit: lane.edit,
    reload: lane.reload,
    cancel: lane.cancel,
    flush: lane.flush,
    block: lane.block,
    unblock: lane.unblock,
    reset: lane.reset,
    hasWork: lane.hasWork,
    getSnapshot: lane.getSnapshot,
  }
}
