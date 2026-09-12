import { PanelRight, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { StoryPicker, type StoryPickerProps } from '../StoryPicker'
import { TurnNavigator, type TurnNavigatorProps } from '../TurnNavigator'

interface StoryStageControlsProps {
  isMobile: boolean
  picker: StoryPickerProps
  history: TurnNavigatorProps
  directorPanelVisible?: boolean
  onToggleDirectorPanel?: () => void
}

export function StoryStageControls({ isMobile, picker, history, directorPanelVisible, onToggleDirectorPanel }: StoryStageControlsProps) {
  const { t } = useTranslation()
  return (
    <>
      {isMobile ? (
        <TurnNavigator {...history} renderTrigger={(openHistory) => <StoryPicker {...picker} onOpenHistory={history.items.length ? openHistory : undefined} />} />
      ) : <StoryPicker {...picker} />}
      {onToggleDirectorPanel && (
        <Button type="button" variant={isMobile ? 'ghost' : 'outline'} size={isMobile ? 'icon' : 'sm'} className={cn(!isMobile && 'h-7 gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 text-[11px] hover:bg-[var(--nova-hover)]', !isMobile && (directorPanelVisible ? 'text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'))} onClick={onToggleDirectorPanel} aria-label={directorPanelVisible ? t('storyStage.hideDirectorPanel') : t('storyStage.showDirectorPanel')} title={t('storyStage.directorPanel')}>
          {isMobile ? <SlidersHorizontal /> : <PanelRight className="h-3.5 w-3.5" />}
          <span className="max-lg:sr-only">{t('storyStage.directorPanel')}</span>
        </Button>
      )}
    </>
  )
}
