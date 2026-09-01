import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Copy, GripVertical, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { EmptyState } from '@/components/common/EmptyState'
import {
  gamePlanningSectionDescription,
  gamePlanningSectionTitle,
  gamePlanningTemplateDescription,
  gamePlanningTemplateName,
} from '../../game-planning'
import type { GamePlanningSection, GamePlanningTemplate } from '../../types'

interface GamePlanningEditorProps {
  draft: GamePlanningTemplate | null
  setDraft: (draft: GamePlanningTemplate | null) => void
  onCopy: (template: GamePlanningTemplate) => void | Promise<void>
  onValidityChange: (valid: boolean) => void
}

export function GamePlanningEditor({ draft, setDraft, onCopy, onValidityChange }: GamePlanningEditorProps) {
  const { t } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const validation = useMemo(() => validateSections(draft?.sections || []), [draft?.sections])
  const nameValid = Boolean(draft?.name.trim())
  const readOnly = !draft?.custom

  useEffect(
    () => onValidityChange(Boolean(draft) && nameValid && validation.valid),
    [draft, nameValid, onValidityChange, validation.valid],
  )

  if (!draft) {
    return (
      <EmptyState
        title={t('settingPanel.editor.noGamePlanningSelected')}
        description={t('settingPanel.editor.noGamePlanningSelectedDesc')}
      />
    )
  }

  const patch = (next: Partial<GamePlanningTemplate>) => setDraft({ ...draft, ...next })
  const patchSection = (id: string, next: Partial<GamePlanningSection>) => patch({ sections: draft.sections.map((section) => section.id === id ? { ...section, ...next } : section) })
  const addSection = () => patch({ sections: [...draft.sections, { id: createSectionID(), title: '', description: '' }] })
  const removeSection = (id: string) => patch({ sections: draft.sections.filter((section) => section.id !== id) })
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const from = draft.sections.findIndex((section) => section.id === active.id)
    const to = draft.sections.findIndex((section) => section.id === over.id)
    if (from >= 0 && to >= 0) patch({ sections: arrayMove(draft.sections, from, to) })
  }

  return (
    <div className="h-full min-h-0 overflow-y-auto px-4 py-5 sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-5">
        {!draft.custom ? (
          <Card className="border-border bg-muted/30 shadow-none">
            <CardHeader>
              <CardTitle className="text-sm">{t('settingPanel.gamePlanning.builtinTitle')}</CardTitle>
              <CardDescription>{t('settingPanel.gamePlanning.builtinDescription')}</CardDescription>
              <CardAction>
                <Button type="button" variant="outline" size="sm" onClick={() => void onCopy(draft)}>
                  <Copy data-icon="inline-start" />
                  {t('settingPanel.gamePlanning.copy')}
                </Button>
              </CardAction>
            </CardHeader>
          </Card>
        ) : null}

        <Card>
          <CardHeader>
            <CardTitle>{t('settingPanel.gamePlanning.details')}</CardTitle>
            <CardDescription>{t('settingPanel.gamePlanning.detailsDescription')}</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field data-invalid={!nameValid || undefined}>
                <FieldLabel htmlFor="game-planning-name">{t('settingPanel.editor.name')}</FieldLabel>
                <Input
                  id="game-planning-name"
                  value={gamePlanningTemplateName(draft, t)}
                  disabled={readOnly}
                  aria-invalid={!nameValid || undefined}
                  onChange={(event) => patch({ name: event.target.value })}
                />
                {!nameValid ? (
                  <FieldDescription className="text-destructive">
                    {t('settingPanel.gamePlanning.nameRequired')}
                  </FieldDescription>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="game-planning-description">{t('settingPanel.editor.description')}</FieldLabel>
                <Textarea
                  id="game-planning-description"
                  value={gamePlanningTemplateDescription(draft, t)}
                  disabled={readOnly}
                  rows={3}
                  onChange={(event) => patch({ description: event.target.value })}
                />
              </Field>
            </FieldGroup>
          </CardContent>
        </Card>

        <section className="flex flex-col gap-3" aria-labelledby="game-planning-sections-title">
          <div className="flex items-end justify-between gap-3">
            <div>
              <h3 id="game-planning-sections-title" className="text-base font-semibold text-foreground">
                {t('settingPanel.gamePlanning.sections')}
              </h3>
              <p className="mt-1 text-sm text-muted-foreground">{t('settingPanel.gamePlanning.sectionsDescription')}</p>
            </div>
            {draft.custom ? (
              <Button type="button" variant="outline" size="sm" onClick={addSection}>
                <Plus data-icon="inline-start" />
                {t('settingPanel.gamePlanning.addSection')}
              </Button>
            ) : null}
          </div>

          {!validation.valid ? <p role="alert" className="text-sm text-destructive">{t(validation.messageKey)}</p> : null}

          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
            <SortableContext items={draft.sections.map((section) => section.id)} strategy={verticalListSortingStrategy}>
              <div className="flex flex-col gap-3">
                {draft.sections.map((section, index) => (
                  <PlanningSectionCard
                    key={section.id}
                    section={section}
                    index={index}
                    readOnly={readOnly}
                    displayTitle={gamePlanningSectionTitle(draft, section, t)}
                    displayDescription={gamePlanningSectionDescription(draft, section, t)}
                    duplicateTitle={validation.duplicateTitles.has(section.title.trim().toLocaleLowerCase())}
                    onChange={(next) => patchSection(section.id, next)}
                    onRemove={() => removeSection(section.id)}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        </section>
      </div>
    </div>
  )
}

interface PlanningSectionCardProps {
  section: GamePlanningSection
  index: number
  readOnly: boolean
  displayTitle: string
  displayDescription: string
  duplicateTitle: boolean
  onChange: (next: Partial<GamePlanningSection>) => void
  onRemove: () => void
}

function PlanningSectionCard({
  section,
  index,
  readOnly,
  displayTitle,
  displayDescription,
  duplicateTitle,
  onChange,
  onRemove,
}: PlanningSectionCardProps) {
  const { t } = useTranslation()
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging } = useSortable({ id: section.id, disabled: readOnly })
  return (
    <Card ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'relative z-10 border-primary/40 shadow-lg' : undefined}>
      <CardHeader>
        <CardTitle className="text-sm">{t('settingPanel.gamePlanning.sectionNumber', { number: index + 1 })}</CardTitle>
        <CardDescription>{displayTitle.trim() || t('settingPanel.gamePlanning.untitledSection')}</CardDescription>
        {!readOnly ? (
          <CardAction className="flex items-center gap-1">
            <Button
              ref={setActivatorNodeRef}
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t('settingPanel.gamePlanning.reorderSection')}
              {...attributes}
              {...listeners}
            >
              <GripVertical />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t('settingPanel.gamePlanning.removeSection')}
              onClick={onRemove}
            >
              <Trash2 />
            </Button>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        <FieldGroup>
          <Field data-invalid={!section.title.trim() || duplicateTitle || undefined}>
            <FieldLabel htmlFor={`planning-section-title-${section.id}`}>{t('settingPanel.gamePlanning.sectionTitle')}</FieldLabel>
            <Input
              id={`planning-section-title-${section.id}`}
              value={displayTitle}
              disabled={readOnly}
              aria-invalid={!section.title.trim() || duplicateTitle || undefined}
              onChange={(event) => onChange({ title: event.target.value })}
            />
            {duplicateTitle ? <FieldDescription className="text-destructive">{t('settingPanel.gamePlanning.duplicateSectionTitle')}</FieldDescription> : null}
          </Field>
          <Field>
            <FieldLabel htmlFor={`planning-section-description-${section.id}`}>{t('settingPanel.gamePlanning.sectionDescription')}</FieldLabel>
            <Textarea
              id={`planning-section-description-${section.id}`}
              value={displayDescription}
              disabled={readOnly}
              rows={4}
              onChange={(event) => onChange({ description: event.target.value })}
            />
            <FieldDescription>{t('settingPanel.gamePlanning.sectionDescriptionHint')}</FieldDescription>
          </Field>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

interface SectionValidation {
  valid: boolean
  messageKey: string
  duplicateTitles: Set<string>
}

function validateSections(sections: GamePlanningSection[]): SectionValidation {
  const counts = new Map<string, number>()
  for (const section of sections) {
    const key = section.title.trim().toLocaleLowerCase()
    if (key) counts.set(key, (counts.get(key) || 0) + 1)
  }
  const duplicateTitles = new Set([...counts.entries()].filter(([, count]) => count > 1).map(([title]) => title))
  if (sections.length === 0) return { valid: false, messageKey: 'settingPanel.gamePlanning.sectionsRequired', duplicateTitles }
  if (sections.some((section) => !section.title.trim())) return { valid: false, messageKey: 'settingPanel.gamePlanning.sectionTitleRequired', duplicateTitles }
  if (duplicateTitles.size > 0) return { valid: false, messageKey: 'settingPanel.gamePlanning.duplicateSectionTitle', duplicateTitles }
  return { valid: true, messageKey: '', duplicateTitles }
}

function createSectionID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `section-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
}
