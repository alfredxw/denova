import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical, Plus, Trash2 } from 'lucide-react'
import { Fragment, useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import {
  gamePlanningSectionDescription,
  gamePlanningSectionTitle,
  gamePlanningTemplateDescription,
  gamePlanningTemplateName,
} from '../../game-planning'
import type { GamePlanningSection, GamePlanningTemplate } from '../../types'
import { presetActionButtonClassName, presetInputClassName } from '../preset-config/editor-styles'
import { PresetEmptyState } from '../preset-config/PresetEmptyState'
import { PresetMetadataPanel } from '../preset-config/PresetEditorChrome'

interface GamePlanningEditorProps {
  draft: GamePlanningTemplate | null
  setDraft: (draft: GamePlanningTemplate | null) => void
  onValidityChange: (valid: boolean) => void
}

export function GamePlanningEditor({ draft, setDraft, onValidityChange }: GamePlanningEditorProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const validation = useMemo(() => validateSections(draft?.sections || []), [draft?.sections])
  const nameValid = Boolean(draft?.name.trim())

  useEffect(
    () => onValidityChange(Boolean(draft) && nameValid && validation.valid),
    [draft, nameValid, onValidityChange, validation.valid],
  )

  if (!draft) {
    return (
      <PresetEmptyState
        title={t('settingPanel.editor.noGamePlanningSelected')}
        description={t('settingPanel.editor.noGamePlanningSelectedDesc')}
      />
    )
  }

  const displayName = gamePlanningTemplateName(draft, t)
  const displayDescription = gamePlanningTemplateDescription(draft, t)
  const editableSnapshot = (): GamePlanningTemplate => {
    if (draft.custom || draft.builtin_overridden) return draft
    return {
      ...draft,
      name: displayName,
      description: displayDescription,
      sections: draft.sections.map((section) => ({
        ...section,
        title: gamePlanningSectionTitle(draft, section, t),
        description: gamePlanningSectionDescription(draft, section, t),
      })),
      builtin_overridden: true,
    }
  }
  const patch = (next: Partial<GamePlanningTemplate>) => setDraft({ ...editableSnapshot(), ...next })
  const patchSection = (id: string, next: Partial<GamePlanningSection>) => {
    const editable = editableSnapshot()
    setDraft({
      ...editable,
      sections: editable.sections.map((section) => section.id === id ? { ...section, ...next } : section),
    })
  }
  const addSection = () => {
    const editable = editableSnapshot()
    setDraft({
      ...editable,
      sections: [...editable.sections, { id: createSectionID(), title: '', description: '' }],
    })
  }
  const removeSection = (id: string) => {
    const editable = editableSnapshot()
    if (editable.sections.length <= 1) return
    setDraft({ ...editable, sections: editable.sections.filter((section) => section.id !== id) })
  }
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const editable = editableSnapshot()
    const from = editable.sections.findIndex((section) => section.id === active.id)
    const to = editable.sections.findIndex((section) => section.id === over.id)
    if (from >= 0 && to >= 0) setDraft({ ...editable, sections: arrayMove(editable.sections, from, to) })
  }
  let status = t('settingPanel.builtIn')
  if (draft.custom) status = t('settingPanel.custom')
  else if (draft.builtin_overridden) status = t('settingPanel.builtInOverridden')

  return (
    <div data-testid="game-planning-editor" className="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto">
      <PresetMetadataPanel
        name={displayName}
        description={displayDescription}
        status={status}
        hint={t(draft.custom ? 'settingPanel.storyDirector.customEditable' : 'settingPanel.storyDirector.builtInEditHint')}
        onNameChange={(name) => patch({ name })}
        onDescriptionChange={(description) => patch({ description })}
      />
      {!nameValid ? (
        <p role="alert" className="border-b border-[var(--preset-line)] px-3 py-2 text-xs text-[var(--nova-danger)] sm:px-4">
          {t('settingPanel.gamePlanning.nameRequired')}
        </p>
      ) : null}

      <section className="shrink-0 bg-[var(--preset-surface)]" aria-labelledby="game-planning-sections-title">
        <header className="flex items-center justify-between gap-3 p-3 sm:p-4">
          <div className="min-w-0">
            <h3 id="game-planning-sections-title" className="text-xs font-medium text-[var(--nova-text)]">
              {t('settingPanel.gamePlanning.sections')}
            </h3>
            <p className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">
              {t('settingPanel.gamePlanning.sectionsDescription')}
            </p>
          </div>
          <Button className={presetActionButtonClassName} type="button" variant="outline" size="sm" onClick={addSection}>
            <Plus data-icon="inline-start" />
            {t('settingPanel.gamePlanning.addSection')}
          </Button>
        </header>
        <Separator />

        {draft.sections.length === 0 ? (
          <p role="alert" className="px-3 py-4 text-xs text-[var(--nova-danger)] sm:px-4">
            {t('settingPanel.gamePlanning.sectionsRequired')}
          </p>
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
            <SortableContext items={draft.sections.map((section) => section.id)} strategy={verticalListSortingStrategy}>
              {draft.sections.map((section, index) => (
                <Fragment key={section.id}>
                  {index > 0 ? <Separator /> : null}
                  <PlanningSectionItem
                    section={section}
                    displayTitle={gamePlanningSectionTitle(draft, section, t)}
                    displayDescription={gamePlanningSectionDescription(draft, section, t)}
                    duplicateTitle={validation.duplicateSectionIDs.has(section.id)}
                    canRemove={draft.sections.length > 1}
                    onChange={(next) => patchSection(section.id, next)}
                    onRemove={() => removeSection(section.id)}
                  />
                </Fragment>
              ))}
            </SortableContext>
          </DndContext>
        )}
      </section>
    </div>
  )
}

