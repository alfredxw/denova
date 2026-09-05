import { useLayoutEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, FileText, PanelRightOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ToolCallChatMessage } from '@/lib/api'
import { CollapsibleTrigger } from '@/components/ui/collapsible'
import { decodeToolResultEnvelope, type ToolResultEnvelope } from '@/lib/tool-result-envelope'
import { Tool, ToolContent } from '@/components/ai-elements/tool'
import { ToolApprovalPanel } from './ToolApprovalCard'
import type { AskInteractionResolver } from './AskInteractionCard'
import { AgentSourceBadge } from './message-source-badge'
import { ToolStatusIcon } from './message-tool-status'
import { StreamingToolInput } from './StreamingToolInput'
import { toolDisplayName } from './tool-display-name'
import { formatMaybeJSON, hasSpecializedToolDetail, ToolCallDetail, toolDetailSummary } from './message-tool-detail'
import { toolPresentationKind } from '@/lib/tool-presentation'
import { workspaceFileName } from '@/lib/workspace-path'
import { taskSubAgentSessionKey } from './subagent-session'
import { ToolInspectorButton } from './ToolInspector'

// Preview creative inputs as they arrive; operational tools stay compact regardless of input length.
const STREAMING_INPUT_PREVIEW_TOOLS = new Set([
  'write',
  'edit',
  'write_lore_items',
  'prepare_interactive_turn',
  'submit_interactive_turn',
])

