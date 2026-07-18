import { useState } from 'react'
import { Loader2, Sparkles, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isSaveShortcut } from '@/lib/keyboard'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { MarkdownViewToggle } from '@/components/common/MarkdownEditPreview'
import { ThemedMarkdownRenderer } from '@/components/common/MarkdownRenderer'
import { SearchHighlightTextarea } from '@/components/common/SearchHighlightTextarea'
import { workspaceAssetURL, type LoreItem } from '@/lib/api'
import type { ImagePreset } from '../../types'
import { BooleanSwitchField } from './BooleanSwitchField'
import { actionButtonClassName, EmptyState, Field, IMPORTANCE_OPTIONS, iconActionClassName, inputClassName, LOAD_MODE_OPTIONS, loadModeDescription, LORE_RESIDENT_TOTAL_WARNING_BYTES, loreImportanceLabel, loreLoadModeLabel, loreTypeLabel, selectClassName, TYPE_OPTIONS } from './editor-shared'

export function LoreEditor({
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
}: {
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
}) {
  const { t } = useTranslation()
  const [imageDialogOpen, setImageDialogOpen] = useState(false)
  const [bodyPreview, setBodyPreview] = useState(false)
  if (!draft) {
    return <EmptyState title={t('settingPanel.editor.noLoreSelected')} description={t('settingPanel.editor.noLoreSelectedDesc')} />
  }

  const residentWarning = draft.enabled !== false && draft.load_mode === 'resident' && residentTotalBytes > LORE_RESIDENT_TOTAL_WARNING_BYTES
  const imagePath = draft.image?.image_path || ''
  const imageSrc = imagePath ? workspaceAssetURL(imagePath) : ''
  const hasImage = Boolean(imageSrc)
  const validImagePresets = imagePresets.filter((preset) => !preset.invalid)
  const selectedImagePresetId = imagePresetId || validImagePresets[0]?.id || 'game-cg'
  const openGenerateLabel = imagePath ? t('settingPanel.loreImage.openRegenerate') : t('settingPanel.loreImage.openGenerate')
  const topGridClassName = cn(
    'grid shrink-0 items-stretch gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2.5 sm:px-4',
    hasImage && 'lg:grid-cols-[15rem_minmax(0,1fr)] 2xl:grid-cols-[18rem_minmax(0,1fr)]',
  )
  const imageAction = (
    <Button className={iconActionClassName} variant="outline" size="icon-sm" disabled={imageGenerating} onClick={() => setImageDialogOpen(true)} aria-label={openGenerateLabel} title={openGenerateLabel}>
      {imageGenerating ? <Loader2 className="animate-spin" /> : <Sparkles />}
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
          <div className="min-h-[420px] flex-1 p-3 sm:p-4">
            <div className="mb-2 flex min-w-0 items-center justify-between gap-3">
              <span className="text-xs font-medium text-[var(--nova-text)]">{t('settingPanel.field.content')}</span>
              <MarkdownViewToggle preview={bodyPreview} onPreviewChange={setBodyPreview} />
            </div>
            {bodyPreview ? (
              <div className="min-h-0 flex-1 overflow-y-auto bg-[var(--nova-bg)] px-5 py-4">
                <ThemedMarkdownRenderer content={draft.content || ''} className="max-w-4xl text-xs leading-5" />
              </div>
            ) : (
              <SearchHighlightTextarea
                autoResize
                maxRows={Number.POSITIVE_INFINITY}
                highlightQuery={searchQuery}
                aria-label={t('settingPanel.field.content')}
                className="nova-field min-h-[360px] overflow-y-hidden overscroll-y-auto! resize-none font-mono text-sm leading-7 shadow-none focus-visible:ring-0"
                value={draft.content || ''}
                onChange={(event) => setDraft({ ...draft, content: event.target.value })}
                onKeyDown={(event) => {
                  if (isSaveShortcut(event)) {
                    event.preventDefault()
                    event.stopPropagation()
                    onSave()
                  }
                }}
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
        <button type="button" className="group h-full w-full overflow-hidden bg-[var(--nova-surface)]" aria-label={t('settingPanel.loreImage.openPreview')} title={t('settingPanel.loreImage.openPreview')}>
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
                {imagePresets.length > 0 ? imagePresets.map((preset) => (
                  <SelectItem key={preset.id} value={preset.id}>{preset.name}</SelectItem>
                )) : (
                  <SelectItem value="game-cg">{t('settingPanel.editor.defaultImagePreset')}</SelectItem>
                )}
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
            <Trash2 className="h-4 w-4" />
            {t('settingPanel.loreImage.clear')}
          </Button>
          <Button className={actionButtonClassName} variant="outline" size="sm" disabled={imageGenerating} onClick={onGenerateImage}>
            {imageGenerating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}
            {imagePath ? t('settingPanel.loreImage.regenerate') : t('settingPanel.loreImage.generate')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
