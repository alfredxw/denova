import { LoreDetailTabs } from '@/features/lore/LoreDetailTabs'
import { useId } from 'react'
import { Star } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SearchHighlightTextarea } from '@/components/common/SearchHighlightTextarea'
import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { type LoreItem, type LoreMaterial } from '@/lib/api'
import { presetIconActionClassName as iconActionClassName, presetInputClassName as inputClassName, presetSelectClassName as selectClassName } from '../preset-config/editor-styles'
import { PresetEmptyState as EmptyState } from '../preset-config/PresetEmptyState'
import { PresetField as Field } from '../preset-config/PresetField'
import { BooleanSwitchField } from './BooleanSwitchField'
import { IMPORTANCE_OPTIONS, LOAD_MODE_OPTIONS, loadModeDescription, LORE_RESIDENT_TOTAL_WARNING_BYTES, loreImportanceLabel, loreLoadModeLabel, loreTypeLabel, TYPE_OPTIONS } from '@/features/lore/options'
import type { DocumentReviewController, DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import type { DocumentReviewSnapshot } from '@/components/Editor/documentReviewAnchors'
import { LoreContentEditor } from '@/features/lore/LoreContentEditor'
import { hasLoreProtagonistTag, splitLoreTags, toggleLoreProtagonistTag } from '@/features/lore/tags'

export function LoreEditor({
  projectId,
  onInspectMaterial,
  draft,
  tagDraft,
  residentTotalBytes,
  searchQuery,
  setDraft,
  setTagDraft,
  onSave,
  documentReview,
  documentReviewNavigationIntent,
  onPrepareReviewSnapshot,
}: {
  projectId: string
  onInspectMaterial?: (material: LoreMaterial) => void
  draft: LoreItem | null
  tagDraft: string
  residentTotalBytes: number
  searchQuery?: string
  setDraft: (draft: LoreItem | null) => void
  setTagDraft: (value: string) => void
  onSave: () => void
  documentReview?: DocumentReviewController
  documentReviewNavigationIntent?: DocumentReviewNavigationIntent | null
  onPrepareReviewSnapshot?: () => Promise<DocumentReviewSnapshot>
}) {
  const { t } = useTranslation()
  const tagInputId = useId()
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noLoreSelected')} description={t('settingPanel.editor.noLoreSelectedDesc')} />
  }

  const residentWarning = draft.enabled !== false && draft.load_mode === 'resident' && residentTotalBytes > LORE_RESIDENT_TOTAL_WARNING_BYTES
  const protagonistTagActive = draft.type === 'character' && hasLoreProtagonistTag(splitLoreTags(tagDraft))
  const toggleProtagonistTag = () => {
    const tags = toggleLoreProtagonistTag(splitLoreTags(tagDraft))
    setDraft({ ...draft, tags })
    setTagDraft(tags.join('，'))
  }

  return (
    <LoreDetailTabs projectId={projectId} item={draft} onChange={setDraft} onInspectMaterial={onInspectMaterial}>
      {/* Mobile grows with content for page scrolling; desktop needs Radix's wrapper to inherit the pane height. */}
      <ScrollArea
        className="min-h-0 flex-1"
        viewportProps={{ className: 'md:[&>div]:h-full' }}
        role="region"
        aria-label={t('settingPanel.lore.editorScrollArea')}
      >
        <div className="flex min-h-full min-w-0 flex-col">
          <div className="shrink-0 border-b px-3 py-2.5 sm:px-4">
            <div className="grid min-w-0 gap-1.5" role="group" aria-label={t('settingPanel.lore.metadata')}>
              <div
                data-slot="lore-primary-fields"
                className={cn(
                  'grid min-w-0 grid-cols-2 gap-2 md:grid-cols-3',
                  'xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]',
                )}
              >
                <Field label={t('settingPanel.field.name')} className={cn('col-span-2', 'xl:col-span-1')}>
                  <Input className={inputClassName} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
                </Field>
                <BooleanSwitchField label={t('settingPanel.field.enabled')} checked={draft.enabled ?? true} onCheckedChange={(enabled) => setDraft({ ...draft, enabled })} />
                <Field label={t('settingPanel.field.type')}>
                  <Select value={draft.type} onValueChange={(value) => setDraft({ ...draft, type: value as LoreItem['type'] })}>
                    <SelectTrigger size="sm" className={selectClassName}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectGroup>
                        {TYPE_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>{loreTypeLabel(option.value, t)}</SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t('settingPanel.field.importance')}>
                  <Select value={draft.importance} onValueChange={(value) => setDraft({ ...draft, importance: value as LoreItem['importance'] })}>
                    <SelectTrigger size="sm" className={selectClassName}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectGroup>
                        {IMPORTANCE_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>{loreImportanceLabel(option.value, t)}</SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t('settingPanel.field.loadMode')}>
                  <Select value={draft.load_mode || 'auto'} onValueChange={(value) => setDraft({ ...draft, load_mode: value as LoreItem['load_mode'] })}>
                    <SelectTrigger size="sm" className={selectClassName}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent className="nova-panel border text-[var(--nova-text)]">
                      <SelectGroup>
                        {LOAD_MODE_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>{loreLoadModeLabel(option.value, t)}</SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
              <div data-slot="lore-secondary-fields" className="grid min-w-0 items-start gap-2 md:grid-cols-[minmax(10rem,0.8fr)_minmax(0,1.2fr)]">
                <Field label={t('settingPanel.field.tags')} htmlFor={tagInputId}>
                  <div className="flex min-w-0 items-center gap-1.5">
                    <Input id={tagInputId} className={cn(inputClassName, 'min-w-0 flex-1')} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
                    {draft.type === 'character' ? (
                      <TooltipIconButton
                        label={t(protagonistTagActive ? 'loreWorkspace.unmarkProtagonist' : 'loreWorkspace.markProtagonist')}
                        variant="outline"
                        size="icon"
                        tooltipSide="bottom"
                        aria-pressed={protagonistTagActive}
                        onClick={toggleProtagonistTag}
                        className={cn(
                          iconActionClassName,
                          protagonistTagActive && 'border-[var(--nova-warning)] bg-[var(--nova-warning-bg)] text-[var(--nova-warning)] hover:bg-[var(--nova-warning-bg)] hover:text-[var(--nova-warning)]',
                        )}
                      >
                        <Star data-icon="inline-start" className={protagonistTagActive ? 'fill-current' : undefined} />
                      </TooltipIconButton>
                    ) : null}
                  </div>
                </Field>
                <Field label={t('settingPanel.field.brief')}>
                  <SearchHighlightTextarea
                    autoResize
                    highlightQuery={searchQuery}
                    className="nova-field min-h-14 resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
                    value={draft.brief_description || ''}
                    onChange={(event) => setDraft({ ...draft, brief_description: event.target.value })}
                    placeholder={t('settingPanel.placeholder.brief')}
                  />
                </Field>
              </div>
              <div className="min-w-0 text-[11px] leading-4 text-[var(--nova-text-faint)]">
                {draft.load_mode === 'resident' ? t('settingPanel.lore.residentDesc') : loadModeDescription(draft.load_mode, t)}
                {residentWarning ? (
                  <span className="ml-2 text-[var(--nova-warning)]">
                    {t('settingPanel.lore.residentWarning', { size: Math.ceil(residentTotalBytes / 1024), threshold: LORE_RESIDENT_TOTAL_WARNING_BYTES / 1024 })}
                  </span>
                ) : null}
              </div>
            </div>
          </div>
          <div className="flex min-h-[420px] min-w-0 flex-1 flex-col bg-[var(--nova-bg)]">
            <LoreContentEditor
                projectId={projectId}
                resourceKey={draft.id}
                value={draft.content || ''}
                onChange={(content) => setDraft({ ...draft, content })}
                highlightQuery={searchQuery}
                onSaveShortcut={onSave}
                review={documentReview && onPrepareReviewSnapshot ? {
                  target: { kind: 'lore_item', id: draft.id, field: 'content' },
                  resourceLabel: draft.name,
                  controller: documentReview,
                  prepareSnapshot: onPrepareReviewSnapshot,
                  navigationIntent: documentReviewNavigationIntent,
                } : undefined}
                richAriaLabel={t('settingPanel.field.content')}
                sourceAriaLabel={t('settingPanel.field.content')}
                editorClassName="text-xs leading-5 [&_.tiptap]:min-h-0 [&_.tiptap]:min-w-0 [&_.tiptap]:flex-1 [&_.tiptap]:px-5 [&_.tiptap]:pb-4 [&_.tiptap]:pt-2 sm:[&_.tiptap]:px-6"
            />
          </div>
        </div>
      </ScrollArea>
    </LoreDetailTabs>
  )
}
