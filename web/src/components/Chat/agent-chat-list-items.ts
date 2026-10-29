import { agentViewToRenderMessage, agentViewNavigationAnchor, agentViewContent, agentViewAskInteraction, agentViewStableKey, buildAgentSubAgentTimelineGroups, isAgentRunMetadataView, isAgentTraceView, type AgentMessageView } from '@/lib/agent-message-view'
import { buildSubAgentSummaryMessage } from './subagent-session'
import { buildAgentRunPresentation } from './agent-run-presentation'
import { buildToolCallTree, type AgentProcessNode } from './tool-call-tree'
import { chatListItemRunID, type AgentChatListItem } from './AgentChatListRow'
import type { AgentTimelineAttachment } from './AgentMessageList'

export function buildAgentChatListItems({ views, isStreaming, activeRunId, isExecutionActive, visibleActivityContent, collapseTraceGroups, groupSubAgentTimeline, timelineAttachments, disclosures = {}, activeTraceDisplay = 'expanded' }: { views: AgentMessageView[]; isStreaming: boolean; activeRunId?: string; isExecutionActive: boolean; visibleActivityContent: string; collapseTraceGroups: boolean; groupSubAgentTimeline: boolean; timelineAttachments: AgentTimelineAttachment[]; disclosures?: Record<string, boolean>; activeTraceDisplay?: 'expanded' | 'collapsed' }): AgentChatListItem[] {
  const items: AgentChatListItem[] = []
  if (!isStreaming && views.every(isAgentRunMetadataView)) {
    items.push({ kind: 'empty', key: 'empty' })
    return items
  }
  const subAgentGroups = groupSubAgentTimeline ? buildAgentSubAgentTimelineGroups(views) : []
  const subAgentGroupsByStart = new Map(subAgentGroups.map(group => [group.startIndex, group]))
  const groupedSubAgentIndexes = new Set(subAgentGroups.flatMap(group => group.viewIndexes))

  function appendProcess(key: string, processViews: AgentMessageView[], active: boolean, sourceIndex: number, showTiming: boolean, owner?: { runId: string; navigationAnchor: string }) {
    const deferred = processViews.some(view => view.metadata.execution_details_deferred)
    const running = !deferred && (active || processViews.some(view => view.streaming || view.status === 'running'))
    const disclosure = disclosures[key]
    const expanded = !deferred && (disclosure ?? (running && activeTraceDisplay === 'expanded'))
    const runId = owner?.runId || processViews.findLast(view => view.metadata.run_id)?.metadata.run_id || ''
    const navigationAnchor = owner?.navigationAnchor || processViews.map(agentViewNavigationAnchor).findLast(Boolean) || ''
    items.push({ kind: 'process', key, views: processViews, runId, running, expanded, showTiming, navigationAnchor })
    const appendNodes = (nodes: AgentProcessNode[], depth: number) => {
      const groups = groupSubAgentTimeline ? buildAgentSubAgentTimelineGroups(nodes.map(node => node.view)) : []
      const byStart = new Map(groups.map(group => [group.startIndex, group]))
      const grouped = new Set(groups.flatMap(group => group.viewIndexes))
      nodes.forEach((node, index) => {
        const group = byStart.get(index)
        if (group) {
          const approval = group.views.find(view => agentViewAskInteraction(view)?.status === 'pending')
          if (approval) items.push({ kind: 'message', key: `message-${agentViewStableKey(approval)}`, view: approval, sourceIndex, processKey: key, depth, ownerRunId: runId, navigationAnchor })
          else {
            const summary = buildSubAgentSummaryMessage(group.views)
            if (summary) items.push({ kind: 'legacy-message', key: `subagent-${group.key}`, message: summary, openView: group.views[0], sourceIndex, processKey: key, depth, ownerRunId: runId, navigationAnchor })
          }
          return
        }
        if (grouped.has(index)) return
        const view = node.view
        if (!(view.kind === 'reasoning' && !view.streaming && !agentViewContent(view).trim())) {
          items.push({ kind: 'message', key: agentMessageItemKey(view, sourceIndex), view, sourceIndex, processKey: key, depth, ownerRunId: runId, navigationAnchor })
        }
        appendNodes(node.children, depth + 1)
      })
    }
    // Collapsing progress must not hide a decision or a failed operation.
    const visibleViews = expanded ? processViews : processViews.filter(view => agentViewAskInteraction(view)?.status === 'pending' || view.status === 'error')
    appendNodes(buildToolCallTree(visibleViews), 0)
  }

  for (let index = 0; index < views.length; index += 1) {
    const view = views[index]
    if (isAgentRunMetadataView(view)) continue
    const subAgentGroup = subAgentGroupsByStart.get(index)
    if (subAgentGroup) {
      const pendingApprovalView = subAgentGroup.views.find(item => agentViewAskInteraction(item)?.status === 'pending')
      if (pendingApprovalView) {
        const approvalMessage = agentViewToRenderMessage(pendingApprovalView)
        if (approvalMessage) {
          items.push({
            kind: 'legacy-message', key: `subagent-approval-${subAgentGroup.key || index}`,
            message: approvalMessage, sourceIndex: index, openView: pendingApprovalView,
          })
          continue
        }
      }
      const summary = buildSubAgentSummaryMessage(subAgentGroup.views)
      if (summary) {
        items.push({ kind: 'legacy-message', key: `subagent-${subAgentGroup.key || index}`, message: summary, sourceIndex: index, openView: subAgentGroup.views[0] })
        continue
      }
    }
    if (groupedSubAgentIndexes.has(index)) continue
    if (collapseTraceGroups) {
      const run = buildAgentRunPresentation(views, index, isExecutionActive)
      if (run) {
        const owner = { runId: run.runID, navigationAnchor: run.sections.flatMap(section => section.kind === 'process' ? section.views : [section.view]).map(agentViewNavigationAnchor).findLast(Boolean) || '' }
        const timedKey = (run.sections.find(section => section.kind === 'process' && section.active) || run.sections.find(section => section.kind === 'process'))?.key
        for (const section of run.sections) {
          if (section.kind === 'message') items.push({ kind: 'message', key: section.key, view: section.view, sourceIndex: index })
          else appendProcess(section.key, section.views, section.active, index, section.key === timedKey, owner)
        }
        const next = views[run.nextIndex]
        if (!isStreaming && !run.sections.some(section => section.kind === 'process' && section.active)
          && !run.sections.some(section => section.kind === 'message' && section.view.kind === 'assistant')
          && !(next?.kind === 'error' && next.metadata.run_id === run.runID)) {
          items.push({ kind: 'run-actions', key: `${run.key}-actions`, runId: run.runID })
        }
        index = run.nextIndex - 1
        continue
      }
    }
    if (collapseTraceGroups && isAgentTraceView(view)) {
      // Keep trailing tools grouped even when they follow a Game narrative.
      const traceViews: AgentMessageView[] = []
      let nextIndex = index
      while (nextIndex < views.length && isAgentTraceView(views[nextIndex])) {
        traceViews.push(views[nextIndex])
        nextIndex += 1
      }
      const activeStreamingTrace = isActiveStreamingTrace(views, nextIndex, isExecutionActive)
      appendProcess(`trace-${agentViewStableKey(traceViews[0]) || index}`, traceViews, activeStreamingTrace, index, true)
      index = nextIndex - 1
      continue
    }
    if (view.kind === 'clear') {
      items.push({ kind: 'clear', key: agentMessageItemKey(view, index), createdAt: readString(view.data.created_at) || view.metadata.created_at })
      continue
    }
    items.push({ kind: 'message', key: agentMessageItemKey(view, index), view, sourceIndex: index })
  }

  for (const attachment of timelineAttachments) {
    const runId = attachment.runId.trim()
    if (!runId) continue
    let insertAt = -1
    for (let index = items.length - 1; index >= 0; index -= 1) {
      if (chatListItemRunID(items[index]) === runId) {
        insertAt = index + 1
        break
      }
    }
    if (insertAt < 0) continue
    while (insertAt < items.length && items[insertAt]?.kind === 'attachment') insertAt += 1
    items.splice(insertAt, 0, {
      kind: 'attachment',
      key: `attachment-${attachment.id}`,
      runId,
      content: attachment.content,
    })
  }

  if (isStreaming) {
    if (visibleActivityContent) {
      items.push({
        kind: 'activity', key: `activity-${visibleActivityContent.length}`, content: visibleActivityContent,
        runId: activeRunId && !items.some(item => chatListItemRunID(item) === activeRunId) ? activeRunId : undefined,
      })
    } else if (views.length === 0) {
      items.push({ kind: 'typing', key: 'typing' })
    }
  }

  return items
}

function agentMessageItemKey(view: AgentMessageView, index: number) {
  const prefix = view.kind === 'clear' ? 'clear' : 'message'
  const stableKey = agentViewStableKey(view)
  if (stableKey) return `${prefix}-${stableKey}`
  if (view.metadata.created_at) return `${prefix}-${view.metadata.created_at}-${index}`
  return `${prefix}-${index}`
}

function isActiveStreamingTrace(views: AgentMessageView[], afterTraceIndex: number, isStreaming: boolean) {
  if (!isStreaming) return false
  for (let index = afterTraceIndex; index < views.length; index += 1) {
    const view = views[index]
    if (isAgentRunMetadataView(view)) continue
    if (view.kind === 'user') return false
    if (view.kind === 'assistant' && agentViewContent(view).trim()) {
      // A prose row is the semantic boundary after the preceding trace. The
      // prose may still be streaming, but its thinking/tools disclosure is no
      // longer the active tail and must match the completed presentation.
      return false
    }
  }
  return true
}

function readString(value: unknown) { return typeof value === 'string' ? value : '' }
