import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { TextSelection } from '@/lib/api'

interface ContextHandoffTrayProps {
  selections: TextSelection[]
  onRemove?: (index: number) => void
}

export function ContextHandoffTray({ selections, onRemove }: ContextHandoffTrayProps) {
  const { t } = useTranslation()

  return (
    <div className="mb-2 grid gap-1.5" role="list" aria-label={t('chat.handoff.listLabel')}>
      {selections.map((selection, index) => {
        const source = selection.source === 'editor_selection'
          ? t('chat.handoff.source.editorSelection')
          : (selection.source || t('chat.handoff.source.editorSelection'))
        const purpose = selection.purpose === 'ask_agent'
          ? t('chat.handoff.purpose.askAgent')
          : (selection.purpose || t('chat.handoff.purpose.askAgent'))
        const version = selection.version || t('chat.handoff.versionUnknown')
        const sizeBytes = new TextEncoder().encode(selection.content).byteLength

        return (
          <div
            key={`${selection.fileName}:${selection.startLine}:${selection.endLine}:${index}`}
            className="flex min-w-0 items-start gap-2 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1.5 text-xs"
            role="listitem"
          >
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium text-[var(--nova-text)]" title={selection.fileName}>
                {selection.fileName}:L{selection.startLine}
                {selection.endLine !== selection.startLine && `-L${selection.endLine}`}
              </div>
              <div className="mt-0.5 flex min-w-0 flex-wrap gap-x-2 gap-y-0.5 text-[var(--nova-text-muted)]">
                <span>{source}</span>
                <span>{purpose}</span>
                <span>{version}</span>
                <span>{t('chat.handoff.sizeBytes', { bytes: sizeBytes })}</span>
              </div>
            </div>
            {onRemove ? (
              <button
                type="button"
                className="shrink-0 rounded p-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                aria-label={t('chat.handoff.remove', { source: selection.fileName })}
                onClick={() => onRemove(index)}
              >
                <X aria-hidden="true" className="h-3.5 w-3.5" />
              </button>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}
