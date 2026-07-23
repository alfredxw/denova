import { useCallback, useEffect, useRef, useState } from 'react'
import { getMessagesPage, getSessions, type SessionSummary } from '@/lib/api'
import { normalizeAgentUIMessages, type AgentUIMessage } from '@/lib/agent-ui'
import { filterInternalPlanUIMessages } from './agent-chat-state'

interface WritingAgentHistoryOptions {
  setMessages: (messages: AgentUIMessage[] | ((messages: AgentUIMessage[]) => AgentUIMessage[])) => void
}

/**
 * Owns writing-session history loading and its request ordering guarantees.
 * Authoritative reloads replace provisional stream state; pagination only
 * prepends a page when no newer authoritative reload has superseded it.
 */
export function useWritingAgentHistory({ setMessages }: WritingAgentHistoryOptions) {
  const [sessions, setSessions] = useState<SessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState('')
  const [hasEarlierMessages, setHasEarlierMessages] = useState(false)
  const [isLoadingEarlierHistory, setIsLoadingEarlierHistory] = useState(false)
  const historyRequestGenerationRef = useRef(0)
  const earlierHistoryRequestRef = useRef(0)
  const earlierHistoryLoadingRef = useRef(false)
  const historyPageRef = useRef<{
    sessionId?: string
    nextBefore: string
    hasMore: boolean
  }>({
    nextBefore: '0',
    hasMore: false,
  })

  const loadSessions = useCallback(async () => {
    try {
      const list = await getSessions()
      setSessions(list)
      setActiveSessionId(list.find((item) => item.active)?.id || list[0]?.id || '')
      return list
    } catch (error) {
      console.error('加载会话列表失败', error)
      return []
    }
  }, [])

  const loadHistoryAuthoritative = useCallback(
    async (sessionId?: string) => {
      const generation = historyRequestGenerationRef.current + 1
      historyRequestGenerationRef.current = generation
      earlierHistoryRequestRef.current += 1
      earlierHistoryLoadingRef.current = false
      setIsLoadingEarlierHistory(false)

      const page = await getMessagesPage(sessionId)
      if (generation !== historyRequestGenerationRef.current) {
        throw new Error('Writing history reload was superseded before it could become authoritative')
      }

      historyPageRef.current = {
        sessionId,
        nextBefore: page.nextBefore,
        hasMore: page.hasMore,
      }
      setHasEarlierMessages(page.hasMore)
      setMessages(filterInternalPlanUIMessages(page.messages))
    },
    [setMessages],
  )

  const loadHistory = useCallback(
    async (sessionId?: string) => {
      try {
        await loadHistoryAuthoritative(sessionId)
      } catch (error) {
        console.error('加载历史失败', error)
      }
    },
    [loadHistoryAuthoritative],
  )

  const loadEarlierHistory = useCallback(async () => {
    const currentPage = historyPageRef.current
    if (!currentPage.hasMore || earlierHistoryLoadingRef.current) return

    const historyGeneration = historyRequestGenerationRef.current
    const requestID = earlierHistoryRequestRef.current + 1
    earlierHistoryRequestRef.current = requestID
    earlierHistoryLoadingRef.current = true
    setIsLoadingEarlierHistory(true)

    try {
      const page = await getMessagesPage(currentPage.sessionId, {
        before: currentPage.nextBefore,
      })
      if (historyGeneration !== historyRequestGenerationRef.current || requestID !== earlierHistoryRequestRef.current) return

      const earlierMessages = filterInternalPlanUIMessages(page.messages)
      historyPageRef.current = {
        ...currentPage,
        nextBefore: page.nextBefore,
        hasMore: page.hasMore,
      }
      setHasEarlierMessages(page.hasMore)
      setMessages((messages) => normalizeAgentUIMessages([...earlierMessages, ...messages]))
    } catch (error) {
      if (historyGeneration === historyRequestGenerationRef.current && requestID === earlierHistoryRequestRef.current) {
        console.error('加载更早历史失败', error)
      }
    } finally {
      if (requestID === earlierHistoryRequestRef.current) {
        earlierHistoryLoadingRef.current = false
        setIsLoadingEarlierHistory(false)
      }
    }
  }, [setMessages])

  useEffect(
    () => () => {
      historyRequestGenerationRef.current += 1
      earlierHistoryRequestRef.current += 1
    },
    [],
  )

  return {
    activeSessionId,
    hasEarlierMessages,
    isLoadingEarlierHistory,
    loadEarlierHistory,
    loadHistory,
    loadHistoryAuthoritative,
    loadSessions,
    sessions,
    setActiveSessionId,
  }
}
