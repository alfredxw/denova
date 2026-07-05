import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Copy, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { EventPackageModule, OpeningTrait, OpeningTraitPool, RuleCheck, StateOp, StoryDirectorActorStateSystem, StoryDirectorAttribute, StoryDirectorOpeningSelector, StoryDirectorStatSystem, StoryDirectorTRPGSystem, TellerEventCard } from '../../types'
import { SortablePresetList } from './SortablePresetList'
import { cloneWithNewId, formatPresetJSON, itemKey, joinListInput, nextPresetId, parseIntegerInput, parseNumberInput, splitListInput } from './utils'

const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 text-xs focus:ring-0'
const iconActionClassName = 'nova-nav-item border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const visualEditorShellClassName = 'grid h-[clamp(360px,calc(100dvh-15rem),720px)] min-h-0 gap-3 overflow-hidden lg:grid-cols-[260px_minmax(0,1fr)]'
const detailScrollPaneClassName = 'min-h-0 overflow-y-auto pr-1 [scrollbar-gutter:stable]'

export function EventPackageVisualEditor({
  value,
  onChange,
  onValidityChange,
}: {
  value: EventPackageModule
  onChange: (value: EventPackageModule) => void
  onValidityChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeCardId, setActiveCardId] = useState('')
  const cards = value.events || []

  useEffect(() => onValidityChange(true), [onValidityChange])
  useEffect(() => {
    if (!cards.some((card, index) => itemKey(card, index, 'card') === activeCardId)) {
      setActiveCardId(cards[0] ? itemKey(cards[0], 0, 'card') : '')
    }
  }, [activeCardId, cards])

  const setCards = (events: TellerEventCard[]) => onChange({ ...value, events })
  const activeIndex = cards.findIndex((card, index) => itemKey(card, index, 'card') === activeCardId)
  const activeCard = activeIndex >= 0 ? cards[activeIndex] : null
  const patchCard = (patch: Partial<TellerEventCard>) => {
    if (!activeCard) return
    setCards(cards.map((card, index) => (index === activeIndex ? { ...card, ...patch } : card)))
  }
  const addCard = () => {
    const card: TellerEventCard = { id: nextPresetId('event-card'), type_name: '', enabled: true }
    setCards([...cards, card])
    setActiveCardId(card.id || '')
  }
  const copyCard = () => {
    if (!activeCard) return
    const card = cloneWithNewId(activeCard, 'event-card')
    setCards([...cards, card])
    setActiveCardId(card.id || '')
  }
  const deleteCard = () => {
    if (!activeCard) return
    const next = cards.filter((_, index) => index !== activeIndex)
    setCards(next)
    setActiveCardId(next[0] ? itemKey(next[0], 0, 'card') : '')
  }

  return (
    <div className={visualEditorShellClassName} data-testid="event-package-card-editor">
      <SortablePresetList
        items={cards}
        activeId={activeCardId}
        getId={(card, index) => itemKey(card, index, 'card')}
        getTitle={(card, index) => card.type_name || card.id || `${t('settingPanel.presetConfig.eventCard')} ${index + 1}`}
        getSubtitle={(card) => [card.category, card.intensity].filter(Boolean).join(' · ')}
        addLabel={t('settingPanel.presetConfig.addEventCard')}
        emptyLabel={t('settingPanel.presetConfig.eventCards')}
        onAdd={addCard}
        onActiveIdChange={setActiveCardId}
        onItemsChange={setCards}
      />
      <div className={detailScrollPaneClassName} data-testid="event-package-card-detail-scroll">
        {activeCard ? <EventCardDetails item={activeCard} onPatch={patchCard} onCopy={copyCard} onDelete={deleteCard} /> : <EmptyDetail>{t('settingPanel.presetConfig.emptyEventCards')}</EmptyDetail>}
      </div>
    </div>
  )
}

