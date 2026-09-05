import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import {
  booleanMeta,
  DetailPre,
  DetailStack,
  fieldMeta,
  formatMaybeJSON,
  inlinePreview,
  MetaLine,
  numberValue,
  numericMeta,
  parseRecord,
  recordArray,
  recordValue,
  splitCanonicalResult,
  stringArray,
  stringValue,
  type ToolDetailAdapter,
  type ToolDetailRenderer,
  type ToolDetailRenderProps,
} from './shared'
import { LinkifyWorkspacePaths, WorkspacePathText } from './path-text'

export const workspaceToolDetailAdapters: Record<string, ToolDetailAdapter> = {
  read: outputAdapter(renderReadInput, renderReadOutput),
  glob: outputAdapter(renderGlobInput, renderGlobOutput),
  grep: outputAdapter(renderGrepInput, renderGrepOutput),
  bash: outputAdapter(renderShellInput, renderShellOutput),
  pwsh: outputAdapter(renderShellInput, renderShellOutput),
  write: inputAdapter(renderWriteInput, renderMutationOutput),
  edit: { ...inputAdapter(renderEditInput, renderMutationOutput), summarize: summarizeEditFailure },
}

function outputAdapter(renderInput: ToolDetailRenderer, renderOutput: ToolDetailRenderer): ToolDetailAdapter {
  return { layout: 'output', renderInput, renderOutput }
}

function inputAdapter(renderInput: ToolDetailRenderer, renderOutput: ToolDetailRenderer): ToolDetailAdapter {
  return { layout: 'input', renderInput, renderOutput }
}

function renderReadInput({ input, rawInput }: ToolDetailRenderProps) {
  const path = stringValue(input.path) || rawInput
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]"><WorkspacePathText>{path}</WorkspacePathText></DetailPre>
      <MetaLine items={[
        numericMeta('offset', input.offset),
        numericMeta('byte_offset', input.byte_offset),
        numericMeta('limit', input.limit),
        numericMeta('depth', input.depth),
        booleanMeta('hidden', input.hidden),
      ]} />
    </DetailStack>
  )
}

function renderReadOutput({ result, t }: ToolDetailRenderProps) {
  const parts = splitCanonicalResult(result)
  if (parts.metadata?.schema !== 'resource.read.v1') return <DetailPre>{formatMaybeJSON(result)}</DetailPre>

  const recovery = recordValue(parts.metadata.recovery)
  const content = parts.payload.replace(/\n+$/, '') || stringValue(recovery.suggestion) || t('chat.tool.noReturn')
  const lines = parseReadLines(content)
  if (!lines) return <DetailPre>{content}</DetailPre>

  return (
    <div data-nova-read-output className="grid min-w-0 grid-cols-[max-content_minmax(0,1fr)] gap-x-2 text-[var(--nova-text)]">
      {lines.map((line, index) => (
        <div key={`${line.number}-${index}`} className="contents">
          <span aria-hidden data-nova-read-line-number className="select-none text-right text-[var(--nova-text-faint)]">
            {line.number}
          </span>
          <DetailPre data-nova-read-line-content>{line.content}</DetailPre>
        </div>
      ))}
    </div>
  )
}

function renderGlobInput({ input }: ToolDetailRenderProps) {
  const paths = stringArray(input.paths)
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]">{paths.length > 0 ? paths.join('\n') : '.'}</DetailPre>
      <MetaLine items={[
        booleanMeta('hidden', input.hidden),
        booleanMeta('gitignore', input.gitignore),
        numericMeta('limit', input.limit),
        stringValue(input.cursor) ? `cursor=${inlinePreview(stringValue(input.cursor))}` : '',
      ]} />
    </DetailStack>
  )
}

function renderGlobOutput(props: ToolDetailRenderProps) {
  return renderCanonicalSearchOutput(props, (content) => (
    <div className="min-w-0 space-y-0.5">
      {content.split('\n').map((line, index) => (
        <DetailPre key={`${line}-${index}`}><WorkspacePathText>{line}</WorkspacePathText>{index < content.split('\n').length - 1 ? '\n' : ''}</DetailPre>
      ))}
    </div>
  ))
}

function renderGrepInput({ input, rawInput }: ToolDetailRenderProps) {
  const command = stringValue(input.command) || rawInput
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]"><LinkifyWorkspacePaths text={command} /></DetailPre>
      <MetaLine items={[fieldMeta('cursor', input.cursor)]} />
    </DetailStack>
  )
}

function renderGrepOutput(props: ToolDetailRenderProps) {
  return renderCanonicalSearchOutput(props, (content) => (
    <div className="min-w-0 space-y-0.5">
      {content.split('\n').map((line, index, lines) => <GrepLine key={`${line}-${index}`} line={line} trailingNewline={index < lines.length - 1} />)}
    </div>
  ))
}

