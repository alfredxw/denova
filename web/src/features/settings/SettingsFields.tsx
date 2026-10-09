import { cloneElement, isValidElement, useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { LOCALE_OPTIONS } from '@/i18n'
import { AGENT_APPROVAL_MODES } from '@/features/agent-approval/modes'
import { DEFAULT_NARRATIVE_STYLE_ID, narrativeStyleName } from '@/features/interactive/narrative-style'
import type { Teller } from '@/features/interactive/types'
import type { AgentApprovalMode, ShellEnvironmentMode } from './types'

const fieldCls = 'nova-field min-h-7 flex-1 rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'
const FIELD_INHERIT_VALUE = '__inherit__'
const TRACE_CAPTURE_OPTIONS = [
  { value: 'summary', labelKey: 'settings.debug.traceCaptureSummary' },
  { value: 'debug', labelKey: 'settings.debug.traceCaptureDebug' },
  { value: 'off', labelKey: 'settings.debug.traceCaptureOff' },
] as const
const TRACE_EXPORTER_OPTIONS = [
  { value: 'local', labelKey: 'settings.debug.traceExporterLocal' },
] as const
export function FieldRow({ label, children }: { label: string; children: ReactNode }) {
  const generatedID = useId()
  const childID = isValidElement<{ id?: string }>(children) ? children.props.id : undefined
  const controlID = childID || generatedID
  const control = isValidElement<{ id?: string }>(children)
    ? cloneElement(children, { id: controlID })
    : children
  return (
    <SettingsFieldRow
      title={label}
      htmlFor={controlID}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {control}
    </SettingsFieldRow>
  )
}

function ValueRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-1.5"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1"
    >
      {children}
    </SettingsFieldRow>
  )
}

export function TextSizeRow({ label, description, children }: { label: string; description: string; children: ReactNode }) {
  return (
    <SettingsFieldRow
      title={label}
      description={description}
      className="nova-settings-row rounded-md border-0 bg-transparent px-2 py-2"
      contentClassName="sm:w-44 sm:flex-none"
      controlClassName="flex-1 sm:max-w-md"
    >
      {children}
    </SettingsFieldRow>
  )
}

export function ReadOnly({ label, value }: { label: string; value?: string }) {
  const { t } = useTranslation()
  return (
    <ValueRow label={label}>
      <code className="min-h-7 flex-1 truncate rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5 text-[var(--nova-text-muted)]">
        {value || t('common.notSet')}
      </code>
    </ValueRow>
  )
}

export function Text({ label, value, placeholder, type = 'text', disabled, onChange }: {
  label: string; value?: string; placeholder?: string; type?: string; disabled?: boolean
  onChange: (v: string) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type={type}
        value={value ?? ''}
        placeholder={placeholder}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className={`${fieldCls} disabled:opacity-50`}
      />
    </FieldRow>
  )
}

export function Num({ label, value, placeholder, step = 1, min, max, onChange }: {
  label: string; value: number | null; placeholder?: string
  step?: number
  min?: number
  max?: number
  onChange: (v: number | null) => void
}) {
  return (
    <FieldRow label={label}>
      <input
        type="number"
        step={step}
        min={min}
        max={max}
        value={value ?? ''}
        placeholder={placeholder}
        onChange={(e) => {
          const raw = e.target.value
          onChange(raw === '' ? null : Number(raw))
        }}
        className={fieldCls}
      />
    </FieldRow>
  )
}

