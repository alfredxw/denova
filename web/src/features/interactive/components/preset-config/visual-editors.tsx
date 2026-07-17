import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Copy, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { ActorStateField, ActorStateInitialActor, ActorStateTemplate, EventPackageModule, StoryDirectorActorStateSystem, TellerEventCard } from '../../types'
import { PresetTabsList } from './PresetTabsList'
import { cloneWithNewId, formatPresetJSON, itemKey, joinListInput, nextPresetId, parseIntegerInput, parseNumberInput, splitListInput } from './utils'

const inputClassName = 'nova-field h-8 text-xs focus-visible:ring-0'
const selectClassName = 'nova-field h-8 w-full text-xs focus:ring-0'
const iconActionClassName = 'nova-nav-item rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'
const fieldGridClassName = 'grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-3'
const visualEditorShellClassName = 'preset-visual-editor-shell grid h-full min-h-0 min-w-0 gap-3 overflow-hidden'
const nestedEditorShellClassName = 'grid min-h-0 grid-cols-[repeat(auto-fit,minmax(min(100%,16rem),1fr))] gap-2'
const detailScrollPaneClassName = 'min-w-0 overflow-hidden rounded-[14px] bg-[var(--nova-surface)] p-3'

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
    const nextCard = { ...activeCard, ...patch }
    if (patch.id !== undefined) setActiveCardId(itemKey(nextCard, activeIndex, 'card'))
    setCards(cards.map((card, index) => (index === activeIndex ? nextCard : card)))
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
      <PresetTabsList
        items={cards}
        activeId={activeCardId}
        getId={(card, index) => itemKey(card, index, 'card')}
        getTitle={(card, index) => card.type_name || card.id || `${t('settingPanel.presetConfig.eventCard')} ${index + 1}`}
        getSubtitle={(card) => [card.category, card.intensity].filter(Boolean).join(' · ')}
        addLabel={t('settingPanel.presetConfig.addEventCard')}
        emptyLabel={t('settingPanel.presetConfig.eventCards')}
        layout="rail"
        testIdPrefix="event-package-cards"
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
      <div className={fieldGridClassName}>
        <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={item.id || ''} onChange={(event) => onPatch({ id: event.target.value })} /></Field>
        <Field label={t('settingPanel.presetConfig.typeName')}><Input className={inputClassName} value={item.type_name || ''} onChange={(event) => onPatch({ type_name: event.target.value })} /></Field>
        <Field label={t('settingPanel.orchestration.category')}><Input className={inputClassName} value={item.category || ''} onChange={(event) => onPatch({ category: event.target.value })} /></Field>
        <Field label={t('settingPanel.field.tags')}><Input className={inputClassName} value={joinListInput(item.tags)} onChange={(event) => onPatch({ tags: splitListInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.intensity')}><Input className={inputClassName} value={item.intensity || ''} onChange={(event) => onPatch({ intensity: event.target.value })} /></Field>
        <SwitchField label={t('settingPanel.field.enabled')} checked={item.enabled !== false} onChange={(enabled) => onPatch({ enabled })} />
      </div>
      <Field label={t('settingPanel.presetConfig.descriptionMarkdown')}>
        <Textarea autoResize={false} className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={item.description_markdown || ''} onChange={(event) => onPatch({ description_markdown: event.target.value })} />
      </Field>
    </DetailPanel>
  )
}

export function ActorStateVisualEditor({
  value,
  onChange,
  onValidityChange,
}: {
  value: StoryDirectorActorStateSystem
  onChange: (value: StoryDirectorActorStateSystem) => void
  onValidityChange: (valid: boolean) => void
  resetKey?: string
}) {
  const { t } = useTranslation()
  const [activeTemplateId, setActiveTemplateId] = useState('')
  const [activeFieldId, setActiveFieldId] = useState('')
  const [detailValid, setDetailValid] = useState(true)
  const templates = value.templates || []
  const initialActors = value.initial_actors || []
  useEffect(() => {
    if (!templates.some((template, index) => actorStateTemplateKey(template, index) === activeTemplateId)) {
      setActiveTemplateId(templates[0] ? actorStateTemplateKey(templates[0], 0) : '')
    }
  }, [activeTemplateId, templates])
  useEffect(() => {
    setDetailValid(true)
  }, [activeTemplateId])
  useEffect(() => {
    onValidityChange(detailValid)
  }, [detailValid, onValidityChange])

  const setTemplates = (nextTemplates: ActorStateTemplate[]) => onChange({ ...value, templates: nextTemplates })
  const setInitialActors = (nextActors: ActorStateInitialActor[]) => onChange({ ...value, initial_actors: nextActors })
  const activeTemplateIndex = templates.findIndex((template, index) => actorStateTemplateKey(template, index) === activeTemplateId)
  const activeTemplate = activeTemplateIndex >= 0 ? templates[activeTemplateIndex] : null
  const patchTemplate = (patch: Partial<ActorStateTemplate>) => {
    if (!activeTemplate) return
    const nextTemplate = { ...activeTemplate, ...patch }
    const nextTemplates = templates.map((template, index) => (index === activeTemplateIndex ? nextTemplate : template))
    if (patch.id === undefined) {
      setTemplates(nextTemplates)
      return
    }

    setActiveTemplateId(actorStateTemplateKey(nextTemplate, activeTemplateIndex))
    onChange({
      ...value,
      templates: nextTemplates,
      initial_actors: initialActors.map((actor) => (
        actor.template_id === activeTemplate.id ? { ...actor, template_id: nextTemplate.id || '' } : actor
      )),
    })
  }
  const addTemplate = () => {
    const id = nextPresetId('state-template').replace(/-/g, '_')
    const template: ActorStateTemplate = { id, name: '', description: '', fields: [] }
    setTemplates([...templates, template])
    setActiveTemplateId(id)
  }
  const copyTemplate = () => {
    if (!activeTemplate) return
    const template = cloneWithNewId(activeTemplate, 'state-template') as ActorStateTemplate
    template.id = (template.id || nextPresetId('state-template')).replace(/-/g, '_')
    setTemplates([...templates, template])
    setActiveTemplateId(actorStateTemplateKey(template, templates.length))
  }
  const deleteTemplate = () => {
    if (!activeTemplate) return
    const removedId = activeTemplate.id
    const nextTemplates = templates.filter((_, index) => index !== activeTemplateIndex)
    onChange({ ...value, templates: nextTemplates, initial_actors: initialActors.filter((actor) => actor.template_id !== removedId) })
    setActiveTemplateId(nextTemplates[0] ? actorStateTemplateKey(nextTemplates[0], 0) : '')
  }

  return (
    <div className={visualEditorShellClassName} data-testid="actor-state-visual-editor">
      <PresetTabsList
        items={templates}
        activeId={activeTemplateId}
        getId={actorStateTemplateKey}
        getTitle={(template, index) => template.name || template.id || `${t('settingPanel.actorState.template')} ${index + 1}`}
        getSubtitle={(template) => t('settingPanel.actorState.fieldSummary', { count: template.fields?.length || 0 })}
        addLabel={t('settingPanel.actorState.addTemplate')}
        emptyLabel={t('settingPanel.actorState.templates')}
        layout="rail"
        testIdPrefix="actor-state-templates"
        onAdd={addTemplate}
        onActiveIdChange={setActiveTemplateId}
        onItemsChange={setTemplates}
      />
      <div className={detailScrollPaneClassName} data-testid="actor-state-detail-scroll">
        {activeTemplate ? (
          <ActorStateTemplateDetails
            item={activeTemplate}
            initialActors={initialActors}
            activeFieldId={activeFieldId}
            setActiveFieldId={setActiveFieldId}
            onPatch={patchTemplate}
            onCopy={copyTemplate}
            onDelete={deleteTemplate}
            onInitialActorsChange={setInitialActors}
            onValidChange={setDetailValid}
          />
        ) : <EmptyDetail>{t('settingPanel.actorState.emptyTemplates')}</EmptyDetail>}
      </div>
    </div>
  )
}

function ActorStateTemplateDetails({
  item,
  initialActors,
  activeFieldId,
  setActiveFieldId,
  onPatch,
  onCopy,
  onDelete,
  onInitialActorsChange,
  onValidChange,
}: {
  item: ActorStateTemplate
  initialActors: ActorStateInitialActor[]
  activeFieldId: string
  setActiveFieldId: (id: string) => void
  onPatch: (patch: Partial<ActorStateTemplate>) => void
  onCopy: () => void
  onDelete: () => void
  onInitialActorsChange: (actors: ActorStateInitialActor[]) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [defaultValid, setDefaultValid] = useState(true)
  const [initialActorValid, setInitialActorValid] = useState(true)
  const fields = item.fields || []
  const activeIndex = fields.findIndex((field, index) => actorStateFieldKey(field, index) === activeFieldId)
  const activeField = activeIndex >= 0 ? fields[activeIndex] : null

  useEffect(() => {
    if (!fields.some((field, index) => actorStateFieldKey(field, index) === activeFieldId)) {
      setActiveFieldId(fields[0] ? actorStateFieldKey(fields[0], 0) : '')
    }
  }, [activeFieldId, fields, setActiveFieldId])
  useEffect(() => {
    if (!activeField) setDefaultValid(true)
  }, [activeField])
	useEffect(() => {
		const names = fields.map((field) => normalizeActorStateFieldName(field.name))
		const fieldsValid = names.every((name) => Boolean(name) && !name.includes('/')) && new Set(names).size === names.length
		onValidChange(defaultValid && initialActorValid && fieldsValid)
	}, [defaultValid, fields, initialActorValid, onValidChange])

  const setFields = (nextFields: ActorStateField[]) => onPatch({ fields: nextFields })
  const patchField = (patch: Partial<ActorStateField>) => {
    if (!activeField) return
    const nextField = { ...activeField, ...patch }
    setFields(fields.map((field, index) => (index === activeIndex ? nextField : field)))
  }
  const addField = () => {
		const field: ActorStateField = { name: '', type: 'number', visibility: 'visible', order: (fields.length + 1) * 10 }
    setFields([...fields, field])
		setActiveFieldId(actorStateFieldKey(field, fields.length))
  }
  const copyField = () => {
    if (!activeField) return
		const field: ActorStateField = { ...activeField, id: undefined, path: undefined, name: `${activeField.name} ${t('settingPanel.actorState.explorer.copySuffix')}`.trim() }
    setFields([...fields, field])
    setActiveFieldId(actorStateFieldKey(field, fields.length))
  }
  const deleteField = () => {
    if (!activeField) return
    const next = fields.filter((_, index) => index !== activeIndex)
    setFields(next)
    setActiveFieldId(next[0] ? actorStateFieldKey(next[0], 0) : '')
  }

  return (
    <div className="grid gap-4">
      <DetailPanel
        title={item.name || item.id || t('settingPanel.actorState.template')}
        description={item.description || t('settingPanel.actorState.description')}
        meta={t('settingPanel.actorState.fieldSummary', { count: fields.length })}
        actions={<DetailActions onCopy={onCopy} onDelete={onDelete} />}
      >
        <div className={fieldGridClassName}>
          <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={item.id || ''} onChange={(event) => onPatch({ id: event.target.value })} /></Field>
          <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={item.name || ''} onChange={(event) => onPatch({ name: event.target.value })} /></Field>
          <Field label={t('common.description')}><Input className={inputClassName} value={item.description || ''} onChange={(event) => onPatch({ description: event.target.value })} /></Field>
        </div>
      </DetailPanel>
      <EditorSection
        title={t('settingPanel.actorState.fields')}
        meta={t('settingPanel.actorState.fieldSummary', { count: fields.length })}
      >
        <div className={nestedEditorShellClassName}>
          <PresetTabsList
            items={fields}
            activeId={activeFieldId}
            getId={actorStateFieldKey}
			getTitle={(field, index) => field.name || `${t('settingPanel.actorState.field')} ${index + 1}`}
            getSubtitle={(field) => [field.type, field.visibility].filter(Boolean).join(' · ')}
            addLabel={t('settingPanel.actorState.addField')}
            emptyLabel={t('settingPanel.actorState.fields')}
            layout="rail"
            testIdPrefix="actor-state-fields"
            onAdd={addField}
            onActiveIdChange={setActiveFieldId}
            onItemsChange={setFields}
          />
          <div className={detailScrollPaneClassName}>
            {activeField ? (
              <ActorStateFieldDetails
                field={activeField}
                onPatch={patchField}
                onCopy={copyField}
                onDelete={deleteField}
                onValidChange={setDefaultValid}
              />
            ) : <EmptyDetail>{t('settingPanel.actorState.emptyFields')}</EmptyDetail>}
          </div>
        </div>
      </EditorSection>
      <InitialActorsEditor
        templateId={item.id}
        templateName={item.name || item.id}
        actors={initialActors}
        onChange={onInitialActorsChange}
        onValidChange={setInitialActorValid}
      />
    </div>
  )
}

function ActorStateFieldDetails({
  field,
  onPatch,
  onCopy,
  onDelete,
  onValidChange,
}: {
  field: ActorStateField
  onPatch: (patch: Partial<ActorStateField>) => void
  onCopy: () => void
  onDelete: () => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <DetailPanel
      dense
		title={field.name || t('settingPanel.actorState.field')}
		description={t('settingPanel.actorState.explorer.nameIdHelp')}
      meta={field.type || 'number'}
      actions={<DetailActions onCopy={onCopy} onDelete={onDelete} />}
    >
      <div className={fieldGridClassName}>
        <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={field.name || ''} onChange={(event) => onPatch({ name: event.target.value })} /></Field>
        <Field label={t('settingPanel.presetConfig.type')}>
          <Select value={field.type || 'number'} onValueChange={(type) => onPatch({ type })}>
            <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="number">number</SelectItem>
              <SelectItem value="string">string</SelectItem>
              <SelectItem value="bool">bool</SelectItem>
              <SelectItem value="enum">enum</SelectItem>
              <SelectItem value="object">object</SelectItem>
              <SelectItem value="list">list</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.presetConfig.min')}><Input className={inputClassName} inputMode="decimal" value={String(field.min ?? '')} onChange={(event) => onPatch({ min: parseNumberInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.max')}><Input className={inputClassName} inputMode="decimal" value={String(field.max ?? '')} onChange={(event) => onPatch({ max: parseNumberInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.presetConfig.visibility')}>
          <Select value={field.visibility || 'visible'} onValueChange={(visibility) => onPatch({ visibility: visibility as ActorStateField['visibility'] })}>
            <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="visible">visible</SelectItem>
              <SelectItem value="hidden">hidden</SelectItem>
              <SelectItem value="spoiler">spoiler</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label={t('settingPanel.presetConfig.order')}><Input className={inputClassName} inputMode="numeric" value={String(field.order ?? '')} onChange={(event) => onPatch({ order: parseIntegerInput(event.target.value) })} /></Field>
        <Field label={t('settingPanel.actorState.options')}><Input className={inputClassName} value={joinListInput(field.options)} onChange={(event) => onPatch({ options: splitListInput(event.target.value) })} /></Field>
      </div>
      <ActorStateDefaultValueField field={field} onPatch={onPatch} onValidChange={onValidChange} />
      <Field label={t('common.description')}>
        <Textarea autoResize={false} className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={field.description || ''} onChange={(event) => onPatch({ description: event.target.value })} />
      </Field>
      <Field label={t('settingPanel.actorState.updateInstruction')}>
        <Textarea autoResize={false} className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={field.update_instruction || ''} onChange={(event) => onPatch({ update_instruction: event.target.value })} />
      </Field>
    </DetailPanel>
  )
}

function ActorStateDefaultValueField({
  field,
  onPatch,
  onValidChange,
}: {
  field: ActorStateField
  onPatch: (patch: Partial<ActorStateField>) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const type = field.type || 'number'
  useEffect(() => {
    if (type !== 'object' && type !== 'list') onValidChange(true)
  }, [onValidChange, type])
  if (type === 'object' || type === 'list') {
    return (
      <JSONFragmentEditor
        label={t('settingPanel.presetConfig.defaultValue')}
        value={field.default ?? (type === 'list' ? [] : {})}
        expected={type === 'list' ? 'array' : 'object'}
        onChange={(defaultValue) => onPatch({ default: defaultValue })}
        onValidChange={onValidChange}
      />
    )
  }
  if (type === 'bool') {
    return (
      <Field label={t('settingPanel.presetConfig.defaultValue')}>
        <Select value={String(field.default === true)} onValueChange={(value) => onPatch({ default: value === 'true' })}>
          <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
          <SelectContent className="nova-panel border text-[var(--nova-text)]">
            <SelectItem value="true">true</SelectItem>
            <SelectItem value="false">false</SelectItem>
          </SelectContent>
        </Select>
      </Field>
    )
  }
  return (
    <Field label={t('settingPanel.presetConfig.defaultValue')}>
      <Input
        className={inputClassName}
        inputMode={type === 'number' ? 'decimal' : undefined}
        value={String(field.default ?? '')}
        onChange={(event) => onPatch({ default: type === 'number' ? parseNumberInput(event.target.value) : event.target.value })}
      />
    </Field>
  )
}

function InitialActorsEditor({
  templateId,
  templateName,
  actors,
  onChange,
  onValidChange,
}: {
  templateId: string
  templateName: string
  actors: ActorStateInitialActor[]
  onChange: (actors: ActorStateInitialActor[]) => void
  onValidChange: (valid: boolean) => void
}) {
  const { t } = useTranslation()
  const [activeActorId, setActiveActorId] = useState('')
  const templateActors = actors.filter((actor) => actor.template_id === templateId)
  const activeIndex = templateActors.findIndex((actor, index) => actorStateActorKey(actor, index) === activeActorId)
  const activeActor = activeIndex >= 0 ? templateActors[activeIndex] : null
  useEffect(() => {
    if (!templateActors.some((actor, index) => actorStateActorKey(actor, index) === activeActorId)) {
      setActiveActorId(templateActors[0] ? actorStateActorKey(templateActors[0], 0) : '')
    }
  }, [activeActorId, templateActors])
  useEffect(() => {
    if (!activeActor) onValidChange(true)
  }, [activeActor, onValidChange])
  const setTemplateActors = (nextTemplateActors: ActorStateInitialActor[]) => onChange([
    ...actors.filter((actor) => actor.template_id !== templateId),
    ...nextTemplateActors,
  ])
  const patchActor = (patch: Partial<ActorStateInitialActor>) => {
    if (!activeActor) return
    const nextActor = { ...activeActor, ...patch, template_id: templateId }
    if (patch.id !== undefined) setActiveActorId(actorStateActorKey(nextActor, activeIndex))
    setTemplateActors(templateActors.map((actor, index) => (index === activeIndex ? nextActor : actor)))
  }
  const addActor = () => {
    const id = nextPresetId('actor').replace(/-/g, '_')
    const actor: ActorStateInitialActor = { id, name: '', template_id: templateId, role: '', state: {} }
    setTemplateActors([...templateActors, actor])
    setActiveActorId(id)
  }
  const copyActor = () => {
    if (!activeActor) return
    const actor = cloneWithNewId(activeActor, 'actor') as ActorStateInitialActor
    actor.id = (actor.id || nextPresetId('actor')).replace(/-/g, '_')
    actor.template_id = templateId
    setTemplateActors([...templateActors, actor])
    setActiveActorId(actorStateActorKey(actor, templateActors.length))
  }
  const deleteActor = () => {
    if (!activeActor) return
    const next = templateActors.filter((_, index) => index !== activeIndex)
    setTemplateActors(next)
    setActiveActorId(next[0] ? actorStateActorKey(next[0], 0) : '')
  }
  return (
    <EditorSection
      title={t('settingPanel.actorState.initialActors')}
      description={templateName}
      meta={t('settingPanel.actorState.initialActorSummary', { count: templateActors.length })}
    >
      <div className={nestedEditorShellClassName}>
        <PresetTabsList
          items={templateActors}
          activeId={activeActorId}
          getId={actorStateActorKey}
          getTitle={(actor, index) => actor.name || actor.id || `${t('settingPanel.actorState.initialActor')} ${index + 1}`}
          getSubtitle={(actor) => [templateName, actor.role].filter(Boolean).join(' · ')}
          addLabel={t('settingPanel.actorState.addInitialActor')}
          emptyLabel={t('settingPanel.actorState.initialActors')}
          layout="rail"
          testIdPrefix="actor-state-initial-actors"
          onAdd={addActor}
          onActiveIdChange={setActiveActorId}
          onItemsChange={setTemplateActors}
        />
        <div className={detailScrollPaneClassName}>
          {activeActor ? (
            <DetailPanel
              dense
              title={activeActor.name || activeActor.id || t('settingPanel.actorState.initialActor')}
              description={activeActor.role || templateName}
              meta={templateId}
              actions={<DetailActions onCopy={copyActor} onDelete={deleteActor} />}
            >
              <div className={fieldGridClassName}>
                <Field label={t('settingPanel.presetConfig.id')}><Input className={inputClassName} value={activeActor.id || ''} onChange={(event) => patchActor({ id: event.target.value })} /></Field>
                <Field label={t('settingPanel.field.name')}><Input className={inputClassName} value={activeActor.name || ''} onChange={(event) => patchActor({ name: event.target.value })} /></Field>
                <Field label={t('settingPanel.actorState.templateID')}><Input className={inputClassName} value={templateId} disabled /></Field>
                <Field label={t('settingPanel.actorState.role')}><Input className={inputClassName} value={activeActor.role || ''} onChange={(event) => patchActor({ role: event.target.value })} /></Field>
                <Field label={t('common.description')}><Input className={inputClassName} value={activeActor.description || ''} onChange={(event) => patchActor({ description: event.target.value })} /></Field>
              </div>
              <JSONFragmentEditor
                label={t('settingPanel.actorState.initialState')}
                value={activeActor.state || {}}
                expected="object"
                onChange={(state) => patchActor({ state: state as Record<string, unknown> })}
                onValidChange={onValidChange}
              />
            </DetailPanel>
          ) : <EmptyDetail>{t('settingPanel.actorState.emptyInitialActors')}</EmptyDetail>}
        </div>
      </div>
    </EditorSection>
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
      <Textarea autoResize={false} className="nova-field min-h-28 resize-y font-mono text-xs leading-5 shadow-none focus-visible:ring-0" value={text} onChange={(event) => update(event.target.value)} />
      {error ? <div className="mt-1 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{error}</div> : null}
    </Field>
  )
}

function actorStateTemplateKey(item: ActorStateTemplate, index: number) {
  return item.id || `template-${index}`
}

function actorStateFieldKey(item: ActorStateField, index: number) {
	void item
	return `field-${index}`
}

function normalizeActorStateFieldName(value: string | undefined) {
	return (value || '').normalize('NFKC').trim().toLocaleLowerCase()
}

function actorStateActorKey(item: ActorStateInitialActor, index: number) {
  return item.id || `${item.template_id || 'actor'}-${index}`
}

function DetailPanel({
  children,
  dense = false,
  className = '',
  title,
  description,
  meta,
  actions,
}: {
  children: ReactNode
  dense?: boolean
  className?: string
  title?: string
  description?: string
  meta?: string
  actions?: ReactNode
}) {
  return (
    <section className={cn('grid min-w-0 gap-3 rounded-[14px] bg-[var(--nova-surface-2)]', dense ? 'p-3' : 'p-4', className)}>
      {title || description || meta || actions ? (
        <EditorSectionHeader title={title} description={description} meta={meta} actions={actions} />
      ) : null}
      {children}
    </section>
  )
}

function EmptyDetail({ children }: { children: ReactNode }) {
  return <div className="flex min-h-48 items-center justify-center rounded-[12px] border border-dashed border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-8 text-center text-xs text-[var(--nova-text-faint)]">{children}</div>
}

function EditorSection({
  title,
  description,
  meta,
  actions,
  children,
  className,
}: {
  title: string
  description?: string
  meta?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn('grid gap-3 rounded-[14px] bg-[var(--nova-surface-2)] p-3', className)}>
      <EditorSectionHeader title={title} description={description} meta={meta} actions={actions} />
      {children}
    </section>
  )
}

function EditorSectionHeader({
  title,
  description,
  meta,
  actions,
}: {
  title?: string
  description?: string
  meta?: string
  actions?: ReactNode
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        {title ? <div className="truncate text-sm font-semibold text-[var(--nova-text)]">{title}</div> : null}
        {description ? <div className="mt-1 line-clamp-2 text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</div> : null}
      </div>
      {meta || actions ? (
        <div className="flex shrink-0 items-center gap-2">
          {meta ? (
            <Badge variant="outline" className="h-6 rounded-full border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 text-[10px] font-normal text-[var(--nova-text-faint)]">
              {meta}
            </Badge>
          ) : null}
          {actions}
        </div>
      ) : null}
    </div>
  )
}

function DetailActions({ onCopy, onDelete }: { onCopy?: () => void; onDelete?: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex justify-end gap-2">
      <Button className={iconActionClassName} variant="outline" size="icon-sm" disabled={!onCopy} onClick={onCopy} aria-label={t('settingPanel.presetConfig.copy')} title={t('settingPanel.presetConfig.copy')}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
      <Button className={iconActionClassName} variant="outline" size="icon-sm" disabled={!onDelete} onClick={onDelete} aria-label={t('common.delete')} title={t('common.delete')}>
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid min-w-0 gap-1.5 text-xs text-[var(--nova-text-muted)]">
      <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
      {children}
    </label>
  )
}

function SwitchField({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="flex items-end">
      <div className="flex h-9 w-full items-center justify-between gap-2 rounded-[14px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]">
        <span className="truncate text-[11px] text-[var(--nova-text-faint)]">{label}</span>
        <Switch checked={checked} onCheckedChange={onChange} />
      </div>
    </div>
  )
}
