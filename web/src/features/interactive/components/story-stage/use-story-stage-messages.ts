import { useMemo } from 'react'
import type { ChatMessage } from '@/lib/api'
import { chatMessagesToAgentUIMessages } from '@/lib/agent-legacy-message'
import type { Snapshot, TurnEvent } from '../../types'
import { isDirectorDisplayEvent } from '../director-console/utils'
import type { TurnNavigationItem } from '../TurnNavigator'
import { sanitizeStoredNarrative } from '../../stream-parser'
import {
  interactiveImages,
  latestInteractiveImageError,
  latestInteractiveImageStatus,
  latestMergedInteractiveImage,
  mergeInteractiveImages,
  readInteractiveImage,
  readInteractiveImageError,
} from './interactive-images'
import { publicRuleRollFromResolution } from './rule-roll'
import { buildTokenUsageMessage, mergeTokenUsageMessages } from './token-usage'
import { normalizeMessageContent } from './utils'

interface UseStoryStageMessagesOptions {
  snapshot: Snapshot | null
  rewindTurnId?: string
  liveMessages: ChatMessage[]
  streaming: boolean
  stageKey: string
  liveTurnNavigationAnchorId: string
  publicRuleRollVisible: boolean
  optimisticInteractiveImages: Record<string, import('@/lib/api').InteractiveImage[]>
  belongsToStage: (stageKey: string) => boolean
  renderKeyFor: (turnId: string, role: 'user' | 'assistant') => string | undefined
}

