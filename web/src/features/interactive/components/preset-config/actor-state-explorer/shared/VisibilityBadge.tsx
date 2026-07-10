import { Badge } from '@/components/ui/badge'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

const VISIBILITY_STYLES: Record<string, string> = {
  visible: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
  hidden: 'border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] text-[var(--nova-danger)]',
  spoiler: 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]',
}

export function VisibilityBadge({ visibility }: { visibility: string }) {
  const { t } = useTranslation()
  const style = VISIBILITY_STYLES[visibility] || 'border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text-faint)]'
  const label = visibility === 'visible' || visibility === 'hidden' || visibility === 'spoiler'
    ? t(`settingPanel.actorState.explorer.${visibility}`)
    : visibility

  return (
    <Badge
      variant="outline"
      className={cn(
        'h-5 rounded-full px-1.5 text-[10px] font-medium',
        style,
      )}
    >
      {label}
    </Badge>
  )
}
