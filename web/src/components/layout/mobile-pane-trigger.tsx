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
}

/** Consistent mobile entry point for panes collapsed by AdaptiveSurface. */
export function MobilePaneTrigger({ side, label, onClick, children, className }: MobilePaneTriggerProps) {
  const Icon = side === 'left' ? PanelLeft : PanelRight
  return (
    <Button
      type="button"
      variant="outline"
      size={children ? 'sm' : 'icon'}
      className={cn('nova-icon-button text-muted-foreground', className)}
      aria-label={label}
      title={children ? undefined : label}
      onClick={onClick}
    >
      <Icon data-icon="inline-start" />
      {children}
    </Button>
  )
}