function GrepLine({ line, trailingNewline }: { line: string; trailingNewline: boolean }) {
  const match = line.match(/^(.*?)(:\d+(?::\d+)?)(:.*)?$/)
  if (!match) return <DetailPre><LinkifyWorkspacePaths text={line} />{trailingNewline ? '\n' : ''}</DetailPre>
  return (
    <DetailPre>
      <WorkspacePathText>{match[1]}</WorkspacePathText>{match[2]}{match[3] || ''}{trailingNewline ? '\n' : ''}
    </DetailPre>
  )
}

function renderShellInput({ input, rawInput }: ToolDetailRenderProps) {
  const command = stringValue(input.command) || rawInput
  const cwd = stringValue(input.cwd)
  const env = recordValue(input.env)
  const envText = Object.entries(env).map(([key, value]) => `${key}=${String(value)}`).join(' ')
  return (
    <DetailStack className="space-y-1.5">
      <DetailPre className="text-[var(--nova-text)]"><LinkifyWorkspacePaths text={command} cwd={cwd} /></DetailPre>
      <MetaLine items={[
        cwd ? `cwd=${cwd}` : '',
        input.pty === true ? 'PTY' : '',
        numericMeta('timeout', input.timeout_seconds, 's'),
        envText ? `env ${envText}` : '',
      ]} />
    </DetailStack>
  )
}

function renderShellOutput({ input, result, t }: ToolDetailRenderProps) {
  const parts = splitCanonicalResult(result)
  if (parts.metadata?.schema !== 'process.result.v1') return <DetailPre><LinkifyWorkspacePaths text={formatMaybeJSON(result)} cwd={stringValue(input.cwd)} /></DetailPre>
  const recovery = recordValue(parts.metadata.recovery)
  let content = parts.payload.trimEnd() || stringValue(recovery.suggestion) || t('chat.tool.noReturn')
  if (content === '[Command executed successfully with no output]') content = t('chat.tool.noReturn')
  return <DetailPre><LinkifyWorkspacePaths text={content} cwd={stringValue(input.cwd)} /></DetailPre>
}

function renderWriteInput({ input, t }: ToolDetailRenderProps) {
  const content = stringValue(input.content)
  return (
    <DetailStack>
      <DetailPre className="border-b border-[var(--nova-border)] pb-2 text-[var(--nova-text)]">
        <WorkspacePathText>{stringValue(input.path)}</WorkspacePathText>
      </DetailPre>
      <DetailPre className={content ? undefined : 'text-[var(--nova-text-faint)]'}>
        {content || t('chat.tool.emptyContent')}
      </DetailPre>
    </DetailStack>
  )
}

function renderEditInput({ input, result, t }: ToolDetailRenderProps) {
  const operation = stringValue(input.operation)
  // Preserve array positions so diagnostics always refer to the original input.
  const edits = Array.isArray(input.edits) ? input.edits.map(recordValue) : []
  const failure = parseEditFailure(result)
  return (
    <DetailStack>
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 border-b border-[var(--nova-border)] pb-2">
        {operation === 'delete'
          ? <span className="min-w-0 break-all text-[var(--nova-text)]">{stringValue(input.path)}</span>
          : <WorkspacePathText className="min-w-0 break-all text-[var(--nova-text)]">{stringValue(input.path)}</WorkspacePathText>}
        {operation === 'delete' ? <span className="text-[10px] text-[var(--nova-danger)]">{t('chat.tool.edit.delete')}</span> : null}
      </div>
      {edits.map((edit, index) => {
        const issues = failure?.issues.filter(issue => issue.edit_index === index) || []
        const oldText = stringValue(edit.old_string)
        const previewTruncated = issues.length > 0 && oldText.length > 512
        return (
          <div key={index} data-nova-edit-index={index} className={cn('space-y-1.5', index > 0 && 'border-t border-[var(--nova-border)] pt-2')}>
            <div className="font-medium text-[var(--nova-text)]">{t('chat.tool.edit.item', { number: index + 1 })}</div>
            {issues.map((issue, issueIndex) => (
              <div key={issueIndex} className="text-[var(--nova-danger)]">{editIssueMessage(issue, t)}</div>
            ))}
            <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-2 border-l-2 border-red-500/45 bg-red-500/5 px-2 py-1.5 text-[var(--nova-danger)]">
              <span aria-hidden>−</span><DetailPre>{previewTruncated ? inlinePreview(oldText, 512) : oldText}</DetailPre>
            </div>
            {previewTruncated ? <div className="text-[var(--nova-text-faint)]">{t('chat.tool.edit.previewTruncated')}</div> : null}
            <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-2 border-l-2 border-emerald-500/45 bg-emerald-500/5 px-2 py-1.5 text-[var(--nova-accent-green)]">
              <span aria-hidden>+</span><DetailPre>{stringValue(edit.new_string)}</DetailPre>
            </div>
            {edit.replace_all === true ? <div className="text-[10px] text-[var(--nova-text-faint)]">{t('chat.tool.edit.replaceAll')}</div> : null}
          </div>
        )
      })}
    </DetailStack>
  )
}

