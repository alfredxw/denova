import { useState, type ReactElement } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface ReadonlyPromptSourceBlockProps {
  title: string
  source?: string
  content?: string
}

export function ReadonlyPromptSourceBlock({ title, source, content = '' }: ReadonlyPromptSourceBlockProps): ReactElement {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <button type="button" onClick={() => setOpen((value) => !value)} className="flex w-full items-center gap-2 px-3 py-2 text-left" aria-expanded={open}>
        {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[11px] font-medium">{title}</span>
          {source ? <span className="block truncate text-[10px] text-[var(--nova-text-faint)]">{source}</span> : null}
        </span>
        <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('agents.prompt.badge.readonly')}</span>
      </button>
      {open ? <pre className="max-h-64 overflow-auto whitespace-pre-wrap border-t border-[var(--nova-border)] p-3 text-[11px] leading-5 text-[var(--nova-text-faint)]">{content || '—'}</pre> : null}
    </div>
  )
}
