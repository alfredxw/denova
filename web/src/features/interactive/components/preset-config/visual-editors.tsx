import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Copy, GripVertical, Plus, Trash2, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { ActorStateField, ActorStateInitialActor, ActorStateTemplate, EventPackageModule, StoryDirectorActorStateSystem, StoryMemoryField, StoryMemoryStructure, TellerEventCard } from '../../types'
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
		const fieldsValid = names.every(Boolean) && new Set(names).size === names.length
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

// ─── Memory Structure Visual Editor ───────────────────────────────────────────

const DERIVED_STRUCTURE_IDS = new Set(['current_state', 'rule_state_summary'])

function isReadOnlyStructure(s: StoryMemoryStructure): boolean {
  return s.read_only === true || s.derived === true || DERIVED_STRUCTURE_IDS.has(s.id)
}

function isBuiltInNonDerived(s: StoryMemoryStructure): boolean {
  return s.built_in === true && !isReadOnlyStructure(s)
}

export function MemoryStructureVisualEditor({
  value,
  onChange,
  onValidityChange,
  resetKey,
}: {
  value: StoryMemoryStructure[]
  onChange: (value: StoryMemoryStructure[]) => void
  onValidityChange: (valid: boolean) => void
  resetKey: string
}) {
  const { t } = useTranslation()
  const [activeStructureId, setActiveStructureId] = useState('')
  const [activeFieldId, setActiveFieldId] = useState('')
  const structures = value || []

  // Validate: keyed structures need a valid key_field_id
  useEffect(() => {
    const valid = structures.every((s) => {
      if (s.mode !== 'keyed') return true
      if (!s.key_field_id) return false
      return (s.fields || []).some((f) => f.id === s.key_field_id)
    })
    onValidityChange(valid)
  }, [structures, onValidityChange])

  // Sync active structure when structures change
  useEffect(() => {
    if (!structures.some((structure, index) => itemKey(structure, index, 'structure') === activeStructureId)) {
      setActiveStructureId(structures[0] ? itemKey(structures[0], 0, 'structure') : '')
      setActiveFieldId('')
    }
  }, [structures, activeStructureId])

  // Reset on resetKey change
  useEffect(() => {
    setActiveStructureId(structures[0] ? itemKey(structures[0], 0, 'structure') : '')
    setActiveFieldId('')
  }, [resetKey]) // eslint-disable-line react-hooks/exhaustive-deps

  const activeStructureIndex = structures.findIndex((structure, index) => itemKey(structure, index, 'structure') === activeStructureId)
  const activeStructure = activeStructureIndex >= 0 ? structures[activeStructureIndex] : null
  const structureReadOnly = activeStructure ? isReadOnlyStructure(activeStructure) : false
  const structureBuiltIn = activeStructure ? isBuiltInNonDerived(activeStructure) : false

  const setStructures = (next: StoryMemoryStructure[]) => onChange(next)

  const patchStructure = (id: string, patch: Partial<StoryMemoryStructure>) => {
    const activeIndex = structures.findIndex((structure) => structure.id === id)
    if (activeIndex < 0) return
    const nextStructure = { ...structures[activeIndex], ...patch }
    if (patch.id !== undefined) setActiveStructureId(itemKey(nextStructure, activeIndex, 'structure'))
    setStructures(structures.map((structure, index) => (index === activeIndex ? nextStructure : structure)))
  }

  const addStructure = () => {
    const id = nextPresetId('structure').replace(/-/g, '_')
    const s: StoryMemoryStructure = {
      id,
      name: t('settingPanel.presetConfig.structure'),
      description: '',
      generation_instruction: '',
      mode: 'append',
      fields: [
        { id: 'content', name: t('settingPanel.presetConfig.field'), description: '', required: false, order: 10 },
      ],
      enabled: true,
      order: (structures.length + 1) * 10,
    }
    setStructures([...structures, s])
    setActiveStructureId(id)
    setActiveFieldId('content')
  }

  const copyStructure = () => {
    if (!activeStructure) return
    const cloned = cloneWithNewId(activeStructure, 'structure')
    cloned.id = (cloned.id || `structure_${Date.now()}`).replace(/-/g, '_')
    cloned.built_in = false
    cloned.read_only = false
    cloned.derived = false
    cloned.fields = (activeStructure.fields || []).map((f) => ({ ...f }))
    setStructures([...structures, cloned])
    setActiveStructureId(cloned.id || '')
  }

  const deleteStructure = () => {
    if (!activeStructure || structureReadOnly) return
    const next = structures.filter((s) => s.id !== activeStructure.id)
    setStructures(next)
    setActiveStructureId(next[0] ? itemKey(next[0], 0, 'structure') : '')
    setActiveFieldId('')
  }

  // Field operations
  const setFields = (fields: StoryMemoryField[]) => {
    if (!activeStructure) return
    patchStructure(activeStructure.id, { fields })
  }

  const fields = activeStructure?.fields || []

  useEffect(() => {
    if (!fields.some((f, i) => itemKey(f, i, 'field') === activeFieldId)) {
      setActiveFieldId(fields[0] ? itemKey(fields[0], 0, 'field') : '')
    }
  }, [fields, activeFieldId])

  const activeFieldIndex = fields.findIndex((f, i) => itemKey(f, i, 'field') === activeFieldId)
  const activeField = activeFieldIndex >= 0 ? fields[activeFieldIndex] : null

  const patchField = (patch: Partial<StoryMemoryField>) => {
    if (!activeField || activeFieldIndex < 0) return
    const next = [...fields]
    const nextField = { ...fields[activeFieldIndex], ...patch }
    next[activeFieldIndex] = nextField
    if (patch.id !== undefined) setActiveFieldId(itemKey(nextField, activeFieldIndex, 'field'))
    patchStructure(activeStructure!.id, {
      fields: next,
      ...(patch.id !== undefined && activeStructure!.key_field_id === activeField.id
        ? { key_field_id: nextField.id }
        : {}),
    })
  }

  const addField = () => {
    if (!activeStructure || structureReadOnly) return
    const fid = `field_${fields.length + 1}`
    const f: StoryMemoryField = {
      id: fid,
      name: t('settingPanel.presetConfig.field'),
      description: '',
      required: false,
      order: (fields.length + 1) * 10,
    }
    setFields([...fields, f])
    setActiveFieldId(fid)
  }

  const copyField = () => {
    if (!activeField || structureReadOnly) return
    const cloned: StoryMemoryField = { ...activeField, id: `${activeField.id || 'field'}_copy` }
    setFields([...fields, cloned])
    setActiveFieldId(itemKey(cloned, fields.length, 'field'))
  }

  const deleteField = () => {
    if (!activeField || activeFieldIndex < 0 || structureReadOnly) return
    const next = fields.filter((_, i) => i !== activeFieldIndex)
    setFields(next)
    setActiveFieldId(next[0] ? itemKey(next[0], 0, 'field') : '')
  }

  return (
    <div className={visualEditorShellClassName} data-testid="memory-structure-editor">
      <PresetTabsList
        items={structures}
        activeId={activeStructureId}
        getId={(structure, index) => itemKey(structure, index, 'structure')}
        getTitle={(s) => s.name || s.id}
        getSubtitle={(s) => {
          const parts: string[] = [s.mode || 'append']
          const fieldCount = (s.fields || []).length
          parts.push(t('settingPanel.memoryStructure.fieldCount', { count: fieldCount }))
          if (s.enabled === false) parts.push(t('settingPanel.disabled'))
          if (isReadOnlyStructure(s)) parts.push(t('settingPanel.presetConfig.derived'))
          return parts.join(' · ')
        }}
        addLabel={t('settingPanel.presetConfig.addStructure')}
        emptyLabel={t('settingPanel.presetConfig.structures')}
        layout="rail"
        testIdPrefix="memory-structures"
        onAdd={addStructure}
        onActiveIdChange={setActiveStructureId}
        onItemsChange={setStructures}
      />
      <div className={detailScrollPaneClassName} data-testid="memory-structure-detail-scroll">
        {activeStructure ? (
          <StructureDetails
            structure={activeStructure}
            readOnly={structureReadOnly}
            builtIn={structureBuiltIn}
            activeFieldId={activeFieldId}
            fields={fields}
            onPatch={(patch) => patchStructure(activeStructure.id, patch)}
            onCopyStructure={copyStructure}
            onDeleteStructure={deleteStructure}
            onActiveFieldIdChange={setActiveFieldId}
            onFieldsChange={setFields}
            onPatchField={patchField}
            onAddField={addField}
            onCopyField={copyField}
            onDeleteField={deleteField}
          />
        ) : (
          <EmptyDetail>{t('settingPanel.presetConfig.emptyStructures')}</EmptyDetail>
        )}
      </div>
    </div>
  )
}

