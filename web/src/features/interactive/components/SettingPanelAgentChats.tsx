import { useEffect, useMemo, useRef, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  clearLoreAgentSession,
  getLoreAgentMessages,
  runLoreAgentStream,
  type ChatMessage,
  type LoreAgentResult,
  type LoreItem,
  type SSEEvent,
} from '@/lib/api'
import { Button } from '@/components/ui/button'
import { MessageList } from '@/components/Chat/MessageList'
import { InputArea } from '@/components/Chat/InputArea'
import type { ReferencePickerItem } from '@/components/Chat/FileReferencePicker'
import { useSkillCommands } from '@/hooks/useSkillCommands'

const LORE_AGENT_INIT_EVENT = 'nova:lore-agent-init'

type LoreAgentChatMessage = {
  id: string
  role: 'user' | 'assistant' | 'thinking' | 'tool_call' | 'error' | 'clear'
  content: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error'
  toolResult?: string
  references?: LoreItem[]
  result?: LoreAgentResult
}

const loreAgentMessageCache = new Map<string, LoreAgentChatMessage[]>()

function cloneLoreAgentMessages(messages: LoreAgentChatMessage[]) {
  return messages.map((message) => ({
    ...message,
    references: message.references ? [...message.references] : undefined,
    result: message.result ? {
      ...message.result,
      items: [...message.result.items],
      created: [...message.result.created],
      updated: [...message.result.updated],
      deleted_ids: [...message.result.deleted_ids],
    } : undefined,
  }))
}

interface LoreStatusPayload {
  stage?: string
  message?: string
  ops?: number
}

interface LoreToolPayload {
  id?: string
  name?: string
  args?: string
  delta?: string
  content?: string
  item_ids?: string[]
  deleted_ids?: string[]
}

