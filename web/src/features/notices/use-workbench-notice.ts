import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { UpdateCheckResult } from '@/features/settings/types'
import { countCompletedAgentTurnSignals } from '@/lib/agent-message-view'
import type { AgentUIMessage } from '@/lib/agent-ui'

const DISMISSED_UPDATE_VERSION_KEY = 'nova.update.dismissedLatestVersion'
const DISMISSED_STAR_NOTICE_KEY = 'nova.starNotice.dismissed'

export type WorkbenchNotice =
  | { kind: 'update'; latestVersion: string }
  | { kind: 'star' }

interface UseWorkbenchNoticeOptions {
  messages: AgentUIMessage[]
  isStreaming: boolean
}

export function useWorkbenchNotice({ messages, isStreaming }: UseWorkbenchNoticeOptions) {
  const [updateNotice, setUpdateNotice] = useState<Extract<WorkbenchNotice, { kind: 'update' }> | null>(null)
  const [starEligible, setStarEligible] = useState(false)
  const [starDismissed, setStarDismissed] = useState(readStarNoticeDismissed)
  const turnSignalBaselineRef = useRef<number | null>(null)

  useEffect(() => {
    if (starEligible || starDismissed) {
      turnSignalBaselineRef.current = null
      return
    }
    if (isStreaming) {
      if (turnSignalBaselineRef.current === null) {
        turnSignalBaselineRef.current = countCompletedAgentTurnSignals(messages)
      }
      return
    }

    const baseline = turnSignalBaselineRef.current
    turnSignalBaselineRef.current = null
    if (baseline !== null && countCompletedAgentTurnSignals(messages) > baseline) {
      setStarEligible(true)
    }
  }, [isStreaming, messages, starDismissed, starEligible])

  const notice = useMemo<WorkbenchNotice | null>(() => {
    if (updateNotice) return updateNotice
    if (starEligible && !starDismissed) return { kind: 'star' }
    return null
  }, [starDismissed, starEligible, updateNotice])

  const applyUpdateCheckResult = useCallback((result: UpdateCheckResult) => {
    if (!result.update_available || !result.latest_version) {
      setUpdateNotice(null)
      return
    }
    const dismissedVersion = readDismissedUpdateVersion()
    setUpdateNotice(dismissedVersion === result.latest_version
      ? null
      : { kind: 'update', latestVersion: result.latest_version })
  }, [])

  const dismissNotice = useCallback(() => {
    if (notice?.kind === 'update') {
      writeDismissedUpdateVersion(notice.latestVersion)
      setUpdateNotice(null)
      // Do not replace one dismissed prompt with another in the same moment.
      // A later successful Agent turn can make the Star prompt eligible again.
      setStarEligible(false)
      return
    }
    if (notice?.kind === 'star') {
      writeStarNoticeDismissed()
      setStarDismissed(true)
      setStarEligible(false)
    }
  }, [notice])

  return { notice, applyUpdateCheckResult, dismissNotice }
}

function readDismissedUpdateVersion() {
  try {
    return window.localStorage.getItem(DISMISSED_UPDATE_VERSION_KEY) || ''
  } catch {
    return ''
  }
}

function writeDismissedUpdateVersion(version: string) {
  try {
    window.localStorage.setItem(DISMISSED_UPDATE_VERSION_KEY, version)
  } catch (error) {
    console.warn('[workbench-notice] failed to persist dismissed update version', { error })
  }
}

function readStarNoticeDismissed() {
  try {
    return window.localStorage.getItem(DISMISSED_STAR_NOTICE_KEY) === 'true'
  } catch {
    return false
  }
}

function writeStarNoticeDismissed() {
  try {
    window.localStorage.setItem(DISMISSED_STAR_NOTICE_KEY, 'true')
  } catch (error) {
    console.warn('[workbench-notice] failed to persist dismissed Star notice', { error })
  }
}