function StructureDetails({
  structure,
  readOnly,
  builtIn,
  activeFieldId,
  fields,
  onPatch,
  onCopyStructure,
  onDeleteStructure,
  onActiveFieldIdChange,
  onFieldsChange,
  onPatchField,
  onAddField,
  onCopyField,
  onDeleteField,
}: {
  structure: StoryMemoryStructure
  readOnly: boolean
  builtIn: boolean
  activeFieldId: string
  fields: StoryMemoryField[]
  onPatch: (patch: Partial<StoryMemoryStructure>) => void
  onCopyStructure: () => void
  onDeleteStructure: () => void
  onActiveFieldIdChange: (id: string) => void
  onFieldsChange: (fields: StoryMemoryField[]) => void
  onPatchField: (patch: Partial<StoryMemoryField>) => void
  onAddField: () => void
  onCopyField: () => void
  onDeleteField: () => void
}) {
  const { t } = useTranslation()
  const mode = structure.mode || 'append'
  const keyed = mode === 'keyed'
  // Fields whose definitions cannot be edited (built-in non-derived structures)
  const fieldDefsReadOnly = readOnly || builtIn

  return (
    <DetailPanel>
      {/* Structure header actions */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {builtIn ? (
            <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {t('settingPanel.presetConfig.builtIn')}
            </span>
          ) : null}
          {readOnly ? (
            <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">
              {t('settingPanel.presetConfig.derived')}
            </span>
          ) : null}
        </div>
        <DetailActions onCopy={onCopyStructure} onDelete={readOnly ? undefined : onDeleteStructure} />
      </div>

      {readOnly ? (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] text-[var(--nova-text-faint)]">
          {t('settingPanel.presetConfig.readOnlyStructureHint')}
        </div>
      ) : null}

      {builtIn ? (
        <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-2 text-[11px] text-[var(--nova-text-faint)]">
          {t('settingPanel.presetConfig.builtInFieldHint')}
        </div>
      ) : null}

      {/* Structure basic info */}
      <div className={fieldGridClassName}>
        <Field label={t('settingPanel.presetConfig.id')}>
          <Input className={inputClassName} value={structure.id} disabled={readOnly} onChange={(e) => onPatch({ id: e.target.value.replace(/[-\s]/g, '_') })} />
        </Field>
        <Field label={t('settingPanel.field.name')}>
          <Input className={inputClassName} value={structure.name || ''} disabled={readOnly} onChange={(e) => onPatch({ name: e.target.value })} />
        </Field>
        <Field label={t('settingPanel.presetConfig.structureMode')}>
          <Select value={mode} disabled={readOnly} onValueChange={(m) => onPatch({ mode: m as StoryMemoryStructure['mode'] })}>
            <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
            <SelectContent className="nova-panel border text-[var(--nova-text)]">
              <SelectItem value="singleton">{t('settingPanel.presetConfig.modeSingleton')}</SelectItem>
              <SelectItem value="keyed">{t('settingPanel.presetConfig.modeKeyed')}</SelectItem>
              <SelectItem value="append">{t('settingPanel.presetConfig.modeAppend')}</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        {keyed ? (
          <Field label={t('settingPanel.presetConfig.keyField')}>
            <Select value={structure.key_field_id || ''} disabled={readOnly} onValueChange={(v) => onPatch({ key_field_id: v })}>
              <SelectTrigger className={selectClassName}><SelectValue /></SelectTrigger>
              <SelectContent className="nova-panel border text-[var(--nova-text)]">
                {fields.filter((field) => field.id).map((f) => (
                  <SelectItem key={f.id} value={f.id}>{f.name || f.id}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        <Field label={t('settingPanel.presetConfig.order')}>
          <Input className={inputClassName} inputMode="numeric" value={String(structure.order || '')} disabled={readOnly} onChange={(e) => onPatch({ order: parseIntegerInput(e.target.value) || 0 })} />
        </Field>
        <SwitchField label={t('settingPanel.field.enabled')} checked={structure.enabled !== false} onChange={(enabled) => onPatch({ enabled })} />
      </div>

      <Field label={t('common.description')}>
        <Textarea autoResize={false} className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={structure.description || ''} disabled={readOnly} onChange={(e) => onPatch({ description: e.target.value })} />
      </Field>

      <Field label={t('settingPanel.presetConfig.generationInstruction')}>
        <Textarea autoResize={false} className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={structure.generation_instruction || ''} disabled={readOnly} onChange={(e) => onPatch({ generation_instruction: e.target.value })} />
      </Field>

      {/* Fields accordion editor */}
      <FieldAccordionList
        fields={fields}
        activeFieldId={activeFieldId}
        readOnly={fieldDefsReadOnly}
        onActiveFieldIdChange={onActiveFieldIdChange}
        onFieldsChange={onFieldsChange}
        onPatchField={onPatchField}
        onAddField={onAddField}
        onCopyField={onCopyField}
        onDeleteField={readOnly ? undefined : onDeleteField}
      />
    </DetailPanel>
  )
}

function FieldAccordionList({
  fields,
  activeFieldId,
  readOnly,
  onActiveFieldIdChange,
  onFieldsChange,
  onPatchField,
  onAddField,
  onCopyField,
  onDeleteField,
}: {
  fields: StoryMemoryField[]
  activeFieldId: string
  readOnly: boolean
  onActiveFieldIdChange: (id: string) => void
  onFieldsChange: (fields: StoryMemoryField[]) => void
  onPatchField: (patch: Partial<StoryMemoryField>) => void
  onAddField: () => void
  onCopyField: () => void
  onDeleteField?: () => void
}) {
  const { t } = useTranslation()
  const [expandedId, setExpandedId] = useState<string>(activeFieldId || '')
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  // Sync expanded with active
  useEffect(() => {
    const expandedStillExists = fields.some((field, index) => itemKey(field, index, 'field') === expandedId)
    if (activeFieldId && (!expandedId || !expandedStillExists)) setExpandedId(activeFieldId)
  }, [activeFieldId, expandedId, fields])

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const ids = fields.map((f, i) => itemKey(f, i, 'field'))
    const oldIndex = ids.indexOf(String(active.id))
    const newIndex = ids.indexOf(String(over.id))
    if (oldIndex < 0 || newIndex < 0) return
    onFieldsChange(arrayMove(fields, oldIndex, newIndex))
  }

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? '' : id))
    onActiveFieldIdChange(id)
  }

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-medium text-[var(--nova-text-faint)]">
          {t('settingPanel.presetConfig.fields')} ({fields.length})
        </span>
        {!readOnly ? (
          <Button className={iconActionClassName} variant="outline" size="icon-sm" onClick={onAddField} aria-label={t('settingPanel.presetConfig.addField')} title={t('settingPanel.presetConfig.addField')}>
            <Plus className="h-3.5 w-3.5" />
          </Button>
        ) : null}
      </div>
      {fields.length === 0 ? (
        <div className="rounded-[var(--nova-radius)] border border-dashed border-[var(--nova-border)] px-3 py-4 text-xs text-[var(--nova-text-faint)]">
          {t('settingPanel.presetConfig.emptyFields')}
        </div>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={fields.map((f, i) => itemKey(f, i, 'field'))} strategy={verticalListSortingStrategy}>
            <div className="space-y-1">
              {fields.map((field, index) => {
                const id = itemKey(field, index, 'field')
                const isExpanded = expandedId === id
                return (
                  <FieldAccordionItem
                    key={id}
                    id={id}
                    field={field}
                    index={index}
                    expanded={isExpanded}
                    readOnly={readOnly}
                    onToggle={() => toggleExpand(id)}
                    onPatch={onPatchField}
                    onCopy={onCopyField}
                    onDelete={onDeleteField}
                  />
                )
              })}
            </div>
          </SortableContext>
        </DndContext>
      )}
    </div>
  )
}

