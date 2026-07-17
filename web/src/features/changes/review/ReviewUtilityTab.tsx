import { Rows3, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

export function ReviewUtilityTab({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex h-9 shrink-0 items-end border-b border-[var(--nova-border)] bg-[var(--nova-bg)] px-2">
      <div className="flex h-8 min-w-36 items-center gap-2 rounded-t-md border border-b-0 border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 font-medium text-[var(--nova-text)]">
        <Rows3 className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />
        <span className="min-w-0 flex-1 truncate">{t('changes.review')}</span>
      </div>
      <Button type="button" size="icon-xs" variant="ghost" onClick={onClose} className="mb-0.5 ml-auto" aria-label={t('common.close')}><X /></Button>
    </div>
  )
}
