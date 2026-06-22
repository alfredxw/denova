import { useState } from 'react'
import { AlertCircle, ChevronDown, ChevronRight, Loader2, ScrollText, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ContextAnalysis, ContextAnalysisCompaction, ContextAnalysisPart } from '@/lib/api'
import { focusDialogContentOnOpen } from './dialog-focus'

export const CONTEXT_ANALYSIS_SIMULATED_MESSAGE = '[Nova context analysis probe]'

export function ContextAnalysisDialog({ open, loading, error, analysis, onOpenChange, onRemoveCompaction }: {
  open: boolean
  loading: boolean
  error: string | null
  analysis: ContextAnalysis | null
  onOpenChange: (open: boolean) => void
  onRemoveCompaction?: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [removingCompaction, setRemovingCompaction] = useState(false)
  const [removeError, setRemoveError] = useState<string | null>(null)
  const finalMessageParts = analysis ? buildFinalMessageParts(analysis.context_messages) : []
  const handleRemoveCompaction = async () => {
    if (!onRemoveCompaction || removingCompaction) return
    setRemovingCompaction(true)
    setRemoveError(null)
    try {
      await onRemoveCompaction()
    } catch (e) {
      setRemoveError(t('chat.contextAnalysis.removeCompactionFailed', { error: (e as Error).message }))
    } finally {
      setRemovingCompaction(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        tabIndex={-1}
        onOpenAutoFocus={focusDialogContentOnOpen}
        className="flex max-h-[86vh] max-w-5xl flex-col gap-0 overflow-hidden border-[var(--nova-border)] bg-[var(--nova-bg)] p-0 text-[var(--nova-text)]"
      >
        <DialogHeader className="border-b border-[var(--nova-border)] px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-sm">
            <ScrollText className="h-4 w-4 text-[var(--nova-text-muted)]" />
            {t('chat.contextAnalysis.title')}
          </DialogTitle>
          <DialogDescription className="text-xs text-[var(--nova-text-faint)]">
            {t('chat.contextAnalysis.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {loading ? (
            <div className="flex min-h-44 items-center justify-center gap-2 text-xs text-[var(--nova-text-muted)]">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t('chat.contextAnalysis.loading')}
            </div>
          ) : error ? (
            <div className="flex min-h-32 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-3 py-2 text-xs text-[var(--nova-danger)]">
              <AlertCircle className="h-4 w-4 shrink-0" />
              {error}
            </div>
          ) : analysis ? (
            <div className="space-y-4">
              <ContextUsageSummary analysis={analysis} />
              <ContextAnalysisSection title={t('chat.contextAnalysis.systemPrompt')} parts={analysis.system_prompt_parts} />
              <ContextAnalysisSection
                title={t('chat.contextAnalysis.finalMessages')}
                parts={finalMessageParts}
                showRole
                compaction={analysis.compaction}
                removingCompaction={removingCompaction}
                removeCompactionError={removeError}
                onRemoveCompaction={analysis.compaction?.removable ? handleRemoveCompaction : undefined}
              />
            </div>
          ) : (
            <div className="min-h-32 text-xs text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.empty')}</div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ContextUsageSummary({ analysis }: { analysis: ContextAnalysis }) {
  const { t } = useTranslation()
  const usage = analysis.context_usage_ratio ? Math.round(analysis.context_usage_ratio * 100) : 0
  const items = [
    { label: t('chat.contextAnalysis.tokenEstimate'), value: formatNumber(analysis.token_estimate ?? 0) },
    { label: t('chat.contextAnalysis.contextWindow'), value: analysis.context_window_tokens ? formatNumber(analysis.context_window_tokens) : t('common.notSet') },
    { label: t('chat.contextAnalysis.contextUsage'), value: analysis.context_window_tokens ? `${usage}%` : t('common.notSet') },
    { label: t('chat.contextAnalysis.compaction'), value: analysis.compaction_active ? t('chat.contextAnalysis.compactionActive', { epoch: analysis.compaction_epoch ?? 0 }) : t('chat.contextAnalysis.compactionInactive') },
    { label: t('chat.contextAnalysis.wouldCompact'), value: analysis.would_compact ? t('common.yes') : t('common.no') },
  ]
  return (
    <div className="grid gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2 text-[11px] sm:grid-cols-5">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <div className="truncate text-[var(--nova-text-faint)]">{item.label}</div>
          <div className="mt-0.5 truncate font-medium text-[var(--nova-text)]">{item.value}</div>
        </div>
      ))}
    </div>
  )
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function buildFinalMessageParts(messages: ContextAnalysisPart[]): ContextAnalysisPart[] {
  return messages.map((part, index) => ({
    ...part,
    title: `#${index + 1} ${part.title || part.source}`,
  }))
}

function ContextAnalysisSection({ title, parts, showRole = false, compaction, removingCompaction = false, removeCompactionError, onRemoveCompaction }: {
  title: string
  parts: ContextAnalysisPart[]
  showRole?: boolean
  compaction?: ContextAnalysisCompaction
  removingCompaction?: boolean
  removeCompactionError?: string | null
  onRemoveCompaction?: () => void
}) {
  const { t } = useTranslation()
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-xs font-medium text-[var(--nova-text)]">{title}</h3>
        <span className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.partCount', { count: parts.length })}</span>
      </div>
      <div className="space-y-2">
        {parts.length > 0 ? parts.map((part, index) => (
          <ContextAnalysisPartBlock
            key={`${part.id || part.title}:${index}`}
            part={part}
            showRole={showRole}
            compaction={isCompactionPart(part) ? compaction : undefined}
            removingCompaction={removingCompaction}
            removeCompactionError={isCompactionPart(part) ? removeCompactionError : undefined}
            onRemoveCompaction={isCompactionPart(part) ? onRemoveCompaction : undefined}
          />
        )) : (
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-faint)]">
            {t('chat.contextAnalysis.noParts')}
          </div>
        )}
      </div>
    </section>
  )
}

