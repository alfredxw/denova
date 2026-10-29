import { useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Empty, EmptyDescription } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { withErrorLogID } from '@/lib/api-client'
import { readTextFile } from '@/lib/text-file'
import { createInteractiveTeller } from '../../api'
import {
  buildNarrativeImportInstruction, directNarrativeSlots, NarrativeImportError, NARRATIVE_PRESET_FILE_LIMIT,
  orderedImportPrompts, parseNarrativePreset, type NarrativeImportTask, type NarrativePreset,
} from '../../narrative-preset-import'

/** Both import paths create one ordinary native style; selection never activates it. */
export function NarrativePresetImportDialog({ onClose, onConvert, onImported }: {
  onClose: () => void
  onConvert: (task: NarrativeImportTask) => void
  onImported: (id: string) => void
}) {
  const { t, i18n } = useTranslation()
  const fileInput = useRef<HTMLInputElement>(null)
  const [preset, setPreset] = useState<NarrativePreset | null>(null)
  const [filename, setFilename] = useState('')
  const [name, setName] = useState('')
  const [orderIndex, setOrderIndex] = useState(0)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const prompts = preset ? orderedImportPrompts(preset, orderIndex) : []
  const visiblePrompts = prompts.filter(prompt => (prompt.name || prompt.identifier).toLowerCase().includes(search.toLowerCase()))
  const selectedPrompts = prompts.filter(prompt => selected.has(prompt.identifier))
  const busy = loading || saving
  const canImport = Boolean(preset && !busy && name.trim() && selected.size)
  const allSelected = prompts.length > 0 && selected.size === prompts.length

  function resetSelection(source: NarrativePreset, index: number, selection: 'all' | 'enabled' = 'all') {
    setOrderIndex(index)
    setSelected(new Set(orderedImportPrompts(source, index).filter(prompt => selection === 'all' || prompt.enabled).map(prompt => prompt.identifier)))
  }

  async function readFile(file: File | undefined) {
    if (!file) return
    setLoading(true)
    setError('')
    setPreset(null)
    setFilename(file.name)
    try {
      if (file.size > NARRATIVE_PRESET_FILE_LIMIT) throw new NarrativeImportError('tooLarge')
      const source = parseNarrativePreset(await readTextFile(file))
      setPreset(source)
      setName(file.name.replace(/\.json$/i, ''))
      setSearch('')
      resetSelection(source, 0)
    } catch (error) {
      console.warn('[narrative-preset-import] Failed to read preset', error)
      setError(t(`narrativeStyle.import.error.${error instanceof NarrativeImportError ? error.key : 'invalidFile'}`))
    } finally {
      setLoading(false)
    }
  }

  async function startImport(method: 'direct' | 'ai') {
    setError('')
    try {
      const id = `imported-${crypto.randomUUID()}`
      if (method === 'direct') {
        setSaving(true)
        const saved = await createInteractiveTeller({
          id, name: name.trim(),
          description: t('narrativeStyle.import.source', { filename }),
          slots: directNarrativeSlots(selectedPrompts),
        })
        console.info('[narrative-preset-import] Saved direct import', { id: saved.id, slots: saved.slots.length })
        onImported(saved.id)
        return
      }
      onConvert({
        id,
        instruction: buildNarrativeImportInstruction({
          id, name: name.trim(), filename, language: i18n.resolvedLanguage || i18n.language,
          prompts: selectedPrompts,
        }),
        displayMessage: t('narrativeStyle.import.task', { name: name.trim() }),
      })
    } catch (error) {
      console.warn('[narrative-preset-import] Import failed', { method, error })
      setError(error instanceof NarrativeImportError
        ? t(`narrativeStyle.import.error.${error.key}`)
        : withErrorLogID(t('narrativeStyle.import.error.saveFailed'), error))
    } finally {
      setSaving(false)
    }
  }

  return <Dialog open onOpenChange={open => { if (!open && !saving) onClose() }}>
    <DialogContent showCloseButton={!saving} className="flex max-h-[calc(100dvh-2rem)] min-h-0 flex-col overflow-hidden max-md:overflow-y-hidden sm:max-w-3xl" data-testid="narrative-preset-import">
      <DialogHeader>
        <DialogTitle>{t('narrativeStyle.import.title')}</DialogTitle>
        <DialogDescription>{t('narrativeStyle.import.description')}</DialogDescription>
      </DialogHeader>
      <div className="flex shrink-0 flex-col gap-3">
        <FieldGroup className="gap-3 sm:grid sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="narrative-preset-file">{t('narrativeStyle.import.file')}</FieldLabel>
            <input ref={fileInput} id="narrative-preset-file" type="file" accept=".json,application/json" hidden disabled={busy} onChange={event => {
              void readFile(event.target.files?.[0])
              event.target.value = ''
            }} />
            <div className="flex min-w-0 items-center gap-3">
              <Button variant="outline" disabled={busy} onClick={() => fileInput.current?.click()}>{t('narrativeStyle.import.selectFile')}</Button>
              {filename && <span className="min-w-0 truncate text-sm" title={filename}>{filename}</span>}
            </div>
          </Field>
          {preset && <Field>
            <FieldLabel htmlFor="narrative-preset-name">{t('narrativeStyle.import.name')}</FieldLabel>
            <Input id="narrative-preset-name" value={name} disabled={busy} onChange={event => setName(event.target.value)} />
          </Field>}
          {preset && preset.prompt_order.length > 1 && <Field>
            <FieldLabel htmlFor="narrative-preset-order">{t('narrativeStyle.import.configuration')}</FieldLabel>
            <Select value={String(orderIndex)} disabled={busy} onValueChange={value => resetSelection(preset, Number(value))}>
              <SelectTrigger id="narrative-preset-order"><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>{preset.prompt_order.map((_, index) => <SelectItem key={index} value={String(index)}>{t('narrativeStyle.import.configurationNumber', { number: index + 1 })}</SelectItem>)}</SelectGroup></SelectContent>
            </Select>
          </Field>}
        </FieldGroup>
        {loading && <div role="status" className="flex items-center gap-2 text-sm"><Loader2 className="size-4 animate-spin" />{t('router.loading')}</div>}
        {error && <InlineErrorNotice message={error} />}
        {preset && <>
          <Input aria-label={t('narrativeStyle.import.search')} placeholder={t('narrativeStyle.import.search')} value={search} disabled={busy} onChange={event => setSearch(event.target.value)} />
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" disabled={busy} onClick={() => setSelected(new Set(allSelected ? [] : prompts.map(prompt => prompt.identifier)))}>{t(`narrativeStyle.import.${allSelected ? 'deselectAll' : 'selectAll'}`)}</Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => resetSelection(preset, orderIndex, 'enabled')}>{t('narrativeStyle.import.reset')}</Button>
          </div>
        </>}
      </div>
      {preset && <div className="min-h-0 flex-1 overflow-y-auto">
          <Accordion type="multiple" className="min-w-0">
            {visiblePrompts.map(prompt => <AccordionItem key={prompt.identifier} value={prompt.identifier}>
              <div className="flex items-center gap-3">
                <Checkbox aria-label={prompt.name || prompt.identifier} disabled={busy} checked={selected.has(prompt.identifier)} onCheckedChange={checked => setSelected(current => {
                  const next = new Set(current)
                  if (checked === true) next.add(prompt.identifier)
                  else next.delete(prompt.identifier)
                  return next
                })} />
                <div className="min-w-0 flex-1"><AccordionTrigger aria-label={prompt.name || prompt.identifier}>
                  <span className="flex min-w-0 flex-1 items-start gap-2">
                    <span className="min-w-0 flex-1 break-words [overflow-wrap:anywhere]">{prompt.name || prompt.identifier}</span>
                    <Badge variant={prompt.enabled ? 'secondary' : 'outline'}>{t(`narrativeStyle.import.${prompt.enabled ? 'enabled' : 'disabled'}`)}</Badge>
                  </span>
                </AccordionTrigger></div>
              </div>
              <AccordionContent><pre className="whitespace-pre-wrap break-words text-xs [overflow-wrap:anywhere]">{prompt.content}</pre></AccordionContent>
            </AccordionItem>)}
          </Accordion>
          {!visiblePrompts.length && <Empty><EmptyDescription>{t('narrativeStyle.import.noMatches')}</EmptyDescription></Empty>}
      </div>}
      <DialogFooter className="flex-col gap-3 sm:flex-row sm:items-center">
        {preset && <span className="mr-auto text-xs text-muted-foreground">
          {t('narrativeStyle.import.selected', { count: selected.size, total: prompts.length })}
          {' · '}{t('narrativeStyle.import.enabledCount', { count: selectedPrompts.filter(prompt => prompt.enabled).length })}
        </span>}
        <div className="flex shrink-0 items-center justify-end gap-2 max-sm:[&>button]:px-2 max-sm:[&>button]:text-xs">
          <Button variant="outline" disabled={saving} onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={!canImport} title={t('narrativeStyle.import.directHint')} onClick={() => void startImport('direct')}>
            {saving && <Loader2 data-icon="inline-start" className="animate-spin" />}
            {t('narrativeStyle.import.direct')}
          </Button>
          <Button variant="outline" disabled={!canImport} title={t('narrativeStyle.import.aiHint')} onClick={() => void startImport('ai')}>
            {t('narrativeStyle.import.convert')}
          </Button>
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>
}
