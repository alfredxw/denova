import { ArrowLeft, BookOpen, Check, Pencil, SlidersHorizontal, Sparkles } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import type { BookOpeningPreset } from '../opening'
import type { StorySummary } from '../types'

interface StoryOpeningPanelProps {
  story?: StorySummary
  storyId: string
  streaming: boolean
  presets: BookOpeningPreset[]
  selectedPreset: BookOpeningPreset | null
  customText: string
  bottomInset?: number
  loreEmpty?: boolean
  onSelectPreset: (presetId: string) => void
  onCustomTextChange: (text: string) => void
  onStartAI: () => void
  onStartPreset: () => void
  onStartCustom: () => void
  onConfigureDirector?: () => void
  onRequestLoreInit?: () => void
  onBackToSetup?: () => void
}

export function StoryOpeningPanel({
  story,
  storyId,
  streaming,
  presets,
  selectedPreset,
  customText,
  bottomInset = 0,
  loreEmpty = false,
  onSelectPreset,
  onCustomTextChange,
  onStartAI,
  onStartPreset,
  onStartCustom,
  onConfigureDirector,
  onRequestLoreInit,
  onBackToSetup,
}: StoryOpeningPanelProps) {
  const { t } = useTranslation()
  const disabled = !storyId || streaming
  const storyOrigin = story?.origin?.trim() || t('storyStage.opening.noOrigin')

  return (
    <div
      className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 pt-6 sm:px-7 sm:pt-8 lg:px-10"
      style={{ paddingBottom: Math.max(32, bottomInset + 28) }}
    >
      <section aria-labelledby="story-opening-title" className="mx-auto w-full max-w-4xl">
        <header className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <div className="mb-1 flex items-center gap-2 text-[11px] font-medium tracking-[0.12em] text-[var(--nova-text-faint)]">
              <span className="h-px w-5 bg-[var(--nova-accent)]/70" aria-hidden="true" />
              {t('storyStage.opening.eyebrow')}
            </div>
            <h2 id="story-opening-title" className="text-balance text-xl font-semibold tracking-[-0.02em] text-[var(--nova-text)] sm:text-2xl">
              {t('storyStage.opening.emptyTitle')}
            </h2>
            <p className="mt-1 max-w-2xl text-pretty text-xs leading-5 text-[var(--nova-text-faint)] sm:text-sm sm:leading-6">
              {t('storyStage.opening.emptyDescription')}
            </p>
          </div>
          {onBackToSetup ? <Button type="button" variant="ghost" size="sm" className="self-start gap-1.5 text-[var(--nova-text-muted)] sm:self-auto" onClick={onBackToSetup}><ArrowLeft className="h-3.5 w-3.5" />{t('storyStage.opening.backToSetup')}</Button> : null}
        </header>

        <Tabs defaultValue="ai" className="gap-0 overflow-hidden rounded-[calc(var(--nova-radius)+4px)] border border-[var(--nova-border)] bg-[var(--nova-surface)] shadow-[0_18px_50px_rgba(0,0,0,0.14)]">
          <TabsList className="grid h-11 w-full grid-cols-3 gap-0 rounded-none border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 group-data-horizontal/tabs:h-11">
            <OpeningTab value="ai" icon={<Sparkles />} label={t('storyStage.opening.tabAI')} />
            <OpeningTab value="preset" icon={<BookOpen />} label={t('storyStage.opening.tabPreset')} count={presets.length} />
            <OpeningTab value="custom" icon={<Pencil />} label={t('storyStage.opening.tabCustom')} />
          </TabsList>

          <TabsContent value="ai" className="m-0 p-5 sm:p-6">
            <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_15rem] lg:items-end">
              <div className="min-w-0">
                <div className="flex h-9 w-9 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]">
                  <Sparkles className="h-4 w-4" />
                </div>
                <h3 className="mt-4 text-base font-semibold tracking-[-0.01em] text-[var(--nova-text)]">{t('storyStage.opening.aiTitle')}</h3>
                <p className="mt-1 max-w-xl text-xs leading-5 text-[var(--nova-text-faint)]">{t('storyStage.opening.aiDescription')}</p>
                <div className="mt-5 border-l-2 border-[var(--nova-accent)]/50 pl-3">
                  <div className="text-[10px] font-medium tracking-[0.1em] text-[var(--nova-text-faint)]">{t('storyStage.opening.storyBrief')}</div>
                  <p className="mt-1 line-clamp-3 text-sm leading-6 text-[var(--nova-text-muted)]">{storyOrigin}</p>
                </div>
              </div>
              <div className="flex flex-col gap-2 lg:items-stretch">
                <Button type="button" size="sm" className="justify-center gap-1.5" disabled={disabled} onClick={onStartAI}>
                  <Sparkles data-icon="inline-start" />
                  {t('storyStage.opening.startAI')}
                </Button>
                {onConfigureDirector ? (
                  <Button type="button" variant="ghost" size="sm" className="justify-center gap-1.5 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]" onClick={onConfigureDirector}>
                    <SlidersHorizontal data-icon="inline-start" />
                    {t('storyStage.opening.configureDirector')}
                  </Button>
                ) : null}
              </div>
            </div>
          </TabsContent>

          <TabsContent value="preset" className="m-0 p-4 sm:p-5">
            {presets.length > 0 ? (
              <div className="grid min-h-[20rem] gap-4 lg:grid-cols-[minmax(13rem,0.8fr)_minmax(0,1.5fr)]">
                <div className="min-h-0">
                  <div className="mb-2 flex items-center justify-between gap-3 px-1">
                    <h3 className="text-xs font-medium text-[var(--nova-text)]">{t('storyStage.opening.presetLibrary')}</h3>
                    <span className="font-mono text-[10px] tabular-nums text-[var(--nova-text-faint)]">{presets.length}</span>
                  </div>
                  <div role="listbox" aria-label={t('storyStage.opening.bookPresetSelect')} className="grid max-h-[22rem] gap-2 overflow-y-auto pr-1 sm:grid-cols-2 lg:grid-cols-1">
                    {presets.map((preset) => {
                      const selected = selectedPreset?.id === preset.id
                      const title = preset.title || t('storyStage.opening.bookPresetUntitled')
                      return (
                        <button
                          key={preset.id}
                          type="button"
                          role="option"
                          aria-selected={selected}
                          aria-label={t('storyStage.opening.selectPreset', { title })}
                          className={`group relative min-w-0 rounded-[10px] border px-3 py-2.5 text-left transition-[border-color,background-color,transform] duration-200 active:translate-y-px ${selected ? 'border-[var(--nova-accent)]/55 bg-[var(--nova-active)]' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)] hover:border-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)]'}`}
                          onClick={() => onSelectPreset(preset.id)}
                        >
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="min-w-0 flex-1 truncate text-xs font-medium text-[var(--nova-text)]">{title}</span>
                            {selected ? <Check className="h-3.5 w-3.5 shrink-0 text-[var(--nova-accent)]" /> : null}
                          </div>
                          <p className="mt-1.5 line-clamp-2 text-[11px] leading-4 text-[var(--nova-text-faint)]">{preset.content}</p>
                        </button>
                      )
                    })}
                  </div>
                </div>

                <article className="flex min-h-[18rem] min-w-0 flex-col rounded-[12px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
                  <header className="flex items-center justify-between gap-3 border-b border-[var(--nova-border)] px-4 py-3">
                    <div className="min-w-0">
                      <div className="text-[10px] font-medium tracking-[0.1em] text-[var(--nova-text-faint)]">{t('storyStage.opening.presetPreview')}</div>
                      <h3 className="mt-0.5 truncate text-sm font-semibold text-[var(--nova-text)]">{selectedPreset?.title || t('storyStage.opening.bookPresetUntitled')}</h3>
                    </div>
                    <BookOpen className="h-4 w-4 shrink-0 text-[var(--nova-text-faint)]" />
                  </header>
                  <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
                    <p className="whitespace-pre-wrap text-pretty text-sm leading-7 text-[var(--nova-text-muted)]">{selectedPreset?.content}</p>
                  </div>
                  <footer className="flex justify-end border-t border-[var(--nova-border)] px-4 py-3">
                    <Button type="button" size="sm" className="gap-1.5" disabled={disabled || !selectedPreset} onClick={onStartPreset}>
                      <BookOpen data-icon="inline-start" />
                      {t('storyStage.opening.startBookPreset')}
                    </Button>
                  </footer>
                </article>
              </div>
            ) : (
              <div className="flex min-h-[20rem] flex-col items-start justify-center px-2 py-8 sm:px-8">
                <div className="flex h-10 w-10 items-center justify-center rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-faint)]">
                  <BookOpen className="h-4 w-4" />
                </div>
                <h3 className="mt-4 text-sm font-semibold text-[var(--nova-text)]">{t('storyStage.opening.noPresetTitle')}</h3>
                <p className="mt-1 max-w-md text-xs leading-5 text-[var(--nova-text-faint)]">{t('storyStage.opening.noPresetDescription')}</p>
              </div>
            )}
          </TabsContent>

          <TabsContent value="custom" className="m-0 p-5 sm:p-6">
            <div className="mx-auto max-w-2xl">
              <h3 className="text-base font-semibold tracking-[-0.01em] text-[var(--nova-text)]">{t('storyStage.opening.customTitle')}</h3>
              <p className="mt-1 text-xs leading-5 text-[var(--nova-text-faint)]">{t('storyStage.opening.customDescription')}</p>
              <Textarea
                autoResize
                className="nova-field mt-4 min-h-40 resize-y text-sm leading-6"
                placeholder={t('storyStage.opening.customPlaceholder')}
                value={customText}
                onChange={(event) => onCustomTextChange(event.target.value)}
              />
              <div className="mt-2 flex items-center justify-between gap-3">
                <span className="font-mono text-[10px] tabular-nums text-[var(--nova-text-faint)]">{t('storyStage.opening.characterCount', { count: customText.length })}</span>
                <Button type="button" size="sm" className="gap-1.5" disabled={disabled || !customText.trim()} onClick={onStartCustom}>
                  <Pencil data-icon="inline-start" />
                  {t('storyStage.opening.startCustom')}
                </Button>
              </div>
            </div>
          </TabsContent>
        </Tabs>

        {loreEmpty && onRequestLoreInit ? (
          <aside className="mt-4 flex flex-col gap-3 rounded-[10px] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <div className="text-xs font-medium text-[var(--nova-text)]">{t('loreInit.interactiveTitle')}</div>
              <div className="mt-0.5 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('loreInit.interactiveDescription')}</div>
            </div>
            <Button type="button" variant="outline" size="xs" className="shrink-0" onClick={onRequestLoreInit}>{t('loreInit.openAgent')}</Button>
          </aside>
        ) : null}
      </section>
    </div>
  )
}

function OpeningTab({ value, icon, label, count }: { value: string; icon: ReactNode; label: string; count?: number }) {
  return (
    <TabsTrigger value={value} className="h-11 min-w-0 rounded-none border-0 border-b-2 border-b-transparent px-2 text-xs shadow-none after:hidden data-active:border-b-[var(--nova-accent)] data-active:bg-[var(--nova-active)] data-active:shadow-none sm:px-4">
      <span className="flex min-w-0 items-center justify-center gap-1.5">
        <span className="flex h-4 w-4 items-center justify-center [&_svg]:h-4 [&_svg]:w-4">{icon}</span>
        <span className="truncate text-center">{label}</span>
        {typeof count === 'number' ? <span className="font-mono text-[10px] tabular-nums text-[var(--nova-text-faint)]">{count}</span> : null}
      </span>
    </TabsTrigger>
  )
}
