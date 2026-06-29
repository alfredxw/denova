import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Check, ChevronDown, FileText, Pencil, Plus, Trash2, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from '@/components/ui/dialog'
import type { StyleRule, Teller, TellerPromptSlot } from '../types'

const TELLER_TARGET_OPTIONS = [{ value: 'system' }, { value: 'turn_context' }, { value: 'state_memory' }] as const

type TellerTarget = TellerPromptSlot['target']
const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'
const STYLE_CONTENT_LIMIT = 8000
const STYLE_FILE_ACCEPT = '.txt,.md,.markdown,text/plain,text/markdown,text/x-markdown'

export function TellerEditor({ draft, setDraft, tagDraft, setTagDraft, activeSlotId, setActiveSlotId, onSave }: { workspace: string; draft: Teller | null; setDraft: (draft: Teller | null) => void; tagDraft: string; setTagDraft: (value: string) => void; activeSlotId: string; setActiveSlotId: (id: string) => void; onSave: () => void }) {
  const { t } = useTranslation()
  const activeSlot = draft?.slots?.find((slot) => slot.id === activeSlotId) || draft?.slots?.[0] || null
  const [targetPickerOpen, setTargetPickerOpen] = useState(false)
  const [randomEventRateInput, setRandomEventRateInput] = useState(() => formatRandomEventRate(draft?.random_event_rate))

  useEffect(() => {
    setTargetPickerOpen(false)
  }, [activeSlotId])

  useEffect(() => {
    setRandomEventRateInput(formatRandomEventRate(draft?.random_event_rate))
  }, [draft?.id])

  const updateSlotById = (slotId: string, patch: Partial<TellerPromptSlot>) => {
    if (!draft) return
    setDraft({
      ...draft,
      slots: draft.slots.map((slot) => (slot.id === slotId ? { ...slot, ...patch } : slot)),
    })
  }

  const updateSlot = (patch: Partial<TellerPromptSlot>) => {
    if (!draft || !activeSlot) return
    updateSlotById(activeSlot.id, patch)
  }

  const addSlot = () => {
    if (!draft) return
    const id = `slot-${Date.now()}`
    const slot: TellerPromptSlot = {
      id,
      name: '新规则',
      target: 'turn_context',
      enabled: true,
      content: '',
    }
    setDraft({ ...draft, slots: [...(draft.slots || []), slot] })
    setActiveSlotId(id)
  }

  const deleteSlot = () => {
    if (!draft || !activeSlot) return
    const nextSlots = draft.slots.filter((slot) => slot.id !== activeSlot.id)
    setDraft({ ...draft, slots: nextSlots })
    setActiveSlotId(nextSlots[0]?.id || '')
  }

  const updateRandomEventRate = (value: string) => {
    setRandomEventRateInput(value)
    if (!draft || !isDecimalInput(value)) return
    setDraft({
      ...draft,
      random_event_rate: parseDecimalInput(value),
    })
  }

  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noTellerSelected')} description={t('settingPanel.editor.noTellerSelectedDesc')} />
  }

  const selectedTarget = targetOption(activeSlot?.target || 'turn_context')

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden">
      <div className="grid shrink-0 gap-3 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4 lg:grid-cols-[minmax(220px,1fr)_minmax(220px,1fr)_150px_150px]">
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
        </Field>
        <Field label={t('settingPanel.field.description')}>
          <Input className={inputClassName} value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} placeholder={t('settingPanel.placeholder.description')} />
        </Field>
        <Field label={t('settingPanel.field.randomEventRate')}>
          <Input
            className={inputClassName}
            aria-label={t('settingPanel.field.randomEventRate')}
            inputMode="decimal"
            value={randomEventRateInput}
            onChange={(event) => updateRandomEventRate(event.target.value)}
          />
        </Field>
        <Field label={t('settingPanel.field.tags')}>
          <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
        </Field>
        <div className="flex items-end">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{draft.custom ? t('settingPanel.custom') : t('settingPanel.builtIn')}</span>
        </div>
      </div>

      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mb-3">
          <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.styleRules.title')}</div>
          <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.styleRules.desc')}</div>
        </div>
        <InteractiveStyleRulesEditor rules={draft.style_rules ?? []} onChange={(rules) => setDraft({ ...draft, style_rules: rules })} />
      </div>

      <div className="grid min-h-[520px] flex-1 grid-cols-1 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="flex max-h-60 min-h-0 flex-col overflow-hidden border-b border-[var(--nova-border)] bg-[var(--nova-surface)] lg:max-h-none lg:border-b-0 lg:border-r">
          <div className="flex h-11 items-center justify-between border-b border-[var(--nova-border)] px-3">
            <div className="text-xs font-medium text-[var(--nova-text-muted)]">{t('settingPanel.injectRules.title')}</div>
            <Button className={iconActionClassName} variant="outline" size="icon" onClick={addSlot} aria-label={t('settingPanel.injectRules.new')}>
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-2">
              {(draft.slots || []).map((slot) => (
                <div key={slot.id} className={`mb-1 flex min-h-12 w-full items-center gap-2 rounded-md border px-3 py-2 text-xs transition ${activeSlot?.id === slot.id ? 'border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] shadow-[inset_3px_0_0_var(--nova-accent)]' : 'border-transparent text-[var(--nova-text-muted)] hover:border-[var(--nova-border)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}>
                  <button type="button" onClick={() => setActiveSlotId(slot.id)} className="flex min-w-0 flex-1 items-center gap-2 text-left">
                    <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{slot.name}</span>
                      <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]">
                        <span className="truncate">{targetLabel(slot.target, t)}</span>
                        <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${slot.enabled ? 'bg-[var(--nova-accent-green)]' : 'bg-[var(--nova-text-faint)]/35'}`} />
                        <span className="shrink-0">{slot.enabled ? t('settingPanel.enabled') : t('settingPanel.disabled')}</span>
                      </span>
                    </span>
                  </button>
                  <ToggleSwitch checked={slot.enabled} compact onChange={(enabled) => updateSlotById(slot.id, { enabled })} />
                </div>
              ))}
            </div>
          </ScrollArea>
        </aside>

        {activeSlot ? (
          <section className="flex min-h-0 flex-col">
            <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
              <div className="grid gap-3 lg:grid-cols-[minmax(220px,1fr)_minmax(240px,320px)_32px]">
                <Field label={t('settingPanel.field.ruleName')}>
                  <Input className={inputClassName} value={activeSlot.name} onChange={(event) => updateSlot({ name: event.target.value })} />
                </Field>
                <div className="grid gap-1.5">
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.field.injectTarget')}</span>
                  <Popover open={targetPickerOpen} onOpenChange={setTargetPickerOpen}>
                    <PopoverTrigger asChild>
                      <button type="button" aria-label={t('settingPanel.field.injectTarget')} className={`${selectClassName} flex w-full items-center justify-between gap-2 px-3 text-left text-[var(--nova-text)]`}>
                        <span className="min-w-0 flex-1 truncate">
                          {targetLabel(selectedTarget.value as TellerTarget, t)} · {targetSummary(selectedTarget.value as TellerTarget, t)}
                        </span>
                        <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition ${targetPickerOpen ? 'rotate-180' : ''}`} />
                      </button>
                    </PopoverTrigger>
                    <PopoverContent align="start" sideOffset={6} className="nova-panel w-[320px] border border-[var(--nova-border)] p-1.5 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
                      {TELLER_TARGET_OPTIONS.map((option) => (
                        <button
                          key={option.value}
                          type="button"
                          onClick={() => {
                            updateSlot({
                              target: option.value as TellerTarget,
                            })
                            setTargetPickerOpen(false)
                          }}
                          className={`flex w-full items-start gap-2 rounded-md px-3 py-2.5 text-left transition ${activeSlot.target === option.value ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
                        >
                          <span className={`mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded-full border ${activeSlot.target === option.value ? 'border-[var(--nova-accent)] bg-[var(--nova-accent)]/15 text-[var(--nova-accent)]' : 'border-[var(--nova-border)] text-transparent'}`}>
                            <Check className="h-3 w-3" />
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block text-xs font-medium">{targetLabel(option.value as TellerTarget, t)}</span>
                            <span className="mt-0.5 block text-[11px] leading-4 text-[var(--nova-text-faint)]">{targetSummary(option.value as TellerTarget, t)}</span>
                          </span>
                        </button>
                      ))}
                    </PopoverContent>
                  </Popover>
                </div>
                <div className="flex items-end justify-end">
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={(draft.slots || []).length <= 1} onClick={deleteSlot} aria-label={t('settingPanel.injectRules.delete')}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <div className="lg:col-span-3">
                  <div className="min-w-0 rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2.5">
                    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                      <span>{targetLabel(selectedTarget.value as TellerTarget, t)}</span>
                      <span className="h-1 w-1 rounded-full bg-[var(--nova-text-faint)]/50" />
                      <span className="text-[var(--nova-text-faint)]">{targetSummary(selectedTarget.value as TellerTarget, t)}</span>
                    </div>
                    <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{targetDetail(selectedTarget.value as TellerTarget, t)}</div>
                  </div>
                </div>
              </div>
            </div>
            <div className="min-h-[420px] flex-1 p-4 lg:min-h-0">
              <Textarea
                className="nova-field h-full min-h-[360px] resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
                value={activeSlot.content}
                onChange={(event) => updateSlot({ content: event.target.value })}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
              />
            </div>
          </section>
        ) : (
          <EmptyState title={t('settingPanel.injectRules.emptyTitle')} description={t('settingPanel.injectRules.emptyDesc')} />
        )}
      </div>
    </div>
  )
}

