import { useEffect, useState } from 'react'
import { FileText, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { ImagePreset, ImagePresetSlot } from '../../types'
import { PresetEmptyState, PresetMetadataPanel } from '../preset-config/PresetEditorChrome'
import { Field, iconActionClassName, inputClassName, presetStatusLabel, selectClassName } from './editor-shared'

const IMAGE_PRESET_PROMPT_LIMIT = 4000
const IMAGE_PRESET_TARGET_OPTIONS = [{ value: 'agent_system' }, { value: 'tool_request' }] as const
type ImagePresetTarget = ImagePresetSlot['target']

export function ImagePresetEditor({
  draft,
  setDraft,
  onSave,
}: {
  draft: ImagePreset | null
  setDraft: (draft: ImagePreset | null) => void
  onSave: () => void
}) {
  const { t } = useTranslation()
  const [activeSlotId, setActiveSlotId] = useState('')
  const slots = draft ? normalizedImagePresetSlots(draft, t('settingPanel.imagePreset.target.tool_request')) : []
  const activeSlot = slots.find((slot) => slot.id === activeSlotId) || slots[0] || null
  const slotIDs = slots.map((slot) => slot.id).join('|')

  useEffect(() => {
    setActiveSlotId((current) => {
      if (current && slots.some((slot) => slot.id === current)) return current
      return slots[0]?.id || ''
    })
  }, [draft?.id, slotIDs])

  if (!draft) {
    return <PresetEmptyState title={t('settingPanel.editor.noImagePresetSelected')} description={t('settingPanel.editor.noImagePresetSelectedDesc')} />
  }

  const setSlots = (nextSlots: ImagePresetSlot[]) => {
    setDraft({ ...draft, slots: nextSlots, prompt: imagePresetPromptForTarget(nextSlots, 'tool_request'), version: 2 })
  }

  const updateSlotById = (slotId: string, patch: Partial<ImagePresetSlot>) => {
    setSlots(slots.map((slot) => (slot.id === slotId ? { ...slot, ...patch } : slot)))
  }

  const addSlot = () => {
    const id = `slot-${Date.now()}`
    const slot: ImagePresetSlot = {
      id,
      name: t('settingPanel.imagePreset.newRuleName'),
      target: 'tool_request',
      enabled: true,
      content: '',
    }
    setSlots([...slots, slot])
    setActiveSlotId(id)
  }

  const deleteSlot = () => {
    if (!activeSlot || slots.length <= 1) return
    const nextSlots = slots.filter((slot) => slot.id !== activeSlot.id)
    setSlots(nextSlots)
    setActiveSlotId(nextSlots[0]?.id || '')
  }

  const selectedTarget = activeSlot?.target || 'tool_request'
  const contentValue = activeSlot?.content || ''
  const editHint = draft.custom ? t('settingPanel.storyDirector.customEditable') : t('settingPanel.storyDirector.builtInCopyHint')

  return (
    <div data-testid="image-preset-editor" className="image-preset-editor flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <PresetMetadataPanel
        name={draft.name}
        description={draft.description}
        status={presetStatusLabel(draft, t)}
        hint={editHint}
        onNameChange={(name) => setDraft({ ...draft, name })}
        onDescriptionChange={(description) => setDraft({ ...draft, description })}
      />
      <div className="image-preset-layout grid min-h-[320px] min-w-0 flex-1 overflow-y-auto">
        <aside className="image-preset-rules flex max-h-56 min-h-0 min-w-0 flex-col overflow-hidden border-b border-[var(--preset-line)] bg-[var(--preset-surface)]">
          <div className="flex h-11 items-center justify-between border-b border-[var(--nova-border)] px-3">
            <div className="text-xs font-medium text-[var(--nova-text-muted)]">{t('settingPanel.imagePreset.rulesTitle')}</div>
            <Button className={iconActionClassName} variant="outline" size="icon" onClick={addSlot} aria-label={t('settingPanel.injectRules.new')}>
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </div>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-2">
              {slots.map((slot) => (
                <div key={slot.id} className={`mb-0.5 flex min-h-10 w-full items-center gap-2 rounded-[9px] border px-2.5 py-1.5 text-xs transition ${activeSlot?.id === slot.id ? 'border-[var(--preset-line)] bg-[var(--nova-active)] text-[var(--nova-text)]' : 'border-transparent text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}>
                  <button type="button" onClick={() => setActiveSlotId(slot.id)} className="flex min-w-0 flex-1 items-center gap-2 text-left">
                    <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{slot.name}</span>
                      <span className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] text-[var(--nova-text-faint)]">
                        <span className="truncate">{imagePresetTargetLabel(slot.target, t)}</span>
                        <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${slot.enabled ? 'bg-[var(--nova-accent-green)]' : 'bg-[var(--nova-text-faint)]/35'}`} />
                        <span className="shrink-0">{slot.enabled ? t('settingPanel.enabled') : t('settingPanel.disabled')}</span>
                      </span>
                    </span>
                  </button>
                  <Switch
                    checked={slot.enabled}
                    onCheckedChange={(enabled) => updateSlotById(slot.id, { enabled })}
                    aria-label={slot.enabled ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')}
                    title={slot.enabled ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')}
                  />
                </div>
              ))}
            </div>
          </ScrollArea>
        </aside>

        {activeSlot ? (
          <section className="flex min-h-0 min-w-0 flex-col overflow-hidden">
            <div className="shrink-0 border-b border-[var(--preset-line)] bg-[var(--preset-surface)] p-3 sm:p-4">
              <div className="image-preset-rule-grid grid min-w-0 gap-3">
                <Field label={t('settingPanel.field.ruleName')}>
                  <Input className={inputClassName} value={activeSlot.name} onChange={(event) => updateSlotById(activeSlot.id, { name: event.target.value })} />
                </Field>
                <Field label={t('settingPanel.field.injectTarget')}>
                  <Select value={selectedTarget} onValueChange={(value) => updateSlotById(activeSlot.id, { target: value as ImagePresetTarget })}>
                    <SelectTrigger className={selectClassName}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      {IMAGE_PRESET_TARGET_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {imagePresetTargetLabel(option.value, t)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <div className="flex items-end justify-end">
                  <Button className={iconActionClassName} variant="outline" size="icon" disabled={slots.length <= 1} onClick={deleteSlot} aria-label={t('settingPanel.injectRules.delete')}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <div className="image-preset-rule-summary">
                  <div className="min-w-0 rounded-[12px] border border-[var(--preset-line)] bg-[var(--preset-raised)] px-3 py-2.5">
                    <div className="flex items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
                      <span>{imagePresetTargetLabel(selectedTarget, t)}</span>
                      <span className="h-1 w-1 rounded-full bg-[var(--nova-text-faint)]/50" />
                      <span className="text-[var(--nova-text-faint)]">{imagePresetTargetSummary(selectedTarget, t)}</span>
                    </div>
                    <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-muted)]">{imagePresetTargetDetail(selectedTarget, t)}</div>
                  </div>
                </div>
              </div>
            </div>
            <div className="min-h-[280px] flex-1 p-3 sm:p-4">
              <div className="mb-2 flex min-w-0 items-center justify-between gap-3">
                <span className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.imagePreset.ruleContent')}</span>
                <span className="shrink-0 font-mono text-[10px] text-[var(--nova-text-faint)]">{contentValue.length}/{IMAGE_PRESET_PROMPT_LIMIT}</span>
              </div>
              <Textarea
                autoResize={false}
                className="nova-field h-[calc(100%-1.75rem)] min-h-[240px] resize-none font-mono text-sm leading-7 shadow-none"
                value={contentValue}
                maxLength={IMAGE_PRESET_PROMPT_LIMIT}
                onChange={(event) => updateSlotById(activeSlot.id, { content: event.target.value.slice(0, IMAGE_PRESET_PROMPT_LIMIT) })}
                placeholder={t('settingPanel.imagePreset.promptPlaceholder')}
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
          <PresetEmptyState title={t('settingPanel.injectRules.emptyTitle')} description={t('settingPanel.imagePreset.emptyRulesDesc')} />
        )}
      </div>
    </div>
  )
}

export function normalizedImagePresetSlots(preset: Partial<ImagePreset> | null | undefined, fallbackName = 'tool_request'): ImagePresetSlot[] {
  if (!preset) return []
  const slots = Array.isArray(preset.slots) ? preset.slots : []
  if (slots.length > 0) {
    return slots.map((slot, index) => ({
      id: sanitizeImagePresetSlotId(slot.id) || `slot-${index + 1}`,
      name: slot.name?.trim() || sanitizeImagePresetSlotId(slot.id) || `slot-${index + 1}`,
      target: isImagePresetTarget(slot.target) ? slot.target : 'tool_request',
      enabled: slot.enabled !== false,
      content: (slot.content || '').slice(0, IMAGE_PRESET_PROMPT_LIMIT),
    }))
  }
  const prompt = preset.prompt?.trim() || ''
  return [{
    id: 'tool_request',
    name: fallbackName,
    target: 'tool_request',
    enabled: true,
    content: prompt.slice(0, IMAGE_PRESET_PROMPT_LIMIT),
  }]
}

export function enabledImagePresetSlotCount(preset: Partial<ImagePreset>) {
  return normalizedImagePresetSlots(preset).filter((slot) => slot.enabled).length
}

function imagePresetPromptForTarget(slots: ImagePresetSlot[], target: ImagePresetTarget) {
  return slots
    .filter((slot) => slot.enabled && slot.target === target && slot.content.trim())
    .map((slot) => `## ${slot.name}（${slot.target}）\n\n${slot.content.trim()}`)
    .join('\n\n')
}

function sanitizeImagePresetSlotId(id: string | undefined) {
  return (id || '').replace(/[^a-zA-Z0-9_-]/g, '').trim()
}

function isImagePresetTarget(value: string | undefined): value is ImagePresetTarget {
  return value === 'agent_system' || value === 'tool_request'
}

function imagePresetTargetLabel(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.target.${target}`)
}

function imagePresetTargetSummary(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.targetSummary.${target}`)
}

function imagePresetTargetDetail(target: ImagePresetTarget, t: (key: string) => string) {
  return t(`settingPanel.imagePreset.targetDetail.${target}`)
}
