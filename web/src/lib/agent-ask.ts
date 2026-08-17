import type { AgentAskAnswer, AgentAskResolution } from './api'

export type AgentAskResolveAction =
  | { status: 'answered'; answers: AgentAskAnswer[] }
  | { status: 'cancelled' }

interface AgentAskTransport {
  answer: (answers: AgentAskAnswer[]) => Promise<AgentAskResolution>
  cancel: () => Promise<AgentAskResolution>
}

/** Returns the canonical terminal result immediately, then reloads durable
 * history so every mounted timeline converges on the same Ask record. */
export async function resolveAgentAskAndRefresh(
  action: AgentAskResolveAction,
  transport: AgentAskTransport,
  refreshHistory: () => void | Promise<void>,
): Promise<AgentAskResolution> {
  const resolution = action.status === 'answered'
    ? await transport.answer(action.answers)
    : await transport.cancel()

  void Promise.resolve()
    .then(refreshHistory)
    .catch((error) => console.error('[agent-ask] failed to refresh canonical history', error))
  return resolution
}