function EventCardDetails({
  item,
  onPatch,
  onCopy,
  onDelete,
}: {
  item: TellerEventCard
  onPatch: (patch: Partial<TellerEventCard>) => void
  onCopy: () => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  return (
    <DetailPanel dense>
      <DetailActions onCopy={onCopy} onDelete={onDelete} />
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={item.id || ''} onChange={(event) => onPatch({ id: event.target.value })} /></Field>
        <Field label={t('settingPanel.presetConfig.typeName')}><Input className={inputClassName} value={item.type_name || ''} onChange={(event) => onPatch({ type_name: event.target.value })} /></Field>
        <Field label={t('settingPanel.orchestration.category')}><Input className={inputClassName} value={item.category || ''} onChange={(event) => onPatch({ category: event.target.value })} /></Field>
        <Field label={t('settingPanel.field.tags')}><Input className={inputClassName} value={joinListInput(item.tags)} onChange={(event) => onPatch({ tags: splitListInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.weight')}><Input className={inputClassName} inputMode="decimal" value={String(item.weight ?? '')} onChange={(event) => onPatch({ weight: parseNumberInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.cooldown')}><Input className={inputClassName} inputMode="numeric" value={String(item.cooldown_turns ?? '')} onChange={(event) => onPatch({ cooldown_turns: parseIntegerInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.intensity')}><Input className={inputClassName} value={item.intensity || ''} onChange={(event) => onPatch({ intensity: event.target.value })} /></Field>
        <SwitchField label={t('settingPanel.field.enabled')} checked={item.enabled !== false} onChange={(enabled) => onPatch({ enabled })} />
      </div>
      <Field label={t('settingPanel.presetConfig.descriptionMarkdown')}>
        <Textarea className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={item.description_markdown || ''} onChange={(event) => onPatch({ description_markdown: event.target.value })} />
      </Field>
    </DetailPanel>
  )
}

export function StatSystemVisualEditor({
  value,
  onChange,
  onValidityChange,
}: {
  value: StoryDirectorStatSystem
  onChange: (value: StoryDirectorStatSystem) => void
  onValidityChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState('')
  const attributes = value.attributes || []
  useEffect(() => onValidityChange(true), [onValidityChange])
  useEffect(() => {
    if (!attributes.some((item, index) => statAttributeId(item, index) === activeId)) {
      setActiveId(attributes[0] ? statAttributeId(attributes[0], 0) : '')
    }
  }, [activeId, attributes])
  const setAttributes = (next: StoryDirectorAttribute[]) => onChange({ ...value, attributes: next })
  const activeIndex = attributes.findIndex((item, index) => statAttributeId(item, index) === activeId)
  const active = activeIndex >= 0 ? attributes[activeIndex] : null
  const patchActive = (patch: Partial<StoryDirectorAttribute>) => {
    if (!active) return
    setAttributes(attributes.map((item, index) => (index === activeIndex ? { ...item, ...patch } : item)))
  }
  const addAttribute = () => {
    const id = nextPresetId('attribute')
    const item: StoryDirectorAttribute = { id, path: `state.${id.replace(/-/g, '_')}`, name: '', visibility: 'visible' }
    setAttributes([...attributes, item])
    setActiveId(id)
  }
  const copyAttribute = () => {
    if (!active) return
    const item = cloneWithNewId(active, 'attribute')
    setAttributes([...attributes, item])
    setActiveId(statAttributeId(item, attributes.length))
  }
  const deleteAttribute = () => {
    if (!active) return
    const next = attributes.filter((_, index) => index !== activeIndex)
    setAttributes(next)
    setActiveId(next[0] ? statAttributeId(next[0], 0) : '')
  }
  return (
    <div className="grid min-h-[360px] gap-3 lg:grid-cols-[260px_minmax(0,1fr)]">
      <SortablePresetList
        items={attributes}
        activeId={activeId}
        getId={statAttributeId}
        getTitle={(item, index) => item.name || item.path || `${t('settingPanel.presetConfig.attribute')} ${index + 1}`}
        getSubtitle={(item) => [item.type, item.visibility].filter(Boolean).join(' · ')}
        addLabel={t('settingPanel.presetConfig.addAttribute')}
        emptyLabel={t('settingPanel.storyDirector.statSystem')}
        onAdd={addAttribute}
        onActiveIdChange={setActiveId}
        onItemsChange={setAttributes}
      />
      {active ? (
        <DetailPanel>
          <DetailActions onCopy={copyAttribute} onDelete={deleteAttribute} />
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={active.id || ''} onChange={(event) => patchActive({ id: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.path')}><Input className={inputClassName} value={active.path || ''} onChange={(event) => patchActive({ path: event.target.value })} /></Field>
            <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={active.name || ''} onChange={(event) => patchActive({ name: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.type')}><Input className={inputClassName} value={active.type || ''} onChange={(event) => patchActive({ type: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.defaultValue')}><Input className={inputClassName} inputMode="decimal" value={String(active.default ?? '')} onChange={(event) => patchActive({ default: parseNumberInput(event.target.value) })} /></Field>
            <Field label={t('settingPanel.presetConfig.min')}><Input className={inputClassName} inputMode="decimal" value={String(active.min ?? '')} onChange={(event) => patchActive({ min: parseNumberInput(event.target.value) })} /></Field>
            <Field label={t('settingPanel.presetConfig.max')}><Input className={inputClassName} inputMode="decimal" value={String(active.max ?? '')} onChange={(event) => patchActive({ max: parseNumberInput(event.target.value) })} /></Field>
            <Field label={t('settingPanel.presetConfig.visibility')}>
              <Select value={active.visibility || 'visible'} onValueChange={(visibility) => patchActive({ visibility: visibility as StoryDirectorAttribute['visibility'] })}>
                <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectItem value="visible">visible</SelectItem>
                  <SelectItem value="hidden">hidden</SelectItem>
                  <SelectItem value="spoiler">spoiler</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field label={t('common.description')}>
            <Textarea className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={active.description || ''} onChange={(event) => patchActive({ description: event.target.value })} />
          </Field>
        </DetailPanel>
      ) : <EmptyDetail>{t('settingPanel.presetConfig.emptyAttributes')}</EmptyDetail>}
    </div>
  )
}

export function ActorStateVisualEditor({
  value,
  onChange,
  onValidityChange,
  resetKey,
}: {
  value: StoryDirectorActorStateSystem
  onChange: (value: StoryDirectorActorStateSystem) => void
  onValidityChange: (valid: boolean) => void
  resetKey?: string
}) {
  const { t } = useTranslation()
  const initialText = formatPresetJSON(value || { templates: [], initial_actors: [] })
  const [text, setText] = useState(initialText)
  const [error, setError] = useState('')
  const lastAcceptedValueRef = useRef(initialText)

  useEffect(() => {
    const next = formatPresetJSON(value || { templates: [], initial_actors: [] })
    lastAcceptedValueRef.current = next
    setText(next)
    setError('')
  }, [resetKey])

  useEffect(() => {
    const next = formatPresetJSON(value || { templates: [], initial_actors: [] })
    if (next !== lastAcceptedValueRef.current && (!error || text === lastAcceptedValueRef.current)) {
      lastAcceptedValueRef.current = next
      setText(next)
      setError('')
    }
  }, [error, text, value])

  useEffect(() => {
    onValidityChange(!error)
  }, [error, onValidityChange])

  const updateText = (next: string) => {
    setText(next)
    try {
      const parsed = JSON.parse(next || '{}') as StoryDirectorActorStateSystem
      const accepted = {
        templates: Array.isArray(parsed.templates) ? parsed.templates : [],
        initial_actors: Array.isArray(parsed.initial_actors) ? parsed.initial_actors : [],
      }
      lastAcceptedValueRef.current = formatPresetJSON(accepted)
      setError('')
      onChange(accepted)
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="grid gap-2">
      <Textarea
        className="nova-field h-[clamp(360px,calc(100dvh-18rem),720px)] resize-none font-mono text-xs leading-5 shadow-none focus-visible:ring-0"
        value={text}
        onChange={(event) => updateText(event.target.value)}
        spellCheck={false}
      />
      <p className={`text-[11px] ${error ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]'}`}>
        {error || t('settingPanel.actorState.jsonHint')}
      </p>
    </div>
  )
}

export function TRPGSystemVisualEditor({
  value,
  onChange,
  onValidityChange,
}: {
  value: StoryDirectorTRPGSystem
  onChange: (value: StoryDirectorTRPGSystem) => void
  onValidityChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeId, setActiveId] = useState('')
  const [successOpsValid, setSuccessOpsValid] = useState(true)
  const [failureOpsValid, setFailureOpsValid] = useState(true)
  const rules = value.rule_templates || []
  useEffect(() => onValidityChange(successOpsValid && failureOpsValid), [failureOpsValid, onValidityChange, successOpsValid])
  useEffect(() => {
    if (!rules.some((item, index) => itemKey(item, index, 'rule') === activeId)) {
      setActiveId(rules[0] ? itemKey(rules[0], 0, 'rule') : '')
    }
    setSuccessOpsValid(true)
    setFailureOpsValid(true)
  }, [activeId, rules])
  const setRules = (rule_templates: RuleCheck[]) => onChange({ ...value, rule_templates })
  const activeIndex = rules.findIndex((item, index) => itemKey(item, index, 'rule') === activeId)
  const active = activeIndex >= 0 ? rules[activeIndex] : null
  const patchActive = (patch: Partial<RuleCheck>) => {
    if (!active) return
    setRules(rules.map((item, index) => (index === activeIndex ? { ...item, ...patch } : item)))
  }
  const addRule = () => {
    const item: RuleCheck = { id: nextPresetId('rule'), label: '', kind: 'dice', mode: 'default' }
    setRules([...rules, item])
    setActiveId(item.id || '')
  }
  const copyRule = () => {
    if (!active) return
    const item = cloneWithNewId(active, 'rule')
    setRules([...rules, item])
    setActiveId(item.id || '')
  }
  const deleteRule = () => {
    if (!active) return
    const next = rules.filter((_, index) => index !== activeIndex)
    setRules(next)
    setActiveId(next[0] ? itemKey(next[0], 0, 'rule') : '')
  }
  return (
    <div className="grid min-h-[360px] gap-3 lg:grid-cols-[260px_minmax(0,1fr)]">
      <SortablePresetList
        items={rules}
        activeId={activeId}
        getId={(item, index) => itemKey(item, index, 'rule')}
        getTitle={(item, index) => item.label || item.id || `${t('settingPanel.orchestration.ruleTemplates')} ${index + 1}`}
        getSubtitle={(item) => [item.kind, item.mode, item.dice].filter(Boolean).join(' · ')}
        addLabel={t('settingPanel.orchestration.addRuleTemplate')}
        emptyLabel={t('settingPanel.orchestration.ruleTemplates')}
        onAdd={addRule}
        onActiveIdChange={setActiveId}
        onItemsChange={setRules}
      />
      {active ? (
        <DetailPanel>
          <DetailActions onCopy={copyRule} onDelete={deleteRule} />
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            <Field label={t('settingPanel.orchestration.ruleId')}><Input className={inputClassName} value={active.id || ''} onChange={(event) => patchActive({ id: event.target.value })} /></Field>
            <Field label={t('settingPanel.orchestration.ruleLabel')}><Input className={inputClassName} value={active.label || ''} onChange={(event) => patchActive({ label: event.target.value })} /></Field>
            <Field label={t('settingPanel.orchestration.ruleKind')}><Input className={inputClassName} value={active.kind || ''} onChange={(event) => patchActive({ kind: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.mode')}>
              <Select value={active.mode || 'default'} onValueChange={(mode) => patchActive({ mode: mode as RuleCheck['mode'] })}>
                <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
                <SelectContent className="nova-panel border text-[var(--nova-text)]">
                  <SelectItem value="default">default</SelectItem>
                  <SelectItem value="d20_dc">d20_dc</SelectItem>
                  <SelectItem value="d100_under">d100_under</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Field label={t('settingPanel.presetConfig.attributePath')}><Input className={inputClassName} value={active.attribute_path || ''} onChange={(event) => patchActive({ attribute_path: event.target.value })} /></Field>
            <Field label={t('settingPanel.orchestration.ruleExpression')}><Input className={inputClassName} value={active.expression || ''} onChange={(event) => patchActive({ expression: event.target.value })} /></Field>
            <Field label={t('settingPanel.orchestration.ruleDice')}><Input className={inputClassName} value={active.dice || ''} onChange={(event) => patchActive({ dice: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.modifier')}><Input className={inputClassName} inputMode="decimal" value={String(active.modifier ?? '')} onChange={(event) => patchActive({ modifier: parseNumberInput(event.target.value) })} /></Field>
            <Field label={t('settingPanel.orchestration.ruleDifficulty')}><Input className={inputClassName} inputMode="decimal" value={String(active.difficulty ?? '')} onChange={(event) => patchActive({ difficulty: parseNumberInput(event.target.value) })} /></Field>
            <Field label={t('settingPanel.presetConfig.resourceCostPath')}><Input className={inputClassName} value={active.resource_cost_path || ''} onChange={(event) => patchActive({ resource_cost_path: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.resourceCost')}><Input className={inputClassName} inputMode="decimal" value={String(active.resource_cost ?? '')} onChange={(event) => patchActive({ resource_cost: parseNumberInput(event.target.value) })} /></Field>
            <SwitchField label={t('settingPanel.presetConfig.terminalOnFailure')} checked={active.terminal_on_failure === true} onChange={(terminal_on_failure) => patchActive({ terminal_on_failure })} />
            <Field label={t('settingPanel.presetConfig.terminalType')}><Input className={inputClassName} value={active.terminal_type || ''} onChange={(event) => patchActive({ terminal_type: event.target.value })} /></Field>
            <Field label={t('settingPanel.presetConfig.terminalReason')}><Input className={inputClassName} value={active.terminal_reason || ''} onChange={(event) => patchActive({ terminal_reason: event.target.value })} /></Field>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <JSONFragmentEditor
              label={t('settingPanel.presetConfig.successStateOps')}
              value={active.success_state_ops || []}
              expected="array"
              onChange={(success_state_ops) => patchActive({ success_state_ops: success_state_ops as StateOp[] })}
              onValidChange={setSuccessOpsValid}
            />
            <JSONFragmentEditor
              label={t('settingPanel.presetConfig.failureStateOps')}
              value={active.failure_state_ops || []}
              expected="array"
              onChange={(failure_state_ops) => patchActive({ failure_state_ops: failure_state_ops as StateOp[] })}
              onValidChange={setFailureOpsValid}
            />
          </div>
        </DetailPanel>
      ) : <EmptyDetail>{t('settingPanel.orchestration.noRuleTemplates')}</EmptyDetail>}
    </div>
  )
}

export function OpeningSelectorVisualEditor({
  value,
  onChange,
  onValidityChange,
}: {
  value: StoryDirectorOpeningSelector
  onChange: (value: StoryDirectorOpeningSelector) => void
  onValidityChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activePoolId, setActivePoolId] = useState('')
  const [activeTraitId, setActiveTraitId] = useState('')
  const [initialOpsValid, setInitialOpsValid] = useState(true)
  const [traitOpsValid, setTraitOpsValid] = useState(true)
  const pools = value.trait_pools || []
  useEffect(() => onValidityChange(initialOpsValid && traitOpsValid), [initialOpsValid, onValidityChange, traitOpsValid])
  useEffect(() => {
    if (!pools.some((item, index) => itemKey(item, index, 'pool') === activePoolId)) {
      setActivePoolId(pools[0] ? itemKey(pools[0], 0, 'pool') : '')
    }
    setTraitOpsValid(true)
  }, [activePoolId, pools])
  const setPools = (trait_pools: OpeningTraitPool[]) => onChange({ ...value, trait_pools })
  const activePoolIndex = pools.findIndex((item, index) => itemKey(item, index, 'pool') === activePoolId)
  const activePool = activePoolIndex >= 0 ? pools[activePoolIndex] : null
  const patchPool = (patch: Partial<OpeningTraitPool>) => {
    if (!activePool) return
    setPools(pools.map((item, index) => (index === activePoolIndex ? { ...item, ...patch } : item)))
  }
  const addPool = () => {
    const item: OpeningTraitPool = { id: nextPresetId('trait-pool'), name: '', draw_count: 1, traits: [] }
    setPools([...pools, item])
    setActivePoolId(item.id || '')
  }
  const copyPool = () => {
    if (!activePool) return
    const item = cloneWithNewId(activePool, 'trait-pool')
    setPools([...pools, item])
    setActivePoolId(item.id || '')
  }
  const deletePool = () => {
    if (!activePool) return
    const next = pools.filter((_, index) => index !== activePoolIndex)
    setPools(next)
    setActivePoolId(next[0] ? itemKey(next[0], 0, 'pool') : '')
  }

  return (
    <div className="grid gap-3">
      <div className="grid gap-3 md:grid-cols-[220px_minmax(0,1fr)]">
        <SwitchField label={t('settingPanel.field.enabled')} checked={value.enabled !== false} onChange={(enabled) => onChange({ ...value, enabled })} />
        <JSONFragmentEditor
          label={t('settingPanel.presetConfig.initialStateOps')}
          value={value.initial_state_ops || []}
          expected="array"
          onChange={(initial_state_ops) => onChange({ ...value, initial_state_ops: initial_state_ops as StateOp[] })}
          onValidChange={setInitialOpsValid}
        />
      </div>
      <div className="grid min-h-[360px] gap-3 lg:grid-cols-[260px_minmax(0,1fr)]">
        <SortablePresetList
          items={pools}
          activeId={activePoolId}
          getId={(item, index) => itemKey(item, index, 'pool')}
          getTitle={(item, index) => item.name || item.id || `${t('settingPanel.presetConfig.traitPool')} ${index + 1}`}
          getSubtitle={(item) => `${t('settingPanel.presetConfig.drawCount')}: ${item.draw_count ?? 1} · ${(item.traits || []).length}`}
          addLabel={t('settingPanel.presetConfig.addTraitPool')}
          emptyLabel={t('settingPanel.presetConfig.traitPools')}
          onAdd={addPool}
          onActiveIdChange={setActivePoolId}
          onItemsChange={setPools}
        />
        {activePool ? (
          <TraitPoolDetails
            item={activePool}
            activeTraitId={activeTraitId}
            setActiveTraitId={setActiveTraitId}
            onPatch={patchPool}
            onCopy={copyPool}
            onDelete={deletePool}
            onValidChange={setTraitOpsValid}
          />
        ) : <EmptyDetail>{t('settingPanel.presetConfig.emptyTraitPools')}</EmptyDetail>}
      </div>
    </div>
  )
}

function TraitPoolDetails({
  item,
  activeTraitId,
  setActiveTraitId,
  onPatch,
  onCopy,
  onDelete,
  onValidChange,
}: {
  item: OpeningTraitPool
  activeTraitId: string
  setActiveTraitId: (id: string) => void
  onPatch: (patch: Partial<OpeningTraitPool>) => void
  onCopy: () => void
  onDelete: () => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const traits = item.traits || []
  const activeIndex = traits.findIndex((trait, index) => itemKey(trait, index, 'trait') === activeTraitId)
  const activeTrait = activeIndex >= 0 ? traits[activeIndex] : null
  useEffect(() => {
    if (!traits.some((trait, index) => itemKey(trait, index, 'trait') === activeTraitId)) {
      setActiveTraitId(traits[0] ? itemKey(traits[0], 0, 'trait') : '')
    }
  }, [activeTraitId, setActiveTraitId, traits])
  const setTraits = (next: OpeningTrait[]) => onPatch({ traits: next })
  const patchTrait = (patch: Partial<OpeningTrait>) => {
    if (!activeTrait) return
    setTraits(traits.map((trait, index) => (index === activeIndex ? { ...trait, ...patch } : trait)))
  }
  const addTrait = () => {
    const trait: OpeningTrait = { id: nextPresetId('trait'), name: '', weight: 1, ops: [] }
    setTraits([...traits, trait])
    setActiveTraitId(trait.id || '')
  }
  const copyTrait = () => {
    if (!activeTrait) return
    const trait = cloneWithNewId(activeTrait, 'trait')
    setTraits([...traits, trait])
    setActiveTraitId(trait.id || '')
  }
  const deleteTrait = () => {
    if (!activeTrait) return
    const next = traits.filter((_, index) => index !== activeIndex)
    setTraits(next)
    setActiveTraitId(next[0] ? itemKey(next[0], 0, 'trait') : '')
  }
  return (
    <DetailPanel>
      <DetailActions onCopy={onCopy} onDelete={onDelete} />
      <div className="grid gap-3 md:grid-cols-3">
        <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={item.id || ''} onChange={(event) => onPatch({ id: event.target.value })} /></Field>
        <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={item.name || ''} onChange={(event) => onPatch({ name: event.target.value })} /></Field>
        <Field label={t('settingPanel.presetConfig.drawCount')}><Input className={inputClassName} inputMode="numeric" value={String(item.draw_count ?? '')} onChange={(event) => onPatch({ draw_count: parseIntegerInput(event.target.value) })} /></Field>
      </div>
      <div className="grid min-h-[300px] gap-3 lg:grid-cols-[240px_minmax(0,1fr)]">
        <SortablePresetList
          items={traits}
          activeId={activeTraitId}
          getId={(trait, index) => itemKey(trait, index, 'trait')}
          getTitle={(trait, index) => trait.name || trait.id || `${t('settingPanel.presetConfig.trait')} ${index + 1}`}
          getSubtitle={(trait) => trait.summary || ''}
          addLabel={t('settingPanel.presetConfig.addTrait')}
          emptyLabel={t('settingPanel.presetConfig.traits')}
          onAdd={addTrait}
          onActiveIdChange={setActiveTraitId}
          onItemsChange={setTraits}
        />
        {activeTrait ? (
          <DetailPanel dense>
            <DetailActions onCopy={copyTrait} onDelete={deleteTrait} />
            <div className="grid gap-3 md:grid-cols-3">
              <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={activeTrait.id || ''} onChange={(event) => patchTrait({ id: event.target.value })} /></Field>
              <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={activeTrait.name || ''} onChange={(event) => patchTrait({ name: event.target.value })} /></Field>
              <Field label={t('settingPanel.presetConfig.weight')}><Input className={inputClassName} inputMode="decimal" value={String(activeTrait.weight ?? '')} onChange={(event) => patchTrait({ weight: parseNumberInput(event.target.value) })} /></Field>
            </div>
            <Field label={t('settingPanel.presetConfig.summary')}><Textarea className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={activeTrait.summary || ''} onChange={(event) => patchTrait({ summary: event.target.value })} /></Field>
            <JSONFragmentEditor
              label={t('settingPanel.presetConfig.traitOps')}
              value={activeTrait.ops || []}
              expected="array"
              onChange={(ops) => patchTrait({ ops: ops as StateOp[] })}
              onValidChange={onValidChange}
            />
          </DetailPanel>
        ) : <EmptyDetail>{t('settingPanel.presetConfig.emptyTraits')}</EmptyDetail>}
      </div>
    </DetailPanel>
  )
}

function JSONFragmentEditor({
  label,
  value,
  expected,
  onChange,
  onValidChange,
}: {
  label: string
  value: unknown
  expected: 'array' | 'object'
  onChange: (value: unknown) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const valueSignature = useMemo(() => JSON.stringify(value ?? (expected === 'array' ? [] : {})), [expected, value])
  const [text, setText] = useState(() => formatPresetJSON(value ?? (expected === 'array' ? [] : {})))
  const [error, setError] = useState('')

  useEffect(() => {
    setText(JSON.stringify(value ?? (expected === 'array' ? [] : {}), null, 2))
    setError('')
    onValidChange(true)
  }, [expected, onValidChange, valueSignature])

  const update = (next: string) => {
    setText(next)
    try {
      const parsed = JSON.parse(next)
      if (expected === 'array' && !Array.isArray(parsed)) throw new Error(t('settingPanel.presetConfig.jsonArrayRequired'))
      if (expected === 'object' && (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))) throw new Error(t('settingPanel.storyDirector.jsonObjectRequired'))
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
      <Textarea className="nova-field min-h-28 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0" value={text} onChange={(event) => update(event.target.value)} />
      {error ? <div className="mt-1 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{error}</div> : null}
    </Field>
  )
}

function statAttributeId(item: StoryDirectorAttribute, index: number) {
  return item.id || item.path || `attribute-${index}`
}

function DetailPanel({ children, dense = false, className = '' }: { children: ReactNode; dense?: boolean; className?: string }) {
  return <section className={`min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] ${dense ? 'p-3' : 'p-4'} grid gap-3 ${className}`}>{children}</section>
}

function EmptyDetail({ children }: { children: ReactNode }) {
  return <div className="flex min-h-40 items-center justify-center rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-6 text-xs text-[var(--nova-text-faint)]">{children}</div>
}

function DetailActions({ onCopy, onDelete }: { onCopy: () => void; onDelete: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex justify-end gap-2">
      <Button className={iconActionClassName} variant="outline" size="icon-sm" onClick={onCopy} aria-label={t('settingPanel.presetConfig.copy')} title={t('settingPanel.presetConfig.copy')}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
      <Button className={iconActionClassName} variant="outline" size="icon-sm" onClick={onDelete} aria-label={t('common.delete')} title={t('common.delete')}>
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid min-w-0 gap-1 text-xs text-[var(--nova-text-muted)]">
      <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function SwitchField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="flex items-end">
      <div className="flex h-8 w-full items-center justify-between gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2">
        <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
        <Switch checked={checked} onCheckedChange={onChange} />
      </div>
    </div>
  )
}
