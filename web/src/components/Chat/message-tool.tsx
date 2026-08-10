import { useLayoutEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ToolCallChatMessage } from '@/lib/api'
import { decodeToolResultEnvelope, type ToolResultEnvelope } from '@/lib/tool-result-envelope'
import { useBottomScrollLock } from '@/hooks/useBottomScrollLock'
import { Tool, ToolContent } from '@/components/ai-elements/tool'
import { ToolApprovalPanel } from './ToolApprovalCard'
import type { AskInteractionResolver } from './AskInteractionCard'
import { AgentSourceBadge } from './message-source-badge'
import { ToolStatusIcon } from './message-tool-status'

export function ToolExecutionBlock({ message, onResolve, onLayoutChange }: { message: ToolCallChatMessage; onResolve?: AskInteractionResolver; onLayoutChange?: (element: HTMLElement) => void }) {
  const { t } = useTranslation()
  const approvalInteraction = message.ask?.kind === 'tool_approval' ? message.ask : undefined
  const approvalPending = approvalInteraction?.status === 'pending'
  const [expanded, setExpanded] = useState(() => approvalPending)
  const info = parseToolCallContent(message.content || '')
  const name = message.name || info.name
  const rawArgs = message.args !== undefined ? message.args : info.args
  const args = formatMaybeJSON(rawArgs)
  const status = message.status || 'running'
  const result = message.result || ''
  const isDelegationTool = name === 'task'
  const taskSubAgent = isDelegationTool ? (message.subagent_type || parseTaskSubagentType(rawArgs)) : ''
  const isChapterBodyHidden = message.sse_display_notice === 'chapter_body_hidden'
  const isDirectorPlanHidden = isChapterBodyHidden && message.agent_kind === 'interactive_director'
  const chapterBodyHiddenPath = isChapterBodyHidden ? extractToolArgPath(rawArgs) : ''
  const chapterGeneratedChars = isChapterBodyHidden && typeof message.sse_generated_chars === 'number' ? message.sse_generated_chars : undefined
  const displayName = isDelegationTool ? t('chat.subagent.taskLabel') : name
  const detailArgs = isDelegationTool ? formatTaskDelegationArgs(rawArgs) : (isChapterBodyHidden ? '' : args)
  const hasResult = status === 'success'
  useLayoutEffect(() => {
    if (approvalPending) setExpanded(true)
  }, [approvalPending, approvalInteraction?.id])
  const isStreamingContent = !approvalInteraction && !isChapterBodyHidden && status === 'running' && isContentTool(name) && rawArgs.length > 50
  const streamPreview = isStreamingContent ? extractStreamingContent(rawArgs) : ''
  // Short or disabled content streams use the compact writing activity label.
  const isContentToolLoading = !isChapterBodyHidden && !isStreamingContent && status === 'running' && isContentTool(name)
  const contentToolChars = isContentToolLoading && typeof message.sse_generated_chars === 'number' ? message.sse_generated_chars : undefined
  const summary = taskSubAgent
    ? t('chat.subagent.delegating', { name: taskSubAgent })
    : buildToolArgSummary(args) || (isStreamingContent ? t('chat.tool.writing') : t('chat.tool.preparing'))
  const resultBody = stripToolResultMetadata(result)
  const resultEnvelope = decodeToolResultEnvelope(resultBody)
  const resultSeverity = status === 'error' ? 'error' : resultEnvelope?.severity || 'success'
  const showReadableOutcome = resultSeverity !== 'success'
  const resultPreview = resultEnvelope
    ? buildToolResultEnvelopeSummary(t, resultEnvelope)
    : buildPreview(resultBody, 80)
  const detailResult = resultEnvelope ? formatMaybeJSON(resultBody) : result
  const displaySummary = isChapterBodyHidden
    ? chapterGeneratedChars !== undefined
      ? t(isDirectorPlanHidden ? (hasResult ? 'chat.tool.fileWrittenWithCount' : 'chat.tool.fileWritingWithCount') : (hasResult ? 'chat.tool.chapterWrittenWithCount' : 'chat.tool.chapterWritingWithCount'), { count: chapterGeneratedChars })
      : (isDirectorPlanHidden ? (hasResult ? t('chat.tool.fileWritten') : t('chat.tool.fileWriting')) : (hasResult ? t('chat.tool.chapterWritten') : t('chat.tool.chapterWriting')))
    : (hasResult
      ? resultPreview || t('chat.tool.done')
      : status === 'error'
        ? buildPreview(resultBody, 160) || t('chat.tool.failed')
      : isContentToolLoading
        ? (contentToolChars !== undefined ? t('chat.tool.fileWritingWithCount', { count: contentToolChars }) : t('chat.tool.fileWriting'))
        : summary)
  const headerSummary = approvalPending ? t('agentApproval.approval.waiting') : displaySummary
  const hasDetail = Boolean(approvalInteraction || detailArgs || result || isChapterBodyHidden)
  const streamPreviewScrollLock = useBottomScrollLock<HTMLDivElement>({
    enabled: isStreamingContent,
    resetKey: `${message.id || name}:tool-stream-preview`,
    contentKey: `${status}:${rawArgs.length}:${streamPreview.length}`,
  })

  return (
    <div className="flex justify-start">
      <Tool open={expanded} onOpenChange={setExpanded} className="mb-0 w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div
          data-nova-tool-header
          className={`grid min-h-10 min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 px-3 py-2 ${showReadableOutcome ? 'gap-y-1' : ''}`}
        >
          <ToolStatusIcon status={resultSeverity === 'error' ? 'error' : status} warning={resultSeverity === 'warning'} />
          <div className="flex min-w-0 items-center gap-2 overflow-hidden">
            <span className="shrink-0 font-medium text-[var(--nova-text)]">{t('chat.tool.calling')}</span>
            <code
              className="min-w-0 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--nova-text-muted)]"
            >
              {displayName}
            </code>
            {taskSubAgent && (
              <span
                className="min-w-0 truncate rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]"
              >
                {t('chat.subagent.delegating', { name: taskSubAgent })}
              </span>
            )}
            {message.subagent && <AgentSourceBadge message={message} compact />}
            {approvalPending && (
              <span className="shrink-0 rounded-full border border-amber-500/25 bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-300">
                {t('agentApproval.approval.waiting')}
              </span>
            )}
            {!showReadableOutcome && (
              <span className="min-w-0 flex-1 truncate text-[var(--nova-text-faint)]">
                {headerSummary}
              </span>
            )}
          </div>
          {hasDetail && !isStreamingContent && (
            <button
              type="button"
              className="col-start-3 row-start-1 shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              onClick={() => setExpanded(!expanded)}
            >
              {expanded ? t('chat.tool.collapse') : t('chat.tool.details')}
            </button>
          )}
          {showReadableOutcome && (
            <span className={`col-start-2 col-end-4 whitespace-normal pt-1 leading-4 ${resultSeverity === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-danger)]'}`}>
              {displaySummary}
            </span>
          )}
        </div>
        {/* Show the live content preview while arguments stream in. */}
        {isStreamingContent && streamPreview && (
          <div
            ref={streamPreviewScrollLock.ref}
            onScroll={streamPreviewScrollLock.onScroll}
            onWheel={streamPreviewScrollLock.onWheel}
            onKeyDown={streamPreviewScrollLock.onKeyDown}
            data-nova-scroll-lock="tool-stream-preview"
            className="min-w-0 max-w-full max-h-32 overflow-x-hidden overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-accent-green)] whitespace-pre-wrap [overflow-anchor:none] [overflow-wrap:anywhere]"
          >
            {streamPreview}
          </div>
        )}
        {!isStreamingContent && (
          <ToolContent className={`grid min-w-0 max-w-full gap-2 overflow-x-hidden overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5 font-mono text-[11px] leading-relaxed text-[var(--nova-text-muted)] ${approvalInteraction ? 'max-h-80' : 'max-h-48'}`}>
            {approvalInteraction && <ToolApprovalPanel message={message} onResolve={onResolve} embedded onLayoutChange={onLayoutChange} />}
            {isChapterBodyHidden && (
              <div className="grid gap-1 font-sans">
                {chapterBodyHiddenPath && (
                  <div className="min-w-0">
                    <span className="text-[var(--nova-text-faint)]">{t(isDirectorPlanHidden ? 'chat.tool.filePath' : 'chat.tool.chapterPath')}</span>
                    <code className="ml-1 break-all font-mono text-[var(--nova-text-muted)]">{chapterBodyHiddenPath}</code>
                  </div>
                )}
                {chapterGeneratedChars !== undefined && (
                  <div className="text-[var(--nova-text-faint)]">
                    {t(isDirectorPlanHidden ? 'chat.tool.fileGeneratedChars' : 'chat.tool.chapterGeneratedChars', { count: chapterGeneratedChars })}
                  </div>
                )}
                <div className="text-[var(--nova-text-faint)]">{t(isDirectorPlanHidden ? 'chat.tool.fileBodyHidden' : 'chat.tool.chapterBodyHidden')}</div>
              </div>
            )}
            {detailArgs && !approvalInteraction?.approval?.command && <pre className="m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]">{detailArgs}</pre>}
            {taskSubAgent && result && <div className="text-[var(--nova-text-muted)]">{t('chat.subagent.result')}</div>}
            {result && <pre className={`m-0 min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere] ${resultSeverity === 'error' ? 'text-[var(--nova-danger)]' : resultSeverity === 'warning' ? 'text-[var(--nova-warning)]' : 'text-[var(--nova-accent-green)]'}`}>{detailResult}</pre>}
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
  const canExpand = content.trim().replace(/\s+/g, ' ').length > 160
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
      <div className="w-full overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface)] text-xs shadow-[var(--nova-shadow)]">
        <div className="flex items-start gap-3 px-3 py-2.5">
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
                <button
                  type="button"
                  className="shrink-0 rounded border border-transparent px-1.5 py-0.5 text-[var(--nova-text-muted)] transition hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                  onClick={() => setExpanded(!expanded)}
                >
                  {expanded ? t('chat.tool.collapse') : t('chat.tool.expand')}
                </button>
              )}
            </div>
          </div>
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
    return typeof data.subagent_type === 'string' ? data.subagent_type : ''
  } catch {
    const match = args.match(/"subagent_type"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
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
  const isContinuablePage = result.status === 'partial'
    && result.continuation !== undefined
    && (result.schema === 'resource.read.v1' || result.schema === 'workspace.search.v1')
  if (!headline && isContinuablePage) {
    headline = t('chat.tool.result.pageReady')
  } else if (!headline && (result.status === 'partial' || result.truncated)) {
    headline = t(result.status === 'partial' ? 'chat.tool.result.partial' : 'chat.tool.result.truncated')
  }
  if (!headline && result.severity !== 'success') headline = result.status

  const continuation = result.continuation
    ? t(result.continuation.kind === 'offset' ? 'chat.tool.result.moreOffset' : 'chat.tool.result.moreCursor', {
        value: result.continuation.kind === 'cursor' ? buildPreview(result.continuation.value, 72) : result.continuation.value,
      })
    : ''
  // Web recovery strategies already have localized, actionable labels. Avoid
  // duplicating the provider's often bilingual suggested_action beside them.
  // Continuations already state the next page, while a process non-zero exit
  // may be the intended diagnostic result rather than a command to "fix".
  const recovery = !isWebAccess
    && !result.continuation
    && !(result.schema === 'process.result.v1' && result.status === 'failed')
    && result.recovery
    ? result.recovery
    : ''
  return [headline, webRetryKey ? t(webRetryKey) : '', continuation, recovery].filter(Boolean).join(' · ')
}

function formatMaybeJSON(value: string) {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function buildToolArgSummary(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const path = data.file_path || data.path || data.cwd || data.command
    if (typeof path === 'string' && path) return path
  } catch {
    // Non-JSON arguments use the generic preview.
  }
  return buildPreview(args, 120)
}

function extractToolArgPath(args: string) {
  if (!args) return ''
  try {
    const data = JSON.parse(args) as Record<string, unknown>
    const path = data.file_path || data.path
    return typeof path === 'string' ? path : ''
  } catch {
    const match = args.match(/"(?:file_path|path)"\s*:\s*"([^"]+)"/)
    return match?.[1] || ''
  }
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

/** Identifies tools whose large content arguments benefit from a live preview. */
function isContentTool(name: string): boolean {
  return ['write', 'edit'].includes(name)
}

/** Extract generated text from complete or incrementally streamed tool arguments. */
function extractStreamingContent(rawArgs: string): string {
  try {
    const parsed = JSON.parse(rawArgs) as Record<string, unknown>
    if (typeof parsed.content === 'string') return parsed.content
    if (Array.isArray(parsed.edits)) {
      const replacements = parsed.edits.flatMap((entry) => {
        if (!entry || typeof entry !== 'object') return []
        const value = (entry as Record<string, unknown>).new_string
        return typeof value === 'string' ? [value] : []
      })
      if (replacements.some((value) => value.length > 0)) return replacements.join('\n\n')
    }
    if (typeof parsed.new_string === 'string') return parsed.new_string
  } catch {
    // The accumulated stream can be incomplete; scan its valid string tokens.
  }

  const content = extractStreamingJSONStringValues(rawArgs, 'content')
  if (content.length > 0) return content[0]
  const replacements = extractStreamingJSONStringValues(rawArgs, 'new_string')
  return replacements.some((value) => value.length > 0) ? replacements.join('\n\n') : ''
}

type StreamingJSONStringToken = {
  value: string
  end: number
  complete: boolean
}

function extractStreamingJSONStringValues(rawArgs: string, targetKey: string): string[] {
  const values: string[] = []
  let offset = 0
  while (offset < rawArgs.length) {
    if (rawArgs[offset] !== '"') {
      offset += 1
      continue
    }
    const key = readStreamingJSONString(rawArgs, offset)
    if (!key.complete) break
    offset = key.end
    let cursor = skipJSONWhitespace(rawArgs, offset)
    if (key.value !== targetKey || rawArgs[cursor] !== ':') continue
    cursor = skipJSONWhitespace(rawArgs, cursor + 1)
    if (rawArgs[cursor] !== '"') {
      offset = cursor
      continue
    }
    const value = readStreamingJSONString(rawArgs, cursor)
    values.push(value.value)
    offset = value.end
    if (!value.complete) break
  }
  return values
}

function readStreamingJSONString(source: string, start: number): StreamingJSONStringToken {
  let escaped = false
  for (let index = start + 1; index < source.length; index += 1) {
    const char = source[index]
    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '"') {
      return {
        value: decodeStreamingJSONString(source.slice(start + 1, index)),
        end: index + 1,
        complete: true,
      }
    }
  }
  return {
    value: decodeStreamingJSONString(source.slice(start + 1)),
    end: source.length,
    complete: false,
  }
}

function decodeStreamingJSONString(rawValue: string): string {
  try {
    return JSON.parse(`"${rawValue}"`) as string
  } catch {
    let decoded = ''
    for (let index = 0; index < rawValue.length; index += 1) {
      const char = rawValue[index]
      if (char !== '\\') {
        decoded += char
        continue
      }
      const escaped = rawValue[index + 1]
      if (escaped === undefined) break
      index += 1
      const simpleEscape = ({
        '"': '"',
        '\\': '\\',
        '/': '/',
        b: '\b',
        f: '\f',
        n: '\n',
        r: '\r',
        t: '\t',
      } as Record<string, string>)[escaped]
      if (simpleEscape !== undefined) {
        decoded += simpleEscape
        continue
      }
      if (escaped === 'u') {
        const hex = rawValue.slice(index + 1, index + 5)
        if (!/^[0-9a-fA-F]{4}$/.test(hex)) break
        decoded += String.fromCharCode(Number.parseInt(hex, 16))
        index += 4
        continue
      }
      decoded += escaped
    }
    return decoded
  }
}

function skipJSONWhitespace(source: string, start: number): number {
  let offset = start
  while (offset < source.length && /\s/.test(source[offset])) offset += 1
  return offset
}

