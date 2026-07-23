import type { Dispatch, ReactNode, SetStateAction } from 'react'
import type { TFunction } from 'i18next'
import { SlidersHorizontal, X } from 'lucide-react'

interface StoryStageHeaderProps {
  isMobile: boolean
  controlsOpen: boolean
  onControlsOpenChange: Dispatch<SetStateAction<boolean>>
  controls: ReactNode
  t: TFunction
}

export function StoryStageHeader({ isMobile, controlsOpen, onControlsOpenChange, controls, t }: StoryStageHeaderProps) {
  if (!isMobile) {
    return (
      <div className="nova-story-stage-header nova-topbar flex min-h-12 flex-wrap items-center justify-start gap-3 border-b px-4 py-2">
        <div className="nova-story-stage-controls flex min-w-0 flex-wrap items-center justify-start gap-2">{controls}</div>
      </div>
    )
  }
  return (
    <div className="pointer-events-none absolute inset-x-0 top-3 z-10 px-3">
      <div className={`pointer-events-auto ml-auto overflow-hidden rounded-[14px] border border-[var(--nova-border)] bg-[var(--nova-surface)]/85 text-[var(--nova-text)] shadow-[0_12px_36px_rgba(0,0,0,0.28)] backdrop-blur-xl transition-[max-height,width,background-color] duration-200 ease-[var(--nova-ease)] ${controlsOpen ? 'w-[min(calc(100vw-1.5rem),390px)] max-h-[48dvh]' : 'w-8 max-h-8'}`}>
        <button type="button" className="flex h-8 w-full items-center gap-2 px-2 text-left text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" aria-label={t('storyStage.mobile.controls')} aria-expanded={controlsOpen} title={t('storyStage.mobile.controls')} onClick={() => onControlsOpenChange((open) => !open)}>
          <span className="flex h-4 w-4 shrink-0 items-center justify-center"><SlidersHorizontal className="h-3.5 w-3.5" /></span>
          {controlsOpen ? <span className="min-w-0 flex-1 truncate text-xs font-semibold text-[var(--nova-text)]">{t('storyStage.mobile.controls')}</span> : null}
          {controlsOpen ? <X className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" /> : null}
        </button>
        {controlsOpen ? (
          <div className="border-t border-[var(--nova-border)] px-3 pb-3 pt-2">
            <div className="flex max-h-[calc(48dvh-3rem)] flex-col gap-2 overflow-y-auto pr-1">{controls}</div>
          </div>
        ) : null}
      </div>
    </div>
  )
}
