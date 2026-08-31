import { BookUser, Library, UserRound } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import { projectFileAssetURL, type LoreItem } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { StoryProtagonist, StoryProtagonistMode } from '../../types'

interface StoryProtagonistSelectorProps {
  projectId: string
  value: StoryProtagonist
  loreItems: LoreItem[]
  onChange: (value: StoryProtagonist) => void
  onRequestLoreInit?: () => void
}

const modes: Array<{ mode: StoryProtagonistMode; icon: typeof UserRound }> = [
  { mode: 'default', icon: UserRound },
  { mode: 'custom', icon: BookUser },
  { mode: 'lore', icon: Library },
]

export function StoryProtagonistSelector({ projectId, value, loreItems, onChange, onRequestLoreInit }: StoryProtagonistSelectorProps) {
  const { t } = useTranslation()
  const [pickerOpen, setPickerOpen] = useState(false)
  const customDraftRef = useRef<StoryProtagonist>(value.mode === 'custom' ? value : { mode: 'custom', name: '', profile: '' })
  const loreDraftRef = useRef<StoryProtagonist>(value.mode === 'lore' ? value : { mode: 'lore' })
  if (value.mode === 'custom') customDraftRef.current = value
  if (value.mode === 'lore') loreDraftRef.current = value
  const characters = useMemo(() => loreItems.filter((item) => item.enabled && item.type === 'character'), [loreItems])
  const selected = characters.find((item) => item.id === value.source_lore_item_id)
  const selectedName = selected?.name || value.name || ''
  const selectedDescription = selected?.brief_description || value.profile || ''

  const selectMode = (mode: StoryProtagonistMode) => {
    if (mode === value.mode) return
    if (mode === 'default') onChange({ mode })
    else if (mode === 'custom') onChange(customDraftRef.current)
    else onChange(loreDraftRef.current)
  }

  const selectLoreCharacter = (item: LoreItem) => {
    const next: StoryProtagonist = {
      mode: 'lore',
      name: item.name,
      profile: item.content || item.brief_description,
      source_lore_item_id: item.id,
      source_lore_updated_at: item.updated_at,
    }
    loreDraftRef.current = next
    onChange(next)
    setPickerOpen(false)
  }

  return (
    <section aria-labelledby="story-protagonist-title" className="rounded-xl border border-border bg-card p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
        <h3 id="story-protagonist-title" className="text-sm font-semibold text-foreground">{t('storyPicker.setup.protagonist.title')}</h3>
        <p className="text-xs text-muted-foreground">{t('storyPicker.setup.protagonist.description')}</p>
      </div>

      <RadioGroup value={value.mode} onValueChange={(mode) => selectMode(mode as StoryProtagonistMode)} className="mt-3 grid gap-2 sm:grid-cols-3" aria-label={t('storyPicker.setup.protagonist.title')}>
        {modes.map(({ mode, icon: Icon }) => {
          const active = value.mode === mode
          return (
            <label key={mode} htmlFor={`protagonist-${mode}`} className={cn('flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 transition-colors', active ? 'border-primary/55 bg-primary/5' : 'border-border bg-background hover:bg-muted/50')}>
              <RadioGroupItem id={`protagonist-${mode}`} value={mode} aria-label={t(`storyPicker.setup.protagonist.${mode}.title`)} className="mt-0.5" />
              <span className="min-w-0">
                <span className="flex items-center gap-1.5 text-xs font-medium text-foreground"><Icon className="size-3.5 text-muted-foreground" />{t(`storyPicker.setup.protagonist.${mode}.title`)}</span>
                <span className="mt-1 block text-[11px] leading-4 text-muted-foreground">{t(`storyPicker.setup.protagonist.${mode}.description`)}</span>
              </span>
            </label>
          )
        })}
      </RadioGroup>

      {value.mode === 'custom' ? (
        <div className="mt-3 grid gap-3 border-t border-border pt-3 sm:grid-cols-[minmax(10rem,0.7fr)_minmax(0,1.6fr)]">
          <label className="text-xs font-medium text-foreground">
            {t('storyPicker.setup.protagonist.custom.name')}
            <Input value={value.name || ''} maxLength={120} className="mt-1 bg-background" placeholder={t('storyPicker.setup.protagonist.custom.namePlaceholder')} onChange={(event) => { const next = { ...value, name: event.target.value }; customDraftRef.current = next; onChange(next) }} />
          </label>
          <label className="text-xs font-medium text-foreground">
            {t('storyPicker.setup.protagonist.custom.profile')}
            <Textarea autoResize value={value.profile || ''} maxLength={32000} className="mt-1 min-h-24 resize-y bg-background" placeholder={t('storyPicker.setup.protagonist.custom.profilePlaceholder')} onChange={(event) => { const next = { ...value, profile: event.target.value }; customDraftRef.current = next; onChange(next) }} />
          </label>
        </div>
      ) : null}

      {value.mode === 'lore' ? (
        <div className="mt-3 border-t border-border pt-3">
          {selectedName ? (
            <div className="flex min-w-0 items-center gap-3 rounded-lg border border-border bg-background p-3">
              <LoreAvatar projectId={projectId} item={selected} name={selectedName} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium text-foreground">{selectedName}</div>
                <div className="mt-0.5 line-clamp-2 text-xs leading-5 text-muted-foreground">{selectedDescription || t('storyPicker.setup.protagonist.lore.noDescription')}</div>
                <div className="mt-1 text-[10px] text-muted-foreground">{t('storyPicker.setup.protagonist.lore.snapshotHint')}</div>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={() => setPickerOpen(true)}>{t('storyPicker.setup.protagonist.lore.change')}</Button>
            </div>
          ) : (
            <div className="flex flex-col items-start justify-between gap-3 rounded-lg border border-dashed border-border bg-background p-3 sm:flex-row sm:items-center">
              <p className="text-xs leading-5 text-muted-foreground">{characters.length > 0 ? t('storyPicker.setup.protagonist.lore.chooseHint') : t('storyPicker.setup.protagonist.lore.empty')}</p>
              <div className="flex shrink-0 gap-2">
                {characters.length === 0 && onRequestLoreInit ? <Button type="button" variant="ghost" size="sm" onClick={onRequestLoreInit}>{t('storyPicker.setup.protagonist.lore.openLore')}</Button> : null}
                <Button type="button" variant="outline" size="sm" aria-label={t('storyPicker.setup.protagonist.lore.choose')} disabled={characters.length === 0} onClick={() => setPickerOpen(true)}>{t('storyPicker.setup.protagonist.lore.choose')}</Button>
              </div>
            </div>
          )}
        </div>
      ) : null}

      <Dialog open={pickerOpen} onOpenChange={setPickerOpen}>
        <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-xl">
          <DialogHeader className="border-b border-border px-4 py-3">
            <DialogTitle>{t('storyPicker.setup.protagonist.lore.dialogTitle')}</DialogTitle>
            <DialogDescription>{t('storyPicker.setup.protagonist.lore.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <Command className="rounded-none p-1">
            <CommandInput placeholder={t('storyPicker.setup.protagonist.lore.search')} />
            <CommandList className="max-h-[min(24rem,60vh)]">
              <CommandEmpty>{t('storyPicker.setup.protagonist.lore.noResults')}</CommandEmpty>
              <CommandGroup>
                {characters.map((item) => (
                  <CommandItem key={item.id} value={`${item.name} ${item.brief_description} ${item.tags.join(' ')}`} role="option" aria-label={`${item.name} ${item.brief_description}`.trim()} data-checked={item.id === value.source_lore_item_id} className="items-start py-2.5" onSelect={() => selectLoreCharacter(item)}>
                    <LoreAvatar projectId={projectId} item={item} name={item.name} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 text-sm font-medium text-foreground">{item.name}</div>
                      <div className="mt-0.5 line-clamp-2 text-xs leading-4 text-muted-foreground">{item.brief_description || item.content}</div>
                      {item.tags.length > 0 ? <div className="mt-1 truncate text-[10px] text-muted-foreground">{item.tags.join(' · ')}</div> : null}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </DialogContent>
      </Dialog>
    </section>
  )
}

function LoreAvatar({ projectId, item, name }: { projectId: string; item?: LoreItem; name: string }) {
  const path = item?.image?.image_path?.trim()
  if (path) return <img src={projectFileAssetURL(projectId, path)} alt="" className="size-10 shrink-0 rounded-lg border border-border object-cover" />
  return <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-sm font-semibold text-muted-foreground">{Array.from(name)[0] || '?'}</span>
}