export function LoreAgentChat({
  workspace,
  items,
  onResult,
  onToolMutation,
}: {
  workspace: string
  items: LoreItem[]
  onResult: (result: LoreAgentResult) => void
  onToolMutation: (itemIds: string[]) => void
}) {
  const { t } = useTranslation()
  const workspaceRef = useRef(workspace)
  const [inputPrefill, setInputPrefill] = useState<{ prompt: string; nonce: number } | null>(null)
  const [referenceIds, setReferenceIds] = useState<string[]>([])
  const [messages, setMessages] = useState<LoreAgentChatMessage[]>(() => (
    workspace ? cloneLoreAgentMessages(loreAgentMessageCache.get(workspace) || []) : []
  ))
  const [running, setRunning] = useState(false)
  const skillCommands = useSkillCommands({ agentKey: 'lore_editor', workspace, fallbackEnabled: true })
  const loreReferenceLabels = useMemo(() => Object.fromEntries(items.map((item) => [item.id, item.name])), [items])
  const loreSuggestions = useMemo<ReferencePickerItem[]>(() => items
    .filter((item) => {
      if (referenceIds.includes(item.id)) return false
      return true
    })
    .map((item) => ({
      value: item.id,
      label: item.name,
      description: `${loreTypeLabel(item.type, t)}${item.tags?.length ? ` · ${item.tags.join(', ')}` : ''}`,
    })), [items, referenceIds, t])

  useEffect(() => {
    setReferenceIds((current) => current.filter((id) => items.some((item) => item.id === id)))
  }, [items])

  useEffect(() => {
    if (!workspace) return
    if (messages.length === 1 && messages[0]?.id === 'load-error') return
    loreAgentMessageCache.set(workspace, cloneLoreAgentMessages(messages))
  }, [messages, workspace])

  useEffect(() => {
    const handleInitRequest = (event: Event) => {
      const detail = (event as CustomEvent<{ prompt?: string }>).detail
      setInputPrefill((current) => ({
        prompt: detail?.prompt || t('settingPanel.loreAgent.initPrompt'),
        nonce: (current?.nonce || 0) + 1,
      }))
      setReferenceIds([])
    }
    window.addEventListener(LORE_AGENT_INIT_EVENT, handleInitRequest)
    return () => window.removeEventListener(LORE_AGENT_INIT_EVENT, handleInitRequest)
  }, [t])

  useEffect(() => {
    workspaceRef.current = workspace
    setInputPrefill(null)
    setReferenceIds([])
    setMessages(workspace ? cloneLoreAgentMessages(loreAgentMessageCache.get(workspace) || []) : [])
    setRunning(false)
    if (!workspace) return
    let cancelled = false
    getLoreAgentMessages()
      .then((history) => {
        if (cancelled) return
        const nextMessages = history.map((message, index) => loreHistoryMessageToChat(message, index, items))
        setMessages((current) => {
          if (nextMessages.length === 0 && current.length > 0) return current
          return nextMessages
        })
      })
      .catch((error) => {
        if (!cancelled) {
          setMessages([{ id: 'load-error', role: 'error', content: error instanceof Error ? error.message : t('settingPanel.loreAgent.historyLoadFailed') }])
        }
      })
    return () => { cancelled = true }
  }, [workspace])

  const removeReference = (id: string) => {
    setReferenceIds((current) => current.filter((entry) => entry !== id))
  }

  const appendMessage = (message: Omit<LoreAgentChatMessage, 'id'>) => {
    setMessages((current) => [...current, { ...message, id: `${Date.now()}-${current.length}` }])
  }

  const appendStreamingMessage = (role: 'assistant' | 'thinking', content: string) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && !last.result) {
        return [...current.slice(0, -1), { ...last, content: `${last.content}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content }]
    })
  }

  const upsertToolCall = (payload: LoreToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || t('settingPanel.loreAgent.tool')
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const nextMessage: LoreAgentChatMessage = {
        id,
        role: 'tool_call',
        content: name,
        name,
        args: payload.args || '',
        status: 'running',
      }
      if (existing >= 0) {
        return current.map((message, index) => index === existing ? { ...message, ...nextMessage, args: message.args || nextMessage.args } : message)
      }
      return [...current, nextMessage]
    })
  }

  const appendToolArgs = (payload: LoreToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: LoreToolPayload) => {
    const id = payload.id
    if (!id) return
    setMessages((current) => current.map((message) => (
      message.id === id && message.role === 'tool_call'
        ? { ...message, status: 'success', toolResult: payload.content || '' }
        : message
    )))
  }

  const send = async (message: string) => {
    const instruction = message.trim()
    if (!instruction || running) return
    const activeWorkspace = workspace
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearLoreAgentSession()
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'clear', content: t('settingPanel.loreAgent.clearDone') })
        setReferenceIds([])
      } catch (error) {
        if (workspaceRef.current !== activeWorkspace) return
        appendMessage({ role: 'error', content: error instanceof Error ? error.message : t('settingPanel.loreAgent.clearFailed') })
      } finally {
        if (workspaceRef.current === activeWorkspace) setRunning(false)
      }
      return
    }
    const refs = [...referenceIds]
    const userReferences = refs
      .map((id) => items.find((item) => item.id === id))
      .filter((item): item is LoreItem => Boolean(item))
    appendMessage({ role: 'user', content: instruction, references: userReferences })
    setReferenceIds([])
    setRunning(true)
    try {
      const stream = await runLoreAgentStream(instruction, refs)
      const reader = stream.getReader()
      while (true) {
        const { done, value: event } = await reader.read()
        if (done) break
        if (workspaceRef.current !== activeWorkspace) break
        handleLoreAgentEvent(event)
      }
    } catch (error) {
      if (workspaceRef.current !== activeWorkspace) return
      appendMessage({ role: 'error', content: error instanceof Error ? error.message : t('settingPanel.loreAgent.runFailed') })
    } finally {
      if (workspaceRef.current === activeWorkspace) setRunning(false)
    }
  }

  const handleLoreAgentEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('thinking', payload?.content || '')
      return
    }
    if (event.event === 'chunk') {
      const payload = parseLoreEventData<{ content?: string }>(event.data)
      appendStreamingMessage('assistant', payload?.content || '')
      return
    }
    if (event.event === 'tool_call') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parseLoreEventData<LoreToolPayload>(event.data)
      if (payload) {
        finishToolCall(payload)
        const changedIDs = [...(payload.item_ids || []), ...(payload.deleted_ids || [])]
        if (changedIDs.length > 0) onToolMutation(changedIDs)
      }
      return
    }
    if (event.event === 'lore_status') {
      const payload = parseLoreEventData<LoreStatusPayload>(event.data)
      const content = payload?.message || t('settingPanel.loreAgent.processing')
      appendMessage({ role: 'assistant', content: payload?.ops ? t('settingPanel.loreAgent.ops', { message: content, count: payload.ops }) : content })
      return
    }
    if (event.event === 'lore_result') {
      const result = parseLoreEventData<LoreAgentResult>(event.data)
      if (!result) {
        appendMessage({ role: 'error', content: t('settingPanel.loreAgent.badResult') })
        return
      }
      onResult(result)
      appendMessage({ role: 'assistant', content: loreAgentResultSummary(result, t), result })
      return
    }
    if (event.event === 'error') {
      const payload = parseLoreEventData<{ message?: string }>(event.data)
      appendMessage({ role: 'error', content: payload?.message || t('settingPanel.loreAgent.runFailed') })
    }
  }

  const chatMessages = messages.map((message) => loreAgentMessageToChatMessage(message, t))

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-[var(--nova-surface-2)]">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-4">
        <div className="text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreAgent.persistHint')}</div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {messages.length === 0 ? (
          <div className="flex h-full items-center justify-center px-5 py-4">
            <div className="max-w-md rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
              <div className="text-sm font-medium text-[var(--nova-text)]">{t('settingPanel.loreAgent.emptyTitle')}</div>
              <div className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.loreAgent.emptyDesc')}</div>
              <Button
                type="button"
                className="mt-4 h-8 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-xs"
                variant="outline"
                onClick={() => {
                  setInputPrefill((current) => ({
                    prompt: t('settingPanel.loreAgent.initPrompt'),
                    nonce: (current?.nonce || 0) + 1,
                  }))
                }}
              >
                <Sparkles className="h-3.5 w-3.5" />
                {t('settingPanel.loreAgent.initAction')}
              </Button>
            </div>
          </div>
        ) : (
          <MessageList
            messages={chatMessages}
            isStreaming={running}
            activityContent={running ? t('settingPanel.loreAgent.running') : ''}
            collapseTraceBeforeAssistant
            scrollResetKey={`lore-agent:${workspace || 'none'}`}
            bottomPaddingClassName="pb-6"
          />
        )}
      </div>

      <InputArea
        onSend={(message) => void send(message)}
        disabled={running}
        inputPrefill={inputPrefill}
        onInputPrefillConsumed={() => setInputPrefill(null)}
        loreReferences={referenceIds}
        loreReferenceLabels={loreReferenceLabels}
        onLoreReferenceAdd={(id) => setReferenceIds((current) => current.includes(id) ? current : [...current, id])}
        onLoreReferenceRemove={removeReference}
        loreSuggestions={loreSuggestions}
        skills={skillCommands}
        commandScope="skills"
        placeholder={t('settingPanel.loreAgent.placeholder')}
        disabledPlaceholder={t('settingPanel.loreAgent.executing')}
      />
    </div>
  )
}

function loreAgentMessageToChatMessage(message: LoreAgentChatMessage, t: (key: string, options?: Record<string, unknown>) => string): ChatMessage {
  const referenceNames = (message.references || []).map((item) => item.name).filter(Boolean)
  return agentMessageToChatMessage(message, appendReferenceSummary(message.content, referenceNames), t)
}

function agentMessageToChatMessage(
  message: Pick<LoreAgentChatMessage, 'id' | 'role' | 'content' | 'name' | 'args' | 'status' | 'toolResult'>,
  content: string,
  t: (key: string, options?: Record<string, unknown>) => string,
): ChatMessage {
  if (message.role === 'clear') {
    return {
      id: message.id,
      type: 'clear',
      role: 'system',
      content: content || t('settingPanel.contextCleared'),
    }
  }
  if (message.role === 'tool_call') {
    return {
      id: message.id,
      role: 'tool_call',
      content: message.name || content,
      name: message.name || content,
      args: message.args || '',
      status: message.status,
      result: message.toolResult || '',
      streaming: message.status === 'running',
    }
  }
  return {
    id: message.id,
    role: message.role,
    content,
    streaming: message.role === 'thinking' && message.status === 'running',
  }
}

function appendReferenceSummary(content: string, names: string[]) {
  const visibleNames = names.map((name) => name.trim()).filter(Boolean)
  if (visibleNames.length === 0) return content
  return `${content}\n\n${visibleNames.map((name) => `@${name}`).join(' ')}`
}

function parseLoreEventData<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

function loreHistoryMessageToChat(message: ChatMessage, index: number, items: LoreItem[]): LoreAgentChatMessage {
  if (message.type === 'clear') {
    return {
      id: `history-clear-${index}`,
      role: 'clear',
      content: '',
    }
  }
  const role = message.role === 'user' ? 'user' : message.role === 'error' ? 'error' : 'assistant'
  return {
    id: `history-${index}`,
    role,
    content: message.content || '',
    references: role === 'user' ? loreReferencesFromContent(message.content || '', items) : undefined,
  }
}

function loreReferencesFromContent(content: string, items: LoreItem[]) {
  return items.filter((item) => item.name && content.includes(`@${item.name}`))
}

function loreAgentResultSummary(result: LoreAgentResult, t: (key: string, options?: Record<string, unknown>) => string) {
  const changed = [
    result.created?.length ? `${t('settingPanel.result.created')} ${result.created.length}` : '',
    result.updated?.length ? `${t('settingPanel.result.updated')} ${result.updated.length}` : '',
    result.deleted_ids?.length ? `${t('settingPanel.result.deleted')} ${result.deleted_ids.length}` : '',
  ].filter(Boolean).join('，')
  return `${result.message || t('settingPanel.loreAgent.done')}${changed ? `（${changed}）` : ''}`
}

function loreTypeLabel(type: LoreItem['type'], t: (key: string) => string) {
  const key = `lore.type.${type}`
  const label = t(key)
  return label === key ? t('lore.type.other') : label
}
