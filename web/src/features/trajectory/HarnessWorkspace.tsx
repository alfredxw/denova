import { useCallback, useMemo, useState } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/common/LoadingState'
import { ContinualLearningPage } from '@/features/agents/ContinualLearningPage'
import { HarnessOptimizerChat } from '@/features/agents/HarnessOptimizerChat'
import type { LabSettings, LayeredSettings } from '@/features/settings/types'
import { useLayeredSettingsDraft } from '@/features/settings/use-layered-settings-draft'
import type { GlobalAgentRunTraceSummary, ResourceTarget } from '@/lib/api'
import { HarnessRunPicker } from './HarnessRunPicker'

const GLOBAL_TARGET: ResourceTarget = { kind: 'global' }

interface HarnessWorkspaceProps {
  runs: GlobalAgentRunTraceSummary[]
  runsLoading: boolean
  selectedEvidence: ReadonlySet<string>
  onToggleEvidence: (trajectoryURI: string) => void
  onClearEvidence: () => void
  onViewRun: (trajectoryURI: string) => void
}

/** User-level Harness State and Optimizer workspace shared by Writing and Game. */
export function HarnessWorkspace({
  runs,
  runsLoading,
  selectedEvidence,
  onToggleEvidence,
  onClearEvidence,
  onViewRun,
}: HarnessWorkspaceProps) {
  const { t } = useTranslation()
  const [optimizerOpen, setOptimizerOpen] = useState(true)
  const [stateRefreshToken, setStateRefreshToken] = useState(0)
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow } = useLayeredSettingsDraft({
    target: GLOBAL_TARGET,
    layer: 'user',
    sourcePrefix: 'harness-workspace',
  })
  const inheritedScheduleEnabled = resolveInheritedLabBoolean(layered, 'continual_learning_schedule', false)
  const inheritedScheduleIntervalHours = resolveInheritedLabNumber(layered, 'continual_learning_interval_hours', 24, 1, 720)
  const evidence = useMemo(() => {
    const selected = runs
      .filter((run) => selectedEvidence.has(run.trajectory_uri))
      .map((run) => run.trajectory_uri)
    return selected.length > 0 ? selected : undefined
  }, [runs, selectedEvidence])

  const setLabField = <K extends keyof LabSettings>(key: K, value: LabSettings[K]) => {
    setDraft((current) => ({
      ...current,
      labs: { ...current.labs, [key]: value },
    }))
  }

  const handleOptimizerSettled = useCallback(() => {
    setStateRefreshToken((value) => value + 1)
  }, [])

  if (!layered) {
    return <LoadingState label={t('common.loading')} className="h-full min-h-0" />
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--nova-bg)]">
      {error ? <div className="shrink-0 border-b border-[var(--nova-border)] bg-red-500/5 px-4 py-2 text-xs text-red-400">{error}</div> : null}
      <AdaptiveSurface
        className="min-h-0 flex-1"
        mainClassName="min-h-0 min-w-0"
        right={{
          id: 'harness-optimizer',
          title: t('continualLearning.optimizer.title'),
          side: 'right',
          icon: <Sparkles className="size-4" />,
          enabled: true,
          desktopVisible: optimizerOpen,
          desktopClassName: 'min-h-0 border-l border-[var(--nova-border)] bg-[var(--nova-surface)]',
          mobileClassName: 'w-[min(94vw,480px)] bg-[var(--nova-surface)]',
          content: (
            <HarnessOptimizerChat
              evidence={evidence}
              evidenceControl={(
                <HarnessRunPicker
                  runs={runs}
                  selected={selectedEvidence}
                  loading={runsLoading}
                  onToggle={onToggleEvidence}
                  onClear={onClearEvidence}
                  onView={onViewRun}
                />
              )}
              onSettled={handleOptimizerSettled}
            />
          ),
        }}
        rightResize={{
          layoutKey: 'nova-harness-optimizer-layout',
          label: t('layout.resize.right'),
          defaultSize: '440px',
          minSize: '320px',
          maxSize: '65%',
          mainMinSize: '300px',
        }}
      >
        {({ isMobile, openRight }) => (
          <ContinualLearningPage
            refreshToken={stateRefreshToken}
            scheduleSettings={{
              enabled: draft.labs?.continual_learning_schedule ?? null,
              inheritedEnabled: inheritedScheduleEnabled,
              intervalHours: draft.labs?.continual_learning_interval_hours ?? null,
              inheritedIntervalHours: inheritedScheduleIntervalHours,
              onEnabledChange: (value) => setLabField('continual_learning_schedule', value),
              onIntervalHoursChange: (value) => setLabField('continual_learning_interval_hours', value),
            }}
            headerActions={(
              <>
                <AutosaveStatusIndicator
                  status={autosaveStatus}
                  error={autosaveError}
                  onRetry={() => saveNow().catch(() => undefined)}
                />
                <Button
                  type="button"
                  size="sm"
                  variant={optimizerOpen ? 'secondary' : 'outline'}
                  aria-pressed={optimizerOpen}
                  onClick={() => {
                    if (isMobile) {
                      setOptimizerOpen(true)
                      openRight()
                      return
                    }
                    setOptimizerOpen((value) => !value)
                  }}
                >
                  <Sparkles />{t('continualLearning.openOptimizer')}
                </Button>
              </>
            )}
          />
        )}
      </AdaptiveSurface>
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
