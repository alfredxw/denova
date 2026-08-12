import { Clock3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { formatDateTime } from '@/i18n'
import type { ContinualLearningScheduleStatus } from '@/lib/api'
import { SwitchWithInheritance } from './agent-form-controls'

export interface HarnessOptimizationScheduleSettings {
  enabled: boolean | null
  inheritedEnabled: boolean
  intervalHours: number | null
  inheritedIntervalHours: number
  onEnabledChange: (enabled: boolean | null) => void
  onIntervalHoursChange: (hours: number | null) => void
}

export function HarnessOptimizationSchedule({
  status,
  settings,
}: {
  status: ContinualLearningScheduleStatus | null
  settings: HarnessOptimizationScheduleSettings
}) {
  const { t } = useTranslation()
  const enabled = settings.enabled ?? settings.inheritedEnabled
  return (
    <section
      className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 border-y border-[var(--nova-border)] py-2"
      aria-labelledby="harness-optimization-schedule-title"
    >
      <div className="flex min-w-48 flex-1 items-center gap-2">
        <Clock3 className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <div className="min-w-0">
          <h2 id="harness-optimization-schedule-title" className="text-[11px] font-medium text-[var(--nova-text)]">
            {t('settings.labs.continualLearningSchedule')}
          </h2>
          <p className="truncate text-[10px] text-[var(--nova-text-faint)]">
            {status?.last_success
              ? t('continualLearning.schedule.lastSuccess', { time: formatDateTime(status.last_success) })
              : t('continualLearning.schedule.neverRun')}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-[10px] text-[var(--nova-text-muted)]">{t('continualLearning.schedule.switch')}</span>
        <SwitchWithInheritance
          checked={enabled}
          inherited={settings.enabled === null}
          onChange={settings.onEnabledChange}
          onReset={() => settings.onEnabledChange(null)}
          ariaLabel={t('continualLearning.schedule.switch')}
        />
      </div>
      <label className={`flex items-center gap-1.5 text-[10px] ${enabled ? 'text-[var(--nova-text-muted)]' : 'text-[var(--nova-text-faint)]'}`}>
        <span>{t('continualLearning.schedule.every')}</span>
        <Input
          type="number"
          min={1}
          max={720}
          value={settings.intervalHours ?? ''}
          placeholder={String(settings.inheritedIntervalHours)}
          disabled={!enabled}
          aria-label={t('settings.labs.continualLearningIntervalHours')}
          className="h-7 w-16 px-2 text-center text-[11px]"
          onChange={(event) => {
            if (event.target.value === '') {
              settings.onIntervalHoursChange(null)
              return
            }
            const parsed = Number(event.target.value)
            if (Number.isFinite(parsed)) {
              settings.onIntervalHoursChange(Math.min(720, Math.max(1, Math.trunc(parsed))))
            }
          }}
        />
        <span>{t('continualLearning.schedule.hours')}</span>
      </label>
    </section>
  )
}
