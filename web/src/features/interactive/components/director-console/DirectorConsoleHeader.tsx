import { ArrowUpRight, Clapperboard, Drama, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { StoryDirector, StorySummary } from '../../types'
import { ReplyTargetCharsInlineEditor } from '../ReplyTargetCharsInlineEditor'
import { StoryDirectorPicker } from '../StoryDirectorPicker'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from '../story-state/display-preference'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'
import type { DirectorStatusLike } from './types'
import { directorStatusLabel } from './utils'

// 控制台 header：标题行内含导演台入口（右侧状态+打开按钮），
// 信息条默认展示当前导演、每轮目标字数（行内编辑）与主舞台状态展示。
export function DirectorConsoleHeader({ branchId, turnCount, story, storyDirectors, onDirectorChange, onReplyTargetCharsChange, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange, directorStatus, onOpenBackstage }: { branchId: string; turnCount: number; story?: StorySummary; storyDirectors: StoryDirector[]; onDirectorChange?: (directorId: string) => void; onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>; stateDisplayPreference?: StoryStateDisplayPreference; onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void; directorStatus?: DirectorStatusLike; onOpenBackstage: () => void }) {
  const { t } = useTranslation()
  return (
    <header className="shrink-0 border-b border-[var(--nova-border)] bg-[color-mix(in_srgb,var(--director-canvas)_92%,transparent)] px-4 pb-3 pt-4 backdrop-blur-xl">
      <div className="flex min-w-0 items-center gap-3">
        <div data-testid="director-panel-icon" className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-[12px] border border-[var(--nova-border)] bg-[var(--director-panel)] text-[var(--director-brass)]" aria-label={t('directorPanel.consoleTitle')} title={t('directorPanel.consoleTitle')}>
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
        <DirectorOpenButton status={directorStatus} onOpen={onOpenBackstage} />
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

// 导演台入口：融在标题行右侧，导演运行状态（转圈/红点）与打开动作一体。
function DirectorOpenButton({ status, onOpen }: { status?: DirectorStatusLike; onOpen: () => void }) {
  const { t } = useTranslation()
  const running = status?.status === 'running'
  const failed = status?.status === 'failed'
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={t('directorPanel.openBackstage')}
      title={`${t('directorPanel.openBackstage')} · ${directorStatusLabel(status, false, t)}`}
      className="group flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--nova-border)] bg-[var(--director-panel)] py-1.5 pl-2.5 pr-2 text-[10px] font-medium text-[var(--nova-text-muted)] transition-colors hover:border-[var(--director-brass)] hover:text-[var(--nova-text)]"
    >
      {running ? <Loader2 className="h-3 w-3 animate-spin text-[var(--director-brass)]" /> : failed ? <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-[var(--nova-danger)]" /> : null}
      <Drama className="h-3 w-3 text-[var(--director-brass)]" />
      <span>{t('workbench.activity.director')}</span>
      <ArrowUpRight className="h-3 w-3 text-[var(--nova-text-faint)] transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
    </button>
  )
}