interface PlanningSectionItemProps {
  section: GamePlanningSection
  displayTitle: string
  displayDescription: string
  duplicateTitle: boolean
  canRemove: boolean
  onChange: (next: Partial<GamePlanningSection>) => void
  onRemove: () => void
}

function PlanningSectionItem({
  section,
  displayTitle,
  displayDescription,
  duplicateTitle,
  canRemove,
  onChange,
  onRemove,
}: PlanningSectionItemProps) {
  const { t } = useTranslation()
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id: section.id })
  const titleMissing = !section.title.trim()

  return (
    <article
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(
        'grid grid-cols-[1.75rem_minmax(0,1fr)_1.75rem] items-start gap-x-2 px-2 py-2.5 transition-colors sm:gap-x-3 sm:px-3 sm:py-3',
        isDragging && 'relative z-10 bg-[var(--nova-active)] shadow-lg',
      )}
    >
      <Button
        ref={setActivatorNodeRef}
        type="button"
        variant="ghost"
        size="icon-xs"
        aria-label={t('settingPanel.gamePlanning.reorderSection')}
        {...attributes}
        {...listeners}
      >
        <GripVertical />
      </Button>

      <FieldGroup className="gap-2">
        <Field className="gap-1" data-invalid={titleMissing || duplicateTitle || undefined}>
          <FieldLabel className="sr-only" htmlFor={`planning-section-title-${section.id}`}>
            {t('settingPanel.gamePlanning.sectionTitle')}
          </FieldLabel>
          <Input
            id={`planning-section-title-${section.id}`}
            className={cn(presetInputClassName, 'font-medium')}
            value={displayTitle}
            placeholder={t('settingPanel.gamePlanning.sectionTitle')}
            aria-invalid={titleMissing || duplicateTitle || undefined}
            onChange={(event) => onChange({ title: event.target.value })}
          />
          {titleMissing ? <FieldError>{t('settingPanel.gamePlanning.sectionTitleRequired')}</FieldError> : null}
          {duplicateTitle ? <FieldError>{t('settingPanel.gamePlanning.duplicateSectionTitle')}</FieldError> : null}
        </Field>
        <Field className="gap-1">
          <FieldLabel className="sr-only" htmlFor={`planning-section-description-${section.id}`}>
            {t('settingPanel.gamePlanning.sectionDescription')}
          </FieldLabel>
          <Textarea
            id={`planning-section-description-${section.id}`}
            value={displayDescription}
            placeholder={t('settingPanel.gamePlanning.sectionDescription')}
            minRows={1}
            maxRows={6}
            className="nova-field min-h-0 text-xs leading-5 focus-visible:ring-0"
            onChange={(event) => onChange({ description: event.target.value })}
          />
        </Field>
      </FieldGroup>

      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        disabled={!canRemove}
        aria-label={t('settingPanel.gamePlanning.removeSection')}
        onClick={onRemove}
      >
        <Trash2 />
      </Button>
    </article>
  )
}

interface SectionValidation {
  valid: boolean
  duplicateSectionIDs: Set<string>
}

function validateSections(sections: GamePlanningSection[]): SectionValidation {
  const seenTitles = new Set<string>()
  const duplicateSectionIDs = new Set<string>()
  for (const section of sections) {
    const key = section.title.trim().toLocaleLowerCase()
    if (!key) continue
    if (seenTitles.has(key)) duplicateSectionIDs.add(section.id)
    else seenTitles.add(key)
  }
  return {
    valid: sections.length > 0 && sections.every((section) => Boolean(section.title.trim())) && duplicateSectionIDs.size === 0,
    duplicateSectionIDs,
  }
}

function createSectionID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `section-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}
