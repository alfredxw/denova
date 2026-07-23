import type { ActiveChatTask, IDEContext, TextSelection } from '@/lib/api'
import type { UserMessageReference } from '@/lib/api-client/types'
import type { AgentUIMessage } from '@/lib/agent-ui'
import {
  agentViewContent,
  buildAgentMessageViews,
  isPlanProtocolToolName,
  type AgentMessageView,
  type AgentPartRef,
} from '@/lib/agent-message-view'

interface UserMessageReferenceOptions {
  loreReferenceLabels?: Record<string, string>
  reviewFeedbackDisplay?: {
    comments: Array<{ id: string; body: string; path?: string; review_path?: string; review_line?: number }>
  }
}

export function buildUserMessageReferences(
  prepared: {
    references: string[]
    loreReferences: string[]
    styleScenes: string[]
    textSelections: TextSelection[]
  },
  options: UserMessageReferenceOptions,
): UserMessageReference[] {
  const result: UserMessageReference[] = []
  for (const path of prepared.references) result.push({ kind: 'file', label: path })
  for (const id of prepared.loreReferences) result.push({ kind: 'lore', id, label: options.loreReferenceLabels?.[id] || id })
  for (const scene of prepared.styleScenes) result.push({ kind: 'style', label: scene })
  for (const selection of prepared.textSelections) {
    result.push({
      kind: 'selection',
      label: selection.fileName,
      start_line: selection.startLine,
      end_line: selection.endLine,
      detail: boundedReferenceDetail(selection.content),
    })
  }
  for (const comment of options.reviewFeedbackDisplay?.comments ?? []) {
    result.push({
      kind: 'review_comment',
      id: comment.id,
      label: comment.review_path || comment.path || comment.id,
      ...(comment.review_line !== undefined ? { start_line: comment.review_line, end_line: comment.review_line } : {}),
      detail: boundedReferenceDetail(comment.body),
    })
  }
  return result
}

function boundedReferenceDetail(value: string): string {
  const normalized = value.trim()
  return normalized.length > 512 ? `${normalized.slice(0, 512)}…` : normalized
}

export function normalizeIDEContext(context?: IDEContext) {
  if (!context?.currentFile && !context?.openFiles?.length) return undefined
  return {
    current_file: context.currentFile || undefined,
    open_files: context.openFiles?.length ? context.openFiles : undefined,
  }
}

export function appendDataMessage(
  setUIMessages: (updater: (messages: AgentUIMessage[]) => AgentUIMessage[]) => void,
  type: `data-agent-${string}`,
  data: Record<string, unknown>,
) {
  setUIMessages(messages => [
    ...messages,
    {
      id: `${type}-${Date.now()}-${messages.length}`,
      role: 'assistant',
      parts: [{ type, data, id: `${type}-${Date.now()}` } as AgentUIMessage['parts'][number]],
    } as AgentUIMessage,
  ])
}

export function agentBypassCommand(input: string): string | null {
  if (!input.startsWith('/')) return null
  const cmd = input.slice(1).split(' ')[0]
  return ['clear', 'compact', 'status', 'help'].includes(cmd) ? cmd : null
}

export function parseInlineReferences(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)@([^\s@]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) {
    const value = match[1]
    if (value.startsWith('资料:')) continue
    result.add(value)
  }
  return Array.from(result)
}

export function parseInlineStyleScenes(input: string): string[] {
  const result = new Set<string>()
  const regex = /(?:^|\s)#([^\s#]+)/g
  let match: RegExpExecArray | null
  while ((match = regex.exec(input)) !== null) result.add(match[1])
  return Array.from(result)
}

const CHAT_PLAN_MODES_STORAGE_KEY = 'nova.chat.plan_modes.v1'

export function readChatPlanModes(): Record<string, boolean> {
  if (typeof window === 'undefined') return {}
  const raw = window.localStorage.getItem(CHAT_PLAN_MODES_STORAGE_KEY)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const result: Record<string, boolean> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof key === 'string' && typeof value === 'boolean') result[key] = value
    }
    return result
  } catch {
    return {}
  }
}

