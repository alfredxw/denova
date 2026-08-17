import type { ReactNode } from 'react'
import { PanelLeft, PanelRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface MobilePaneTriggerProps {
  side: 'left' | 'right'
  label: string
  onClick: () => void
  children?: ReactNode
  className?: string
  appearance?: 'default' | 'compact'
}

/** Consistent mobile entry point for panes collapsed by AdaptiveSurface. */
export function MobilePaneTrigger({ side, label, onClick, children, className, appearance = 'default' }: MobilePaneTriggerProps) {
  const Icon = side === 'left' ? PanelLeft : PanelRight
  const compact = appearance === 'compact' && !children
  return (
    <Button
      type="button"
      variant={compact ? 'ghost' : 'outline'}
      size={children ? 'sm' : compact ? 'icon-xs' : 'icon'}
      className={cn(!compact && 'nova-icon-button', 'text-muted-foreground', className)}
      aria-label={label}
      onClick={onClick}
    >
      <Icon data-icon="inline-start" />
      {children}
    </Button>
  )
}
