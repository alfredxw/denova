import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { isSaveShortcut } from '@/lib/keyboard'
import { cn } from '@/lib/utils'

interface FeaturePageShellProps {
  icon: LucideIcon
  title: ReactNode
  subtitle?: ReactNode
  /** Compact navigation control that must remain visible before a truncating title. */
  leadingContent?: ReactNode
  headerContent?: ReactNode
  actions?: ReactNode
  /** Flushes autosave on Cmd/Ctrl+S without exposing a redundant save button. */
  onSaveShortcut?: () => void | Promise<unknown>
  error?: string | null
  errorTitle?: string
  children: ReactNode
  className?: string
  topbarClassName?: string
  mobileHeader?: 'default' | 'toolbar' | 'hidden'
}

/** Primary-page frame; the workbench owns navigation between destinations. */
export function FeaturePageShell({
  icon: Icon,
  title,
  subtitle,
  leadingContent,
  headerContent,
  actions,
  onSaveShortcut,
  error,
  errorTitle,
  children,
  className,
  topbarClassName,
  mobileHeader = 'default',
}: FeaturePageShellProps) {
  return (
    <div
      className={cn('flex h-full min-h-0 w-full flex-col text-foreground', className)}
      onKeyDownCapture={onSaveShortcut ? (event) => {
        if (!isSaveShortcut(event)) return
        event.preventDefault()
        event.stopPropagation()
        void Promise.resolve(onSaveShortcut()).catch(() => undefined)
      } : undefined}
    >
      <header data-slot="feature-page-header" className={cn(
        'nova-topbar flex min-h-10 shrink-0 flex-nowrap items-center gap-2 overflow-hidden border-b px-3 py-1.5 text-xs sm:px-4',
        topbarClassName,
        mobileHeader === 'hidden' && 'max-lg:hidden',
      )}>
        <div className={cn('contents', mobileHeader === 'toolbar' && 'max-lg:hidden')}>{leadingContent}</div>
        <Icon className={cn('size-3.5 shrink-0 text-muted-foreground', mobileHeader === 'toolbar' && 'max-lg:hidden')} aria-hidden="true" />
        <div className={cn('flex min-w-0 flex-1 items-baseline gap-2', mobileHeader === 'toolbar' && 'max-lg:hidden')}>
          <h2 className="min-w-0 truncate text-xs font-medium text-foreground">{title}</h2>
          {subtitle ? <span className="hidden min-w-0 truncate text-[11px] text-muted-foreground sm:inline">{subtitle}</span> : null}
        </div>
        {headerContent ? <div className="flex shrink-0 items-center gap-2">{headerContent}</div> : null}
        {actions && (
          <div data-slot="feature-page-actions" className="flex shrink-0 items-center gap-1 sm:gap-2">
            {actions}
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
