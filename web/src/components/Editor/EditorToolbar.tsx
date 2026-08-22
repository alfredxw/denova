import { BookOpen, Check, Crosshair, ImagePlus, PanelLeft, Save, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { formatDateTime, formatLocaleNumber } from '@/i18n'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { EditorSettingsPanel } from './EditorSettingsPanel'
import type { EditorSettings, ReadingTypographySettings } from './EditorSettingsPanel'

export type SaveStatus = 'dirty' | 'auto-saving' | 'auto-saved' | 'manual-saving' | 'manual-saved' | 'error'

const SAVE_STATUS_META: Record<SaveStatus, { labelKey: string; ariaLabelKey: string; className: string; dotClassName?: string; subtle?: boolean }> = {
  dirty: {
    labelKey: 'editor.status.dirty',
    ariaLabelKey: 'editor.status.dirtyAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'bg-[var(--nova-text-faint)] opacity-60',
    subtle: true,
  },
  'auto-saving': {
    labelKey: 'editor.status.autoSaving',
    ariaLabelKey: 'editor.status.autoSavingAria',
    className: 'text-[var(--nova-text-faint)]',
    dotClassName: 'animate-pulse bg-[var(--nova-text-muted)] opacity-70',
    subtle: true,
  },
  'auto-saved': {
    labelKey: 'editor.status.autoSaved',
    ariaLabelKey: 'editor.status.autoSavedAria',
    className: 'text-[var(--nova-text-faint)]',
    subtle: true,
  },
  'manual-saving': {
    labelKey: 'editor.status.manualSaving',
    ariaLabelKey: 'editor.status.manualSavingAria',
    className: 'text-[var(--nova-text-muted)]',
  },
  'manual-saved': {
    labelKey: 'editor.status.manualSaved',
    ariaLabelKey: 'editor.status.manualSavedAria',
    className: 'text-[var(--nova-accent-green)]',
  },
  error: {
    labelKey: 'editor.status.error',
    ariaLabelKey: 'editor.status.errorAria',
    className: 'text-[var(--nova-danger)]',
  },
}

interface EditorToolbarProps {
  fileName: string
  displayTitle?: string
  chapterPath?: string
  chapterWords?: number
  updatedAt?: string
  currentLine?: number
  saveStatus: SaveStatus | null
  onSave: () => void | Promise<void>
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
  updatedAt,
  currentLine,
  saveStatus,
  onSave,
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
  const saveStatusMeta = saveStatus ? SAVE_STATUS_META[saveStatus] : null
  const saveStatusLabel = saveStatusMeta ? t(saveStatusMeta.labelKey) : ''
  const saveStatusAriaLabel = saveStatusMeta ? t(saveStatusMeta.ariaLabelKey) : ''
  const updatedTime = formatDateTime(updatedAt)

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
            {updatedTime ? (
              <>
                <span aria-hidden>·</span>
                <span className="sm:hidden">{updatedTime}</span>
                <span className="hidden sm:inline">{t('editor.updatedAt', { time: updatedTime })}</span>
              </>
            ) : null}
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
          {saveStatusMeta && (
            <span
              className={`inline-flex h-5 min-w-5 items-center justify-end gap-1 text-[11px] transition-colors ${saveStatusMeta.className}`}
              aria-live="polite"
              aria-label={saveStatusAriaLabel}
            >
              {saveStatus === 'auto-saved' ? (
                <Check className="h-3 w-3 opacity-45" />
              ) : saveStatusMeta.dotClassName ? (
                <span className={`h-1.5 w-1.5 rounded-full ${saveStatusMeta.dotClassName}`} />
              ) : null}
              <span className={saveStatusMeta.subtle ? 'sr-only' : ''}>{saveStatusLabel}</span>
            </span>
          )}
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
          <Popover open={settingsOpen} onOpenChange={onSettingsOpenChange}>
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
              className="nova-editor-settings-panel w-[340px] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
            >
              <EditorSettingsPanel
                settings={settings}
                onChange={onSettingsChange}
                onClose={() => onSettingsOpenChange(false)}
                readingTypography={readingTypography}
              />
            </PopoverContent>
          </Popover>
        </div>
      </TooltipProvider>
    </div>
  )
}
