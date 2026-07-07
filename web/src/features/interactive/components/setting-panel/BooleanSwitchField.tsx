import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'

export function BooleanSwitchField({
  label,
  checked,
  onCheckedChange,
  disabled,
  className = '',
}: {
  label: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const actionLabel = checked ? t('settingPanel.switch.disableStatus') : t('settingPanel.switch.enableStatus')
  return (
    <label className={`grid min-w-0 gap-1.5 ${className}`}>
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      <span className="flex h-8 w-full items-center justify-between gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-xs text-[var(--nova-text-muted)]">
        <span className="truncate">{checked ? t('settingPanel.enabled') : t('settingPanel.disabled')}</span>
        <Switch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} aria-label={actionLabel} title={actionLabel} />
      </span>
    </label>
  )
}
