import { AlertTriangle, Check, FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ReviewThreadFile } from '../types'

interface ReviewFileNavigatorProps {
  files: ReviewThreadFile[]
  selectedPath: string
  disabled?: boolean
  onSelect: (path: string) => void
}

export function ReviewFileNavigator({ files, selectedPath, disabled = false, onSelect }: ReviewFileNavigatorProps) {
  const { t } = useTranslation()
  return (
    <aside data-review-file-navigator className="flex h-full min-h-0 w-56 shrink-0 flex-col border-l border-[var(--nova-border)] bg-[var(--nova-surface)] max-lg:order-first max-lg:h-auto max-lg:max-h-44 max-lg:w-full max-lg:border-b max-lg:border-l-0">
      <div className="flex h-9 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3 text-[10px] font-medium uppercase tracking-wide text-[var(--nova-text-faint)]">
        <span>{t('router.files', { defaultValue: 'Files' })}</span>
        <span>{files.length}</span>
      </div>
      <div role="listbox" aria-label={t('router.files', { defaultValue: 'Files' })} className="min-h-0 flex-1 overflow-y-auto p-1.5 max-lg:flex max-lg:overflow-x-auto max-lg:overflow-y-hidden">
        {files.map((file) => {
          const selected = file.path === selectedPath
          const conflicted = file.continuity === 'conflicted' || file.continuity === 'discontinuous' || file.apply_state === 'conflicted'
          return (
            <button
              key={file.path}
              type="button"
              role="option"
              aria-selected={selected}
              disabled={disabled}
              onClick={() => onSelect(file.path)}
              className={`mb-1 flex w-full min-w-0 items-center gap-2 rounded-md border px-2 py-2 text-left transition-colors max-lg:mb-0 max-lg:mr-1.5 max-lg:w-56 max-lg:shrink-0 ${selected ? 'border-[var(--nova-accent-blue)] bg-[var(--nova-active)]' : 'border-transparent hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)]'} disabled:cursor-not-allowed disabled:opacity-50`}
              title={file.path}
            >
              <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
              <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[var(--nova-text)]">{file.path}</span>
              {conflicted ? <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-[var(--nova-warning)]" /> : file.review_status === 'accepted' ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-success)]" /> : null}
              {file.pending_edit_ids.length > 0 && <span className="shrink-0 rounded border border-[var(--nova-border)] px-1 text-[9px] text-[var(--nova-warning)]">{file.pending_edit_ids.length}</span>}
            </button>
          )
        })}
      </div>
    </aside>
  )
}