export function BoolTri({ label, value, inherited, onChange }: {
  label: string; value: boolean | null; inherited?: boolean | null
  onChange: (v: boolean | null) => void
}) {
  const { t } = useTranslation()
  const inheritedLabel = inherited === null || inherited === undefined ? t('common.notSet') : t(inherited ? 'settings.bool.true' : 'settings.bool.false')
  const selectValue = value === null ? FIELD_INHERIT_VALUE : String(value)
  return (
    <FieldRow label={label}>
      <Select value={selectValue} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? null : v === 'true')}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            <SelectItem value="true">{t('settings.bool.true')}</SelectItem>
            <SelectItem value="false">{t('settings.bool.false')}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function AgentApprovalModeSelect({ value, disabled, onChange }: {
  value: AgentApprovalMode
  disabled?: boolean
  onChange: (value: AgentApprovalMode) => void
}) {
  const { t } = useTranslation()
  return (
    <FieldRow label={t('settings.agent.approvalMode')}>
      <div className="grid gap-1.5">
        <Select value={value} disabled={disabled} onValueChange={(next) => onChange(next as AgentApprovalMode)}>
          <SelectTrigger size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            {AGENT_APPROVAL_MODES.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {t(`agentApproval.mode.${mode}.label`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-[11px] leading-4 text-[var(--nova-text-faint)]">{t(`agentApproval.mode.${value}.description`)}</span>
      </div>
    </FieldRow>
  )
}

export function ShellEnvironmentSelect({ value, inherited, onChange }: {
  value?: ShellEnvironmentMode
  inherited?: ShellEnvironmentMode
  onChange: (value: ShellEnvironmentMode | undefined) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'auto'
  return (
    <FieldRow label={t('settings.agent.shellEnvironmentMode')}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(next) => onChange(next === FIELD_INHERIT_VALUE ? undefined : next as ShellEnvironmentMode)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: t(`settings.agent.shellEnvironment.${inheritedValue}`) })}</SelectItem>
          <SelectItem value="auto">{t('settings.agent.shellEnvironment.auto')}</SelectItem>
          <SelectItem value="process">{t('settings.agent.shellEnvironment.process')}</SelectItem>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function TraceCaptureSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'summary'
  const inheritedLabel = t(TRACE_CAPTURE_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.debug.traceCaptureSummary')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {TRACE_CAPTURE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function TraceExporterSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === inherited) ? inherited || 'local' : 'local'
  const isValidValue = TRACE_EXPORTER_OPTIONS.some((option) => option.value === value)
  const selectValue = isValidValue ? value || FIELD_INHERIT_VALUE : FIELD_INHERIT_VALUE
  const inheritedLabel = t(TRACE_EXPORTER_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.debug.traceExporterLocal')
  return (
    <FieldRow label={label}>
      <Select value={selectValue} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {TRACE_EXPORTER_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function LanguageSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedLabel = t(LOCALE_OPTIONS.find((option) => option.value === (inherited || 'auto'))?.labelKey || 'locale.auto')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {LOCALE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

const THEME_OPTIONS = [
  { value: 'dark', labelKey: 'settings.theme.dark' },
  { value: 'light', labelKey: 'settings.theme.light' },
  { value: 'system', labelKey: 'settings.theme.system' },
] as const

const MOTION_INTENSITY_OPTIONS = [
  { value: 'system', labelKey: 'settings.motion.system' },
  { value: 'full', labelKey: 'settings.motion.full' },
  { value: 'reduced', labelKey: 'settings.motion.reduced' },
  { value: 'off', labelKey: 'settings.motion.off' },
] as const

export function ThemeSelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'dark'
  const inheritedLabel = t(THEME_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.theme.dark')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {THEME_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function MotionIntensitySelect({ label, value, inherited, onChange }: {
  label: string
  value?: string
  inherited?: string
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedValue = inherited || 'system'
  const inheritedLabel = t(MOTION_INTENSITY_OPTIONS.find((option) => option.value === inheritedValue)?.labelKey || 'settings.motion.system')
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedLabel })}</SelectItem>
            {MOTION_INTENSITY_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>{t(option.labelKey)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}

export function TellerSelect({ label, value, inherited, tellers, onChange }: {
  label: string
  value?: string
  inherited?: string
  tellers: Teller[]
  onChange: (v: string) => void
}) {
  const { t } = useTranslation()
  const inheritedTeller = tellers.find((teller) => teller.id === inherited)
  const inheritedName = inheritedTeller ? narrativeStyleName(inheritedTeller, t) : inherited || DEFAULT_NARRATIVE_STYLE_ID
  return (
    <FieldRow label={label}>
      <Select value={value || FIELD_INHERIT_VALUE} onValueChange={(v) => onChange(v === FIELD_INHERIT_VALUE ? '' : v)}>
        <SelectTrigger size="sm" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          <SelectGroup>
            <SelectItem value={FIELD_INHERIT_VALUE}>{t('common.defaultValue', { value: inheritedName })}</SelectItem>
            {tellers.map((teller) => (
              <SelectItem key={teller.id} value={teller.id}>{narrativeStyleName(teller, t)}</SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </FieldRow>
  )
}