function FieldAccordionItem({
  id,
  field,
  index,
  expanded,
  readOnly,
  onToggle,
  onPatch,
  onCopy,
  onDelete,
}: {
  id: string
  field: StoryMemoryField
  index: number
  expanded: boolean
  readOnly: boolean
  onToggle: () => void
  onPatch: (patch: Partial<StoryMemoryField>) => void
  onCopy: () => void
  onDelete?: () => void
}) {
  const { t } = useTranslation()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`rounded-[var(--nova-radius)] bg-[var(--nova-surface-2)] ${isDragging ? 'opacity-60' : ''}`}
    >
      {/* Collapsed header row */}
      <div className="flex items-center gap-1 px-1.5 py-1.5">
        {!readOnly ? (
          <button
            type="button"
            className="nova-nav-item flex h-7 w-6 shrink-0 items-center justify-center rounded text-[var(--nova-text-faint)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            aria-label={`${t('settingPanel.presetConfig.field')} ${index + 1}`}
            {...attributes}
            {...listeners}
          >
            <GripVertical className="h-3.5 w-3.5" />
          </button>
        ) : null}
        <button type="button" onClick={onToggle} className="flex min-w-0 flex-1 items-center gap-1.5 text-left">
          <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-[var(--nova-text-faint)] transition-transform ${expanded ? 'rotate-90' : ''}`} />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-xs font-medium text-[var(--nova-text)]">{field.name || field.id}</span>
          </span>
          <span className="flex shrink-0 items-center gap-1">
            {field.required ? (
              <span className="rounded bg-[var(--nova-accent)]/15 px-1.5 py-0.5 text-[10px] text-[var(--nova-accent)]">{t('settingPanel.presetConfig.required')}</span>
            ) : (
              <span className="rounded bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.presetConfig.optional')}</span>
            )}
            {field.enabled === false ? (
              <span className="rounded bg-[var(--nova-surface)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-faint)]">{t('settingPanel.disabled')}</span>
            ) : null}
          </span>
        </button>
      </div>

      {/* Expanded edit form */}
      {expanded ? (
        <div className="grid gap-3 border-t border-[var(--nova-border)]/50 px-3 py-3">
          <div className="flex justify-end">
            <DetailActions onCopy={onCopy} onDelete={onDelete} />
          </div>
          <div className={fieldGridClassName}>
            <Field label={t('settingPanel.presetConfig.id')}>
              <Input className={inputClassName} value={field.id} disabled={readOnly} onChange={(e) => onPatch({ id: e.target.value.replace(/[-\s]/g, '_') })} />
            </Field>
            <Field label={t('settingPanel.field.name')}>
              <Input className={inputClassName} value={field.name || ''} disabled={readOnly} onChange={(e) => onPatch({ name: e.target.value })} />
            </Field>
            <Field label={t('settingPanel.presetConfig.order')}>
              <Input className={inputClassName} inputMode="numeric" value={String(field.order || '')} disabled={readOnly} onChange={(e) => onPatch({ order: parseIntegerInput(e.target.value) || 0 })} />
            </Field>
            <SwitchField label={t('settingPanel.presetConfig.required')} checked={field.required === true} onChange={(required) => onPatch({ required })} />
            <SwitchField label={t('settingPanel.field.enabled')} checked={field.enabled !== false} onChange={(enabled) => onPatch({ enabled })} />
          </div>
          <Field label={t('common.description')}>
            <Textarea autoResize={false} className="nova-field min-h-20 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={field.description || ''} disabled={readOnly} onChange={(e) => onPatch({ description: e.target.value })} />
          </Field>
          <Field label={t('settingPanel.presetConfig.generationInstruction')}>
            <Textarea autoResize={false} className="nova-field min-h-24 resize-y text-xs leading-5 shadow-none focus-visible:ring-0" value={field.generation_instruction || ''} disabled={readOnly} onChange={(e) => onPatch({ generation_instruction: e.target.value })} />
          </Field>
        </div>
      ) : null}
    </div>
  )
}