export function ToolExecutionBlock({ message, showAgentSource = true, onResolve, onLayoutChange, onOpenSubAgentSession }: { message: ToolCallChatMessage; showAgentSource?: boolean; onResolve?: AskInteractionResolver; onLayoutChange?: (element: HTMLElement) => void; onOpenSubAgentSession?: (sessionKey: string) => void }) {
  const { t } = useTranslation()
  const approvalInteraction = message.ask?.kind === 'tool_approval' ? message.ask : undefined
  const approvalPending = approvalInteraction?.status === 'pending'
  const [expanded, setExpanded] = useState(() => approvalPending)
  const info = parseToolCallContent(message.content || '')
  const name = message.name || info.name
  const inputStreaming = message.streaming === true
  const rawArgs = message.args !== undefined ? message.args : info.args
  const showStreamingInput = !approvalInteraction && inputStreaming && rawArgs.length > 0 && STREAMING_INPUT_PREVIEW_TOOLS.has(name)
  const canInterpretInput = !inputStreaming
  const canShowDetail = Boolean(approvalInteraction) || canInterpretInput
  const args = canInterpretInput ? formatMaybeJSON(rawArgs) : rawArgs
  const status = message.status || 'running'
  const result = message.result || ''
  const presentationKind = toolPresentationKind(message, 'call')
  const isDelegationTool = presentationKind === 'delegation'
  const isTaskWait = name === 'task_wait'
  const isScriptTool = presentationKind === 'script'
  const taskSubAgent = canInterpretInput && isDelegationTool ? (message.subagent_type || parseTaskSubagentType(rawArgs)) : ''
  // The raw input remains opaque while streaming. File cards may read only the
  // path field as a non-authoritative label; this never rewrites the input.
  const fileTarget = isWorkspaceFileTool(name) ? extractToolArgPath(rawArgs) : ''
  const fileTargetSummary = fileTarget ? workspaceFileName(fileTarget) : ''
  let displayName = toolDisplayName(name, t)
  if (isDelegationTool) displayName = t('chat.subagent.taskLabel')
  if (isTaskWait) displayName = t('chat.subagent.waitLabel')
  const detailArgs = canInterpretInput
    ? (isDelegationTool ? formatTaskDelegationArgs(rawArgs) : args)
    : ''
  const hasResult = status === 'success'
  useLayoutEffect(() => {
    if (approvalPending) setExpanded(true)
  }, [approvalPending, approvalInteraction?.id])
  const commandDescription = isDescribedTool(name) ? readToolArgDescription(rawArgs) : ''
  let summary = fileTargetSummary || commandDescription || t('chat.tool.preparing')
  if (!inputStreaming) {
    if (isTaskWait) {
      summary = t('chat.subagent.waiting')
    } else if (taskSubAgent) {
      summary = t('chat.subagent.delegating', { name: taskSubAgent })
    } else {
      summary = commandDescription || fileTargetSummary || buildToolArgSummary(args) || t('chat.tool.preparing')
    }
  }
  const resultBody = stripToolResultMetadata(result)
  const taskSessionKey = name === 'task' && status === 'success' ? taskSubAgentSessionKey(resultBody) : ''
  const opensTaskSession = Boolean(taskSessionKey && onOpenSubAgentSession)
  const specializedSummary = canInterpretInput ? toolDetailSummary(name, rawArgs, resultBody, t) : ''
  if (specializedSummary) summary = specializedSummary
  const resultEnvelope = decodeToolResultEnvelope(resultBody)
  const resultSeverity = status === 'error' ? 'error' : resultEnvelope?.severity || 'success'
  const showReadableOutcome = resultSeverity !== 'success'
  const showStackedOutcome = resultSeverity === 'warning'
  let resultPreview = buildPreview(resultBody, 80)
  if (resultEnvelope) resultPreview = buildToolResultEnvelopeSummary(t, resultEnvelope)
  if (isTaskWait && resultBody) resultPreview = taskWaitResultSummary(resultBody, t)
  const fileResultSummary = fileTargetSummary
    ? (resultPreview && (showReadableOutcome || resultEnvelope?.status === 'partial')
        ? `${fileTargetSummary} · ${resultPreview}`
        : fileTargetSummary)
    : ''
  const detailResult = resultEnvelope ? formatMaybeJSON(resultBody) : result
  let displaySummary = summary
  if (status === 'cancelled') displaySummary = t('chat.tool.result.cancelled')
  if (status === 'error') displaySummary = (name === 'edit' ? specializedSummary : '') || buildPreview(resultBody, 160) || t('chat.tool.failed')
  if (hasResult) displaySummary = specializedSummary || commandDescription || fileResultSummary || resultPreview || t('chat.tool.done')
  const headerSummary = approvalPending ? t('agentApproval.approval.waiting') : displaySummary
  const hasDetail = Boolean(approvalInteraction || detailArgs || result)
  const canToggleDetail = hasDetail && canShowDetail && !opensTaskSession

  return (
    <div className="flex justify-start">
      <Tool open={expanded} onOpenChange={opensTaskSession ? undefined : setExpanded} className="mb-0 w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[11px] shadow-sm">
        {/* One hover surface includes both the disclosure trigger and its secondary action. */}
        <div className={`flex min-w-0 items-center transition-colors ${canToggleDetail || opensTaskSession ? 'hover:bg-[var(--nova-hover)]' : ''}`} data-nova-tool-header-row>
          <CollapsibleTrigger
            type="button"
            disabled={!canToggleDetail && !opensTaskSession}
            onClick={opensTaskSession ? (event) => {
              event.preventDefault()
              onOpenSubAgentSession?.(taskSessionKey)
            } : undefined}
            aria-label={opensTaskSession ? t('chat.subagent.openSession') : undefined}
            data-nova-tool-header
            className={`grid min-h-9 min-w-0 flex-1 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-1.5 px-2.5 py-1.5 text-left leading-4 enabled:cursor-pointer disabled:cursor-default ${showStackedOutcome ? 'gap-y-0.5' : ''}`}
          >
            <ToolStatusIcon status={resultSeverity === 'error' ? 'error' : status} warning={resultSeverity === 'warning'} />
            <div className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
              <span
                className="min-w-0 max-w-[42%] shrink-0 truncate font-medium text-[var(--nova-text)]"
                title={displayName === name ? undefined : name}
              >
                {displayName}
              </span>
              {isScriptTool && (
                <span className="shrink-0 rounded border border-emerald-500/25 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] text-emerald-700 dark:text-emerald-300">
                  {t('chat.tool.scriptBadge')}
                </span>
              )}
              {taskSubAgent && (
                <span
                  className="min-w-0 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]"
                >
                  {t('chat.subagent.delegating', { name: taskSubAgent })}
                </span>
              )}
              {showAgentSource && message.subagent ? <AgentSourceBadge message={message} compact /> : null}
              {approvalPending && (
                <span className="shrink-0 rounded-full border border-amber-500/25 bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-300">
                  {t('agentApproval.approval.waiting')}
                </span>
              )}
              {!showStackedOutcome && (
                <span
                  data-nova-tool-summary
                  className={`min-w-0 flex-1 truncate ${resultSeverity === 'error' ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}
                  title={headerSummary}
                >
                  {headerSummary}
                </span>
              )}
              {opensTaskSession && (
                <PanelRightOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" aria-hidden="true" />
              )}
            </div>
            {showStackedOutcome && (
              <span className="col-start-2 col-end-3 whitespace-normal pt-0.5 leading-4 text-[var(--nova-warning)]">
                {displaySummary}
              </span>
            )}
          </CollapsibleTrigger>
          {expanded && canToggleDetail && <ToolInspectorButton className="mr-2" />}
        </div>
        {showStreamingInput && (
          <StreamingToolInput rawInput={rawArgs} streamKey={message.id || name} />
        )}
        {canShowDetail && !approvalInteraction && hasSpecializedToolDetail(name) && (
          <ToolCallDetail
            name={name}
            rawArgs={rawArgs}
            result={resultBody}
            resultSeverity={resultSeverity}
          />
        )}
        {canShowDetail && (approvalInteraction || !hasSpecializedToolDetail(name)) && (
          <ToolContent className="min-w-0 max-w-full border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)]">
            <div className={`grid min-w-0 max-w-full gap-2 overflow-x-hidden overflow-y-auto px-3 py-2.5 ${approvalInteraction ? 'max-h-80' : 'max-h-48'}`}>
              {approvalInteraction && <ToolApprovalPanel message={message} onResolve={onResolve} embedded onLayoutChange={onLayoutChange} />}
              {detailArgs && !approvalInteraction?.approval?.command && <pre className="m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]">{detailArgs}</pre>}
              {taskSubAgent && result && <div className="text-[var(--nova-text-muted)]">{t('chat.subagent.result')}</div>}
              {result && <pre className={`m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere] ${resultSeverity === 'error' ? 'text-[var(--nova-danger)]' : resultSeverity === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-accent-green)]'}`}>{detailResult}</pre>}
            </div>
          </ToolContent>
        )}
      </Tool>
    </div>
  )
}

