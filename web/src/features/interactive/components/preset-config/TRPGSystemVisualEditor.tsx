import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Copy, Database, Dice5, ExternalLink, Plus, Scale, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import type {
  ActorStateModule,
  ActorStateTemplate,
  RuleCheck,
  RuleStateBinding,
  StoryDirectorActorStateSystem,
  StoryDirectorTRPGSystem,
} from '../../types'
import {
  defaultRuleTemplates,
  normalizeRuleTemplate,
  normalizeTRPGSystem,
  RULE_FAILURE_POLICY_OPTIONS,
} from './ruleTemplates'
import { PresetTabsList } from './PresetTabsList'
import {
  cloneWithNewId,
  formatPresetJSON,
  itemKey,
  nextPresetId,
  parseNumberInput,
} from './utils'

const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 w-full text-xs focus:ring-0'
const iconActionClassName = 'nova-nav-item rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const actionButtonClassName = 'nova-nav-item gap-1.5 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const fieldGridClassName = 'grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-3'
const nestedEditorShellClassName = 'grid min-h-0 grid-cols-[repeat(auto-fit,minmax(min(100%,16rem),1fr))] gap-2'
const detailScrollPaneClassName = 'min-w-0 overflow-hidden rounded-[14px] bg-[var(--nova-surface)] p-3'