function renderMutationOutput(props: ToolDetailRenderProps) {
  const { result, t } = props
  const failure = parseEditFailure(result)
  if (failure) {
    return (
      <DetailStack>
        <div>{t('chat.tool.edit.failedBatch')}</div>
        {failure.issues.map((issue, index) => {
          const editIndex = numberValue(issue.edit_index)
          return <div key={index}>{editIndex === undefined ? '' : `${t('chat.tool.edit.item', { number: editIndex + 1 })}: `}{editIssueMessage(issue, t)}</div>
        })}
        {failure.truncated ? <div>{t('chat.tool.edit.issuesTruncated')}</div> : null}
        <div>{t('chat.tool.edit.retryBatch')}</div>
      </DetailStack>
    )
  }
  const receipt = parseRecord(result)
  const schema = stringValue(receipt?.schema)
  if (schema !== 'workspace_change.tool_result.v1' && schema !== 'workspace_change.tool_noop.v1') {
    return <DetailPre>{formatMaybeJSON(result)}</DetailPre>
  }
  if (schema === 'workspace_change.tool_noop.v1') return <span>{t('chat.tool.change.unchanged')}</span>
  const stats = recordValue(receipt?.file_stats)
  const details = [
    t('chat.tool.change.applied'),
    numberValue(stats.lines) === undefined ? '' : t('chat.tool.change.lines', { count: numberValue(stats.lines) }),
    numberValue(stats.characters) === undefined ? '' : t('chat.tool.change.characters', { count: numberValue(stats.characters) }),
  ].filter(Boolean)
  return <span>{details.join(' · ')}</span>
}

function parseEditFailure(result: string) {
  const receipt = parseRecord(result.replace(/^\[tool error\]\s*/, ''))
  if (receipt?.schema !== 'workspace_change.tool_error.v1' || receipt.code !== 'invalid_edit' || receipt.workspace_mutated !== false) return null
  const details = recordValue(receipt.details)
  if (!Array.isArray(details.issues)) return null
  return { issues: recordArray(details.issues), truncated: details.issues_truncated === true }
}

function summarizeEditFailure({ result, t }: ToolDetailRenderProps) {
  const failure = parseEditFailure(result)
  if (!failure) return ''
  const numbers = [...new Set(failure.issues.map(issue => numberValue(issue.edit_index)))]
    .filter((index): index is number => index !== undefined && Number.isInteger(index) && index >= 0)
    .map(index => index + 1)
  return numbers.length
    ? t('chat.tool.edit.failedItems', { items: numbers.join(t('chat.tool.edit.itemSeparator')) })
    : t('chat.tool.edit.failedBatch')
}

function editIssueMessage(issue: Record<string, unknown>, t: ToolDetailRenderProps['t']) {
  const details = recordValue(issue.details)
  const otherIndex = numberValue(details.other_edit_index)
  switch (issue.code) {
    case 'not_found': return t('chat.tool.edit.issue.notFound')
    case 'not_unique': return t('chat.tool.edit.issue.notUnique')
    case 'empty_old_string': return t('chat.tool.edit.issue.emptyOldString')
    case 'no_change': return t('chat.tool.edit.issue.noChange')
    case 'fragment_too_large': return t('chat.tool.edit.issue.fragmentTooLarge')
    case 'scan_limit': return t('chat.tool.edit.issue.scanLimit')
    case 'replacement_limit': return t('chat.tool.edit.issue.replacementLimit')
    case 'total_replacement_limit': return t('chat.tool.edit.issue.totalReplacementLimit')
    case 'duplicate_id': return otherIndex === undefined ? t('chat.tool.edit.issue.invalid') : t('chat.tool.edit.issue.duplicateID', { number: otherIndex + 1 })
    case 'overlap': return otherIndex === undefined ? t('chat.tool.edit.issue.invalid') : t('chat.tool.edit.issue.overlap', { number: otherIndex + 1 })
    default: return t('chat.tool.edit.issue.invalid')
  }
}

function renderCanonicalSearchOutput(props: ToolDetailRenderProps, render: (content: string) => ReactNode) {
  const parts = splitCanonicalResult(props.result)
  if (parts.metadata?.schema !== 'workspace.search.v1') return <DetailPre>{formatMaybeJSON(props.result)}</DetailPre>
  const recovery = recordValue(parts.metadata.recovery)
  const content = parts.payload.trimEnd() || stringValue(recovery.suggestion) || props.t('chat.tool.noReturn')
  return render(content)
}

function parseReadLines(content: string) {
  const parsed: Array<{ number: string; content: string }> = []
  for (const line of content.split('\n')) {
    const separator = line.indexOf('\t')
    const number = line.slice(0, separator).trim()
    if (separator < 1 || !/^\d+$/.test(number)) return null
    parsed.push({ number, content: line.slice(separator + 1) })
  }
  return parsed
}