function ContextAnalysisPartBlock({ part, showRole, compaction, removingCompaction = false, removeCompactionError, onRemoveCompaction }: {
  part: ContextAnalysisPart
  showRole: boolean
  compaction?: ContextAnalysisCompaction
  removingCompaction?: boolean
  removeCompactionError?: string | null
  onRemoveCompaction?: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const compactionMeta = compaction ? buildCompactionMeta(t, compaction) : ''
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          type="button"
          onClick={() => setOpen((current) => !current)}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          aria-expanded={open}
        >
          {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />}
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[11px] font-medium text-[var(--nova-text)]">{part.title || part.source}</span>
            <span className="block truncate text-[10px] text-[var(--nova-text-faint)]">
              {part.source}
              {showRole && part.role ? ` · ${part.role}` : ''}
              {part.note ? ` · ${part.note}` : ''}
              {compactionMeta ? ` · ${compactionMeta}` : ''}
            </span>
          </span>
        </button>
        {onRemoveCompaction && (
          <button
            type="button"
            disabled={removingCompaction}
            aria-label={t('chat.contextAnalysis.removeCompaction')}
            onClick={onRemoveCompaction}
            className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] text-[var(--nova-text-muted)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {removingCompaction ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
            {removingCompaction ? t('chat.contextAnalysis.removingCompaction') : t('chat.contextAnalysis.removeCompaction')}
          </button>
        )}
        <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.partSize', { chars: part.chars, bytes: part.bytes })}</span>
      </div>
      {removeCompactionError && <div className="border-t border-[var(--nova-border)] px-3 py-2 text-[11px] text-[var(--nova-danger)]">{removeCompactionError}</div>}
      {open && (
        <div className="border-t border-[var(--nova-border)] p-3">
          {part.content.trim() ? (
            <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words text-[11px] leading-5 text-[var(--nova-text-muted)]">{part.content}</pre>
          ) : (
            <div className="text-[11px] text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.emptyPart')}</div>
          )}
        </div>
      )}
    </div>
  )
}

function isCompactionPart(part: ContextAnalysisPart) {
  return part.source === '上下文压缩' || part.content.includes('[Nova Context Compaction]')
}

function buildCompactionMeta(t: ReturnType<typeof useTranslation>['t'], compaction: ContextAnalysisCompaction) {
  const source = compaction.source_turn_count
    ? t('chat.contextAnalysis.sourceTurns', { count: compaction.source_turn_count })
    : t('chat.contextAnalysis.sourceMessages', { count: compaction.source_message_count ?? 0 })
  const ratio = compaction.target_ratio ? Math.round(compaction.target_ratio * 100) : 0
  return t('chat.contextAnalysis.compactionMeta', {
    source,
    before: formatNumber(compaction.tokens_before ?? 0),
    after: formatNumber(compaction.tokens_after ?? 0),
    ratio,
  })
}
