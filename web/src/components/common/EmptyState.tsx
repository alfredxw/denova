import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { Button } from '@/components/ui/button'
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty'
import { cn } from '@/lib/utils'

const emptyStateVariants = cva('border-0', {
  variants: {
    variant: {
      page: 'h-full min-h-64',
      panel: 'min-h-40',
      compact: 'min-h-0 flex-none gap-2 p-3',
      dashed: 'min-h-40 border border-dashed border-border',
    },
  },
  defaultVariants: { variant: 'panel' },
})

interface EmptyStateProps extends VariantProps<typeof emptyStateVariants> {
  icon?: LucideIcon
  title: string
  description?: string
  action?: { label: string; onClick: () => void }
  content?: ReactNode
  className?: string
}

/** 统一空态：全空引导、未选中、无数据等场景的占位展示。 */
export function EmptyState({ icon: Icon, title, description, action, content, variant, className }: EmptyStateProps) {
  return (
    <Empty className={cn(emptyStateVariants({ variant }), className)}>
      <EmptyHeader>
        {Icon ? (
          <EmptyMedia variant="icon">
            <Icon />
          </EmptyMedia>
        ) : null}
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {(action || content) && (
        <EmptyContent>
          {content}
          {action ? (
            <Button variant="outline" size="sm" onClick={action.onClick}>
              {action.label}
            </Button>
          ) : null}
        </EmptyContent>
      )}
    </Empty>
  )
}