export function ToolResultBlock({ content }: { content: string }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const envelope = decodeToolResultEnvelope(stripToolResultMetadata(content))
  const severity = envelope?.severity || 'success'
  const preview = envelope ? buildToolResultEnvelopeSummary(t, envelope) : buildPreview(content, 160)
  const canExpand = content.trim().length > 0
  const isProcessExitWarning = severity === 'warning'
    && envelope?.schema === 'process.result.v1'
    && envelope.status === 'failed'
  const tone = severity === 'error'
    ? 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]'
    : severity === 'warning'
      ? 'border-[var(--nova-warning)]/40 bg-[var(--nova-warning-bg)] text-[var(--nova-warning)]'
      : 'border-[var(--nova-accent-green)]/35 bg-[var(--nova-accent-green)]/10 text-[var(--nova-accent-green)]'

  return (
    <div className="flex justify-start">
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-sm">
        <div className="flex min-w-0 items-center transition-colors hover:bg-[var(--nova-hover)]">
          <button type="button" className="flex min-w-0 flex-1 items-start gap-3 px-3 py-2.5 text-left enabled:cursor-pointer" disabled={!canExpand} aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
            <span className={`mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border ${tone}`}>
              {severity === 'error'
                ? <span className="text-xs font-semibold">!</span>
                : severity === 'warning'
                  ? <AlertTriangle className="h-3.5 w-3.5" />
                  : <CheckCircle2 className="h-3.5 w-3.5" />}
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-[var(--nova-text)]">
                  {t(severity === 'error'
                    ? 'chat.tool.resultFailed'
                    : isProcessExitWarning
                      ? 'chat.tool.resultAttention'
                      : severity === 'warning'
                        ? 'chat.tool.resultPartial'
                        : 'chat.tool.resultDone')}
                </span>
                <span className={`rounded-full border px-2 py-0.5 text-[11px] ${tone}`}>
                  {severity}
                </span>
              </div>
              <div className="mt-1 flex min-w-0 items-center gap-2 text-[var(--nova-text-faint)]">
                <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
                <span className="truncate">{preview || t('chat.tool.noReturn')}</span>
                {canExpand && (
                  <span
                    className="shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                  >
                    {expanded ? t('chat.tool.collapse') : t('chat.tool.expand')}
                  </span>
                )}
              </div>
            </div>
          </button>
          {expanded && <ToolInspectorButton className="mr-2" />}
        </div>
        {expanded && (
          <pre className="m-0 min-w-0 max-w-full max-h-56 overflow-x-hidden overflow-y-auto whitespace-pre-wrap border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)] [overflow-wrap:anywhere]">
            {content}
          </pre>
        )}
      </div>
    </div>
  )
}

