import type { HTMLAttributes } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

type LoadingStateVariant = 'page' | 'panel' | 'inline'
type LoadingStateLayout = 'content' | 'list' | 'conversation' | 'inline'

interface LoadingStateProps extends HTMLAttributes<HTMLDivElement> {
  label: string
  variant?: LoadingStateVariant
  /** Match the placeholder geometry to the content that will replace it. */
  layout?: LoadingStateLayout
}

/** A stable, shared loading surface for route and data initialization boundaries. */
export function LoadingState({
  label,
  variant = 'page',
  layout,
  className,
  ...props
}: LoadingStateProps) {
  const resolvedLayout = layout ?? (
    variant === 'page' ? 'content' : variant === 'panel' ? 'list' : 'inline'
  )

  return (
    <div
      data-slot="loading-state"
      data-variant={variant}
      data-layout={resolvedLayout}
      role="status"
      aria-label={label}
      aria-live="polite"
      className={cn(
        'min-w-0 overflow-hidden',
        variant === 'page' && 'flex h-full min-h-48 flex-col px-6 py-8',
        variant === 'panel' && 'flex min-h-24 flex-col px-3 py-3',
        variant === 'inline' && 'inline-flex min-w-16 items-center',
        className,
      )}
      {...props}
    >
      <div aria-hidden="true" className={cn('w-full', resolvedLayout === 'conversation' && 'mt-auto')}>
        {resolvedLayout === 'content' ? (
          <div data-slot="loading-state-content" className="mx-auto flex w-full max-w-3xl flex-col gap-7 pt-[clamp(1rem,8vh,5rem)]">
            <div className="flex flex-col gap-2.5">
              <Skeleton className="h-5 w-[min(42%,18rem)]" />
              <Skeleton className="h-3 w-[min(26%,11rem)]" />
            </div>
            <div className="flex flex-col gap-3">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-[92%]" />
              <Skeleton className="h-3 w-[78%]" />
              <Skeleton className="h-3 w-[86%]" />
            </div>
          </div>
        ) : resolvedLayout === 'list' ? (
          <div data-slot="loading-state-list" className="flex w-full flex-col gap-2">
            {[76, 58, 68, 48].map((width) => (
              <div key={width} className="flex h-10 items-center gap-3 px-1">
                <Skeleton className="size-7 shrink-0" />
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-2.5" style={{ width: `${width}%` }} />
                  <Skeleton className="h-2 w-[38%]" />
                </div>
              </div>
            ))}
          </div>
        ) : resolvedLayout === 'conversation' ? (
          <div data-slot="loading-state-conversation" className="mx-auto flex w-full max-w-3xl flex-col gap-7 pb-[clamp(1.5rem,10vh,6rem)]">
            <div className="flex w-[72%] flex-col gap-2.5">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-[88%]" />
              <Skeleton className="h-3 w-[64%]" />
            </div>
            <div className="flex w-[48%] flex-col gap-2.5 self-end">
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-3 w-[72%] self-end" />
            </div>
          </div>
        ) : (
          <Skeleton data-slot="loading-state-inline" className="h-2 w-16 rounded-full" />
        )}
      </div>
      <span className="sr-only">{label}</span>
    </div>
  )
}
