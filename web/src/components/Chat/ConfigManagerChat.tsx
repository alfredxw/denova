import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { InputArea } from './InputArea'
import { MessageList } from './MessageList'
import { clearConfigManagerSession, getConfigManagerMessages, runConfigManagerStream } from '@/lib/api'
import type { ChatMessage, ConfigManagerRunRequest, SSEEvent } from '@/lib/api'
import { useSkillCommands } from '@/hooks/useSkillCommands'

interface ConfigManagerChatProps {
  workspace?: string
  origin: string
  resourceId?: string
  storyId?: string
  branchId?: string
  context?: Record<string, string>
  onMutated?: () => void
  className?: string
}

export function ConfigManagerChat({ workspace = '', origin, resourceId, storyId, branchId, context, onMutated, className = '' }: ConfigManagerChatProps) {
  const { t } = useTranslation()
  const activeKeyRef = useRef('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inputAreaHeight, setInputAreaHeight] = useState(0)
  const skills = useSkillCommands({ agentKey: 'config_manager', workspace, fallbackEnabled: true })
  const scope = useMemo(() => ({
    origin,
    resource_id: resourceId,
    story_id: storyId,
    branch_id: branchId,
  }), [branchId, origin, resourceId, storyId])
  const chatKey = useMemo(() => [
    'config-manager',
    workspace,
    origin,
    resourceId || '',
    storyId || '',
    branchId || '',
  ].join(':'), [branchId, origin, resourceId, storyId, workspace])
  const tokenUsageMessages = useMemo(
    () => messages.filter((message) => message.role === 'token_usage'),
    [messages],
  )
  const messageListBottomPadding = inputAreaHeight > 0 ? inputAreaHeight + 20 : undefined

  const loadMessages = useCallback(() => {
    if (!workspace) {
      setMessages([])
      return
    }
    getConfigManagerMessages(scope)
      .then(setMessages)
      .catch((err) => setError(err instanceof Error ? err.message : t('configManager.historyLoadFailed')))
  }, [scope, t, workspace])

  useEffect(() => {
    activeKeyRef.current = chatKey
    setRunning(false)
    setError(null)
    loadMessages()
  }, [chatKey, loadMessages])

  const appendMessage = (message: ChatMessage) => {
    setMessages((current) => [...current, { ...message, id: message.id || `${Date.now()}-${current.length}` }])
  }

  const appendStreaming = (role: ChatMessage['role'], content: string, metadata: ChatEventMetadata = {}) => {
    if (!content) return
    setMessages((current) => {
      const last = current[current.length - 1]
      if (last?.role === role && last.status !== 'success' && sameChatEventSource(last, metadata)) {
        return [...current.slice(0, -1), { ...last, content: `${last.content || ''}${content}` }]
      }
      return [...current, { id: `${Date.now()}-${current.length}`, role, content, ...metadata }]
    })
  }

  const upsertToolCall = (payload: ToolPayload) => {
    const id = payload.id || `tool-${Date.now()}`
    const name = payload.name || t('configManager.tool')
    setMessages((current) => {
      const existing = current.findIndex((message) => message.id === id)
      const next: ChatMessage = { id, role: 'tool_call', content: name, name, args: payload.args || '', status: 'running', ...metadataFromPayload(payload) }
      if (existing >= 0) return current.map((message, index) => index === existing ? { ...message, ...next, args: message.args || next.args } : message)
      return [...current, next]
    })
  }

  const appendToolArgs = (payload: ToolPayload) => {
    if (!payload.id || !payload.delta) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, args: `${message.args || ''}${payload.delta}` }
        : message
    )))
  }

  const finishToolCall = (payload: ToolPayload) => {
    if (!payload.id) return
    setMessages((current) => current.map((message) => (
      message.id === payload.id && message.role === 'tool_call'
        ? { ...message, status: 'success', result: payload.content || '', ...metadataFromPayload(payload) }
        : message
    )))
    onMutated?.()
  }

  const handleEvent = (event: SSEEvent) => {
    if (event.event === 'thinking') {
      const payload = parsePayload<ToolPayload>(event.data)
      appendStreaming('thinking', payload?.content || '', metadataFromPayload(payload))
      return
    }
    if (event.event === 'chunk') {
      const payload = parsePayload<ToolPayload>(event.data)
      appendStreaming('assistant', payload?.content || '', metadataFromPayload(payload))
      return
    }
    if (event.event === 'tool_call') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) upsertToolCall(payload)
      return
    }
    if (event.event === 'tool_args_delta') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) appendToolArgs(payload)
      return
    }
    if (event.event === 'tool_result') {
      const payload = parsePayload<ToolPayload>(event.data)
      if (payload) finishToolCall(payload)
      return
    }
    if (event.event === 'token_usage') {
      const payload = parsePayload<Record<string, unknown>>(event.data)
      if (payload) {
        setMessages((current) => upsertTokenUsageMessage(current, buildTokenUsageMessage(payload)))
      }
      return
    }
    if (event.event === 'error') {
      appendMessage({ role: 'error', content: parsePayload<{ message?: string }>(event.data)?.message || t('configManager.runFailed') })
    }
  }

  const send = async (message: string) => {
    const instruction = message.trim()
    if (!instruction || running) return
    if (instruction === '/clear') {
      setRunning(true)
      try {
        await clearConfigManagerSession(scope)
        setMessages([{ id: `clear-${Date.now()}`, type: 'clear', created_at: new Date().toISOString() }])
      } catch (err) {
        appendMessage({ role: 'error', content: err instanceof Error ? err.message : t('configManager.clearFailed') })
      } finally {
        setRunning(false)
      }
      return
    }
    appendMessage({ role: 'user', content: instruction })
    setRunning(true)
    setError(null)
    const activeChatKey = chatKey
    try {
      const req: ConfigManagerRunRequest = {
        instruction,
        ...scope,
        context,
      }
      const stream = await runConfigManagerStream(req)
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        if (activeKeyRef.current !== activeChatKey) break
        handleEvent(value)
      }
    } catch (err) {
      if (activeKeyRef.current === activeChatKey) appendMessage({ role: 'error', content: err instanceof Error ? err.message : t('configManager.runFailed') })
    } finally {
      if (activeKeyRef.current === activeChatKey) setRunning(false)
    }
  }

  return (
    <div className={`relative flex h-full min-h-0 flex-col overflow-hidden ${className}`}>
      {error && <div className="border-b border-[var(--nova-border)] px-3 py-2 text-xs text-red-400">{error}</div>}
      <MessageList
        messages={messages}
        isStreaming={running}
        activityContent=""
        scrollResetKey={chatKey}
        bottomPaddingClassName="pb-36"
        bottomPaddingPx={messageListBottomPadding}
      />
      <InputArea
        onSend={(value) => void send(value)}
        disabled={running}
        draftKey={chatKey}
        skills={skills}
        commandScope="all"
        builtinCommands={['/clear']}
        placeholder={t('configManager.placeholder')}
        disabledPlaceholder={t('configManager.executing')}
        tokenUsageMessages={tokenUsageMessages}
        agentKey="config_manager"
        workspace={workspace}
        floating
        onHeightChange={setInputAreaHeight}
      />
    </div>
  )
}

