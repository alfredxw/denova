import { useTranslation } from 'react-i18next'
import { ToolContent } from '@/components/ai-elements/tool'
import { cn } from '@/lib/utils'
import type { ToolResultSeverity } from '@/lib/tool-result-envelope'
import { domainToolDetailAdapters } from './tool-detail/domain'
import { generalToolDetailAdapters } from './tool-detail/general'
import {
  DetailPre,
  formatMaybeJSON,
  parseRecord,
  type ToolDetailAdapter,
  type ToolDetailRenderProps,
} from './tool-detail/shared'
import { workspaceToolDetailAdapters } from './tool-detail/workspace'

const toolDetailAdapters: Record<string, ToolDetailAdapter> = {
  ...workspaceToolDetailAdapters,
  ...generalToolDetailAdapters,
  ...domainToolDetailAdapters,
}

interface ToolCallDetailProps {
  name: string
  rawArgs: string
  result: string
  resultSeverity: ToolResultSeverity
}

export function hasSpecializedToolDetail(name: string) {
  return Object.hasOwn(toolDetailAdapters, name)
}

export function ToolCallDetail({ name, rawArgs, result, resultSeverity }: ToolCallDetailProps) {
  const { t } = useTranslation()
  const adapter = toolDetailAdapters[name]
  const input = parseRecord(rawArgs)
  const hasResult = result.length > 0
  const outputTone = resultSeverity === 'error'
    ? 'text-[var(--nova-danger)]'
    : resultSeverity === 'warning'
      ? 'text-[var(--nova-warning)]'
      : 'text-[var(--nova-accent-green)]'
  const renderProps: ToolDetailRenderProps = {
    input: input || {},
    rawInput: rawArgs,
    result,
    t,
  }

  return (
    <ToolContent
      data-nova-tool-detail={name}
      className="max-h-48 min-w-0 max-w-full space-y-0 overflow-x-hidden overflow-y-hidden border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 font-mono text-[11px] leading-relaxed"
    >
      <div className="flex max-h-48 min-h-0 flex-col">
        <section
          data-nova-tool-detail-input
          className={cn(
            'min-w-0 px-3 py-2.5 text-[var(--nova-text-muted)]',
            adapter.layout === 'input'
              ? 'min-h-0 flex-1 overflow-y-auto'
              : hasResult
                ? 'max-h-20 shrink-0 overflow-y-auto'
                : 'max-h-48 overflow-y-auto',
          )}
        >
          {input ? adapter.renderInput(renderProps) : <DetailPre>{formatMaybeJSON(rawArgs)}</DetailPre>}
        </section>
        {hasResult ? (
          <section
            data-nova-tool-detail-output
            className={cn(
              'min-w-0 border-t border-[var(--nova-border)] px-3 py-2.5',
              adapter.layout === 'input'
                ? 'max-h-20 shrink-0 overflow-y-auto'
                : 'min-h-0 flex-1 overflow-y-auto',
              outputTone,
            )}
          >
            {adapter.renderOutput(renderProps)}
          </section>
        ) : null}
      </div>
    </ToolContent>
  )
}

export { formatMaybeJSON }
