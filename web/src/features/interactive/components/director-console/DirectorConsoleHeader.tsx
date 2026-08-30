import { Clapperboard, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Switch } from '@/components/ui/switch'
import type { StoryDirector, StoryPlanningMode, StorySummary } from '../../types'
import { ReplyTargetCharsInlineEditor } from '../ReplyTargetCharsInlineEditor'
import { StoryDirectorPicker } from '../StoryDirectorPicker'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from '../story-state/display-preference'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'

// 控制台 header：标题行内含导演台入口（右侧状态+打开按钮），
// 信息条默认展示当前导演、每轮目标字数（行内编辑）与主舞台状态展示。
export function DirectorConsoleHeader({ branchId, turnCount, story, storyDirectors, onDirectorChange, onReplyTargetCharsChange, onPlanningModeChange, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange }: { branchId: string; turnCount: number; story?: StorySummary; storyDirectors: StoryDirector[]; onDirectorChange?: (directorId: string) => void; onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>; onPlanningModeChange?: (mode: StoryPlanningMode) => void | Promise<void>; stateDisplayPreference?: StoryStateDisplayPreference; onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void }) {
  const { t } = useTranslation()
  const [planningModeUpdating, setPlanningModeUpdating] = useState(false)
  const planningEnabled = story?.planning_mode === 'enabled'
  const updatePlanningMode = async (enabled: boolean) => {
    if (!story || !onPlanningModeChange || planningModeUpdating) return
    setPlanningModeUpdating(true)
    try {
      await onPlanningModeChange(enabled ? 'enabled' : 'disabled')
    } catch (error) {
      console.error('[game-console] Failed to update planning mode', { storyId: story.id, error })
      toast.error(t('directorPanel.planning.updateFailed'))
    } finally {
      setPlanningModeUpdating(false)
    }
  }
  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--director-canvas)_92%,transparent)] px-4 pb-3 pt-4 backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-3">
        <div data-testid="director-panel-icon" className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-label={t('directorPanel.consoleTitle')}>
          <Clapperboard className="h-4.5 w-4.5" />
          <span className="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full border-2 border-[var(--director-canvas)] bg-[var(--director-live)]" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-[9px] font-semibold uppercase tracking-[0.2em] text-[var(--nova-text-faint)]">{t('directorPanel.consoleEyebrow')}</p>
          <h2 className="director-console__display min-w-0 truncate text-base font-semibold leading-6 text-[var(--nova-text)]">{t('directorPanel.consoleTitle')}</h2>
          <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[9px] text-[var(--nova-text-faint)]">
            <span className="truncate">{t('directorPanel.branch', { branch: branchId || 'main' })}</span>
            <span aria-hidden="true">/</span>
            <span className="shrink-0">{t('directorPanel.turnCount', { count: turnCount })}</span>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2 rounded-full border border-[var(--nova-border)] bg-[var(--director-panel)] px-2.5 py-1.5">
          <span className="text-[10px] font-medium text-[var(--nova-text-muted)]">{t(planningEnabled ? 'directorPanel.planning.enabled' : 'directorPanel.planning.disabled')}</span>
          {planningModeUpdating ? <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--director-brass)]" /> : <Switch checked={planningEnabled} disabled={!story || !onPlanningModeChange} onCheckedChange={(enabled) => void updatePlanningMode(enabled)} aria-label={t('directorPanel.planning.toggle')} className="scale-90" />}
        </div>
      </div>
      <div className="mt-3 grid min-w-0 grid-cols-[minmax(0,1fr)_auto_auto] gap-x-3 border-t border-[var(--nova-border-soft)] pt-3">
        <StoryDirectorPicker story={story} storyDirectors={storyDirectors} onChange={onDirectorChange || (() => undefined)} layout="sidebar" />
        <div className="flex min-w-0 flex-col gap-1.5">
          <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.replyTargetChars')}</span>
          <div className="flex h-7 items-center">
            <ReplyTargetCharsInlineEditor story={story} onChange={onReplyTargetCharsChange} />
          </div>
        </div>
        <div className="flex shrink-0 flex-col gap-1.5">
          {/* 与其他两列的小标同高的占位，保证三个控件底边对齐 */}
          <span aria-hidden="true" className="invisible text-[11px] font-medium">.</span>
          <div className="flex h-7 items-center">
            <StateDisplayPreferenceMenu value={stateDisplayPreference} onChange={onStateDisplayPreferenceChange ?? (() => undefined)} compact />
          </div>
        </div>
      </div>
    </header>
  )
}
