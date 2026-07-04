import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Check, ChevronDown, Edit3, FileText, Loader2, Plus, Save, Sparkles, Trash2, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { runConfigManagerStream } from '@/lib/api'
import type { ChatMessage } from '@/lib/api'
import { isSaveShortcut } from '@/lib/keyboard'
import { readTextFile } from '@/lib/text-file'
import { MessageList } from '@/components/Chat/MessageList'
import { appendConfigManagerMessage, configManagerToolKey, parseConfigManagerPayload, reduceConfigManagerMessages, type ConfigManagerToolPayload } from '@/components/Chat/config-manager-events'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from '@/components/ui/dialog'
import { getStyleReferences, readStyleReferenceFile, saveStyleReference, updateStyleReferenceFile } from '../api'
import type { StyleReference, StyleReferenceFileDocument, StyleRule, Teller, TellerPromptSlot } from '../types'

const TELLER_TARGET_OPTIONS = [{ value: 'system' }, { value: 'turn_context' }, { value: 'state_memory' }] as const

type TellerTarget = TellerPromptSlot['target']
const actionButtonClassName = 'nova-nav-item gap-1.5 border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'
const STYLE_SOURCE_LIMIT = 40000
const STYLE_FILE_ACCEPT = '.txt,.md,.markdown,text/plain,text/markdown,text/x-markdown'
const STYLE_MARKDOWN_TAG = 'style_reference_markdown'