// Projects persisted domain turns and ephemeral display events into the single
// render timeline. The projection deliberately excludes this display history
// from model-context assembly; it is a UI-only read model.
export function useStoryStageMessages({
  snapshot,
  rewindTurnId,
  liveMessages,
  streaming,
  stageKey,
  liveTurnNavigationAnchorId,
  publicRuleRollVisible,
  optimisticInteractiveImages,
  belongsToStage,
  renderKeyFor,
}: UseStoryStageMessagesOptions) {
  const storyPathTurns = useMemo(() => {
    const turns = snapshot?.turns || []
    const rewindIndex = rewindTurnId ? turns.findIndex((turn) => turn.id === rewindTurnId) : -1
    return rewindIndex >= 0 ? turns.slice(0, rewindIndex) : turns
  }, [rewindTurnId, snapshot?.turns])

  const latestLiveTurn = useMemo(() => {
    if (liveMessages.length === 0) return null
    const user = liveMessages.find((message) => message.role === 'user')?.content || ''
    const narrative = liveMessages
      .filter((message) => message.role === 'assistant' && !message.subagent)
      .map((message) => message.streaming_target_content || message.content || '')
      .join('')
    return user || narrative ? { user, narrative } : null
  }, [liveMessages])

  const hasPersistedLiveTurn = useMemo(() => {
    const lastTurn = snapshot?.turns?.[snapshot.turns.length - 1]
    if (!lastTurn || !latestLiveTurn || !belongsToStage(stageKey)) return false
    return normalizeMessageContent(lastTurn.user) === normalizeMessageContent(latestLiveTurn.user)
      && normalizeMessageContent(lastTurn.narrative) === normalizeMessageContent(latestLiveTurn.narrative)
  }, [belongsToStage, latestLiveTurn, snapshot?.turns, stageKey])

  const historyMessages = useMemo<ChatMessage[]>(() => storyPathTurns.flatMap((turn) => {
    const messages: ChatMessage[] = [{
      id: `${turn.id}-user`,
      render_key: renderKeyFor(turn.id, 'user'),
      turn_id: turn.id,
      navigation_turn_id: turn.id,
      role: 'user',
      content: turn.user,
    }]
    const displayEvents = (turn.display_events || []).filter((event) => !isDirectorDisplayEvent(event))
    if (!displayEvents.some((event) => event.role === 'thinking') && turn.thinking?.trim()) {
      messages.push({ id: `${turn.id}-thinking`, role: 'thinking', content: turn.thinking, streaming: false })
    }
    const deferredImages: ChatMessage[] = []
    const beforeNarrative: ChatMessage[] = []
    const afterNarrative: ChatMessage[] = []
    let narrativeAnchored = false
    for (const [index, event] of displayEvents.entries()) {
      if (event.role === 'narrative') {
        narrativeAnchored = true
        continue
      }
      const timeline = narrativeAnchored ? afterNarrative : beforeNarrative
      const metadata = {
        created_at: event.created_at,
        run_id: event.run_id,
        agent_kind: event.agent_kind,
        agent_name: event.agent_name,
        root_agent_name: event.root_agent_name,
        run_path: event.run_path,
        subagent: event.subagent,
        subagent_session_id: event.subagent_session_id,
        subagent_type: event.subagent_type,
      }
      if (event.role === 'thinking') {
        timeline.push({
          id: event.id || `${turn.id}-thinking-${index}`,
          role: 'thinking',
          content: event.content || '',
          streaming: false,
          ...metadata,
        })
        continue
      }
      if (event.role === 'tool_call') {
        const toolMessage: ChatMessage = {
          id: event.id || `${turn.id}-tool-${index}`,
          turn_id: event.name === 'generate_interactive_image' ? turn.id : undefined,
          role: 'tool_call',
          content: event.content || event.name || 'unknown_tool',
          name: event.name || event.content,
          args: event.args || '',
          status: event.status || 'success',
          result: event.result || '',
          interactive_image: readInteractiveImage(event.result),
          interactive_image_error: readInteractiveImageError(event.result),
          streaming: false,
          sse_hidden_fields: event.sse_hidden_fields,
          sse_hidden_reason: event.sse_hidden_reason,
          sse_display_notice: event.sse_display_notice,
          sse_generated_chars: event.sse_generated_chars,
          ...metadata,
        }
        if (event.name === 'generate_interactive_image') deferredImages.push(toolMessage)
        else timeline.push(toolMessage)
        continue
      }
      if (event.role === 'assistant') {
        timeline.push({
          id: event.id || `${turn.id}-subagent-${index}`,
          role: 'assistant',
          content: event.content || '',
          streaming: false,
          ...metadata,
        })
      }
    }
    messages.push(...beforeNarrative)
    const ruleRoll = publicRuleRollVisible ? publicRuleRollFromResolution(turn.rule_resolution) : null
    if (ruleRoll) {
      messages.push({ id: `${turn.id}-rule-roll`, turn_id: turn.id, navigation_turn_id: turn.id, role: 'rule_roll', rule_roll: ruleRoll })
    }
    const mergedImages = mergeInteractiveImages(interactiveImages(deferredImages), optimisticInteractiveImages[turn.id])
    messages.push({
      id: `${turn.id}-assistant`,
      render_key: renderKeyFor(turn.id, 'assistant'),
      turn_id: turn.id,
      navigation_turn_id: turn.id,
      role: 'assistant',
      content: sanitizeStoredNarrative(turn.narrative),
      run_id: turn.run_id,
      agent_kind: turn.agent_kind,
      turn_versions: turn.versions,
      turn_version_index: turn.version_idx,
      interactive_image: latestMergedInteractiveImage(mergedImages),
      interactive_images: mergedImages,
      interactive_image_error: latestInteractiveImageError(deferredImages),
      interactive_image_status: mergedImages?.length ? 'success' : latestInteractiveImageStatus(deferredImages),
    })
    messages.push(...afterNarrative)
    return messages
  }), [optimisticInteractiveImages, publicRuleRollVisible, renderKeyFor, storyPathTurns])

  const displayLiveMessages = hasPersistedLiveTurn ? [] : liveMessages.filter((message) => message.role !== 'token_usage')
  const messages = useMemo(() => [...historyMessages, ...displayLiveMessages], [displayLiveMessages, historyMessages])
  const agentMessages = useMemo(() => chatMessagesToAgentUIMessages(messages), [messages])
  const turnNavigationItems = useMemo<TurnNavigationItem[]>(() => {
    const items: TurnNavigationItem[] = storyPathTurns.map((turn) => ({
      anchorId: turn.id,
      user: turn.user,
      narrative: sanitizeStoredNarrative(turn.narrative),
    }))
    if (!hasPersistedLiveTurn && latestLiveTurn) {
      items.push({
        anchorId: liveTurnNavigationAnchorId,
        user: latestLiveTurn.user,
        narrative: latestLiveTurn.narrative,
        pending: streaming || !latestLiveTurn.narrative.trim(),
      })
    }
    return items
  }, [hasPersistedLiveTurn, latestLiveTurn, liveTurnNavigationAnchorId, storyPathTurns, streaming])
  const persistedTokenUsage = useMemo(
    () => (snapshot?.token_usage_events || []).map((event, index) => buildTokenUsageMessage(event, event.id || `token-usage-${index + 1}`)),
    [snapshot?.token_usage_events],
  )
  const liveTokenUsage = useMemo(() => liveMessages.filter((message) => message.role === 'token_usage'), [liveMessages])
  const tokenUsageMessages = useMemo(() => mergeTokenUsageMessages(persistedTokenUsage, liveTokenUsage), [liveTokenUsage, persistedTokenUsage])
  const turnsById = useMemo(() => new Map<string, TurnEvent>((snapshot?.turns || []).map((turn) => [turn.id, turn])), [snapshot?.turns])

  return {
    agentMessages,
    hasPersistedLiveTurn,
    latestLiveTurn,
    messages,
    tokenUsageMessages,
    turnNavigationItems,
    turnsById,
  }
}
