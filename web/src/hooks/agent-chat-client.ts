import {
  analyzeChatContext,
  answerSessionAsk,
  cancelSessionAsk,
  createSession,
  deleteSession,
  executeCommand,
  getActiveChatTask,
  getMessagesPage,
  getSessions,
  recoverChatAgentRuntime,
  removeChatContextCompaction,
  renameSession,
  submitChatCommand,
  submitQueuedChatCommand,
  switchSession,
  type ActiveChatTask,
  type AgentAskAnswer,
  type AgentAskResolution,
  type AgentCommandDelivery,
  type AgentCommandReceipt,
  type AgentQueuedCommandAction,
  type AgentRuntimeRecoveryAction,
  type AgentRuntimeRecoveryReceipt,
  type ContextAnalysis,
  type IDEContext,
  type SessionMessagesPage,
  type SessionSummary,
  type TextSelection,
} from '@/lib/api'
import { jsonHeaders, requestJSON } from '@/lib/api-client/client'
import type { AgentChatTransportOptions } from '@/lib/agent-ui'

/**
 * API boundary consumed by one `useAgentChat` instance.
 *
 * Writing uses the foreground endpoints. AgentChat injects a project/session-bound
 * implementation, which lets multiple hook instances stream independently without
 * mutating the foreground Writing workspace.
 */
export interface AgentChatClient {
  transportOptions?: AgentChatTransportOptions
  fixedSessionId?: string
  getSessions: () => Promise<SessionSummary[]>
  getMessagesPage: (sessionId?: string, options?: { limit?: number; before?: string }) => Promise<SessionMessagesPage>
  getActiveChatTask: () => Promise<ActiveChatTask>
  recoverChatAgentRuntime: (action: AgentRuntimeRecoveryAction) => Promise<AgentRuntimeRecoveryReceipt>
  submitChatCommand: (
    type: AgentCommandDelivery | 'abort',
    commandId: string,
    targetOperationId: string,
    input?: Record<string, unknown>,
    reason?: string,
  ) => Promise<AgentCommandReceipt>
  submitQueuedChatCommand: (
    action: AgentQueuedCommandAction,
    commandId: string,
    targetOperationId: string,
    targetCommandId: string,
    reason?: string,
  ) => Promise<AgentCommandReceipt>
  executeCommand: (command: string) => Promise<string>
  analyzeChatContext: (
    message: string,
    references?: string[],
    loreReferences?: string[],
    styleScenes?: string[],
    textSelections?: TextSelection[],
    planMode?: boolean,
    writingSkill?: string,
    ideContext?: IDEContext,
    imagePresetId?: string,
    tellerId?: string,
  ) => Promise<ContextAnalysis>
  createSession: (title?: string) => Promise<SessionSummary>
  switchSession: (id: string) => Promise<SessionSummary>
  renameSession: (id: string, title: string) => Promise<void>
  deleteSession: (id: string) => Promise<SessionSummary>
  answerSessionAsk: (sessionId: string, askId: string, answers: AgentAskAnswer[]) => Promise<AgentAskResolution>
  cancelSessionAsk: (sessionId: string, askId: string) => Promise<AgentAskResolution>
  removeContextCompaction: () => Promise<boolean>
}

/** Default foreground Writing client. Kept as one stable object for hook dependencies. */
export const writingAgentChatClient: AgentChatClient = {
  getSessions,
  getMessagesPage,
  getActiveChatTask,
  recoverChatAgentRuntime,
  submitChatCommand,
  submitQueuedChatCommand,
  executeCommand,
  analyzeChatContext,
  createSession,
  switchSession,
  renameSession,
  deleteSession,
  answerSessionAsk,
  cancelSessionAsk,
  removeContextCompaction: removeChatContextCompaction,
}

