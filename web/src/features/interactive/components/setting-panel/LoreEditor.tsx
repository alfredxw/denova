import { useState } from 'react'
import { FileCode2, Loader2, Sparkles, Trash2, Type } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { SearchHighlightTextarea } from '@/components/common/SearchHighlightTextarea'
import { MarkdownRichEditor } from '@/components/Editor/MarkdownRichEditor'
import { projectFileAssetURL, type LoreItem } from '@/lib/api'
import type { ImagePreset } from '../../types'
import { presetActionButtonClassName as actionButtonClassName, presetIconActionClassName as iconActionClassName, presetInputClassName as inputClassName, presetSelectClassName as selectClassName } from '../preset-config/editor-styles'
import { PresetEmptyState as EmptyState } from '../preset-config/PresetEmptyState'
import { PresetField as Field } from '../preset-config/PresetField'
import { BooleanSwitchField } from './BooleanSwitchField'
import { IMPORTANCE_OPTIONS, LOAD_MODE_OPTIONS, loadModeDescription, LORE_RESIDENT_TOTAL_WARNING_BYTES, loreImportanceLabel, loreLoadModeLabel, loreTypeLabel, TYPE_OPTIONS } from '@/features/lore/options'
import type { DocumentReviewController, DocumentReviewNavigationIntent } from '@/features/document-review/controller'
import type { DocumentReviewSnapshot } from '@/components/Editor/documentReviewAnchors'