function parseToolCallContent(content: string) {
  const [rawName = 'unknown_tool', ...rest] = content.split('\n')
  const name = rawName.trim() || 'unknown_tool'
  const args = formatMaybeJSON(rest.join('\n').trim())

  return {
    name,
    args,
    summary: buildToolArgSummary(args),
  }
}

function parseTaskSubagentType(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    if (typeof data.subagent_type === 'string') return data.subagent_type
    const starts = Array.isArray(data.starts) ? data.starts : []
    const first = starts[0]
    if (!first || typeof first !== 'object') return ''
    const agent = (first as Record<string, unknown>).agent
    return typeof agent === 'string' && agent.trim() ? agent : 'general-purpose'
  } catch {
    const match = args.match(/"subagent_type"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
  }
}

function taskWaitResultSummary(result: string, t: (key: string, options?: Record<string, unknown>) => string) {
  try {
    const parsed = JSON.parse(result) as { results?: Array<{ ready?: boolean; error?: string }> }
    const outcomes = Array.isArray(parsed.results) ? parsed.results : []
    if (!outcomes.length) return t('chat.subagent.waitComplete')
    const ready = outcomes.filter((outcome) => outcome.ready === true).length
    const failed = outcomes.filter((outcome) => Boolean(outcome.error)).length
    return failed > 0
      ? t('chat.subagent.waitResultWithFailures', { ready, total: outcomes.length, failed })
      : t('chat.subagent.waitResult', { ready, total: outcomes.length })
  } catch {
    return buildPreview(result, 80) || t('chat.subagent.waitComplete')
  }
}

function formatTaskDelegationArgs(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    delete data.subagent_type
    return Object.keys(data).length > 0 ? formatMaybeJSON(JSON.stringify(data)) : ''
  } catch {
    return formatMaybeJSON(args.replace(/"subagent_type"\s*:\s*"[^"]+"\s*,?\s*/g, '').replace(/,\s*}/g, '}'))
  }
}

export function stripToolResultMetadata(result: string) {
  for (const separator of ['\n\n[Denova tool result metadata]', '\n[Denova tool result metadata]']) {
    const markerIndex = result.lastIndexOf(separator)
    if (markerIndex >= 0) return result.slice(0, markerIndex).trimEnd()
  }
  return result
}

