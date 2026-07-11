import { CheckCircle2, MapPin, Target } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { DirectorPlanStatus, TurnEvent } from '../types'

export function PlayerStoryHUD({ turn, directorStatus }: { turn?: TurnEvent; directorStatus?: DirectorPlanStatus }) {
  const { t } = useTranslation()
  const result = turn?.turn_result
  if (!result) return null

  const scene = result.scene_result?.next_scene_id || result.scene_result?.scene_id || result.plan_signals?.scene_transition?.to || ''
  const goal = result.scene_result?.next_scene_goal || result.contract.scene_goal || ''
  const outcome = result.scene_result?.summary || ''
  const memoryStatus = turn?.memory_status || 'pending'
  const planMode = directorStatus?.decision?.mode
  const items = [
    scene ? { icon: MapPin, label: t('storyStage.hud.scene'), value: scene } : null,
    goal ? { icon: Target, label: t('storyStage.hud.goal'), value: goal } : null,
    outcome ? { icon: CheckCircle2, label: t('storyStage.hud.outcome'), value: outcome } : null,
  ].filter(Boolean) as Array<{ icon: typeof MapPin; label: string; value: string }>

  if (items.length === 0) return null
  return (
    <aside aria-label={t('storyStage.hud.label')} className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)]/72 px-3 py-2 backdrop-blur-xl sm:px-4">
      <div className="mx-auto flex max-w-5xl items-center gap-2 overflow-x-auto overscroll-contain">
        {items.map(({ icon: Icon, label, value }) => (
          <div key={label} className="flex min-w-[150px] flex-1 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 py-1.5">
            <Icon className="h-3.5 w-3.5 shrink-0 text-[var(--nova-accent-blue)]" />
            <div className="min-w-0">
              <div className="text-[9px] font-medium uppercase tracking-[0.12em] text-[var(--nova-text-faint)]">{label}</div>
              <div className="truncate text-[11px] text-[var(--nova-text)]" title={value}>{value}</div>
            </div>
          </div>
        ))}
        <div className="flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[10px] text-[var(--nova-text-muted)]" title={t('storyStage.hud.maintenanceHint')}>
          <span className="h-1.5 w-1.5 rounded-full bg-[var(--nova-accent-green)]" />
          {t('storyStage.hud.stateCommitted')}
          <span className="text-[var(--nova-text-faint)]">·</span>
          {t(`storyStage.hud.memory.${memoryStatus}`, { defaultValue: memoryStatus })}
          {planMode ? <><span className="text-[var(--nova-text-faint)]">·</span>{t(`storyStage.hud.plan.${planMode}`, { defaultValue: planMode })}</> : null}
        </div>
      </div>
    </aside>
  )
}
