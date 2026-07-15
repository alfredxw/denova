import { Check, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { StoryStateDisplayPreference } from './display-preference'

const DISPLAY_OPTIONS: StoryStateDisplayPreference[] = ['preview', 'expanded', 'collapsed', 'director-only']

export function StateDisplayPreferenceMenu({ value, onChange, compact = false }: { value: StoryStateDisplayPreference; onChange: (value: StoryStateDisplayPreference) => void; compact?: boolean }) {
  const { t } = useTranslation()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size={compact ? 'icon-sm' : 'sm'}
          aria-label={t('storyStage.state.displayPreference')}
          title={t('storyStage.state.displayPreference')}
        >
          <SlidersHorizontal data-icon="inline-start" />
          {compact ? null : <span>{t(`storyStage.state.display.${value}`)}</span>}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuGroup>
          {DISPLAY_OPTIONS.map((option) => (
            <DropdownMenuItem
              key={option}
              onSelect={() => onChange(option)}
              className="grid cursor-pointer grid-cols-[16px_minmax(0,1fr)] items-start gap-2 px-2 py-2"
            >
              <Check className={cn('mt-0.5 text-muted-foreground', option !== value && 'opacity-0')} />
              <span className="min-w-0">
                <span className="block text-xs font-medium">{t(`storyStage.state.display.${option}`)}</span>
                <span className="mt-0.5 block text-[10px] leading-4 text-muted-foreground">{t(`storyStage.state.display.${option}.hint`)}</span>
              </span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