function buildToolResultEnvelopeSummary(t: ReturnType<typeof useTranslation>['t'], result: ToolResultEnvelope) {
  const isWebAccess = result.schema === 'web_fetch.v1' || result.schema === 'web_search.v1'
  const webStatusKey = isWebAccess ? {
    blocked: 'chat.tool.webAccess.blocked',
    no_results: 'chat.tool.webAccess.noResults',
    providers_unavailable: 'chat.tool.webAccess.providersUnavailable',
  }[result.status] : undefined
  const webRetryKey = isWebAccess && result.retryStrategy ? {
    change_query: 'chat.tool.webAccess.changeQuery',
    use_alternate_source: 'chat.tool.webAccess.useAlternateSource',
    wait_or_reconfigure: 'chat.tool.webAccess.waitOrReconfigure',
    wait_or_use_alternate_source: 'chat.tool.webAccess.waitOrUseAlternateSource',
  }[result.retryStrategy] : undefined

  let headline = webStatusKey ? t(webStatusKey) : ''
  if (!headline && result.schema === 'process.result.v1') {
    if (result.status === 'failed') {
      headline = result.exitCode === undefined
        ? t('chat.tool.result.commandExitedNonZero')
        : t('chat.tool.result.commandExitedWithCode', { code: result.exitCode })
    } else if (result.status === 'timed_out') {
      headline = t('chat.tool.result.timedOut')
    } else if (result.status === 'cancelled') {
      headline = t('chat.tool.result.cancelled')
    }
  }
  const isExpectedPartialPage = result.status === 'partial'
    && result.severity === 'success'
    && (result.schema === 'resource.read.v1' || result.schema === 'workspace.search.v1')
  if (!headline && isExpectedPartialPage) {
    headline = t('chat.tool.result.pageReady')
  } else if (!headline && (result.status === 'partial' || result.truncated)) {
    headline = t(result.status === 'partial' ? 'chat.tool.result.partial' : 'chat.tool.result.truncated')
  }
  if (!headline && result.severity !== 'success') headline = result.status

  let continuation = ''
  if (result.continuation?.kind === 'offset') {
    const messageKey = result.continuation.byteOffset
      ? 'chat.tool.result.moreOffsetWithByteOffset'
      : 'chat.tool.result.moreOffset'
    continuation = t(messageKey, {
      value: result.continuation.value,
      byteOffset: result.continuation.byteOffset,
    })
  } else if (result.continuation?.kind === 'cursor') {
    continuation = t('chat.tool.result.moreCursor', { value: buildPreview(result.continuation.value, 72) })
  }
  // Web recovery strategies already have localized, actionable labels. Avoid
  // duplicating the provider's often bilingual suggested_action beside them.
  // Continuations already state the next page, while a process non-zero exit
  // may be the intended diagnostic result rather than a command to "fix".
  const recovery = !isWebAccess
    && !result.continuation
    && !isExpectedPartialPage
    && !(result.schema === 'process.result.v1' && result.status === 'failed')
    && result.recovery
    ? result.recovery
    : ''
  return [headline, webRetryKey ? t(webRetryKey) : '', continuation, recovery].filter(Boolean).join(' · ')
}

function buildToolArgSummary(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const filePath = data.file_path || data.path
    if (typeof filePath === 'string' && filePath) return workspaceFileName(filePath)
    const target = data.cwd || data.command
    if (typeof target === 'string' && target) return target
  } catch {
    // Non-JSON arguments use the generic preview.
  }
  return buildPreview(args, 120)
}

/** Reads optional model-authored display metadata without making it execution-authoritative. */
function readToolArgDescription(args: string) {
  return extractToolArgString(args, ['description']).trim()
}

function extractToolArgPath(args: string) {
  return extractToolArgString(args, ['file_path', 'path'])
}

function extractToolArgString(args: string, fields: readonly string[]) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    for (const field of fields) {
      const value = data[field]
      if (typeof value === 'string') return value
    }
    return ''
  } catch {
    const match = args.match(new RegExp(`"(?:${fields.join('|')})"\\s*:\\s*"((?:\\\\.|[^"\\\\])*)"`))
    if (!match) return ''
    try {
      return JSON.parse(`"${match[1]}"`) as string
    } catch {
      return match[1]
    }
  }
}

function isWorkspaceFileTool(name: string) {
  return name === 'read' || name === 'write' || name === 'edit'
}

function isDescribedTool(name: string) {
  return name === 'grep' || name === 'bash'
}

function buildPreview(content: string, maxLength: number) {
  const normalized = content.trim().replace(/\s+/g, ' ')
  if (normalized.length <= maxLength) return normalized
  return `${normalized.slice(0, maxLength)}...`
}

export function buildMarkdownPreview(content: string, maxLength: number) {
  const trimmed = content.trim()
  const chars = Array.from(trimmed)
  if (chars.length <= maxLength) return trimmed
  return `${chars.slice(0, maxLength).join('').trimEnd()}\n\n...`
}
