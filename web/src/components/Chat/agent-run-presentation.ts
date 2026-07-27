import {
  agentViewContent,
  agentViewStableKey,
  isAgentSubAgentTimelineView,
  isAgentTraceView,
  type AgentMessageView,
} from '@/lib/agent-message-view'

/** Display-only projection for one contiguous root Agent run. */
export interface AgentRunPresentation {
  active: boolean
  key: string
  nextIndex: number
  processViews: AgentMessageView[]
  resultView?: AgentMessageView
  runID: string
}

/**
 * Builds a stable process/result boundary without changing persisted history.
 * Explicit backend phases win; legacy history falls back to the last root
 * assistant segment once the run is complete.
 */
export function buildAgentRunPresentation(
  views: AgentMessageView[],
  startIndex: number,
  isStreaming: boolean,
): AgentRunPresentation | null {
  const first = views[startIndex]
  const runID = first?.metadata.run_id?.trim() || ''
  if (!runID || !isRootRunView(first)) return null

  const runViews: AgentMessageView[] = []
  let nextIndex = startIndex
  while (nextIndex < views.length) {
    const view = views[nextIndex]
    if (view.kind === 'token-usage' && view.metadata.run_id === runID) {
      nextIndex += 1
      continue
    }
    if (view.metadata.run_id !== runID || (!isRootRunView(view) && !isAgentSubAgentTimelineView(view))) break
    runViews.push(view)
    nextIndex += 1
  }
  if (runViews.length === 0) return null

  const active = isActiveRunSlice(views, nextIndex, isStreaming)
  const resultIndex = selectResultIndex(runViews, active)
  const resultView = resultIndex >= 0 ? runViews[resultIndex] : undefined
  const processViews = resultIndex < 0
    ? runViews
    : runViews.filter((_view, index) => index !== resultIndex)

  return {
    active,
    key: `run-${runID}-${agentViewStableKey(runViews[0])}`,
    nextIndex,
    processViews,
    resultView,
    runID,
  }
}

function isRootRunView(view?: AgentMessageView) {
  if (!view || view.metadata.subagent) return false
  return view.kind === 'assistant' || isAgentTraceView(view)
}

function selectResultIndex(views: AgentMessageView[], active: boolean) {
  for (let index = views.length - 1; index >= 0; index -= 1) {
    const view = views[index]
    if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
    if (view.metadata.display_phase === 'final' || view.metadata.display_phase === 'partial') return index
  }

  for (let index = views.length - 1; index >= 0; index -= 1) {
    const view = views[index]
    if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
    if (!active) return index
    if (view.metadata.display_phase === 'progress') return -1
    return index === views.length - 1 ? index : -1
  }
  return -1
}

function isActiveRunSlice(views: AgentMessageView[], afterRunIndex: number, isStreaming: boolean) {
  if (!isStreaming) return false
  for (let index = afterRunIndex; index < views.length; index += 1) {
    const view = views[index]
    if (view.kind === 'token-usage') continue
    if (view.kind === 'user' || view.kind === 'clear') return false
  }
  return true
}
