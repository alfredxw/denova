import { useTranslation } from 'react-i18next'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { LoadingState } from '@/components/common/LoadingState'
import { ContinualLearningPage } from '@/features/agents/ContinualLearningPage'
import type { LabSettings, LayeredSettings } from '@/features/settings/types'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import type { ResourceTarget } from '@/lib/api'

const GLOBAL_TARGET: ResourceTarget = { kind: 'global' }

interface HarnessWorkspaceProps {
  refreshToken: number
}

/** User-level Harness State workspace shared by Writing and Game. */
export function HarnessWorkspace({ refreshToken }: HarnessWorkspaceProps) {
  const { t } = useTranslation()
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow } = useLayeredSettingsDraft({
    target: GLOBAL_TARGET,
    layer: 'user',
    sourcePrefix: 'harness-workspace',
  })
  const inheritedScheduleEnabled = resolveInheritedLabBoolean(layered, 'continual_learning_schedule', false)
  const inheritedScheduleIntervalHours = resolveInheritedLabNumber(layered, 'continual_learning_interval_hours', 24, 1, 720)

  const setLabField = <K extends keyof LabSettings>(key: K, value: LabSettings[K]) => {
    setDraft((current) => ({
      ...current,
      labs: { ...current.labs, [key]: value },
    }))
  }

  if (!layered) {
    return <LoadingState label={t('common.loading')} className="h-full min-h-0" />
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      {error ? <div className="shrink-0 border-b border-[var(--nova-border)] bg-red-500/5 px-4 py-2 text-xs text-red-400">{error}</div> : null}
      <ContinualLearningPage
        refreshToken={refreshToken}
        scheduleSettings={{
          enabled: draft.labs?.continual_learning_schedule ?? null,
          inheritedEnabled: inheritedScheduleEnabled,
          intervalHours: draft.labs?.continual_learning_interval_hours ?? null,
          inheritedIntervalHours: inheritedScheduleIntervalHours,
          onEnabledChange: (value) => setLabField('continual_learning_schedule', value),
          onIntervalHoursChange: (value) => setLabField('continual_learning_interval_hours', value),
        }}
        headerActions={(
          <AutosaveStatusIndicator
            status={autosaveStatus}
            error={autosaveError}
            onRetry={() => saveNow().catch(() => undefined)}
          />
        )}
      />
    </div>
  )
}

function resolveInheritedLabBoolean(layered: LayeredSettings | null, key: keyof LabSettings, fallback: boolean) {
  let value = fallback
  for (const settings of [layered?.default, layered?.global]) {
    const candidate = settings?.labs?.[key]
    if (typeof candidate === 'boolean') value = candidate
  }
  return value
}

function resolveInheritedLabNumber(
  layered: LayeredSettings | null,
  key: keyof LabSettings,
  fallback: number,
  minimum: number,
  maximum: number,
) {
  let value = fallback
  for (const settings of [layered?.default, layered?.global]) {
    const candidate = settings?.labs?.[key]
    if (typeof candidate === 'number' && Number.isFinite(candidate)) {
      value = Math.min(maximum, Math.max(minimum, Math.trunc(candidate)))
    }
  }
  return value
}
