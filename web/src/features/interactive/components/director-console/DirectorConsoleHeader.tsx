import { useState } from 'react'
import { Clapperboard, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import type { StoryDirector, StorySummary } from '../../types'
import { ReplyTargetCharsControl } from '../ReplyTargetCharsControl'
import { StoryDirectorPicker } from '../StoryDirectorPicker'
import { DEFAULT_STORY_STATE_DISPLAY, type StoryStateDisplayPreference } from '../story-state/display-preference'
import { StateDisplayPreferenceMenu } from '../story-state/StateDisplayPreferenceMenu'

export function DirectorConsoleHeader({ branchId, turnCount, story, storyDirectors, onDirectorChange, onReplyTargetCharsChange, stateDisplayPreference = DEFAULT_STORY_STATE_DISPLAY, onStateDisplayPreferenceChange }: { branchId: string; turnCount: number; story?: StorySummary; storyDirectors: StoryDirector[]; onDirectorChange?: (directorId: string) => void; onReplyTargetCharsChange?: (replyTargetChars: number) => void | Promise<void>; stateDisplayPreference?: StoryStateDisplayPreference; onStateDisplayPreferenceChange?: (value: StoryStateDisplayPreference) => void }) {
  const { t } = useTranslation()
  const [settingsOpen, setSettingsOpen] = useState(false)
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
        <TooltipIconButton
          label={t('directorPanel.consoleSettings')}
          variant="ghost"
          size="icon-xs"
          aria-expanded={settingsOpen}
          className={`shrink-0 border border-[var(--nova-border)] ${settingsOpen ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'bg-[var(--director-panel)] text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]'}`}
          onClick={() => setSettingsOpen((open) => !open)}
        >
          <Settings2 className="h-4 w-4" />
        </TooltipIconButton>
      </div>
      <Collapsible open={settingsOpen}>
        <CollapsibleContent>
          <div className="mt-3 grid min-w-0 gap-3 border-t border-[var(--nova-border-soft)] pt-3">
            <StoryDirectorPicker story={story} storyDirectors={storyDirectors} onChange={onDirectorChange || (() => undefined)} layout="sidebar" />
            <div className="flex min-w-0 flex-col gap-1.5">
              <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyPicker.replyTargetChars')}</span>
              <ReplyTargetCharsControl story={story} onChange={onReplyTargetCharsChange} layout="console" />
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <span className="shrink-0 text-[11px] font-medium text-[var(--nova-text-faint)]">{t('storyStage.state.stageDisplay')}</span>
              <div>
                <StateDisplayPreferenceMenu value={stateDisplayPreference} onChange={onStateDisplayPreferenceChange ?? (() => undefined)} />
              </div>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </header>
  )
}
