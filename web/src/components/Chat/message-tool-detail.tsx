import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { ToolContent } from '@/components/ai-elements/tool'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import type { ToolResultSeverity } from '@/lib/tool-result-envelope'
import { toolDisplayName } from './tool-display-name'
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

export function toolDetailSummary(name: string, rawArgs: string, result: string, t: TFunction) {
  const adapter = toolDetailAdapters[name]
  if (!adapter?.summarize) return ''
  return adapter.summarize({
    input: parseRecord(rawArgs) || {},
    rawInput: rawArgs,
    result,
    t,
  })
}

// Keep parsing and adapter rendering below the disclosure's mount boundary.
// Primitive props also let completed details survive SDK message cloning without rebuilding.
export const ToolCallDetail = memo(function ToolCallDetail(props: ToolCallDetailProps) {
  const unified = toolDetailAdapters[props.name].layout === 'unified'
  return (
    <ToolContent
      data-nova-tool-detail={props.name}
      className={cn(
        'min-w-0 max-w-full border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)]',
        unified ? 'text-xs leading-5' : 'font-mono text-[11px] leading-relaxed',
      )}
    >
      <ToolCallDetailBody {...props} />
    </ToolContent>
  )
})

/** Reuses the same adapters in inline disclosures and the full-size inspector. */
export function ToolCallDetailBody({ name, rawArgs, result, resultSeverity, layout = 'inline' }: ToolCallDetailProps & { layout?: 'inline' | 'inspector' }) {
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

  if (layout === 'inspector') {
    return (
      <div className="flex min-w-0 flex-col gap-5 text-sm leading-6">
        {adapter.layout === 'unified' ? (
          input ? adapter.render(renderProps) : <DetailPre>{formatMaybeJSON(rawArgs)}</DetailPre>
        ) : (
          <>
            <section className="flex min-w-0 flex-col gap-2">
              <h3 className="font-medium">{t('chat.tool.inspector.input')}</h3>
              {input ? adapter.renderInput(renderProps) : <DetailPre>{formatMaybeJSON(rawArgs)}</DetailPre>}
            </section>
            {hasResult && (
              <section className="flex min-w-0 flex-col gap-2">
                <h3 className="font-medium">{t('chat.tool.inspector.output')}</h3>
                <div className={outputTone}>{adapter.renderOutput(renderProps)}</div>
              </section>
            )}
          </>
        )}
      </div>
    )
  }

  if (adapter.layout === 'unified') {
    return (
      <ScrollArea
        data-nova-tool-detail-scroll
        className="max-h-[min(30dvh,18rem)] overflow-hidden"
        viewportProps={{
          'aria-label': t('chat.tool.detail.scrollLabel', { name: toolDisplayName(name, t) }),
          className: 'max-h-[min(30dvh,18rem)]',
          tabIndex: 0,
        }}
      >
        <section
          data-nova-tool-detail-unified
          className="flex min-w-0 flex-col gap-4 px-3 py-3 pr-5 text-[var(--nova-text-muted)]"
        >
          {input ? adapter.render(renderProps) : <DetailPre>{formatMaybeJSON(rawArgs)}</DetailPre>}
        </section>
      </ScrollArea>
    )
  }

  return (
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
  )
}

export { formatMaybeJSON }
