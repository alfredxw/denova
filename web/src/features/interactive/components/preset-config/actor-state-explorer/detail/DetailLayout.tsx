import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function DetailContentFrame({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('mx-auto w-full max-w-[1120px]', className)}>
      {children}
    </div>
  )
}

export function DetailStack({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('space-y-5', className)}>
      {children}
    </div>
  )
}

export function DetailResponsiveGrid({
  children,
  className,
  minWidth = 320,
}: {
  children: ReactNode
  className?: string
  minWidth?: number
}) {
  return (
    <div
      className={cn('grid gap-5', className)}
      style={{ gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, ${minWidth}px), 1fr))` }}
    >
      {children}
    </div>
  )
}
