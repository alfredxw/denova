import { useEffect, useRef, useState } from 'react'
import { MoreHorizontal, SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { AutosaveStatusIndicator, type AutosaveStatus } from '@/components/forms/autosave-status'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Empty, EmptyDescription } from '@/components/ui/empty'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { createSaveLane, type SaveLane } from '@/lib/autosave/save-lane'
import { errorMessage } from '@/lib/error-diagnostics'
import { toast } from '@/lib/toast'
import { createInteractiveTeller } from '../../api'
import { narrativeStyleName, resolveNarrativeStyle } from '../../narrative-style'
import { useInteractiveStore } from '../../stores/interactive-store'
import type { InteractiveStoryUpdateInput, StoryNarrativeStyleOverrides, StorySummary, Teller } from '../../types'
import { ControlSection } from './StoryTuningControls'

interface Props {
  story?: StorySummary
  tellers: Teller[]
  onUpdate?: (input: InteractiveStoryUpdateInput) => void | Promise<void>
  onOpenPresets?: () => void
}

export function StoryNarrativeStyleControls(props: Props) {
  const teller = resolveNarrativeStyle(props.tellers, props.story?.story_teller_id)
  return <NarrativeStyleControls key={`${props.story?.id}:${teller?.id}`} {...props} teller={teller} />
}

