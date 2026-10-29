import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/forms/form-field'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'

export function SectionTitle({ icon, title }: { icon: LucideIcon; title: string }) {
  return <FormSectionHeader icon={icon} title={title} />
}

export function Field({ label, inherited, onReset, children }: {
  label: string
  inherited?: boolean
  onReset?: () => void
  children: ReactNode
}) {
  return (
    <FormField label={label}>
      <div className="flex min-w-0 flex-col gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">{children}</div>
        {inherited !== undefined && <InheritanceStatus label={label} inherited={inherited} onReset={onReset} />}
      </div>
    </FormField>
  )
}

export function ToggleSwitch({ checked, onChange, ariaLabel, statusLabel }: {
  checked: boolean
  onChange: (checked: boolean) => void
  ariaLabel: string
  statusLabel?: string
}) {
  const { t } = useTranslation()
  const label = statusLabel || (checked ? t('agents.option.on') : t('agents.option.off'))
  const dotClass = statusDotClass(label, checked, t)
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5">
      <Switch
        checked={checked}
        onCheckedChange={onChange}
        aria-label={ariaLabel}
      />
      <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dotClass}`} />
    </span>
  )
}

export function SwitchWithInheritance({ checked, onChange, ariaLabel, statusLabel, inherited, onReset }: {
  checked: boolean
  onChange: (checked: boolean) => void
  ariaLabel: string
  statusLabel?: string
  inherited: boolean
  onReset?: () => void
}) {
  return (
    <span className="inline-flex max-w-full flex-wrap items-center gap-2">
      <ToggleSwitch checked={checked} onChange={onChange} ariaLabel={ariaLabel} statusLabel={statusLabel} />
      <InheritanceStatus label={ariaLabel} inherited={inherited} onReset={onReset} />
    </span>
  )
}

/** Inheritance is state; resetting it is a separately named, field-specific action. */
function InheritanceStatus({ label, inherited, onReset }: { label: string; inherited: boolean; onReset?: () => void }) {
  const { t } = useTranslation()
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <span>{t(inherited ? 'agents.badge.inherited' : 'agents.badge.customized')}</span>
      {!inherited && onReset && <Button type="button" variant="ghost" size="xs" onClick={onReset} aria-label={t('agents.action.restoreField', { field: label })}>
        {t('agents.action.restoreInheritance')}
      </Button>}
    </span>
  )
}

function statusDotClass(label: string, checked: boolean, t: (key: string) => string) {
  if (label === t('agents.option.default')) return 'bg-[var(--nova-text-faint)]'
  return checked ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-danger)]'
}