export function LoreEditor({
  projectId,
  draft,
  tagDraft,
  residentTotalBytes,
  imagePresets,
  imagePresetId,
  imageInstruction,
  imageGenerating,
  searchQuery,
  setDraft,
  setTagDraft,
  onImagePresetChange,
  setImageInstruction,
  onGenerateImage,
  onClearImage,
  onSave,
  documentReview,
  documentReviewNavigationIntent,
  onPrepareReviewSnapshot,
}: {
  projectId: string
  draft: LoreItem | null
  tagDraft: string
  residentTotalBytes: number
  imagePresets: ImagePreset[]
  imagePresetId: string
  imageInstruction: string
  imageGenerating: boolean
  searchQuery?: string
  setDraft: (draft: LoreItem | null) => void
  setTagDraft: (value: string) => void
  onImagePresetChange: (id: string) => void
  setImageInstruction: (value: string) => void
  onGenerateImage: () => void
  onClearImage: () => void
  onSave: () => void
  documentReview?: DocumentReviewController
  documentReviewNavigationIntent?: DocumentReviewNavigationIntent | null
  onPrepareReviewSnapshot?: () => Promise<DocumentReviewSnapshot>
}) {
  const { t } = useTranslation()
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  // Default to rich text while retaining the chosen mode when switching lore entries.
  const [contentMode, setContentMode] = useState<'rich' | 'raw'>('rich')
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noLoreSelected')} description={t('settingPanel.editor.noLoreSelectedDesc')} />
  }

  const residentWarning = draft.enabled !== false && draft.load_mode === 'resident' && residentTotalBytes > LORE_RESIDENT_TOTAL_WARNING_BYTES
  const imagePath = draft.image?.image_path || ''
  const imageSrc = imagePath ? projectFileAssetURL(projectId, imagePath) : ''
  const hasImage = Boolean(imageSrc)
  const validImagePresets = imagePresets.filter((preset) => !preset.invalid)
  const selectedImagePresetId = imagePresetId || validImagePresets[0]?.id || 'game-cg'
  const openGenerateLabel = imagePath ? t('settingPanel.loreImage.openRegenerate') : t('settingPanel.loreImage.openGenerate')
  const topGridClassName = cn(
    'grid shrink-0 items-stretch gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5 sm:px-4',
    hasImage && 'lg:grid-cols-[15rem_minmax(0,1fr)] 2xl:grid-cols-[18rem_minmax(0,1fr)]',
  )
  const imageAction = (
    <Button className={iconActionClassName} variant="outline" size="icon-sm" disabled={imageGenerating} onClick={() => setImageDialogOpen(true)} aria-label={openGenerateLabel}>
      {imageGenerating ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
    </Button>
  )

  return (
    <>
      <ScrollArea className="min-h-0 flex-1" role="region" aria-label={t('settingPanel.lore.editorScrollArea')}>
        <div className="flex min-h-full min-w-0 flex-col">
          <div className={topGridClassName}>
            {hasImage ? (
              <div className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-1.5">
                <div className="flex min-w-0 items-center justify-between gap-2">
                  <span className="text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.current')}</span>
                  {imageAction}
                </div>
                <LoreImageCompactControl
                  imageSrc={imageSrc}
                  title={draft.name || t('settingPanel.loreImage.current')}
                  alt={draft.image?.alt_text || draft.name}
                />
              </div>
            ) : null}
            <div className="grid min-w-0 gap-1.5" role="group" aria-label={t('settingPanel.lore.metadata')}>
              {!hasImage ? (
                <div className="flex min-h-7 min-w-0 items-center gap-2">
                  <span className="shrink-0 text-[11px] text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.current')}</span>
                  <span className="min-w-0 flex-1 truncate text-xs text-[var(--nova-text-faint)]">{t('settingPanel.loreImage.empty')}</span>
                  {imageAction}
                </div>
              ) : null}
              <div
                data-slot="lore-primary-fields"
                className={cn(
                  'grid min-w-0 grid-cols-2 gap-2 md:grid-cols-3',
                  hasImage
                    ? '2xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]'
                    : 'xl:grid-cols-[minmax(12rem,2fr)_repeat(4,minmax(7rem,1fr))]',
                )}
              >
                <Field label={t('settingPanel.field.name')} className={cn('col-span-2', hasImage ? '2xl:col-span-1' : 'xl:col-span-1')}>
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
                <Field label={t('settingPanel.field.tags')}>
                  <Input className={inputClassName} value={tagDraft} onChange={(event) => setTagDraft(event.target.value)} placeholder={t('settingPanel.placeholder.tags')} />
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
          {/* Keep the mode switch and editor on one surface so the content remains visually connected. */}
          <div className="flex min-h-[420px] min-w-0 flex-1 flex-col bg-[var(--nova-bg)]">
            <div className="flex shrink-0 items-center px-3 pt-2.5 sm:px-4">
              <div role="group" aria-label={t('settingPanel.field.content')} className="inline-flex shrink-0 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-0.5">
                <button
                  type="button"
                  onClick={() => setContentMode('rich')}
                  aria-pressed={contentMode === 'rich'}
                  className={cn(
                    'nova-nav-item inline-flex h-6 items-center gap-1 rounded px-2 text-[11px]',
                    contentMode === 'rich' ? 'is-active' : 'text-[var(--nova-text-muted)]',
                  )}
                >
                  <Type className="h-3.5 w-3.5" />
                  {t('settingPanel.editor.contentModeRich')}
                </button>
                <button
                  type="button"
                  onClick={() => setContentMode('raw')}
                  aria-pressed={contentMode === 'raw'}
                  className={cn(
                    'nova-nav-item inline-flex h-6 items-center gap-1 rounded px-2 text-[11px]',
                    contentMode === 'raw' ? 'is-active' : 'text-[var(--nova-text-muted)]',
                  )}
                >
                  <FileCode2 className="h-3.5 w-3.5" />
                  {t('common.raw')}
                </button>
              </div>
            </div>
            {contentMode === 'raw' ? (
              <SearchHighlightTextarea
                containerClassName="min-h-0 flex-1"
                autoResize={false}
                spellCheck={false}
                value={draft.content || ''}
                highlightQuery={searchQuery}
                onChange={(event) => setDraft({ ...draft, content: event.target.value })}
                aria-label={t('settingPanel.field.content')}
                className="h-full min-h-0 min-w-0 resize-none rounded-none border-0 bg-transparent px-5 pb-4 pt-2 font-mono text-xs leading-5 text-[var(--nova-text)] shadow-none focus-visible:ring-0 sm:px-6 md:text-xs"
              />
            ) : (
              <MarkdownRichEditor
                projectId={projectId}
                key={draft.id}
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
                aria-label={t('settingPanel.field.content')}
                className="flex min-h-0 min-w-0 flex-1 flex-col text-xs leading-5 [&_.tiptap]:min-h-0 [&_.tiptap]:min-w-0 [&_.tiptap]:flex-1 [&_.tiptap]:px-5 [&_.tiptap]:pb-4 [&_.tiptap]:pt-2 sm:[&_.tiptap]:px-6"
              />
            )}
          </div>
        </div>
      </ScrollArea>
      <LoreImageGenerateDialog
        open={imageDialogOpen}
        itemName={draft.name || t('settingPanel.loreImage.current')}
        imagePath={imagePath}
        imagePresets={validImagePresets}
        imagePresetId={selectedImagePresetId}
        imageInstruction={imageInstruction}
        imageGenerating={imageGenerating}
        onOpenChange={setImageDialogOpen}
        onImagePresetChange={onImagePresetChange}
        setImageInstruction={setImageInstruction}
        onGenerateImage={onGenerateImage}
        onClearImage={onClearImage}
      />
    </>
  )
}