/** Edits one d20 adjudication style through its operational decision flow. */
export function TRPGSystemVisualEditor({
  value,
  actorStateId,
  actorStates = [],
  onChange,
  onActorStateChange,
  onOpenActorState,
  onValidityChange,
}: {
  value: StoryDirectorTRPGSystem
  actorStateId?: string
  actorStates?: ActorStateModule[]
  onChange: (value: StoryDirectorTRPGSystem) => void
  onActorStateChange?: (id: string) => void
  onOpenActorState?: (id: string) => void
  onValidityChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [bindingsValid, setBindingsValid] = useState(true)
  const [activeSection, setActiveSection] = useState('when')

  useEffect(() => onValidityChange(bindingsValid), [bindingsValid, onValidityChange])

  const active = normalizeTRPGSystem(value).rule_templates?.[0] || defaultRuleTemplates()[0]
  const selectedActorState = actorStates.find((item) => item.id === actorStateId)
  const patchActive = (patch: Partial<RuleCheck>) => {
    onChange({ ...value, rule_templates: [normalizeRuleTemplate({ ...active, ...patch }, 0)] })
  }

  return (
    <Tabs value={activeSection} onValueChange={setActiveSection} className="trpg-system-workspace min-h-0 gap-2.5">
      <div className="flex shrink-0 flex-col gap-2 px-1 pt-0.5">
        <p className="max-w-[76ch] text-[11px] leading-4 text-[var(--nova-text-faint)]">
          {t('settingPanel.trpgRule.singleConfigDesc')}
        </p>
        <TabsList variant="line" className="trpg-workflow-tabs grid !h-auto w-full grid-cols-[repeat(auto-fit,minmax(min(100%,10rem),1fr))] items-stretch gap-1 overflow-x-clip rounded-none border-b border-[var(--nova-border)] bg-transparent p-0">
          <WorkflowTab value="when" icon={Dice5} label={t('settingPanel.trpgRule.flow.when')} description={t('settingPanel.trpgRule.flow.whenDesc')} />
          <WorkflowTab value="outcome" icon={Scale} label={t('settingPanel.trpgRule.flow.outcome')} description={t('settingPanel.trpgRule.flow.outcomeDesc')} />
          <WorkflowTab value="state" icon={Database} label={t('settingPanel.trpgRule.flow.state')} description={t('settingPanel.trpgRule.flow.stateDesc')} />
        </TabsList>
      </div>

      <TabsContent value="when" className="mt-0">
        <DetailPanel>
          <Field label={t('settingPanel.orchestration.ruleLabel')}><Input className={inputClassName} value={active.label || ''} onChange={(event) => patchActive({ label: event.target.value })} /></Field>
          <Field label={t('settingPanel.trpgRule.trigger')}><Textarea autoResize={false} className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.trigger || ''} onChange={(event) => patchActive({ trigger: event.target.value })} /></Field>
          <div className={fieldGridClassName}>
            <Field label={t('settingPanel.trpgRule.mustCheckExamples')}><Textarea autoResize={false} className="nova-field min-h-32 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={joinExampleLines(active.must_check_examples)} onChange={(event) => patchActive({ must_check_examples: splitExampleLines(event.target.value) })} placeholder={t('settingPanel.trpgRule.examplesPlaceholder')} /></Field>
            <Field label={t('settingPanel.trpgRule.skipCheckExamples')}><Textarea autoResize={false} className="nova-field min-h-32 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={joinExampleLines(active.skip_check_examples)} onChange={(event) => patchActive({ skip_check_examples: splitExampleLines(event.target.value) })} placeholder={t('settingPanel.trpgRule.examplesPlaceholder')} /></Field>
          </div>
        </DetailPanel>
      </TabsContent>

      <TabsContent value="outcome" className="mt-0">
        <DetailPanel>
          <div className={fieldGridClassName}>
            <Field label={t('settingPanel.trpgRule.dice')}><Input className={inputClassName} value={t('settingPanel.trpgRule.fixedD20')} disabled /></Field>
            <Field label={t('settingPanel.trpgRule.modifier')}><Input className={inputClassName} inputMode="decimal" value={String(active.modifier ?? 0)} onChange={(event) => patchActive({ modifier: parseNumberInput(event.target.value) })} /></Field>
            <RuleSelectField label={t('settingPanel.trpgRule.failurePolicy')} value={active.failure_policy || 'fail_forward'} options={RULE_FAILURE_POLICY_OPTIONS} labelFor={(next) => ruleFailurePolicyLabel(next, t)} onChange={(failure_policy) => patchActive({ failure_policy })} />
          </div>
          <div className={fieldGridClassName}>
            <Field label={t('settingPanel.trpgRule.difficultyGuidance')}><Textarea autoResize={false} className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.difficulty_guidance || ''} onChange={(event) => patchActive({ difficulty_guidance: event.target.value })} /></Field>
            <Field label={t('settingPanel.trpgRule.stateEffectGuidance')}><Textarea autoResize={false} className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.state_effect_guidance || ''} onChange={(event) => patchActive({ state_effect_guidance: event.target.value })} /></Field>
          </div>
          <div className={fieldGridClassName}>
            <Field label={t('settingPanel.trpgRule.successHint')}><Textarea autoResize={false} className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.success_hint || ''} onChange={(event) => patchActive({ success_hint: event.target.value })} /></Field>
            <Field label={t('settingPanel.trpgRule.failureHint')}><Textarea autoResize={false} className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.failure_hint || ''} onChange={(event) => patchActive({ failure_hint: event.target.value })} /></Field>
          </div>
        </DetailPanel>
      </TabsContent>

      <TabsContent value="state" forceMount hidden={activeSection !== 'state'} className="mt-0">
        <DetailPanel>
          <div className={`${fieldGridClassName} items-end`}>
            <Field label={t('settingPanel.trpgRule.actorStateBinding')}>
              <Select value={actorStateId || '__none__'} onValueChange={(next) => onActorStateChange?.(next === '__none__' ? '' : next)}>
                <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectGroup>
                    <SelectItem value="__none__">{t('settingPanel.trpgRule.noActorStateBinding')}</SelectItem>
                    {actorStates.map((item) => <SelectItem key={item.id} value={item.id}>{item.name || item.id}</SelectItem>)}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            {selectedActorState && onOpenActorState ? (
              <Button type="button" variant="outline" size="sm" className={actionButtonClassName} onClick={() => onOpenActorState(selectedActorState.id)}>
                <ExternalLink data-icon="inline-start" />
                {t('settingPanel.trpgRule.flow.openState')}
              </Button>
            ) : null}
          </div>
          <p className="text-[11px] leading-5 text-[var(--nova-text-faint)]">
            {selectedActorState ? t('settingPanel.trpgRule.flow.stateDesc') : t('settingPanel.trpgRule.flow.chooseStateFirst')}
          </p>
        </DetailPanel>
        {selectedActorState ? (
          <StateBindingEditor
            value={active.state_bindings || []}
            actorState={selectedActorState.actor_state}
            onChange={(state_bindings) => patchActive({ state_bindings })}
            onValidChange={setBindingsValid}
          />
        ) : (
          <div className="flex min-h-48 flex-col items-center justify-center gap-2 rounded-[14px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface)] px-6 py-8 text-center">
            <Database className="size-5 text-[var(--nova-text-faint)]" />
            <div className="text-sm font-medium text-[var(--nova-text)]">{t('settingPanel.trpgRule.flow.noStateTitle')}</div>
            <div className="max-w-[52ch] text-xs leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.trpgRule.flow.noStateDesc')}</div>
          </div>
        )}
      </TabsContent>
    </Tabs>
  )
}

