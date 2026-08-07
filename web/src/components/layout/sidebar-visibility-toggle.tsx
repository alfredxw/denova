import { PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface SidebarVisibilityToggleProps {
  visible: boolean
  onToggle: () => void
  className?: string
}

/** Desktop control for a collapsible left sidebar. Compact layouts use MobilePaneTrigger instead. */
export function SidebarVisibilityToggle({ visible, onToggle, className }: SidebarVisibilityToggleProps) {
  const { t } = useTranslation()
  const label = t(visible ? 'layout.sidebar.collapse' : 'layout.sidebar.expand')
  const Icon = visible ? PanelLeftClose : PanelLeftOpen

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-xs"
      className={cn('hidden shrink-0 text-[var(--nova-text-muted)] lg:inline-flex', className)}
      onClick={onToggle}
      aria-label={label}
      aria-pressed={visible}
    >
      <Icon />
    </Button>
  )
}