function LoreImageCompactControl({
  imageSrc,
  title,
  alt,
}: {
  imageSrc: string
  title: string
  alt: string
}) {
  const { t } = useTranslation()

  return (
    <div className="flex h-full min-h-48 min-w-0 overflow-hidden rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
      <ImagePreviewDialog src={imageSrc} title={title} alt={alt}>
        <button type="button" className="group h-full w-full overflow-hidden bg-[var(--nova-surface)]" aria-label={t('settingPanel.loreImage.openPreview')}>
          <img src={imageSrc} alt={alt} className="h-full w-full object-cover transition group-hover:scale-[1.03]" />
        </button>
      </ImagePreviewDialog>
    </div>
  )
}

function LoreImageGenerateDialog({
  open,
  itemName,
  imagePath,
  imagePresets,
  imagePresetId,
  imageInstruction,
  imageGenerating,
  onOpenChange,
  onImagePresetChange,
  setImageInstruction,
  onGenerateImage,
  onClearImage,
}: {
  open: boolean
  itemName: string
  imagePath: string
  imagePresets: ImagePreset[]
  imagePresetId: string
  imageInstruction: string
  imageGenerating: boolean
  onOpenChange: (open: boolean) => void
  onImagePresetChange: (id: string) => void
  setImageInstruction: (value: string) => void
  onGenerateImage: () => void
  onClearImage: () => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[min(calc(100vw-2rem),560px)] gap-3 border border-[var(--nova-border)] bg-[var(--nova-surface)] text-[var(--nova-text)]">
        <DialogHeader>
          <DialogTitle>{imagePath ? t('settingPanel.loreImage.regenerate') : t('settingPanel.loreImage.generate')}</DialogTitle>
          <DialogDescription>{t('settingPanel.loreImage.dialogDesc', { name: itemName })}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-3">
          <Field label={t('settingPanel.loreImage.preset')}>
            <Select value={imagePresetId} onValueChange={onImagePresetChange} disabled={imageGenerating}>
              <SelectTrigger size="sm" className={selectClassName}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="nova-panel border text-[var(--nova-text)]">
                <SelectGroup>
                  {imagePresets.length > 0 ? imagePresets.map((preset) => (
                    <SelectItem key={preset.id} value={preset.id}>{preset.name}</SelectItem>
                  )) : (
                    <SelectItem value="game-cg">{t('settingPanel.editor.defaultImagePreset')}</SelectItem>
                  )}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field label={t('settingPanel.loreImage.instruction')}>
            <Textarea
              className="nova-field min-h-28 resize-y text-xs leading-5 shadow-none focus-visible:ring-0"
              value={imageInstruction}
              onChange={(event) => setImageInstruction(event.target.value)}
              placeholder={t('settingPanel.loreImage.instructionPlaceholder')}
              disabled={imageGenerating}
            />
          </Field>
        </div>

        <DialogFooter className="border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          <Button className={actionButtonClassName} variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={!imagePath || imageGenerating} onClick={onClearImage}>
            <Trash2 data-icon="inline-start" />
            {t('settingPanel.loreImage.clear')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={imageGenerating} onClick={onGenerateImage}>
            {imageGenerating ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
            {imagePath ? t('settingPanel.loreImage.regenerate') : t('settingPanel.loreImage.generate')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
