import { Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface ConfigManagerToggleProps {
  open: boolean
  label: string
  onToggle: () => void
  className?: string
}

/** Canonical page-level toggle for the shared Configuration Manager panel. */
export function ConfigManagerToggle({ open, label, onToggle, className }: ConfigManagerToggleProps) {
  return (
    <Button
      type="button"
      variant={open ? 'secondary' : 'outline'}
      size="sm"
      aria-label={label}
      aria-pressed={open}
      title={label}
      onClick={onToggle}
      className={cn('nova-nav-item', className)}
    >
      <Bot data-icon="inline-start" />
      <span className="hidden sm:inline">{label}</span>
    </Button>
  )
}