export function writeChatPlanModes(value: Record<string, boolean>) {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(CHAT_PLAN_MODES_STORAGE_KEY, JSON.stringify(value))
}

export function planModeForSession(planModes: Record<string, boolean>, sessionId: string, defaultValue: boolean) {
  const id = sessionId || 'default'
  return planModes[id] ?? defaultValue
}

export function findAgentMessageView(messages: AgentUIMessage[], ref: AgentPartRef): AgentMessageView | undefined {
  return buildAgentMessageViews(messages).find((view) => sameAgentPartRef(view.ref, ref))
}

export function collectPlanUserContext(messages: AgentUIMessage[], target: AgentPartRef) {
  const views = buildAgentMessageViews(messages)
  const planIndex = views.findIndex((view) => sameAgentPartRef(view.ref, target))
  const end = planIndex >= 0 ? planIndex : views.length
  let start = 0
  for (let i = end - 1; i >= 0; i -= 1) {
    if (views[i].kind === 'proposed-plan') {
      start = i + 1
      break
    }
  }
  const userMessages = views
    .slice(start, end)
    .filter((view) => view.kind === 'user')
    .map((view) => agentViewContent(view).trim())
    .filter(Boolean)
  if (userMessages.length <= 1) return userMessages[0] || ''
  return [
    `原始请求：\n${userMessages[0]}`,
    `用户补充：\n${userMessages.slice(1).join('\n\n')}`,
  ].join('\n\n')
}

export function filterInternalPlanUIMessages(messages: AgentUIMessage[]) {
  return messages.filter((message) => {
    const text = message.parts.map(part => part.type === 'text' ? part.text : '').join('')
    if (message.role === 'user' && isPlanQuestionAnswerProtocol(text)) return false
    return !message.parts.some(part => isPlanProtocolToolPart(part))
  })
}

function isPlanQuestionAnswerProtocol(content: string) {
  return content.includes('<plan_question_answers>') || content.includes('</plan_question_answers>')
}

function isPlanProtocolToolPart(part: AgentUIMessage['parts'][number]) {
  if (part.type === 'dynamic-tool') return isPlanProtocolToolName(part.toolName)
  if (part.type.startsWith('tool-')) return isPlanProtocolToolName(part.type.replace(/^tool-/, ''))
  return false
}

export function markPlanUIMessageAction(
  messages: AgentUIMessage[],
  target: AgentPartRef,
  action: AgentPlanAction,
) {
  return messages.map(message => ({
    ...message,
    parts: message.parts.map((part, index) => {
      const raw = part as Record<string, unknown>
      const type = typeof raw.type === 'string' ? raw.type : ''
      if (!type.startsWith('data-agent-plan-')) return part
      const data = 'data' in part && part.data && typeof part.data === 'object' && !Array.isArray(part.data)
        ? part.data as Record<string, unknown>
        : {}
      const partID = 'id' in part && typeof part.id === 'string' ? part.id : `${message.id}:${index}`
      const candidate = { messageId: message.id, partId: partID, partIndex: index, type }
      if (!sameAgentPartRef(candidate, target)) return part
      return { ...part, data: { ...data, plan_action: action, status: 'success' } } as AgentUIMessage['parts'][number]
    }),
  }))
}

type AgentPlanAction = 'answered' | 'approved' | 'continue' | 'exited'

function sameAgentPartRef(left: AgentPartRef, right: AgentPartRef) {
  return left.messageId === right.messageId
    && left.partIndex === right.partIndex
    && left.partId === right.partId
    && left.type === right.type
}

export function mergeProjectedQueue(
  queue: ActiveChatTask['queue'],
  next: NonNullable<ActiveChatTask['queue']>[number],
) {
  return [...(queue || []).filter((item) => item.command_id !== next.command_id), next]
}

export function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === 'AbortError'
}
