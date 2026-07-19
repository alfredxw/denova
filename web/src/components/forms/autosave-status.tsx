import { AlertCircle, AlertTriangle, Check, Clock3, Loader2, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export type AutosaveStatus = 'saved' | 'pending' | 'saving' | 'blocked' | 'error'

interface AutosaveStatusIndicatorProps {
  status: AutosaveStatus
  error?: string | null
  onRetry?: () => void | Promise<unknown>
  className?: string
}

const STATUS_STYLES: Record<AutosaveStatus, string> = {
  saved: 'text-[var(--nova-text-faint)]',
  pending: 'text-[var(--nova-text-muted)]',
  saving: 'text-[var(--nova-text-muted)]',
  blocked: 'text-[var(--nova-warning)]',
  error: 'text-[var(--nova-danger)]',
}

/** Shared, non-blocking feedback for configuration pages that persist changes automatically. */
export function AutosaveStatusIndicator({ status, error, onRetry, className }: AutosaveStatusIndicatorProps) {
  const { t } = useTranslation()
  const label = status === 'error' && error
    ? t('common.autosave.errorDetail', { error })
    : t(`common.autosave.${status}`)

  return (
    <div
      role="status"
      aria-label={label}
      aria-live={status === 'error' ? 'assertive' : 'polite'}
      className={cn('flex min-w-0 items-center gap-1.5 text-[11px]', STATUS_STYLES[status], className)}
    >
      {status === 'saved' && <Check className="size-3.5 shrink-0" aria-hidden="true" />}
      {status === 'pending' && <Clock3 className="size-3.5 shrink-0" aria-hidden="true" />}
      {status === 'saving' && <Loader2 className="size-3.5 shrink-0 animate-spin" aria-hidden="true" />}
      {status === 'blocked' && <AlertTriangle className="size-3.5 shrink-0" aria-hidden="true" />}
      {status === 'error' && <AlertCircle className="size-3.5 shrink-0" aria-hidden="true" />}
      <span className="hidden max-w-56 truncate sm:inline" title={label}>{label}</span>
      {status === 'error' && onRetry ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          onClick={() => void Promise.resolve(onRetry()).catch(() => undefined)}
          aria-label={t('common.autosave.retry')}
          title={t('common.autosave.retry')}
          className="h-6 gap-1 px-1.5 text-current hover:text-current"
        >
          <RotateCcw className="size-3" aria-hidden="true" />
          <span className="hidden lg:inline">{t('common.retry')}</span>
        </Button>
      ) : null}
    </div>
  )
}
