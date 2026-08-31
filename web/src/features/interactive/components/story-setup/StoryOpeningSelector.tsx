import { BookOpen, Check, Pencil, Sparkles } from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import type { BookOpeningPreset } from '../../opening'
import { truncateStoryOpeningText } from '../../opening'
import type { StoryOpeningConfig } from '../../types'
import { cn } from '@/lib/utils'

interface StoryOpeningSelectorProps {
  value: StoryOpeningConfig
  presets: BookOpeningPreset[]
  onChange: (value: StoryOpeningConfig) => void
}

export function StoryOpeningSelector({ value, presets, onChange }: StoryOpeningSelectorProps) {
  const { t } = useTranslation()
  const customTextRef = useRef(value.mode === 'custom' ? value.custom_text || '' : '')
  const presetDraftRef = useRef<StoryOpeningConfig | null>(value.mode === 'preset' ? value : null)
  if (value.mode === 'custom') customTextRef.current = value.custom_text || ''
  if (value.mode === 'preset') presetDraftRef.current = value
  const availablePresets = presets.filter((preset) => preset.content.trim())
  const savedPreset = value.mode === 'preset' ? value : presetDraftRef.current
  if (savedPreset?.mode === 'preset' && savedPreset.preset_text?.trim() && !availablePresets.some((preset) => preset.id === savedPreset.preset_id)) {
    availablePresets.unshift({ id: savedPreset.preset_id || 'story-snapshot', title: t('storyPicker.setup.opening.savedPreset'), content: savedPreset.preset_text })
  }
  const selectedPresetID = value.mode === 'preset' ? value.preset_id : presetDraftRef.current?.preset_id
  const selectedPreset = availablePresets.find((preset) => preset.id === selectedPresetID) || availablePresets[0]

  const selectMode = (mode: string) => {
    if (mode === 'preset') {
      if (selectedPreset) {
        const next: StoryOpeningConfig = { mode, preset_id: selectedPreset.id, preset_text: truncateStoryOpeningText(selectedPreset.content) }
        presetDraftRef.current = next
        onChange(next)
      }
      return
    }
    if (mode === 'custom') {
      onChange({ mode, custom_text: customTextRef.current })
      return
    }
    onChange({ mode: 'ai' })
  }

  return (
    <section aria-labelledby="story-opening-choice-title" className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="px-4 pb-3 pt-4 sm:px-5">
        <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
          <h3 id="story-opening-choice-title" className="text-sm font-semibold text-foreground">{t('storyPicker.setup.opening.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('storyPicker.setup.opening.description')}</p>
        </div>
      </div>
      <Tabs value={value.mode} onValueChange={selectMode} className="gap-0 border-t border-border">
        <TabsList className="grid h-10 w-full grid-cols-3 rounded-none border-b border-border bg-muted/40 p-0 group-data-horizontal/tabs:h-10">
          <OpeningTab value="ai" icon={<Sparkles />} label={t('storyPicker.setup.opening.ai')} />
          <OpeningTab value="preset" icon={<BookOpen />} label={t('storyPicker.setup.opening.preset')} disabled={availablePresets.length === 0} />
          <OpeningTab value="custom" icon={<Pencil />} label={t('storyPicker.setup.opening.custom')} />
        </TabsList>

        <TabsContent value="ai" className="m-0 min-h-48 p-4 sm:p-5">
          <div className="flex h-full flex-col justify-center rounded-lg border border-dashed border-border bg-background p-4">
            <span className="flex size-9 items-center justify-center rounded-lg border border-border bg-muted text-primary"><Sparkles className="size-4" /></span>
            <h4 className="mt-3 text-sm font-medium text-foreground">{t('storyPicker.setup.opening.aiTitle')}</h4>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('storyPicker.setup.opening.aiDescription')}</p>
          </div>
        </TabsContent>

        <TabsContent value="preset" className="m-0 min-h-48 p-3 sm:p-4">
          <div role="listbox" aria-label={t('storyPicker.setup.opening.preset')} className="grid max-h-64 gap-2 overflow-y-auto pr-1">
            {availablePresets.map((preset) => {
              const active = selectedPreset?.id === preset.id
              return (
                <button key={preset.id} type="button" role="option" aria-selected={active} aria-label={`${preset.title} ${preset.content}`} className={cn('rounded-lg border p-3 text-left transition-colors', active ? 'border-primary/55 bg-primary/5' : 'border-border bg-background hover:bg-muted/50')} onClick={() => { const next: StoryOpeningConfig = { mode: 'preset', preset_id: preset.id, preset_text: truncateStoryOpeningText(preset.content) }; presetDraftRef.current = next; onChange(next) }}>
                  <span className="flex min-w-0 items-center gap-2 text-xs font-medium text-foreground"><span className="min-w-0 flex-1 truncate">{preset.title}</span>{active ? <Check className="size-3.5 shrink-0 text-primary" /> : null}</span>
                  <span className="mt-1.5 line-clamp-3 block text-xs leading-5 text-muted-foreground">{preset.content}</span>
                </button>
              )
            })}
          </div>
        </TabsContent>

        <TabsContent value="custom" className="m-0 min-h-48 p-4 sm:p-5">
          <Textarea autoResize value={value.mode === 'custom' ? value.custom_text || '' : ''} maxLength={4000} className="min-h-40 resize-y bg-background text-sm leading-6" placeholder={t('storyPicker.setup.opening.customPlaceholder')} onChange={(event) => { customTextRef.current = event.target.value; onChange({ mode: 'custom', custom_text: event.target.value }) }} />
          <div className="mt-1.5 text-right font-mono text-[10px] tabular-nums text-muted-foreground">{t('storyPicker.setup.opening.characterCount', { count: value.mode === 'custom' ? value.custom_text?.length || 0 : 0 })}</div>
        </TabsContent>
      </Tabs>
    </section>
  )
}

function OpeningTab({ value, icon, label, disabled }: { value: string; icon: React.ReactNode; label: string; disabled?: boolean }) {
  return (
    <TabsTrigger value={value} disabled={disabled} className="h-10 min-w-0 rounded-none border-0 border-b-2 border-transparent px-2 text-xs shadow-none after:hidden data-[state=active]:border-primary data-[state=active]:bg-background data-[state=active]:shadow-none">
      <span className="flex min-w-0 items-center gap-1.5 [&_svg]:size-3.5">{icon}<span className="truncate">{label}</span></span>
    </TabsTrigger>
  )
}
