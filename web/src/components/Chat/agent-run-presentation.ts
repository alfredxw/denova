import {
  agentViewContent,
  agentViewStableKey,
  isAgentSubAgentTimelineBridgeView,
  isAgentSubAgentTimelineView,
  isAgentRunMetadataView,
  isAgentTerminalExecutionSummaryView,
  isAgentTraceView,
  type AgentMessageView,
} from '@/lib/agent-message-view'

/** Display-only projection for one contiguous root Agent run. */
export interface AgentRunPresentation {
  key: string
  nextIndex: number
  runID: string
  sections: AgentRunPresentationSection[]
}

export type AgentRunPresentationSection =
  | { active: boolean; key: string; kind: 'process'; views: AgentMessageView[] }
  | { key: string; kind: 'message'; view: AgentMessageView }

/**
 * Builds stable ordered process/prose sections without changing persisted
 * history. A run keeps one process before its terminal result in both active
 * and completed states; AgentExecutionProcess owns the inner progress segments.
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
  let settled = false
  let nextIndex = startIndex
  while (nextIndex < views.length) {
    const view = views[nextIndex]
    if (isAgentRunMetadataView(view) && view.metadata.run_id === runID) {
      if (isAgentTerminalExecutionSummaryView(view)) settled = true
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

  const active = !settled && isActiveRunSlice(views, nextIndex, isStreaming)
  const resultIndex = selectTerminalResultIndex(runViews, active)

  return {
    key: `run-${runID}-${agentViewStableKey(runViews[0])}`,
    nextIndex,
    runID,
    sections: resultIndex >= 0
      ? buildTerminalSections(runViews, resultIndex)
      : buildActiveSections(runViews),
  }
}

function isRootRunView(view?: AgentMessageView) {
  if (!view || view.metadata.subagent) return false
  return view.kind === 'assistant' || isAgentTraceView(view) || isVisibleMediaView(view)
}

function selectTerminalResultIndex(views: AgentMessageView[], active: boolean) {
  for (let index = views.length - 1; index >= 0; index -= 1) {
    const view = views[index]
    if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
    if (view.metadata.display_phase === 'final' || view.metadata.display_phase === 'partial') return index
  }

  if (active) return -1

  for (let index = views.length - 1; index >= 0; index -= 1) {
    const view = views[index]
    if (view.metadata.subagent || view.kind !== 'assistant' || !agentViewContent(view).trim()) continue
    return index
  }
  return -1
}

// Active and completed runs intentionally share the same process boundary. A
// completion event may collapse that boundary, but must not rebuild the visible
// timeline from several sibling disclosures into a different structure.
function buildActiveSections(views: AgentMessageView[]): AgentRunPresentationSection[] {
  const sections: AgentRunPresentationSection[] = []
  appendRunSections(sections, views, true)
  return sections
}

// Once the terminal prose is known, aggregate all earlier progress into one
// disclosure while preserving post-result trace after the prose. Game emits
// state/choice submission tools after its narrative, so moving those tools
// ahead of the narrative would make the completed timeline non-chronological.
function buildTerminalSections(views: AgentMessageView[], resultIndex: number): AgentRunPresentationSection[] {
  const sections: AgentRunPresentationSection[] = []
  appendRunSections(sections, views.slice(0, resultIndex), false)
  const resultView = views[resultIndex]
  sections.push({
    key: `message-${agentViewStableKey(resultView)}`,
    kind: 'message',
    view: resultView,
  })
  appendRunSections(sections, views.slice(resultIndex + 1), false)
  return sections
}

function appendRunSections(
  sections: AgentRunPresentationSection[],
  views: AgentMessageView[],
  active: boolean,
) {
  let processViews: AgentMessageView[] = []
  const flushProcess = () => {
    if (processViews.length === 0) return
    sections.push({
      active,
      key: `process-${agentViewStableKey(processViews[0])}`,
      kind: 'process',
      views: processViews,
    })
    processViews = []
  }
  for (const view of views) {
    if (!isVisibleMediaView(view)) {
      processViews.push(view)
      continue
    }
    flushProcess()
    sections.push({
      key: `message-${agentViewStableKey(view)}`,
      kind: 'message',
      view,
    })
  }
  flushProcess()
}

function isVisibleMediaView(view: AgentMessageView) {
  return view.kind === 'interactive-image' ||
    ((view.kind === 'tool' || view.kind === 'tool-result') && !isAgentTraceView(view))
}

function isActiveRunSlice(views: AgentMessageView[], afterRunIndex: number, isStreaming: boolean) {
  if (!isStreaming) return false
  for (let index = afterRunIndex; index < views.length; index += 1) {
    const view = views[index]
    if (isAgentRunMetadataView(view)) continue
    if (view.kind === 'user' || view.kind === 'clear') return false
  }
  return true
}