export function TellerEditor({ workspace, draft, setDraft, tagDraft, setTagDraft, activeSlotId, setActiveSlotId, onSave }: { workspace: string; draft: Teller | null; setDraft: (draft: Teller | null) => void; tagDraft: string; setTagDraft: (value: string) => void; activeSlotId: string; setActiveSlotId: (id: string) => void; onSave: () => void }) {
  const { t } = useTranslation()
  const activeSlot = draft?.slots?.find((slot) => slot.id === activeSlotId) || draft?.slots?.[0] || null
  const [targetPickerOpen, setTargetPickerOpen] = useState(false)
  const [randomEventRateInput, setRandomEventRateInput] = useState(() => formatRandomEventRate(draft?.random_event_rate))
  const [styleReferences, setStyleReferences] = useState<StyleReference[]>([])

  const refreshStyleReferences = async () => {
    if (!workspace) {
      setStyleReferences([])
      return []
    }
    try {
      const refs = await getStyleReferences()
      setStyleReferences(refs)
      return refs
    } catch (err) {
      console.warn('[teller-editor] 加载共享文风参考失败', err)
      setStyleReferences([])
      return []
    }
  }

  useEffect(() => {
    void refreshStyleReferences()
  }, [workspace])

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
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-xs text-[var(--nova-text-faint)]">{draft.custom ? t('settingPanel.custom') : draft.builtin_overridden ? t('settingPanel.builtInOverridden') : t('settingPanel.builtIn')}</span>
        </div>
      </div>

      <div className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
        <div className="mb-3">
          <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.styleRules.title')}</div>
          <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.styleRules.desc')}</div>
        </div>
        <InteractiveStyleReferencesEditor
          references={styleReferences}
          refreshReferences={refreshStyleReferences}
          globalRefs={draft.style_refs ?? []}
          onGlobalRefsChange={(style_refs) => setDraft({ ...draft, style_refs })}
          rules={draft.style_rules ?? []}
          onRulesChange={(style_rules) => setDraft({ ...draft, style_rules })}
        />
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
                  <ToggleSwitch checked={slot.enabled} onChange={(enabled) => updateSlotById(slot.id, { enabled })} />
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
                autoResize={false}
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

function InteractiveStyleReferencesEditor({ references, refreshReferences, globalRefs, onGlobalRefsChange, rules, onRulesChange }: { references: StyleReference[]; refreshReferences: () => Promise<StyleReference[]>; globalRefs: string[]; onGlobalRefsChange: (refs: string[]) => void; rules: StyleRule[]; onRulesChange: (rules: StyleRule[]) => void }) {
  const { t } = useTranslation()
  const addRule = () => onRulesChange([...rules, { scene: '', style_refs: [] }])
  const removeRule = (index: number) => onRulesChange(rules.filter((_, i) => i !== index))
  const updateRule = (index: number, patch: Partial<StyleRule>) => {
    onRulesChange(rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)))
  }

  return (
    <div className="flex flex-col gap-2">
      <InteractiveGlobalStyleRuleRow references={references} refreshReferences={refreshReferences} refs={globalRefs} onChange={onGlobalRefsChange} />
      {rules.length > 0 && (
        <div className="space-y-2">
          {rules.map((rule, index) => (
            <InteractiveStyleRuleRow key={index} references={references} refreshReferences={refreshReferences} rule={rule} onChange={(patch) => updateRule(index, patch)} onRemove={() => removeRule(index)} />
          ))}
        </div>
      )}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <Button className={actionButtonClassName} variant="outline" size="sm" onClick={addRule}>
            <Plus className="h-3.5 w-3.5" />
            {t('settingPanel.style.addRule')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function InteractiveGlobalStyleRuleRow({ references, refreshReferences, refs, onChange }: { references: StyleReference[]; refreshReferences: () => Promise<StyleReference[]>; refs: string[]; onChange: (refs: string[]) => void }) {
  const { t } = useTranslation()
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <StyleReferenceControls
        references={references}
        refreshReferences={refreshReferences}
        refs={refs}
        onRefsChange={onChange}
        prefix={<Input className={`${inputClassName} cursor-default md:min-w-44 md:flex-1`} readOnly aria-label={t('settingPanel.style.scope')} value={t('settingPanel.style.scopeGlobal')} />}
      />
    </div>
  )
}

function InteractiveStyleRuleRow({ references, refreshReferences, rule, onChange, onRemove }: { references: StyleReference[]; refreshReferences: () => Promise<StyleReference[]>; rule: StyleRule; onChange: (patch: Partial<StyleRule>) => void; onRemove: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2">
      <StyleReferenceControls
        references={references}
        refreshReferences={refreshReferences}
        refs={rule.style_refs || []}
        contents={rule.style_contents || []}
        onRefsChange={(style_refs) => onChange({ style_refs })}
        onContentsChange={(style_contents) => onChange({ style_contents })}
        prefix={<Input className={`${inputClassName} md:min-w-44 md:flex-1`} value={rule.scene} placeholder={t('settingPanel.placeholder.scene')} onChange={(event) => onChange({ scene: event.target.value })} />}
        extraActions={(
          <Button className={`${actionButtonClassName} justify-center hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]`} variant="outline" size="sm" onClick={onRemove}>
            <Trash2 className="h-3.5 w-3.5" />
            {t('common.delete')}
          </Button>
        )}
      />
    </div>
  )
}

function StyleReferenceControls({ references, refreshReferences, refs, contents = [], onRefsChange, onContentsChange, prefix, extraActions }: { references: StyleReference[]; refreshReferences: () => Promise<StyleReference[]>; refs: string[]; contents?: string[]; onRefsChange: (refs: string[]) => void; onContentsChange?: (contents: string[]) => void; prefix?: ReactNode; extraActions?: ReactNode }) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [uploadDraft, setUploadDraft] = useState<StyleUploadDraft | null>(null)
  const [uploading, setUploading] = useState<'extract' | 'direct' | null>(null)
  const [uploadError, setUploadError] = useState('')
  const [uploadNotice, setUploadNotice] = useState('')
  const [extractMessages, setExtractMessages] = useState<ChatMessage[]>([])
  const [uploadDocument, setUploadDocument] = useState<StyleReferenceFileDocument | null>(null)
  const [editOpen, setEditOpen] = useState(false)
  const [editLoading, setEditLoading] = useState(false)
  const [editSaving, setEditSaving] = useState(false)
  const [editError, setEditError] = useState('')
  const [editDocument, setEditDocument] = useState<StyleReferenceFileDocument | null>(null)
  const [editContent, setEditContent] = useState('')
  const [editPath, setEditPath] = useState('')
  const summary = refs.length === 0 && contents.length === 0 ? t('settingPanel.style.noSelected') : t('settingPanel.style.button', { count: refs.length + contents.length })

  const addRef = (path: string) => {
    const normalized = path.trim()
    if (!normalized || refs.includes(normalized)) return
    onRefsChange([...refs, normalized])
  }

  const removeRef = (path: string) => {
    onRefsChange(refs.filter((item) => item !== path))
  }

  const toggleRef = (path: string) => {
    if (refs.includes(path)) removeRef(path)
    else addRef(path)
  }

  const removeLegacyContent = (index: number) => {
    onContentsChange?.(contents.filter((_, i) => i !== index))
  }

  const openUploadDialog = (draft: StyleUploadDraft = emptyStyleUploadDraft()) => {
    setUploadDraft(draft)
    setUploadError('')
    setUploadNotice('')
    setExtractMessages([])
    setUploadDocument(null)
    setUploadOpen(true)
  }

  const openStyleEditor = async (path: string) => {
    const normalized = path.trim()
    if (!normalized) return
    setPickerOpen(false)
    setEditOpen(true)
    setEditLoading(true)
    setEditSaving(false)
    setEditError('')
    setEditDocument(null)
    setEditContent('')
    setEditPath(normalized)
    try {
      const doc = await readStyleReferenceFile(normalized)
      setEditDocument(doc)
      setEditContent(doc.content)
      setEditPath(doc.reference.display_path || normalized)
    } catch (err) {
      setEditError(err instanceof Error ? err.message : t('settingPanel.style.editLoadFailed'))
    } finally {
      setEditLoading(false)
    }
  }

  const saveStyleEditor = async () => {
    if (!editPath || editLoading || editSaving) return
    if (!editContent.trim()) {
      setEditError(t('settingPanel.style.editEmpty'))
      return
    }
    setEditSaving(true)
    setEditError('')
    try {
      await updateStyleReferenceFile({
        path: editDocument?.reference.display_path || editPath,
        content: editContent,
        base_revision: editDocument?.revision || '',
      })
      await refreshReferences()
      setEditOpen(false)
      setEditDocument(null)
      setEditContent('')
      setEditPath('')
    } catch (err) {
      setEditError(err instanceof Error ? err.message : t('settingPanel.style.editSaveFailed'))
    } finally {
      setEditSaving(false)
    }
  }

  const handleFileSelected = async (file: File | undefined) => {
    if (!file) return
    try {
      const content = limitStyleSource(await readTextFile(file))
      openUploadDialog({
        name: filenameTitle(file.name),
        description: t('settingPanel.style.uploadDescription', { filename: file.name }),
        filename: markdownFilename(file.name),
        content,
      })
      setUploadError('')
      setUploadOpen(true)
    } catch (err) {
      console.warn('[teller-editor] 读取风格内容文件失败', err)
    } finally {
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const saveUploadDraft = async () => {
    if (!uploadDraft || uploading) return
    setUploading('direct')
    setUploadError('')
    setUploadNotice('')
    try {
      const request = normalizeStyleUploadDraft(uploadDraft)
      if (uploadDocument) {
        const doc = await updateStyleReferenceFile({
          path: uploadDocument.reference.display_path,
          content: request.content,
          base_revision: uploadDocument.revision || '',
        })
        await refreshReferences()
        addRef(doc.reference.display_path)
        setUploadDocument(doc)
      } else {
        const ref = await saveStyleReference(request)
        await refreshReferences()
        addRef(ref.display_path)
      }
      setUploadOpen(false)
      setUploadDraft(null)
      setUploadDocument(null)
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : t('settingPanel.style.uploadFailed'))
    } finally {
      setUploading(null)
    }
  }

  const extractWithAgent = async () => {
    if (!uploadDraft || uploading) return
    setUploading('extract')
    setUploadError('')
    setUploadNotice('')
    setUploadDocument(null)
    try {
      const request = normalizeStyleUploadDraft(uploadDraft)
      const targetPath = styleReferenceTargetPath(request)
      setExtractMessages([
        {
          id: `style-extract-user-${Date.now()}`,
          role: 'user',
          content: `${t('settingPanel.style.extractSave')}: ${request.name}`,
        },
        {
          id: `style-extract-connect-${Date.now()}`,
          role: 'system',
          content: t('settingPanel.style.extractProgress.connecting'),
        },
      ])
      const stream = await runConfigManagerStream({
        origin: 'teller',
        resource_id: '__style_reference_extract__',
        instruction: buildStyleExtractionInstruction(request),
        context: {
          style_reference_target: targetPath,
          style_reference_name: request.name,
          style_reference_mode: 'extract_and_write_markdown',
        },
      })
      const toolArgsByKey: Record<string, { name: string; args: string }> = {}
      let generated = ''
      const reader = stream.getReader()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const payload = parseConfigManagerPayload<ConfigManagerToolPayload>(value.data)
        setExtractMessages((current) => reduceConfigManagerMessages(current, value, {
          idPrefix: 'style-extract',
          toolLabel: t('configManager.tool'),
          failureMessage: t('settingPanel.style.extractFailed'),
        }))
        if (value.event === 'chunk') {
          generated += readString(payload?.content)
          continue
        }
        if (value.event === 'tool_call') {
          const key = configManagerToolKey(payload)
          const name = readString(payload?.name)
          if (key && name) toolArgsByKey[key] = { name, args: readString(payload?.args) }
          continue
        }
        if (value.event === 'tool_args_delta') {
          const key = configManagerToolKey(payload)
          if (key) {
            const current = toolArgsByKey[key] || { name: readString(payload?.name), args: '' }
            toolArgsByKey[key] = { ...current, name: current.name || readString(payload?.name), args: `${current.args}${readString(payload?.delta)}` }
          }
          continue
        }
        if (value.event === 'tool_result') {
          const key = configManagerToolKey(payload)
          const name = readString(payload?.name)
          if (key && toolArgsByKey[key]) toolArgsByKey[key] = { ...toolArgsByKey[key], name: toolArgsByKey[key].name || name }
          continue
        }
        if (value.event === 'error') {
          throw new Error(readString((payload as { message?: string } | null)?.message) || t('settingPanel.style.extractFailed'))
        }
      }
      const markdownFromTool = extractStyleReferenceMarkdownFromToolArgs(toolArgsByKey)
      const fallbackMarkdown = normalizeExtractedStyleMarkdown(markdownFromTool || generated, request)
      const doc = await readExtractedStyleReferenceDocument(targetPath, request, fallbackMarkdown, t('settingPanel.style.extractMissing'))
      const updated = await refreshReferences()
      const created = updated.find((item) => item.display_path === doc.reference.display_path) || doc.reference
      addRef(created.display_path)
      setUploadDocument(doc)
      setUploadDraft({
        ...request,
        name: created.name || request.name,
        description: created.description || request.description,
        filename: filenameFromStyleReferencePath(created.display_path || targetPath),
        content: limitStyleSource(doc.content),
      })
      setUploadNotice(t('settingPanel.style.extractSaved', { path: created.display_path }))
      setExtractMessages((current) => appendConfigManagerMessage(current, {
        role: 'system',
        content: `${t('settingPanel.style.extractProgress.saved')}: ${created.display_path}`,
      }, 'style-extract'))
    } catch (err) {
      setUploadError(err instanceof Error ? err.message : t('settingPanel.style.extractFailed'))
      setExtractMessages((current) => appendConfigManagerMessage(current, {
        role: 'error',
        content: err instanceof Error ? err.message : t('settingPanel.style.extractFailed'),
      }, 'style-extract'))
    } finally {
      setUploading(null)
    }
  }

  return (
    <div className="min-w-0">
      <input ref={fileInputRef} type="file" accept={STYLE_FILE_ACCEPT} className="hidden" onChange={(event) => void handleFileSelected(event.target.files?.[0])} />
      <div className="flex flex-col gap-2 md:flex-row md:flex-wrap md:items-center">
        {prefix}
        <Popover open={pickerOpen} onOpenChange={setPickerOpen}>
          <PopoverTrigger asChild>
            <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm">
              <FileText className="h-3.5 w-3.5" />
              {t('settingPanel.style.pickReference')}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="nova-panel max-h-72 w-72 overflow-y-auto border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 text-[var(--nova-text)]">
            {references.length === 0 ? (
              <div className="px-2 py-3 text-xs text-[var(--nova-text-faint)]">{t('settingPanel.style.noReferences')}</div>
            ) : references.map((ref) => {
              const selected = refs.includes(ref.display_path)
              return (
                <div key={ref.display_path} className="flex w-full min-w-0 items-start gap-1 rounded-md text-xs hover:bg-[var(--nova-hover)]">
                  <button type="button" className="flex min-w-0 flex-1 items-start gap-2 px-2 py-2 text-left" onClick={() => toggleRef(ref.display_path)}>
                    <Check className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${selected ? 'opacity-100' : 'opacity-0'}`} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[var(--nova-text)]">{ref.name || ref.display_path}</span>
                      <span className="mt-0.5 block truncate text-[11px] text-[var(--nova-text-faint)]">{ref.description || ref.display_path}</span>
                    </span>
                  </button>
                  <Button className={`${iconActionClassName} m-1 shrink-0`} variant="outline" size="icon" onClick={() => void openStyleEditor(ref.display_path)} aria-label={t('settingPanel.style.editReference', { name: ref.name || ref.display_path })} title={t('settingPanel.style.editReference', { name: ref.name || ref.display_path })}>
                    <Edit3 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              )
            })}
          </PopoverContent>
        </Popover>
        <Button className={`${actionButtonClassName} justify-center`} variant="outline" size="sm" onClick={() => openUploadDialog()}>
          <Upload className="h-3.5 w-3.5" />
          {t('settingPanel.style.upload')}
        </Button>
        {extraActions}
      </div>

      <div className="mt-1 truncate px-1 text-xs text-[var(--nova-text-faint)]">→ {summary}</div>
      {(refs.length > 0 || contents.length > 0) && (
        <div className="mt-2 grid gap-1.5">
          {refs.map((path) => {
            const ref = references.find((item) => item.display_path === path)
            return (
              <div key={path} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs">
                <button type="button" className="flex min-w-0 flex-1 items-center gap-2 text-left" onClick={() => void openStyleEditor(path)} aria-label={t('settingPanel.style.editReference', { name: ref?.name || path })}>
                  <FileText className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />
                  <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]" title={path}>{ref?.name || path}</span>
                  <span className="hidden max-w-56 truncate text-[11px] text-[var(--nova-text-faint)] md:block">{path}</span>
                </button>
                <Button className={`${iconActionClassName} hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]`} variant="outline" size="icon" onClick={() => removeRef(path)} aria-label={t('common.delete')}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            )
          })}
          {contents.map((content, index) => (
            <div key={`legacy-${index}`} className="flex min-w-0 items-center gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2 py-1.5 text-xs">
              <span className="rounded border border-[var(--nova-border)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.style.legacyInline')}</span>
              <span className="min-w-0 flex-1 truncate text-[var(--nova-text-muted)]" title={content}>{contentPreview(content)}</span>
              {onContentsChange && (
                <Button className={`${iconActionClassName} hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]`} variant="outline" size="icon" onClick={() => removeLegacyContent(index)} aria-label={t('common.delete')}>
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
          ))}
        </div>
      )}
      <Dialog open={uploadOpen} onOpenChange={(open) => {
        if (!open && uploading) return
        setUploadOpen(open)
      }}>
        <DialogContent className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] w-[min(980px,calc(100vw-2rem))] max-w-[min(980px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('settingPanel.style.uploadDialogTitle')}</DialogTitle>
            <DialogDescription className="mt-1 text-xs text-[var(--nova-text-faint)]">{t('settingPanel.style.uploadDialogDesc')}</DialogDescription>
          </div>
          <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto p-4 md:grid-cols-[minmax(0,1fr)_minmax(260px,320px)] md:overflow-hidden">
            <div className="flex flex-col md:min-h-0">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                  <Upload className="h-3.5 w-3.5" />
                  {t('settingPanel.style.selectFile')}
                </Button>
                <span className="text-[11px] text-[var(--nova-text-faint)]">{(uploadDraft?.content || '').length}/{STYLE_SOURCE_LIMIT}</span>
              </div>
              <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px]">
                <Field label={t('settingPanel.field.name')}>
                  <Input className={inputClassName} value={uploadDraft?.name || ''} onChange={(event) => setUploadDraft((draft) => draft ? { ...draft, name: event.target.value } : draft)} />
                </Field>
                <Field label={t('settingPanel.style.filename')}>
                  <Input
                    className={inputClassName}
                    value={uploadDraft?.filename || ''}
                    onChange={(event) => {
                      setUploadDocument(null)
                      setUploadDraft((draft) => draft ? { ...draft, filename: markdownFilename(event.target.value) } : draft)
                    }}
                  />
                </Field>
              </div>
              <Field className="mt-3" label={t('settingPanel.field.description')}>
                <Input className={inputClassName} value={uploadDraft?.description || ''} onChange={(event) => setUploadDraft((draft) => draft ? { ...draft, description: event.target.value } : draft)} />
              </Field>
              <Textarea
                autoResize={false}
                className="nova-field mt-3 h-[min(34vh,280px)] min-h-[160px] resize-none overflow-y-auto text-sm leading-6 shadow-none [field-sizing:fixed] focus-visible:ring-0 md:h-[min(42vh,360px)] md:min-h-0"
                value={uploadDraft?.content || ''}
                onChange={(event) => {
                  setUploadNotice('')
                  setUploadDraft((draft) => draft ? { ...draft, content: limitStyleSource(event.target.value) } : draft)
                }}
                placeholder={t('settingPanel.style.pastePlaceholder')}
              />
              <div className="mt-2 flex items-center justify-between gap-3 text-[11px] text-[var(--nova-text-faint)]">
                <span className={`min-w-0 truncate text-left ${uploadError ? 'text-[var(--nova-danger)]' : uploadNotice ? 'text-[var(--nova-accent-green)]' : ''}`}>{uploadError || uploadNotice}</span>
              </div>
            </div>
            <StyleExtractionChatPanel messages={extractMessages} active={uploading === 'extract'} />
          </div>
          <DialogFooter className="!mx-0 !mb-0 rounded-none border-t border-[var(--nova-border)] bg-[var(--nova-surface)]/95 !px-4 !py-3">
            <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => setUploadOpen(false)} disabled={uploading !== null}>{t('common.cancel')}</Button>
            <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => void saveUploadDraft()} disabled={!uploadDraft?.content.trim() || uploading !== null}>
              {uploading === 'direct' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : uploadDocument ? <Save className="h-3.5 w-3.5" /> : <Upload className="h-3.5 w-3.5" />}
              {uploadDocument ? t('common.save') : t('settingPanel.style.directSave')}
            </Button>
            <Button className="nova-nav-item gap-1.5 border border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] hover:border-[var(--nova-accent)] hover:bg-[var(--nova-hover)]" size="sm" onClick={() => void extractWithAgent()} disabled={!uploadDraft?.content.trim() || uploading !== null}>
              {uploading === 'extract' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
              {t('settingPanel.style.extractSave')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={editOpen} onOpenChange={(open) => {
        if (!open && editSaving) return
        setEditOpen(open)
        if (!open) {
          setEditError('')
          setEditDocument(null)
          setEditContent('')
          setEditPath('')
        }
      }}>
        <DialogContent className="nova-panel flex max-h-[min(760px,calc(100vh-2rem))] w-[min(860px,calc(100vw-2rem))] max-w-[min(860px,calc(100vw-2rem))] flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0 text-[var(--nova-text)] shadow-[var(--nova-shadow)]">
          <div className="border-b border-[var(--nova-border)] px-4 py-3">
            <DialogTitle className="text-sm font-semibold text-[var(--nova-text)]">{t('settingPanel.style.editDialogTitle')}</DialogTitle>
            <DialogDescription className="mt-1 truncate text-xs text-[var(--nova-text-faint)]">{editDocument?.reference.display_path || editPath || t('settingPanel.style.editDialogDesc')}</DialogDescription>
          </div>
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden p-4">
            {editLoading ? (
              <div className="flex min-h-[320px] items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] text-xs text-[var(--nova-text-faint)]">
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                {t('common.loading')}
              </div>
            ) : (
              <>
                <Textarea
                  autoResize={false}
                  className="nova-field h-[min(52vh,460px)] min-h-[320px] resize-none overflow-y-auto font-mono text-sm leading-6 shadow-none [field-sizing:fixed] focus-visible:ring-0"
                  value={editContent}
                  onChange={(event) => {
                    setEditError('')
                    setEditContent(event.target.value)
                  }}
                  onKeyDown={(event) => {
                    if (isSaveShortcut(event)) {
                      event.preventDefault()
                      event.stopPropagation()
                      void saveStyleEditor()
                    }
                  }}
                  placeholder={t('settingPanel.style.editPlaceholder')}
                />
                <div className="mt-2 flex min-h-5 items-center justify-between gap-3 text-[11px] text-[var(--nova-text-faint)]">
                  <span className={`min-w-0 truncate text-left ${editError ? 'text-[var(--nova-danger)]' : ''}`}>{editError}</span>
                  <span className="shrink-0">{editContent.length}</span>
                </div>
              </>
            )}
          </div>
          <DialogFooter className="!mx-0 !mb-0 rounded-none border-t border-[var(--nova-border)] bg-[var(--nova-surface)]/95 !px-4 !py-3">
            <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => setEditOpen(false)} disabled={editSaving}>{t('common.cancel')}</Button>
            <Button className="nova-nav-item gap-1.5 border border-[var(--nova-accent)]/45 bg-[var(--nova-active)] text-[var(--nova-text)] hover:border-[var(--nova-accent)] hover:bg-[var(--nova-hover)]" size="sm" onClick={() => void saveStyleEditor()} disabled={editLoading || editSaving || !editContent.trim()}>
              {editSaving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
              {editSaving ? t('common.saving') : t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

interface StyleUploadDraft {
  name: string
  description: string
  filename: string
  content: string
}

function StyleExtractionChatPanel({ messages, active }: { messages: ChatMessage[]; active: boolean }) {
  const { t } = useTranslation()
  return (
    <aside className="flex min-h-[220px] min-w-0 flex-col overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)]/75">
      <div className="flex h-10 shrink-0 items-center justify-between border-b border-[var(--nova-border)] px-3">
        <div className="flex min-w-0 items-center gap-2 text-xs font-medium text-[var(--nova-text)]">
          {active ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-[var(--nova-accent)]" /> : <Sparkles className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)]" />}
          <span className="truncate">{t('settingPanel.style.extractProgress.title')}</span>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {messages.length === 0 && !active ? (
          <div className="m-2 rounded-md border border-dashed border-[var(--nova-border)] px-3 py-4 text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.style.extractProgress.empty')}</div>
        ) : (
          <MessageList
            messages={messages}
            isStreaming={active}
            activityContent=""
            scrollResetKey="style-extraction-progress"
            bottomPaddingClassName="pb-4"
            messageStyle={{ fontSize: '12px', lineHeight: 1.55 }}
            collapseTraceBeforeAssistant
          />
        )}
      </div>
    </aside>
  )
}

function limitStyleSource(value: string) {
  return Array.from(value).slice(0, STYLE_SOURCE_LIMIT).join('')
}

function contentPreview(value: string) {
  const normalized = value.replace(/\s+/g, ' ').trim()
  return normalized.length > 80 ? `${normalized.slice(0, 80)}...` : normalized
}

function filenameTitle(filename: string) {
  return filename.replace(/\.[^.]+$/, '').replace(/[-_]+/g, ' ').trim() || 'style-reference'
}

function markdownFilename(filename: string) {
  const base = filenameTitle(filename)
    .replace(/[\\/:*?"<>|]+/g, '')
    .replace(/\s+/g, '-')
    .replace(/^\.+|\.+$/g, '')
    .trim() || `style-${Date.now()}`
  return `${base.replace(/\.(md|markdown|txt)$/i, '')}.md`
}

function emptyStyleUploadDraft(): StyleUploadDraft {
  return {
    name: '',
    description: '',
    filename: markdownFilename(`style-${Date.now()}.md`),
    content: '',
  }
}

function normalizeStyleUploadDraft(draft: StyleUploadDraft): StyleUploadDraft {
  const filename = markdownFilename(draft.filename || draft.name || `style-${Date.now()}.md`)
  return {
    name: draft.name.trim() || filenameTitle(filename),
    description: draft.description.trim(),
    filename,
    content: limitStyleSource(draft.content),
  }
}

function styleReferenceTargetPath(draft: StyleUploadDraft) {
  return `.denova/styles/${draft.filename}`
}

function filenameFromStyleReferencePath(path: string) {
  const filename = path.split('/').pop() || path
  return markdownFilename(filename)
}

async function readExtractedStyleReferenceDocument(targetPath: string, request: StyleUploadDraft, fallbackMarkdown: string, missingMessage: string): Promise<StyleReferenceFileDocument> {
  try {
    return await readStyleReferenceFile(targetPath)
  } catch (err) {
    if (!fallbackMarkdown) throw new Error(missingMessage)
    const ref = await saveStyleReference({ ...request, content: fallbackMarkdown })
    return readStyleReferenceFile(ref.display_path)
  }
}

function buildStyleExtractionInstruction(draft: StyleUploadDraft) {
  return `请把用户提供的源文件提炼为共享小说文风参考 Markdown，并写入目标文风参考文件。

要求：
1. 使用 Markdown，标题为「${draft.name}」。
2. 内容以从源文件中提炼出的典型参考段落为主，辅以少量风格总结、写法引导和硬性规则。
3. 不出现现实作者名、作品名或来源说明。
4. 不要直接保存原文，不要堆砌华丽辞藻，不要写成口号。
5. 参考结构可以包含「总体原则」「场景/心理/对白/感情/战斗/日常/出场/转折/结尾」等小节，但只保留源文件能支持的内容。
6. 必须调用 write_style_references，写入 filename="${draft.filename}"，name="${draft.name}"，description="${draft.description}"，content 为最终提炼后的 Markdown。
7. 不要调用 write_tellers 或其他叙事风格写入工具；本次只处理共享文风参考文件。
8. 如果 write_style_references 工具不可用，才输出以下 XML 标签包裹的 Markdown 作为回退，不要在标签外写解释：

<${STYLE_MARKDOWN_TAG}>
# ${draft.name}

...
</${STYLE_MARKDOWN_TAG}>

源文件内容如下：

\`\`\`text
${draft.content}
\`\`\``
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function extractStyleReferenceMarkdownFromToolArgs(toolArgsByKey: Record<string, { name: string; args: string }>) {
  for (const call of Object.values(toolArgsByKey)) {
    if (call.name !== 'write_style_references') continue
    const content = extractFirstStyleReferenceContent(call.args)
    if (content) return content
  }
  return ''
}

function extractFirstStyleReferenceContent(rawArgs: string) {
  if (!rawArgs.trim()) return ''
  try {
    const data = JSON.parse(rawArgs) as unknown
    if (!data || typeof data !== 'object' || Array.isArray(data)) return ''
    const operations = (data as { operations?: unknown }).operations
    if (!Array.isArray(operations)) return ''
    for (const operation of operations) {
      if (!operation || typeof operation !== 'object' || Array.isArray(operation)) continue
      const reference = (operation as { reference?: unknown }).reference
      if (!reference || typeof reference !== 'object' || Array.isArray(reference)) continue
      const content = readString((reference as { content?: unknown }).content)
      if (content.trim()) return content
    }
  } catch {
    return ''
  }
  return ''
}

function normalizeExtractedStyleMarkdown(value: string, draft: StyleUploadDraft) {
  let markdown = stripStyleMarkdownEnvelope(value).trim()
  markdown = stripMarkdownFence(markdown).trim()
  if (!markdown) return ''
  if (!/^#\s+/m.test(markdown)) {
    markdown = `# ${draft.name}\n\n${markdown}`
  }
  return limitStyleSource(markdown)
}

function stripStyleMarkdownEnvelope(value: string) {
  const pattern = new RegExp(`<${STYLE_MARKDOWN_TAG}>\\s*([\\s\\S]*?)\\s*</${STYLE_MARKDOWN_TAG}>`, 'i')
  const match = value.match(pattern)
  return match?.[1] || value
}

function stripMarkdownFence(value: string) {
  const trimmed = value.trim()
  const match = trimmed.match(/^```(?:markdown|md)?\s*\n([\s\S]*?)\n```$/i)
  return match?.[1] || value
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

function ToggleSwitch({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useTranslation()
  const label = checked ? t('settingPanel.switch.disableRule') : t('settingPanel.switch.enableRule')
  return (
    <Switch checked={checked} onCheckedChange={onChange} aria-label={label} title={label} />
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
