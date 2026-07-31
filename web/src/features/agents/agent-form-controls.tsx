import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { FormField } from '@/components/forms/form-field'
import { FormSectionHeader } from '@/components/forms/form-section-header'
import { Switch } from '@/components/ui/switch'

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
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        {children}
        {inherited !== undefined && <InheritanceBadge inherited={inherited} onReset={onReset} />}
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
    <span className="inline-flex shrink-0 items-center gap-1.5" title={`${ariaLabel}: ${label}`}>
      <Switch
        checked={checked}
        onCheckedChange={onChange}
        aria-label={ariaLabel}
        title={`${ariaLabel}: ${label}`}
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
    <span className="inline-flex shrink-0 items-center gap-1.5">
      <ToggleSwitch checked={checked} onChange={onChange} ariaLabel={ariaLabel} statusLabel={statusLabel} />
      <InheritanceText inherited={inherited} onReset={onReset} />
    </span>
  )
}

function InheritanceText({ inherited, onReset }: { inherited: boolean; onReset?: () => void }) {
  const { t } = useTranslation()
  if (inherited) {
    return <span className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-faint)]">{t('agents.badge.inherited')}</span>
  }
  return (
    <button type="button" onClick={onReset} className="w-7 text-center text-[10px] leading-none text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
      {t('agents.badge.overridden')}
    </button>
  )
}

function InheritanceBadge({ inherited, onReset }: { inherited: boolean; onReset?: () => void }) {
  const { t } = useTranslation()
  return (
    <span className={`inline-flex h-7 max-w-full shrink-0 items-center rounded-[var(--nova-radius)] border px-2 text-[11px] ${inherited ? 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]' : 'border-[var(--nova-border)] bg-[var(--nova-active)] text-[var(--nova-text-muted)]'}`}>
      {inherited ? t('agents.badge.inherited') : (
        <button type="button" onClick={onReset} className="text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]">
          {t('agents.badge.overridden')}
        </button>
      )}
    </span>
  )
}

function statusDotClass(label: string, checked: boolean, t: (key: string) => string) {
  if (label === t('agents.option.default')) return 'bg-[var(--nova-text-faint)]'
  return checked ? 'bg-[var(--nova-success)]' : 'bg-[var(--nova-danger)]'
}
