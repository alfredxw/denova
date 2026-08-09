import type { HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

type LoadingStateVariant = 'page' | 'panel' | 'inline'

interface LoadingStateProps extends HTMLAttributes<HTMLDivElement> {
  label: string
  variant?: LoadingStateVariant
}

/** A stable, shared loading surface for route and data initialization boundaries. */
export function LoadingState({
  label,
  variant = 'page',
  className,
  ...props
}: LoadingStateProps) {
  return (
    <div
      data-slot="loading-state"
      data-variant={variant}
      role="status"
      aria-label={label}
      aria-live="polite"
      className={cn(
        'text-[var(--nova-text-faint)]',
        variant === 'page' && 'flex h-full min-h-48 flex-col items-center justify-center gap-3 px-6 py-10',
        variant === 'panel' && 'flex min-h-24 flex-col items-center justify-center gap-2.5 px-4 py-6',
        variant === 'inline' && 'inline-flex items-center gap-2',
        className,
      )}
      {...props}
    >
      <span className="nova-loading-mark" aria-hidden="true">
        <span className="nova-loading-mark__orbit" />
        <span className="nova-loading-mark__core" />
      </span>
      <span className={cn(
        'leading-none',
        variant === 'page' ? 'text-xs tracking-[0.04em]' : 'text-[11px]',
      )}>
        {label}
      </span>
    </div>
  )
}
