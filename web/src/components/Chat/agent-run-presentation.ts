import {
  agentViewContent,
  agentViewStableKey,
  isAgentSubAgentTimelineBridgeView,
  isAgentSubAgentTimelineView,
  isAgentTraceView,
  type AgentMessageView,
} from '@/lib/agent-message-view'

/** Display-only projection for one contiguous root Agent run. */
export interface AgentRunPresentation {
  active: boolean
  afterResultViews: AgentMessageView[]
  beforeResultViews: AgentMessageView[]
  key: string
  nextIndex: number
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
    const subAgentBridge = isAgentSubAgentTimelineBridgeView(view, runViews)
    if (
      (view.metadata.run_id !== runID && !subAgentBridge) ||
      (!isRootRunView(view) && !isAgentSubAgentTimelineView(view) && !subAgentBridge)
    ) break
    runViews.push(view)
    nextIndex += 1
  }
  if (runViews.length === 0) return null

  const active = isActiveRunSlice(views, nextIndex, isStreaming)
  const resultIndex = selectResultIndex(runViews, active)
  const resultView = resultIndex >= 0 ? runViews[resultIndex] : undefined

  return {
    active,
    afterResultViews: resultIndex < 0 ? [] : runViews.slice(resultIndex + 1),
    beforeResultViews: resultIndex < 0 ? runViews : runViews.slice(0, resultIndex),
    key: `run-${runID}-${agentViewStableKey(runViews[0])}`,
    nextIndex,
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

  // Streaming root prose starts as a candidate. It remains the visible result
  // anchor even when Game continues with post-narrative thinking/tools.
  if (active) {
    for (let index = views.length - 1; index >= 0; index -= 1) {
      const view = views[index]
      if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
      if (view.metadata.display_phase === 'candidate') return index
    }
  }

  for (let index = views.length - 1; index >= 0; index -= 1) {
    const view = views[index]
    if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
    if (!active) return index
    if (view.metadata.display_phase === 'progress') return -1
    return index
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