/** Build an immutable project/session API binding for one AgentChat conversation tab. */
export function createProjectAgentChatClient(workspace: string, sessionId: string): AgentChatClient {
  const scope = { workspace, session_id: sessionId }
  const unsupportedSessionMutation = async (): Promise<never> => {
    throw new Error('AgentChat 对话由项目侧栏管理 / AgentChat conversations are managed from the project sidebar')
  }

  return {
    transportOptions: {
      api: '/api/agent-chat/chat',
      streamApi: '/api/agent-chat/chat/stream',
      scope,
    },
    fixedSessionId: sessionId,
    getSessions: async () => [{
      id: sessionId,
      title: '',
      created_at: '',
      updated_at: '',
      active: true,
      message_count: 0,
    }],
    getMessagesPage: async (_requestedSessionId, options = {}) => {
      const query = new URLSearchParams(scope)
      query.set('limit', String(options.limit || 100))
      if (options.before) query.set('before', options.before)
      const data = await requestJSON<{
        messages?: SessionMessagesPage['messages']
        page?: { next_before?: string; has_more?: boolean; total?: number }
      }>(`/api/agent-chat/session/messages?${query.toString()}`)
      return {
        messages: data.messages || [],
        nextBefore: data.page?.next_before || '0',
        hasMore: data.page?.has_more === true,
        total: data.page?.total || 0,
      }
    },
    getActiveChatTask: () => requestJSON(`/api/agent-chat/chat/active?${new URLSearchParams(scope).toString()}`),
    recoverChatAgentRuntime: (action) => requestJSON('/api/agent-chat/chat/recovery', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ ...scope, action }),
    }),
    submitChatCommand: (type, commandId, targetOperationId, input, reason) => requestJSON('/api/agent-chat/chat/commands', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        ...scope,
        type,
        command_id: commandId,
        target_operation_id: targetOperationId,
        ...(input ? { input } : {}),
        ...(reason ? { reason } : {}),
      }),
    }),
    submitQueuedChatCommand: (action, commandId, targetOperationId, targetCommandId, reason) => requestJSON('/api/agent-chat/chat/commands', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        ...scope,
        type: action,
        command_id: commandId,
        target_operation_id: targetOperationId,
        target_command_id: targetCommandId,
        ...(reason ? { reason } : {}),
      }),
    }),
    executeCommand: async (command) => {
      const data = await requestJSON<{ result?: string }>('/api/agent-chat/command', {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ ...scope, command }),
      })
      return data.result || ''
    },
    analyzeChatContext: (
      message,
      references = [],
      loreReferences = [],
      styleScenes = [],
      textSelections = [],
      planMode,
      writingSkill,
      ideContext,
      imagePresetId,
      tellerId,
    ) => requestJSON('/api/agent-chat/chat/context-analysis', {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({
        ...scope,
        message,
        references,
        lore_references: loreReferences,
        style_scenes: styleScenes,
        selections: textSelections.map((selection) => ({
          file_name: selection.fileName,
          start_line: selection.startLine,
          end_line: selection.endLine,
          content: selection.content,
        })),
        ide_context: normalizeIDEContext(ideContext),
        plan_mode: planMode || false,
        writing_skill: writingSkill || undefined,
        image_preset_id: imagePresetId || undefined,
        teller_id: tellerId || undefined,
      }),
    }),
    createSession: unsupportedSessionMutation,
    switchSession: async (id) => {
      if (id !== sessionId) return unsupportedSessionMutation()
      return {
        id: sessionId,
        title: '',
        created_at: '',
        updated_at: '',
        active: true,
        message_count: 0,
      }
    },
    renameSession: unsupportedSessionMutation,
    deleteSession: unsupportedSessionMutation,
    answerSessionAsk: (_requestedSessionId, askId, answers) => requestJSON(
      `/api/agent-chat/session/asks/${encodeURIComponent(askId)}/answer`,
      {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ ...scope, answers }),
      },
    ),
    cancelSessionAsk: (_requestedSessionId, askId) => requestJSON(
      `/api/agent-chat/session/asks/${encodeURIComponent(askId)}/cancel`,
      {
        method: 'POST',
        headers: jsonHeaders,
        body: JSON.stringify({ ...scope, reason: 'user_cancelled' }),
      },
    ),
    // AgentChat currently exposes automatic compaction only. Returning false keeps the
    // analysis intact without accidentally removing foreground Writing compaction state.
    removeContextCompaction: async () => false,
  }
}

function normalizeIDEContext(context?: IDEContext) {
  if (!context?.currentFile && !context?.openFiles?.length) return undefined
  return {
    current_file: context.currentFile || undefined,
    open_files: context.openFiles?.length ? context.openFiles : undefined,
  }
}