function WorkflowTab({ value, icon: Icon, label, description }: { value: string; icon: typeof Dice5; label: string; description: string }) {
  return (
    <TabsTrigger value={value} className="h-auto min-w-0 w-full justify-start gap-2 rounded-[8px_8px_0_0] px-2.5 py-2 text-left after:bottom-0">
      <Icon data-icon="inline-start" />
      <span className="flex min-w-0 flex-col items-start">
        <span className="text-xs font-semibold">{label}</span>
        <span className="trpg-workflow-tab-description line-clamp-2 text-[10px] font-normal leading-4 text-[var(--nova-text-faint)]">{description}</span>
      </span>
    </TabsTrigger>
  )
}

function StateBindingEditor({
  value,
  actorState,
  onChange,
  onValidChange,
}: {
  value: RuleStateBinding[]
  actorState?: StoryDirectorActorStateSystem
  onChange: (value: RuleStateBinding[]) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState('')
  const [modifiersValid, setModifiersValid] = useState(true)
  const [refsValid, setRefsValid] = useState(true)
  const [changesValid, setChangesValid] = useState(true)
  const templates = actorState?.templates || []

  useEffect(() => onValidChange(modifiersValid && refsValid && changesValid), [changesValid, modifiersValid, onValidChange, refsValid])
  useEffect(() => {
    if (!value.some((item, index) => itemKey(item, index, 'binding') === activeId)) {
      setActiveId(value[0] ? itemKey(value[0], 0, 'binding') : '')
    }
  }, [activeId, value])

  const activeIndex = value.findIndex((item, index) => itemKey(item, index, 'binding') === activeId)
  const active = activeIndex >= 0 ? value[activeIndex] : null
  const setItems = (items: RuleStateBinding[]) => onChange(items.slice(0, 12))
  const patchActive = (patch: Partial<RuleStateBinding>) => {
    if (!active) return
    const nextBinding = { ...active, ...patch }
    if (patch.id !== undefined) setActiveId(itemKey(nextBinding, activeIndex, 'binding'))
    setItems(value.map((item, index) => (index === activeIndex ? nextBinding : item)))
  }
  const addBinding = () => {
    const item: RuleStateBinding = {
      id: nextPresetId('binding'),
      label: '',
      trigger: '',
      actor_template_id: templates[0]?.id || '',
      modifiers: [],
      narrative_state_refs: [],
      outcome_state_changes: [],
    }
    setItems([...value, item])
    setActiveId(item.id || '')
  }
  const copyBinding = () => {
    if (!active) return
    const item = cloneWithNewId(active, 'binding')
    setItems([...value, item])
    setActiveId(item.id || '')
  }
  const deleteBinding = () => {
    if (!active) return
    const next = value.filter((_, index) => index !== activeIndex)
    setItems(next)
    setActiveId(next[0] ? itemKey(next[0], 0, 'binding') : '')
  }

  return (
    <DetailPanel>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.trpgRule.stateBindings')}</div>
          <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{t('settingPanel.trpgRule.stateBindingsDesc')}</div>
        </div>
        <Button type="button" className={actionButtonClassName} variant="outline" size="sm" onClick={addBinding} disabled={value.length >= 12}>
          <Plus className="h-3.5 w-3.5" />
          {t('settingPanel.trpgRule.addBinding')}
        </Button>
      </div>
      <div className={nestedEditorShellClassName}>
        <PresetTabsList
          items={value}
          activeId={activeId}
          getId={(item, index) => itemKey(item, index, 'binding')}
          getTitle={(item, index) => item.label || item.id || `${t('settingPanel.trpgRule.binding')} ${index + 1}`}
          getSubtitle={(item) => `${item.actor_template_id || '-'}${item.target_template_id ? ` -> ${item.target_template_id}` : ''}`}
          addLabel={t('settingPanel.trpgRule.addBinding')}
          emptyLabel={t('settingPanel.trpgRule.emptyBindings')}
          layout="rail"
          testIdPrefix="trpg-state-bindings"
          onAdd={addBinding}
          onActiveIdChange={setActiveId}
          onItemsChange={setItems}
        />
        <div className={detailScrollPaneClassName}>
          {active ? (
            <div className="grid gap-3">
              <div className="flex justify-end gap-2">
                <Button type="button" className={iconActionClassName} variant="outline" size="icon" onClick={copyBinding} aria-label={t('settingPanel.presetConfig.copy')}><Copy className="h-3.5 w-3.5" /></Button>
                <Button type="button" className={iconActionClassName} variant="outline" size="icon" onClick={deleteBinding} aria-label={t('common.delete')}><Trash2 className="h-3.5 w-3.5" /></Button>
              </div>
              <div className={fieldGridClassName}>
                <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={active.id || ''} onChange={(event) => patchActive({ id: event.target.value })} /></Field>
                <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={active.label || ''} onChange={(event) => patchActive({ label: event.target.value })} /></Field>
                <TemplateSelectField label={t('settingPanel.trpgRule.actorTemplate')} value={active.actor_template_id || ''} templates={templates} onChange={(actor_template_id) => patchActive({ actor_template_id })} />
                <TemplateSelectField label={t('settingPanel.trpgRule.targetTemplate')} value={active.target_template_id || ''} templates={templates} allowEmpty onChange={(target_template_id) => patchActive({ target_template_id })} />
              </div>
              <Field label={t('settingPanel.trpgRule.trigger')}><Textarea autoResize={false} className="nova-field min-h-16 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.trigger || ''} onChange={(event) => patchActive({ trigger: event.target.value })} /></Field>
              <StateFieldReferenceEditor binding={active} templates={templates} onChange={patchActive} />
              <JSONFragmentEditor label={t('settingPanel.trpgRule.modifiers')} value={active.modifiers || []} onChange={(modifiers) => patchActive({ modifiers: modifiers as RuleStateBinding['modifiers'] })} onValidChange={setModifiersValid} />
              <JSONFragmentEditor label={t('settingPanel.trpgRule.narrativeStateRefs')} value={active.narrative_state_refs || []} onChange={(narrative_state_refs) => patchActive({ narrative_state_refs: narrative_state_refs as RuleStateBinding['narrative_state_refs'] })} onValidChange={setRefsValid} />
              <JSONFragmentEditor label={t('settingPanel.trpgRule.outcomeStateChanges')} value={active.outcome_state_changes || []} onChange={(outcome_state_changes) => patchActive({ outcome_state_changes: outcome_state_changes as RuleStateBinding['outcome_state_changes'] })} onValidChange={setChangesValid} />
            </div>
          ) : <EmptyDetail>{t('settingPanel.trpgRule.emptyBindings')}</EmptyDetail>}
        </div>
      </div>
    </DetailPanel>
  )
}

function StateFieldReferenceEditor({
  binding,
  templates,
  onChange,
}: {
  binding: RuleStateBinding
  templates: ActorStateTemplate[]
  onChange: (patch: Partial<RuleStateBinding>) => void
}) {
  const { t } = useTranslation()
  const rows: ReactNode[] = []
  const selector = (key: string, label: string, source: string | undefined, fieldId: string | undefined, update: (fieldId: string) => void) => (
    <StateFieldSelect
      key={key}
      label={label}
      source={source}
      fieldId={fieldId}
      actorTemplateId={binding.actor_template_id}
      targetTemplateId={binding.target_template_id}
      templates={templates}
      onChange={update}
    />
  )

  binding.modifiers?.forEach((item, index) => {
    rows.push(selector(`modifier-${index}`, t('settingPanel.trpgRule.fieldReference.modifier', { index: index + 1 }), item.source, item.field_id, (field_id) => {
      onChange({ modifiers: binding.modifiers?.map((candidate, candidateIndex) => candidateIndex === index ? { ...candidate, field_id, field_path: undefined } : candidate) })
    }))
  })
  binding.narrative_state_refs?.forEach((item, index) => {
    if (item.source === 'scene') return
    rows.push(selector(`narrative-${index}`, t('settingPanel.trpgRule.fieldReference.narrative', { index: index + 1 }), item.source, item.field_id, (field_id) => {
      onChange({ narrative_state_refs: binding.narrative_state_refs?.map((candidate, candidateIndex) => candidateIndex === index ? { ...candidate, field_id, field_path: undefined } : candidate) })
    }))
  })
  binding.outcome_state_changes?.forEach((outcome, outcomeIndex) => {
    outcome.state_changes?.forEach((item, changeIndex) => {
      rows.push(selector(`outcome-${outcomeIndex}-${changeIndex}`, t('settingPanel.trpgRule.fieldReference.outcome', { outcome: outcome.outcome || '-', index: changeIndex + 1 }), item.source, item.field_id, (field_id) => {
        onChange({
          outcome_state_changes: binding.outcome_state_changes?.map((candidate, candidateOutcomeIndex) => candidateOutcomeIndex !== outcomeIndex ? candidate : {
            ...candidate,
            state_changes: candidate.state_changes?.map((change, candidateChangeIndex) => candidateChangeIndex === changeIndex ? { ...change, field_id, field_path: undefined } : change),
          }),
        })
      }))
      item.change_formula?.terms?.forEach((term, termIndex) => {
        rows.push(selector(`formula-${outcomeIndex}-${changeIndex}-${termIndex}`, t('settingPanel.trpgRule.fieldReference.formula', { index: termIndex + 1 }), term.source, term.field_id, (field_id) => {
          onChange({
            outcome_state_changes: binding.outcome_state_changes?.map((candidate, candidateOutcomeIndex) => candidateOutcomeIndex !== outcomeIndex ? candidate : {
              ...candidate,
              state_changes: candidate.state_changes?.map((change, candidateChangeIndex) => candidateChangeIndex !== changeIndex ? change : {
                ...change,
                change_formula: {
                  ...change.change_formula,
                  terms: change.change_formula?.terms?.map((candidateTerm, candidateTermIndex) => candidateTermIndex === termIndex ? { ...candidateTerm, field_id, field_path: undefined } : candidateTerm),
                },
              }),
            }),
          })
        }))
      })
    })
  })

  return (
    <div role="group" aria-label={t('settingPanel.trpgRule.fieldReferences')} className="grid gap-2 rounded-[12px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-2.5">
      <div>
        <div className="text-[11px] font-medium text-[var(--nova-text-muted)]">{t('settingPanel.trpgRule.fieldReferences')}</div>
        <div className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('settingPanel.trpgRule.fieldReferencesDesc')}</div>
      </div>
      {rows.length ? <div className={fieldGridClassName}>{rows}</div> : <div className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.trpgRule.fieldReferencesEmpty')}</div>}
    </div>
  )
}

function StateFieldSelect({
  label,
  source,
  fieldId,
  actorTemplateId,
  targetTemplateId,
  templates,
  onChange,
}: {
  label: string
  source?: string
  fieldId?: string
  actorTemplateId?: string
  targetTemplateId?: string
  templates: ActorStateTemplate[]
  onChange: (fieldId: string) => void
}) {
  const templateId = source === 'target' ? targetTemplateId : actorTemplateId
  const fields = templates.find((template) => template.id === templateId)?.fields || []
  const current = String(fieldId || '').normalize('NFKC').trim()
  const options = fields.map((field) => String(field.name || '').normalize('NFKC').trim()).filter(Boolean)
  if (current && !options.includes(current)) options.push(current)
  return (
    <Field label={label}>
      <Select value={current} onValueChange={onChange} disabled={!options.length}>
        <SelectTrigger aria-label={label} className={selectClassName}><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          {options.map((option) => <SelectItem key={option} value={option}>{option}</SelectItem>)}
        </SelectContent>
      </Select>
    </Field>
  )
}

function TemplateSelectField({
  label,
  value,
  templates,
  allowEmpty = false,
  onChange,
}: {
  label: string
  value: string
  templates: ActorStateTemplate[]
  allowEmpty?: boolean
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  if (!templates.length) {
    return <Field label={label}><Input className={inputClassName} value={value} onChange={(event) => onChange(event.target.value)} /></Field>
  }
  const selectValue = value || (allowEmpty ? '__none__' : templates[0]?.id || '__none__')
  return (
    <Field label={label}>
      <Select value={selectValue} onValueChange={(next) => onChange(next === '__none__' ? '' : next)}>
        <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          {allowEmpty ? <SelectItem value="__none__">{t('settingPanel.trpgRule.noTargetTemplate')}</SelectItem> : null}
          {templates.filter((template) => template.id).map((template) => <SelectItem key={template.id} value={template.id}>{template.name || template.id}</SelectItem>)}
        </SelectContent>
      </Select>
    </Field>
  )
}

function RuleSelectField<T extends readonly string[]>({ label, value, options, labelFor, onChange }: {
  label: string
  value: string
  options: T
  labelFor: (value: T[number]) => string
  onChange: (value: T[number]) => void
}) {
  return (
    <Field label={label}>
      <Select value={value} onValueChange={(next) => onChange(next as T[number])}>
        <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
        <SelectContent className="nova-panel border text-[var(--nova-text)]">
          {options.map((option) => <SelectItem key={option} value={option}>{labelFor(option)}</SelectItem>)}
        </SelectContent>
      </Select>
    </Field>
  )
}

function JSONFragmentEditor({ label, value, onChange, onValidChange }: {
  label: string
  value: unknown
  onChange: (value: unknown) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const valueSignature = useMemo(() => JSON.stringify(value ?? []), [value])
  const [text, setText] = useState(() => formatPresetJSON(value ?? []))
  const [error, setError] = useState('')

  useEffect(() => {
    setText(formatPresetJSON(value ?? []))
    setError('')
    onValidChange(true)
  }, [onValidChange, valueSignature])

  const update = (next: string) => {
    setText(next)
    try {
      const parsed = JSON.parse(next)
      if (!Array.isArray(parsed)) throw new Error(t('settingPanel.presetConfig.jsonArrayRequired'))
      setError('')
      onValidChange(true)
      onChange(parsed)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settingPanel.storyDirector.invalidJSON'))
      onValidChange(false)
    }
  }

  return (
    <Field label={label}>
      <Textarea autoResize={false} className="nova-field min-h-28 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0" value={text} onChange={(event) => update(event.target.value)} />
      {error ? <div className="mt-1 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{error}</div> : null}
    </Field>
  )
}

function DetailPanel({ children }: { children: ReactNode }) {
  return <section className="grid min-w-0 gap-3 rounded-[14px] bg-[var(--nova-surface-2)] p-3">{children}</section>
}

function EmptyDetail({ children }: { children: ReactNode }) {
  return <div className="flex min-h-48 items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{children}</div>
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid min-w-0 gap-1.5 text-xs text-[var(--nova-text-muted)]">
      <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function ruleFailurePolicyLabel(value: string | undefined, t: ReturnType<typeof useTranslation>['t']) {
  return t(`settingPanel.trpgRule.failurePolicy.${value || 'fail_forward'}`)
}

function joinExampleLines(values: string[] | undefined) {
  return (values || []).join('\n')
}

function splitExampleLines(value: string) {
  return Array.from(new Set(value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean))).slice(0, 8)
}