function NarrativeStyleControls({ story, tellers, teller, onUpdate, onOpenPresets }: Props & { teller?: Teller }) {
  const { t } = useTranslation()
  const [slots, setSlots] = useState(story?.narrative_style_overrides?.slots ?? {})
  const slotsRef = useRef(slots)
  const laneRef = useRef<SaveLane<StoryNarrativeStyleOverrides['slots']> | null>(null)
  const [status, setStatus] = useState<AutosaveStatus>('saved')
  const [search, setSearch] = useState('')
  const [onlyEnabled, setOnlyEnabled] = useState(false)
  const [busy, setBusy] = useState(false)
  const [saveAsName, setSaveAsName] = useState<string | null>(null)
  const disabled = !story || !teller || !onUpdate || busy
  const styleEnabled = !story?.module_refs?.narrative_style_disabled
  const effectiveSlots = (teller?.slots ?? []).map(slot => ({ ...slot, ...slots[slot.id] }))
  const visibleSlots = effectiveSlots.filter(slot => (!onlyEnabled || slot.enabled) && slot.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()))
  const customized = Object.keys(slots).length > 0

  const reportError = (error: unknown) => {
    console.error('[story-narrative-style] Failed to save story rules', { storyId: story?.id, styleId: teller?.id, error })
    toast.error(errorMessage(error, t('directorPanel.tuning.saveFailed')))
  }

  useEffect(() => {
    if (!story || !teller || !onUpdate) return
    // The lane owns its original story callback. Leaving this panel flushes edits
    // to that story; it must never retarget pending work to the newly selected one.
    const lane = createSaveLane<StoryNarrativeStyleOverrides['slots']>({
      delayMs: 500,
      save: async ({ value }) => { await onUpdate({ narrative_style_overrides: { style_id: teller.id, slots: value } }) },
      onError: (_, error) => reportError(error),
    })
    lane.reset(`${story.id}:${teller.id}`)
    laneRef.current = lane
    const unsubscribe = lane.subscribe(() => setStatus(lane.getSnapshot().status))
    return () => {
      unsubscribe()
      void lane.dispose('flush').catch(reportError)
    }
    // This editor is keyed by story and style; the save callback remains bound to them.
  }, [])

  useEffect(() => {
    if (laneRef.current?.hasWork()) return
    const next = story?.narrative_style_overrides?.slots ?? {}
    slotsRef.current = next
    setSlots(next)
  }, [story?.narrative_style_overrides, status])

  const changeSlots = (next: StoryNarrativeStyleOverrides['slots']) => {
    slotsRef.current = next
    setSlots(next)
    laneRef.current?.edit(next)
  }

  const changeRule = (id: string, patch: { enabled?: boolean; content?: string }) => {
    const original = teller?.slots.find(slot => slot.id === id)
    if (!original) return
    const next = { ...slotsRef.current[id], ...patch }
    if (next.enabled === original.enabled) delete next.enabled
    if (next.content === original.content) delete next.content
    const updated = { ...slotsRef.current }
    if (Object.keys(next).length) updated[id] = next
    else delete updated[id]
    changeSlots(updated)
  }

  const afterSave = async (action: () => void | Promise<void>) => {
    setBusy(true)
    try {
      await laneRef.current?.flush()
      if (laneRef.current?.getSnapshot().status === 'error') return
      await action()
    } catch (error) { reportError(error) } finally { setBusy(false) }
  }

  const selectStyle = (id: string) => void afterSave(async () => {
    await onUpdate?.({ story_teller_id: id, module_refs: { ...story?.module_refs, narrative_style_id: id, narrative_style_disabled: false } })
  })

  const saveAs = () => void afterSave(async () => {
    if (!teller || !saveAsName?.trim()) return
    const saved = await createInteractiveTeller({
      id: `style-${crypto.randomUUID()}`, name: saveAsName.trim(), description: teller.description,
      slots: effectiveSlots, style_refs: teller.style_refs, style_rules: teller.style_rules, context_policy: teller.context_policy,
    })
    const state = useInteractiveStore.getState()
    state.setTellers([...state.tellers, saved])
    setSaveAsName(null)
    toast.success(t('narrativeStyle.import.saved', { name: saved.name }))
  })

  return <ControlSection icon={<SlidersHorizontal className="size-4" />} title={t('directorPanel.tuning.agent.narrativeStyle')}
    action={<DropdownMenu>
      <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" aria-label={t('narrativeStyle.story.actions')} disabled={disabled}><MoreHorizontal /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end"><DropdownMenuGroup>
        <DropdownMenuItem disabled={!customized} onSelect={() => { changeSlots({}); void laneRef.current?.flush().catch(reportError) }}>{t('narrativeStyle.story.reset')}</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => setSaveAsName(t('narrativeStyle.story.copyName', { name: teller ? narrativeStyleName(teller, t) : '' }))}>{t('narrativeStyle.story.saveAs')}</DropdownMenuItem>
      </DropdownMenuGroup><DropdownMenuSeparator /><DropdownMenuGroup>
        <DropdownMenuItem disabled={!onOpenPresets} onSelect={() => void afterSave(() => onOpenPresets?.())}>{t('narrativeStyle.story.manage')}</DropdownMenuItem>
      </DropdownMenuGroup></DropdownMenuContent>
    </DropdownMenu>}
    summary={<div className="flex min-w-0 items-center gap-2 px-2.5 py-2">
      <Select value={teller?.id ?? ''} onValueChange={selectStyle} disabled={disabled}>
        <SelectTrigger size="sm" className="min-w-0 flex-1" aria-label={t('directorPanel.tuning.agent.narrativeStyle')}><SelectValue /></SelectTrigger>
        <SelectContent><SelectGroup>{tellers.map(item => <SelectItem key={item.id} value={item.id}>{narrativeStyleName(item, t)}</SelectItem>)}</SelectGroup></SelectContent>
      </Select>
      <Switch aria-label={t('narrativeStyle.story.enabled')} checked={styleEnabled} disabled={disabled} onCheckedChange={enabled => void afterSave(() => onUpdate?.({ module_refs: { ...story?.module_refs, narrative_style_disabled: !enabled } }))} />
    </div>}>
    <div className="flex min-w-0 flex-col gap-2 p-2.5">
      <div className="flex flex-wrap items-center justify-between gap-1">
        <Badge variant="outline">{t(`narrativeStyle.story.${customized ? 'customized' : 'scope'}`)}</Badge>
        <AutosaveStatusIndicator status={status} showSavedLabel={false} onRetry={() => laneRef.current?.flush()} />
      </div>
      <Input aria-label={t('narrativeStyle.import.search')} placeholder={t('narrativeStyle.import.search')} value={search} onChange={event => setSearch(event.target.value)} />
      <label className="flex items-center gap-2 text-xs text-muted-foreground"><Checkbox checked={onlyEnabled} onCheckedChange={checked => setOnlyEnabled(checked === true)} />{t('narrativeStyle.story.onlyEnabled')}</label>
      <Accordion type="single" collapsible className="max-h-80 min-w-0 overflow-y-auto">
        {visibleSlots.map(slot => <AccordionItem key={slot.id} value={slot.id}>
          <div className="flex items-center gap-2">
            <AccordionTrigger className="min-w-0 flex-1 text-xs" aria-label={slot.name}><span className="min-w-0 break-words [overflow-wrap:anywhere]">{slot.name}</span></AccordionTrigger>
            <Switch aria-label={slot.name} checked={slot.enabled} disabled={disabled} onCheckedChange={enabled => { changeRule(slot.id, { enabled }); void laneRef.current?.flush().catch(reportError) }} />
          </div>
          <AccordionContent><Textarea aria-label={t('narrativeStyle.story.ruleContent', { name: slot.name })} className="min-h-40 text-xs" value={slot.content} disabled={disabled} onChange={event => changeRule(slot.id, { content: event.target.value })} onBlur={() => void laneRef.current?.flush().catch(reportError)} /></AccordionContent>
        </AccordionItem>)}
      </Accordion>
      {!visibleSlots.length && <Empty className="p-3"><EmptyDescription>{t('narrativeStyle.import.noMatches')}</EmptyDescription></Empty>}
      <span className="text-xs text-muted-foreground">{t('narrativeStyle.story.nextGeneration')}</span>
    </div>
    <Dialog open={saveAsName !== null} onOpenChange={open => { if (!open && !busy) setSaveAsName(null) }}>
      <DialogContent aria-describedby={undefined}><DialogHeader><DialogTitle>{t('narrativeStyle.story.saveAs')}</DialogTitle></DialogHeader>
        <Field><FieldLabel htmlFor="story-style-name">{t('narrativeStyle.import.name')}</FieldLabel><Input id="story-style-name" value={saveAsName ?? ''} disabled={busy} onChange={event => setSaveAsName(event.target.value)} /></Field>
        <DialogFooter><Button variant="outline" disabled={busy} onClick={() => setSaveAsName(null)}>{t('common.cancel')}</Button><Button disabled={busy || !saveAsName?.trim()} onClick={saveAs}>{t('common.save')}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </ControlSection>
}