interface ToolPayload {
  id?: string
  name?: string
  args?: string
  delta?: string
  content?: string
  run_id?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
}

type ChatEventMetadata = Pick<ChatMessage, 'run_id' | 'agent_name' | 'root_agent_name' | 'run_path' | 'subagent' | 'subagent_session_id' | 'subagent_type'>

function parsePayload<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T
  } catch {
    return null
  }
}

function metadataFromPayload(payload?: ToolPayload | null): ChatEventMetadata {
  if (!payload) return {}
  return {
    run_id: payload.run_id,
    agent_name: payload.agent_name,
    root_agent_name: payload.root_agent_name,
    run_path: payload.run_path,
    subagent: payload.subagent,
    subagent_session_id: payload.subagent_session_id,
    subagent_type: payload.subagent_type,
  }
}

function buildTokenUsageMessage(data: Record<string, unknown>): ChatMessage {
  const runId = readString(data.run_id)
  return {
    role: 'token_usage',
    id: runId || `token-usage-${Date.now()}`,
    content: readString(data.content),
    run_id: runId,
    agent_kind: readString(data.agent_kind),
    prompt_tokens: readNumber(data.prompt_tokens),
    cached_prompt_tokens: readNumber(data.cached_prompt_tokens),
    uncached_prompt_tokens: readNumber(data.uncached_prompt_tokens),
    cache_hit_rate: readNumber(data.cache_hit_rate),
    completion_tokens: readNumber(data.completion_tokens),
    reasoning_tokens: readNumber(data.reasoning_tokens),
    total_tokens: readNumber(data.total_tokens),
    model_calls: readNumber(data.model_calls),
    generated_bytes: readNumber(data.generated_bytes),
    usage_calls: readUsageCalls(data.usage_calls),
    created_at: readString(data.created_at) || new Date().toISOString(),
  }
}

function upsertTokenUsageMessage(messages: ChatMessage[], next: ChatMessage) {
  if (!next.run_id) return [...messages, next]
  let found = false
  const updated = messages.map((message) => {
    if (message.role === 'token_usage' && message.run_id === next.run_id) {
      found = true
      return { ...message, ...next }
    }
    return message
  })
  return found ? updated : [...updated, next]
}

function readUsageCalls(value: unknown) {
  if (!Array.isArray(value)) return undefined
  const calls = value
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const call = item as Record<string, unknown>
      return {
        index: readNumber(call.index),
        created_at: readString(call.created_at),
        finish_reason: readString(call.finish_reason),
        requested_tools: readStringArray(call.requested_tools),
        after_tools: readStringArray(call.after_tools),
        prompt_tokens: readNumber(call.prompt_tokens),
        cached_prompt_tokens: readNumber(call.cached_prompt_tokens),
        uncached_prompt_tokens: readNumber(call.uncached_prompt_tokens),
        cache_hit_rate: readNumber(call.cache_hit_rate),
        completion_tokens: readNumber(call.completion_tokens),
        reasoning_tokens: readNumber(call.reasoning_tokens),
        total_tokens: readNumber(call.total_tokens),
      }
    })
    .filter((call): call is NonNullable<typeof call> => Boolean(call))
  return calls.length > 0 ? calls : undefined
}

function readStringArray(value: unknown) {
  if (!Array.isArray(value)) return undefined
  const result = value.map((item) => readString(item)).filter(Boolean)
  return result.length > 0 ? result : undefined
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function readNumber(value: unknown) {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

function sameChatEventSource(message: ChatMessage, metadata: ChatEventMetadata) {
  return Boolean(message.subagent) === Boolean(metadata.subagent) &&
    (message.subagent_session_id || '') === (metadata.subagent_session_id || '') &&
    (message.agent_name || '') === (metadata.agent_name || '') &&
    (message.root_agent_name || '') === (metadata.root_agent_name || '') &&
    (message.run_path || []).join('/') === (metadata.run_path || []).join('/')
}
