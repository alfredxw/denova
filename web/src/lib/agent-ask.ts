import type { AgentAskAnswer, AgentAskResolution } from './api'

export type AgentAskResolveAction =
  | { status: 'answered'; answers: AgentAskAnswer[] }
  | { status: 'cancelled'; reason?: 'task_aborted' }

interface AgentAskTransport {
  answer: (answers: AgentAskAnswer[]) => Promise<AgentAskResolution>
  cancel: (reason?: 'task_aborted') => Promise<AgentAskResolution>
}

/** Refresh terminal answers from history. An unresolved verification stays on
 * its current card, including when it belongs to a child with a separate journal. */
export async function resolveAgentAskAndRefresh(
  action: AgentAskResolveAction,
  transport: AgentAskTransport,
  refreshHistory: () => void | Promise<void>,
): Promise<AgentAskResolution> {
  const resolution = action.status === 'answered'
    ? await transport.answer(action.answers)
    : await transport.cancel(action.reason)

  if (resolution.status === 'pending') return resolution

  void Promise.resolve()
    .then(refreshHistory)
    .catch((error) => console.error('[agent-ask] failed to refresh canonical history', error))
  return resolution
}
