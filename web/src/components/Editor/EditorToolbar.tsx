import { BookOpen, Code2, Crosshair, FileText, ImagePlus, PanelLeft, Save, Settings, WrapText } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { formatLocaleNumber } from '@/i18n'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { EditorSettingsPanel } from './EditorSettingsPanel'
import type { EditorSettings, ReadingTypographySettings } from './EditorSettingsPanel'
import { EditorSaveStatus, type SaveStatus } from './EditorSaveStatus'

export type { SaveStatus } from './EditorSaveStatus'
export type WritingEditorMode = 'document' | 'source'

interface EditorToolbarProps {
  fileName: string
  displayTitle?: string
  chapterPath?: string
  chapterWords?: number
  currentLine?: number
  saveStatus: SaveStatus | null
  onSave: () => void | Promise<void>
  editorMode: WritingEditorMode
  onEditorModeChange: (mode: WritingEditorMode) => void
  sourceWordWrap: boolean
  onSourceWordWrapToggle: () => void
  settingsOpen: boolean
  onSettingsOpenChange: (open: boolean) => void
  settings: EditorSettings
  onSettingsChange: (settings: EditorSettings) => void
  readingTypography?: ReadingTypographySettings
  onOpenOutline?: () => void
  onGenerateIllustration?: (chapterPath: string) => void
  onRevealChapter?: (chapterPath: string) => void
  generateIllustrationDisabled: boolean
}

export function EditorToolbar({
  fileName,
  displayTitle,
  chapterPath,
  chapterWords,
  currentLine,
  saveStatus,
  onSave,
  editorMode,
  onEditorModeChange,
  sourceWordWrap,
  onSourceWordWrapToggle,
  settingsOpen,
  onSettingsOpenChange,
  settings,
  onSettingsChange,
  readingTypography,
  onOpenOutline,
  onGenerateIllustration,
  onRevealChapter,
  generateIllustrationDisabled,
}: EditorToolbarProps) {
  const { t } = useTranslation()

  return (
    <div className="nova-editor-toolbar flex h-9 shrink-0 items-center justify-between gap-3 overflow-hidden border-b px-3">
      <div className="flex min-w-0 flex-1 items-center gap-2 text-xs text-[var(--nova-text-muted)]">
        {onOpenOutline ? (
          <TooltipIconButton
            label={t('planning.outlineNavigation')}
            onClick={onOpenOutline}
            size="icon-xs"
            showTooltip={false}
            tooltipSide="bottom"
            className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          >
            <PanelLeft className="h-3.5 w-3.5" />
          </TooltipIconButton>
        ) : null}
        <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <span className="min-w-0 truncate font-medium text-[var(--nova-text)]">{displayTitle || fileName}</span>
        {chapterWords !== undefined ? (
          <span className="flex shrink-0 items-center gap-1.5 whitespace-nowrap text-[11px] font-normal text-[var(--nova-text-faint)]">
            <span>{t('common.words', { count: formatLocaleNumber(chapterWords) })}</span>
            {currentLine !== undefined ? (
              <>
                <span aria-hidden>·</span>
                <span>{t('editor.currentLine', { line: formatLocaleNumber(currentLine) })}</span>
              </>
            ) : null}
          </span>
        ) : null}
      </div>
      <TooltipProvider>
        <div className="flex shrink-0 items-center gap-1">
          <EditorSaveStatus status={saveStatus} />
          <ToggleGroup
            type="single"
            value={editorMode}
            onValueChange={(value) => {
              if (value === 'document' || value === 'source') onEditorModeChange(value)
            }}
            variant="outline"
            size="sm"
            spacing={0}
            aria-label={t('editor.mode.label')}
            className="rounded-[var(--nova-radius)]"
          >
            <ToggleGroupItem
              value="document"
              aria-label={t('editor.mode.document')}
              className="h-6 min-w-6 rounded-[var(--nova-radius)] border-[var(--nova-border)] px-1.5 text-[10px] text-[var(--nova-text-muted)] data-[state=on]:bg-[var(--nova-active)] data-[state=on]:text-[var(--nova-text)] sm:px-2"
            >
              <FileText className="size-3" />
              <span className="hidden sm:inline">{t('editor.mode.document')}</span>
            </ToggleGroupItem>
            <ToggleGroupItem
              value="source"
              aria-label={t('editor.mode.source')}
              className="h-6 min-w-6 rounded-[var(--nova-radius)] border-[var(--nova-border)] px-1.5 text-[10px] text-[var(--nova-text-muted)] data-[state=on]:bg-[var(--nova-active)] data-[state=on]:text-[var(--nova-text)] sm:px-2"
            >
              <Code2 className="size-3" />
              <span className="hidden sm:inline">{t('editor.mode.source')}</span>
            </ToggleGroupItem>
          </ToggleGroup>
          {editorMode === 'source' ? (
            <TooltipIconButton
              label={t(sourceWordWrap ? 'files.editor.disableWordWrap' : 'files.editor.enableWordWrap')}
              size="icon-xs"
              tooltipSide="bottom"
              useTooltipProvider={false}
              className={sourceWordWrap ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)]'}
              aria-pressed={sourceWordWrap}
              onClick={onSourceWordWrapToggle}
            >
              <WrapText className="h-3.5 w-3.5" />
            </TooltipIconButton>
          ) : null}
          {onGenerateIllustration && (
            <TooltipIconButton
              label={generateIllustrationDisabled ? t('editor.generateIllustrationDisabled') : t('editor.generateIllustration')}
              size="icon-xs"
              tooltipSide="bottom"
              useTooltipProvider={false}
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-45"
              disabled={generateIllustrationDisabled || !chapterPath}
              onClick={() => {
                if (chapterPath) onGenerateIllustration(chapterPath)
              }}
            >
              <ImagePlus className="h-3.5 w-3.5" />
            </TooltipIconButton>
          )}
          {chapterPath && onRevealChapter ? (
            <TooltipIconButton
              label={t('editor.revealChapterInOutlineTooltip')}
              onClick={() => onRevealChapter(chapterPath)}
              size="icon-xs"
              tooltipSide="bottom"
              useTooltipProvider={false}
              className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
            >
              <Crosshair className="h-3.5 w-3.5" />
            </TooltipIconButton>
          ) : null}
          <TooltipIconButton
            label={t('editor.save')}
            onClick={onSave}
            size="icon-xs"
            tooltipSide="bottom"
            useTooltipProvider={false}
            className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          >
            <Save className="w-3.5 h-3.5" />
          </TooltipIconButton>
          {editorMode === 'document' ? <Popover open={settingsOpen} onOpenChange={onSettingsOpenChange}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    size="icon-xs"
                    variant="ghost"
                    className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
                    aria-label={t('editor.settings')}
                  >
                    <Settings className="h-3.5 w-3.5" />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent side="bottom" sideOffset={6}>{t('editor.settings')}</TooltipContent>
            </Tooltip>
            <PopoverContent
              align="end"
              side="bottom"
              className="nova-editor-settings-panel w-[320px] max-w-[calc(100vw-16px)] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
            >
              <EditorSettingsPanel
                settings={settings}
                onChange={onSettingsChange}
                onClose={() => onSettingsOpenChange(false)}
                readingTypography={readingTypography}
              />
            </PopoverContent>
          </Popover> : null}
        </div>
      </TooltipProvider>
    </div>
  )
}