function formatRandomEventRate(value: number | undefined) {
  return Number.isFinite(value) ? String(value) : '0'
}

function isDecimalInput(value: string) {
  return /^\d*(?:\.\d*)?$/.test(value.trim())
}

function parseDecimalInput(value: string) {
  const normalized = value.trim()
  if (normalized === '' || normalized === '.') return 0
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : 0
}

function InteractiveStyleRulesEditor({ rules, onChange }: { rules: StyleRule[]; onChange: (rules: StyleRule[]) => void }) {
  const { t } = useTranslation()
  const addRule = () => onChange([...rules, { scene: '', style_contents: [] }])
  const removeRule = (index: number) => onChange(rules.filter((_, i) => i !== index))
  const updateRule = (index: number, patch: Partial<StyleRule>) => {
    onChange(rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)))
  }

  return (
    <div className="flex flex-col gap-3">
      {rules.length > 0 && (
        <div className="space-y-2">
          {rules.map((rule, index) => (
            <InteractiveStyleRuleRow key={index} rule={rule} onChange={(patch) => updateRule(index, patch)} onRemove={() => removeRule(index)} />
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Button className={actionButtonClassName} variant="outline" size="sm" onClick={addRule}>
          <Plus className="h-3.5 w-3.5" />
          {t('settingPanel.style.addRule')}
        </Button>
      </div>
    </div>
  )
}

function InteractiveStyleRuleRow({ rule, onChange, onRemove }: { rule: StyleRule; onChange: (patch: Partial<StyleRule>) => void; onRemove: () => void }) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [contentDraft, setContentDraft] = useState('')
  const contents = rule.style_contents || []
  const summary = contents.length === 0 ? t('settingPanel.style.noSelected') : t('settingPanel.style.button', { count: contents.length })

  const openEditor = (index: number | null, content = '') => {
    setEditingIndex(index)
    setContentDraft(limitStyleContent(content))
    setEditorOpen(true)
  }

  const saveContent = () => {
    const content = limitStyleContent(contentDraft).trim()
    if (!content) return
    const next = [...contents]
    if (editingIndex === null) {
      next.push(content)
    } else {
      next[editingIndex] = content
    }
    onChange({ style_contents: next })
    setEditorOpen(false)
    setEditingIndex(null)
    setContentDraft('')
  }

  const removeContent = (index: number) => {
    onChange({ style_contents: contents.filter((_, i) => i !== index) })
  }

  const handleFileSelected = async (file: File | undefined) => {
    if (!file) return
    try {
      openEditor(null, limitStyleContent(await file.text()))
    } catch (err) {
      console.warn('[teller-editor] 读取风格内容文件失败', err)
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <input ref={fileInputRef} type="file" accept={STYLE_FILE_ACCEPT} className="hidden" onChange={(event) => void handleFileSelected(event.target.files?.[0])} />
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
        <Input className={`${inputClassName} md:min-w-44 md:flex-1`} value={rule.scene} placeholder={t('settingPanel.placeholder.scene')} onChange={(event) => onChange({ scene: event.target.value })} />
        <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
          <Upload className="h-3.5 w-3.5" />
          {t('settingPanel.style.upload')}
        </Button>
        <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm" onClick={() => openEditor(null)}>
          <Plus className="h-3.5 w-3.5" />
          {t('settingPanel.style.addContent')}
        </Button>
        <Button className={`${actionButtonClassName} justify-center hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]`} variant="outline" size="sm" onClick={onRemove}>
          <Trash2 className="h-3.5 w-3.5" />
          {t('common.delete')}
        </Button>
      </div>

      <div className="mt-1 truncate px-1 text-xs text-[var(--nova-text-faint)]">→ {summary}</div>
      {contents.length > 0 && (
        <div className="mt-2 grid gap-1.5">
          {contents.map((content, index) => (
            <div key={index} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs">
              <button type="button" className="min-w-0 flex-1 truncate text-left text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]" title={content} onClick={() => openEditor(index, content)}>
                {contentPreview(content)}
              </button>
              <Button className={iconActionClassName} variant="outline" size="icon" onClick={() => openEditor(index, content)} aria-label={t('settingPanel.style.editContent')}>
                <Pencil className="h-3.5 w-3.5" />
              </Button>
              <Button className={`${iconActionClassName} hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]`} variant="outline" size="icon" onClick={() => removeContent(index)} aria-label={t('common.delete')}>
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className="nova-panel flex max-h-[min(720px,calc(100vh-2rem))] w-[min(720px,calc(100vw-2rem))] max-w-[min(720px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('settingPanel.style.contentDialogTitle')}</DialogTitle>
            <DialogDescription className="mt-1 text-xs text-[var(--nova-text-faint)]">{t('settingPanel.style.contentDialogDesc')}</DialogDescription>
          </div>
          <div className="flex min-h-0 flex-1 flex-col p-4">
            <Textarea
              className="nova-field h-[min(52vh,420px)] min-h-0 resize-none overflow-y-auto text-sm leading-6 shadow-none [field-sizing:fixed] focus-visible:ring-0"
              value={contentDraft}
              onChange={(event) => setContentDraft(limitStyleContent(event.target.value))}
            />
            <div className="mt-2 text-right text-[11px] text-[var(--nova-text-faint)]">{contentDraft.length}/{STYLE_CONTENT_LIMIT}</div>
          </div>
          <DialogFooter className="!mx-0 !mb-0 rounded-none border-t border-[var(--nova-border)] bg-[var(--nova-surface)]/95 !px-4 !py-3">
            <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => setEditorOpen(false)}>{t('common.cancel')}</Button>
            <Button className="nova-nav-item gap-1.5 border border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] hover:border-[var(--nova-accent)] hover:bg-[var(--nova-hover)]" size="sm" onClick={saveContent} disabled={!contentDraft.trim()}>{t('common.save')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function limitStyleContent(value: string) {
  return Array.from(value).slice(0, STYLE_CONTENT_LIMIT).join('')
}

function contentPreview(value: string) {
  const normalized = value.replace(/\s+/g, ' ').trim()
  return normalized.length > 80 ? `${normalized.slice(0, 80)}...` : normalized
}

function Field({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <label className={`grid gap-1.5 ${className}`}>
      <span className="text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-5 text-center">
        <div className="text-sm font-medium text-[var(--nova-text)]">{title}</div>
        <div className="mt-1 text-xs text-[var(--nova-text-faint)]">{description}</div>
      </div>
    </div>
  )
}

function ToggleSwitch({ checked, onChange, compact = false }: { checked: boolean; onChange: (checked: boolean) => void; compact?: boolean }) {
  const { t } = useTranslation()
  const label = checked ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')
  return (
    <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)} title={label} className={`relative shrink-0 rounded-full border transition ${checked ? 'border-[var(--nova-accent-green)]/60 bg-[var(--nova-accent-green)]/25' : 'border-[var(--nova-border)] bg-[var(--nova-surface-2)]'} ${compact ? 'h-5 w-9' : 'h-6 w-11'}`}>
      <span className={`absolute rounded-full bg-[var(--nova-text)] shadow transition ${compact ? `top-0.5 h-4 w-4 ${checked ? 'left-[18px]' : 'left-0.5'}` : `top-0.5 h-5 w-5 ${checked ? 'left-[22px]' : 'left-0.5'}`}`} />
      <span className="sr-only">{label}</span>
    </button>
  )
}

function targetLabel(target: TellerTarget, t: (key: string) => string) {
  return t(targetTranslationKeys(target).label)
}

function targetSummary(target: TellerTarget, t: (key: string) => string) {
  return t(targetTranslationKeys(target).summary)
}

function targetDetail(target: TellerTarget, t: (key: string) => string) {
  return t(targetTranslationKeys(target).detail)
}

function targetOption(target: TellerTarget) {
  return TELLER_TARGET_OPTIONS.find((option) => option.value === target) || TELLER_TARGET_OPTIONS[1]
}

function targetTranslationKeys(target: TellerTarget) {
  if (target === 'system') {
    return {
      label: 'settingPanel.target.system.label',
      summary: 'settingPanel.target.system.summary',
      detail: 'settingPanel.target.system.detail',
    }
  }
  if (target === 'state_memory') {
    return {
      label: 'settingPanel.target.stateMemory.label',
      summary: 'settingPanel.target.stateMemory.summary',
      detail: 'settingPanel.target.stateMemory.detail',
    }
  }
  return {
    label: 'settingPanel.target.turnContext.label',
    summary: 'settingPanel.target.turnContext.summary',
    detail: 'settingPanel.target.turnContext.detail',
  }
}
