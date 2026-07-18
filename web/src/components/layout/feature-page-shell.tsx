import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface FeaturePageShellProps {
  icon: LucideIcon
  title: ReactNode
  subtitle?: ReactNode
  /** Compact navigation control that must remain visible before a truncating title. */
  leadingContent?: ReactNode
  headerContent?: ReactNode
  actions?: ReactNode
  error?: string | null
  errorTitle?: string
  onClose?: () => void
  closeLabel?: string
  children: ReactNode
  className?: string
  topbarClassName?: string
}

/** Shared feature-page frame. Business navigation and page state stay in the caller. */
export function FeaturePageShell({
  icon: Icon,
  title,
  subtitle,
  leadingContent,
  headerContent,
  actions,
  error,
  errorTitle,
  onClose,
  closeLabel,
  children,
  className,
  topbarClassName,
}: FeaturePageShellProps) {
  const { t } = useTranslation()
  const resolvedCloseLabel = closeLabel ?? t('common.close')

  return (
    <div className={cn('flex h-full min-h-0 w-full flex-col text-foreground', className)}>
      <header className={cn(
        'nova-topbar flex min-h-10 shrink-0 flex-nowrap items-center gap-2 overflow-hidden border-b px-3 py-1.5 text-xs sm:px-4',
        topbarClassName,
      )}>
        {leadingContent}
        <Icon className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
        <div className="flex min-w-0 flex-1 items-baseline gap-2">
          <h2 className="min-w-0 truncate text-xs font-medium text-foreground">{title}</h2>
          {subtitle ? <span className="hidden min-w-0 truncate text-[11px] text-muted-foreground sm:inline">{subtitle}</span> : null}
        </div>
        {headerContent ? <div className="flex shrink-0 items-center gap-2">{headerContent}</div> : null}
        {(actions || onClose) && (
          <div className="flex shrink-0 items-center gap-1 sm:gap-2">
            {actions}
            {onClose && (
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                onClick={onClose}
                aria-label={resolvedCloseLabel}
                title={resolvedCloseLabel}
              >
                <X data-icon="inline-start" />
              </Button>
            )}
          </div>
        )}
      </header>
      {error ? <InlineErrorNotice className="mx-3 mt-2" message={error} title={errorTitle} /> : null}
      <div data-slot="feature-page-body" className="flex min-h-0 flex-1 flex-col">
        {children}
      </div>
    </div>
  )
}
