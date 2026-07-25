import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Snapshot, StoryHistoryPage } from '../../types'
import {
  createStoryHistoryWindow,
  prependStoryHistoryPage,
  projectStoryHistorySnapshot,
  reconcileStoryHistoryWindow,
} from './story-history-window'

export function useStoryHistoryWindow(stageKey: string, snapshot: Snapshot | null) {
  const [historyWindow, setHistoryWindow] = useState(() => createStoryHistoryWindow(stageKey, snapshot))

  useEffect(() => {
    setHistoryWindow((current) => reconcileStoryHistoryWindow(current, stageKey, snapshot))
  }, [snapshot, stageKey])

  const displaySnapshot = useMemo(
    () => projectStoryHistorySnapshot(snapshot, historyWindow, stageKey),
    [historyWindow, snapshot, stageKey],
  )
  const prependPage = useCallback((page: StoryHistoryPage) => {
    setHistoryWindow((current) => prependStoryHistoryPage(current, stageKey, page))
  }, [stageKey])
  const resetToLatest = useCallback(() => {
    setHistoryWindow(createStoryHistoryWindow(stageKey, snapshot))
  }, [snapshot, stageKey])

  return { displaySnapshot, historyWindow, prependPage, resetToLatest }
}
