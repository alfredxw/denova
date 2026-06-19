import { useState } from 'react'
import { AlertCircle, ChevronDown, ChevronRight, Loader2, ScrollText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { ContextAnalysis, ContextAnalysisPart } from '@/lib/api'

export const CONTEXT_ANALYSIS_SIMULATED_MESSAGE = '[Nova context analysis probe]'

export function ContextAnalysisDialog({ open, loading, error, analysis, onOpenChange }: {
  open: boolean
  loading: boolean
  error: string | null
  analysis: ContextAnalysis | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const finalMessageParts = analysis ? buildFinalMessageParts(analysis.context_messages) : []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[86vh] max-w-5xl flex-col gap-0 overflow-hidden border-[var(--nova-border)] bg-[var(--nova-bg)] p-0 text-[var(--nova-text)]">
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
              <ContextAnalysisSection title={t('chat.contextAnalysis.systemPrompt')} parts={analysis.system_prompt_parts} />
              <ContextAnalysisSection title={t('chat.contextAnalysis.finalMessages')} parts={finalMessageParts} showRole />
            </div>
          ) : (
            <div className="min-h-32 text-xs text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.empty')}</div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function buildFinalMessageParts(messages: ContextAnalysisPart[]): ContextAnalysisPart[] {
  return messages.map((part, index) => ({
    ...part,
    title: `#${index + 1} ${part.title || part.source}`,
  }))
}

function ContextAnalysisSection({ title, parts, showRole = false }: {
  title: string
  parts: ContextAnalysisPart[]
  showRole?: boolean
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
          <ContextAnalysisPartBlock key={`${part.id || part.title}:${index}`} part={part} showRole={showRole} />
        )) : (
          <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-xs text-[var(--nova-text-faint)]">
            {t('chat.contextAnalysis.noParts')}
          </div>
        )}
      </div>
    </section>
  )
}

function ContextAnalysisPartBlock({ part, showRole }: { part: ContextAnalysisPart; showRole: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
        aria-expanded={open}
      >
        {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[11px] font-medium text-[var(--nova-text)]">{part.title || part.source}</span>
          <span className="block truncate text-[10px] text-[var(--nova-text-faint)]">
            {part.source}
            {showRole && part.role ? ` · ${part.role}` : ''}
            {part.note ? ` · ${part.note}` : ''}
          </span>
        </span>
        <span className="shrink-0 text-[10px] text-[var(--nova-text-faint)]">{t('chat.contextAnalysis.partSize', { chars: part.chars, bytes: part.bytes })}</span>
      </button>
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
