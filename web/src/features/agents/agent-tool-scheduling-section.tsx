import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingsFieldRow } from '@/components/forms/settings-field-row'
import { Input } from '@/components/ui/input'

type Props = {
  toolValue: number | null
  inheritedToolValue: number
  onToolChange: (value: number | null) => void
  subAgentValue: number | null
  inheritedSubAgentValue: number
  onSubAgentChange: (value: number | null) => void
}

export function AgentToolSchedulingSection({
  toolValue,
  inheritedToolValue,
  onToolChange,
  subAgentValue,
  inheritedSubAgentValue,
  onSubAgentChange,
}: Props) {
  const { t } = useTranslation()
  const toolInputID = useId()
  const subAgentInputID = useId()

  return (
    <section className="border-b border-[var(--nova-border)] pb-5">
      <h2 className="mb-3 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--nova-text-muted)]">
        {t('agents.section.toolScheduling')}
      </h2>
      <SettingsFieldRow
        htmlFor={toolInputID}
        title={t('settings.agent.toolParallelism')}
        description={t('agents.tool.parallelismNote')}
        meta={toolValue === null ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('common.inherit', { value: inheritedToolValue })}</span> : undefined}
        controlClassName="sm:w-36"
      >
        <Input
          id={toolInputID}
          type="number"
          min={1}
          max={64}
          value={toolValue ?? ''}
          placeholder={String(inheritedToolValue)}
          aria-label={t('settings.agent.toolParallelism')}
          onChange={(event) => {
            const value = parseParallelism(event.target.value, 64)
            if (value !== undefined) onToolChange(value)
          }}
        />
      </SettingsFieldRow>
      <SettingsFieldRow
        htmlFor={subAgentInputID}
        title={t('settings.agent.subAgentParallelism')}
        description={t('agents.tool.subAgentParallelismNote')}
        meta={subAgentValue === null ? <span className="text-[10px] text-[var(--nova-text-faint)]">{t('common.inherit', { value: inheritedSubAgentValue })}</span> : undefined}
        controlClassName="sm:w-36"
      >
        <Input
          id={subAgentInputID}
          type="number"
          min={1}
          max={32}
          value={subAgentValue ?? ''}
          placeholder={String(inheritedSubAgentValue)}
          aria-label={t('settings.agent.subAgentParallelism')}
          onChange={(event) => {
            const value = parseParallelism(event.target.value, 32)
            if (value !== undefined) onSubAgentChange(value)
          }}
        />
      </SettingsFieldRow>
    </section>
  )
}

function parseParallelism(value: string, maximum: number): number | null | undefined {
  if (value === '') return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? Math.min(maximum, Math.max(1, Math.trunc(parsed))) : undefined
}
