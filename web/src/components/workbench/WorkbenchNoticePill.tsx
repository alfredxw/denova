import { Star, X } from 'lucide-react'
import { motion } from 'motion/react'
import { useTranslation } from 'react-i18next'
import type { WorkbenchNotice } from '@/features/notices/use-workbench-notice'
import { DENOVA_GITHUB_URL } from '@/lib/product-links'

interface WorkbenchNoticePillProps {
  notice: WorkbenchNotice
  expanded: boolean
  starSecondaryText?: 'action' | 'description'
  onOpenSettings: () => void
  onDismiss?: () => void
}

export function WorkbenchNoticePill({
  notice,
  expanded,
  starSecondaryText = 'action',
  onOpenSettings,
  onDismiss,
}: WorkbenchNoticePillProps) {
  const { t } = useTranslation()
  const isStarNotice = notice.kind === 'star'
  const dismissLabel = t(isStarNotice
    ? 'workbench.starNotice.dismiss'
    : 'workbench.updateNotice.dismiss')

  return (
    <motion.div
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 4 }}
      transition={{ duration: 0.16 }}
      className={`relative z-20 flex items-center rounded-[var(--nova-radius)] border bg-[var(--nova-surface)]/95 text-[11px] text-[var(--nova-text)] shadow-[var(--nova-shadow)] backdrop-blur ${isStarNotice ? 'border-[var(--nova-warning)]' : 'border-[var(--nova-accent)]'} ${expanded ? 'w-full' : 'w-44 -translate-x-1'}`}
    >
      {notice.kind === 'update' ? (
        <button
          type="button"
          className="min-w-0 flex-1 truncate px-2 py-1.5 text-left"
          title={t('workbench.updateNotice.available', { version: notice.latestVersion })}
          onClick={onOpenSettings}
        >
          {t('workbench.updateNotice.available', { version: notice.latestVersion })}
        </button>
      ) : (
        <a
          href={DENOVA_GITHUB_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left hover:bg-[var(--nova-hover)]"
          aria-label={t('workbench.starNotice.action')}
          title={t('workbench.starNotice.description')}
          onClick={onDismiss}
        >
          <Star className="h-3.5 w-3.5 shrink-0 fill-[var(--nova-warning)]/20 text-[var(--nova-warning)]" />
          {expanded ? (
            <span className="min-w-0">
              <span className="block truncate font-medium">{t('workbench.starNotice.title')}</span>
              <span className="mt-0.5 line-clamp-2 block text-[10px] leading-4 text-[var(--nova-text-muted)]">
                {t(starSecondaryText === 'description'
                  ? 'workbench.starNotice.description'
                  : 'workbench.starNotice.action')}
              </span>
            </span>
          ) : (
            <span className="min-w-0 truncate font-medium">{t('workbench.starNotice.action')}</span>
          )}
        </a>
      )}
      <button
        type="button"
        className="mr-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-[6px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
        aria-label={dismissLabel}
        title={dismissLabel}
        onClick={onDismiss}
      >
        <X className="h-3.5 w-3.5" />
      </button>
    </motion.div>
  )
}
