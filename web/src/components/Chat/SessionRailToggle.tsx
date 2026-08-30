import { PanelRightClose, PanelRightOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface SessionRailToggleProps {
  visible: boolean
  onVisibleChange: (visible: boolean) => void
}

/** Keeps the session rail action consistent across its closed and open header positions. */
export function SessionRailToggle({
  visible,
  onVisibleChange,
}: SessionRailToggleProps) {
  const { t } = useTranslation()
  const label = t(visible ? 'chat.sessionRail.hide' : 'chat.sessionRail.show')
  const Icon = visible ? PanelRightClose : PanelRightOpen

  return (
    <Button
      type="button"
      variant={visible ? 'secondary' : 'ghost'}
      size="icon-sm"
      onClick={() => onVisibleChange(!visible)}
      aria-label={label}
      aria-pressed={visible}
      title={label}
    >
      <Icon data-icon="inline-start" aria-hidden="true" />
    </Button>
  )
}
