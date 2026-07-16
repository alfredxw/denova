import { BookOpen, Check, ImagePlus, Save, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { TooltipIconButton } from '@/components/common/tooltip-icon-button'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { EditorSettingsPanel } from './EditorSettingsPanel'
import type { EditorSettings } from './EditorSettingsPanel'

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
  saveStatus: SaveStatus | null
  onSave: () => void | Promise<void>
  settingsOpen: boolean
  onSettingsOpenChange: (open: boolean) => void
  settings: EditorSettings
  onSettingsChange: (settings: EditorSettings) => void
  onGenerateIllustration?: (chapterPath: string) => void
  generateIllustrationDisabled: boolean
}

export function EditorToolbar({
  fileName,
  displayTitle,
  chapterPath,
  saveStatus,
  onSave,
  settingsOpen,
  onSettingsOpenChange,
  settings,
  onSettingsChange,
  onGenerateIllustration,
  generateIllustrationDisabled,
}: EditorToolbarProps) {
  const { t } = useTranslation()
  const saveStatusMeta = saveStatus ? SAVE_STATUS_META[saveStatus] : null
  const saveStatusLabel = saveStatusMeta ? t(saveStatusMeta.labelKey) : ''
  const saveStatusAriaLabel = saveStatusMeta ? t(saveStatusMeta.ariaLabelKey) : ''

  return (
    <div className="nova-editor-toolbar flex h-9 shrink-0 items-center justify-between gap-3 overflow-hidden border-b px-3">
      <div className="flex min-w-0 items-center gap-2 text-xs text-[var(--nova-text-muted)]">
        <BookOpen className="h-3.5 w-3.5 shrink-0 text-[var(--nova-text-muted)]" />
        <span className="truncate font-medium text-[var(--nova-text)]">{displayTitle || fileName}</span>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {saveStatusMeta && (
          <span
            className={`inline-flex h-5 min-w-5 items-center justify-end gap-1 text-[11px] transition-colors ${saveStatusMeta.className}`}
            aria-live="polite"
            aria-label={saveStatusAriaLabel}
            title={saveStatusAriaLabel}
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
            className="text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] disabled:cursor-not-allowed disabled:opacity-45"
            disabled={generateIllustrationDisabled || !chapterPath}
            onClick={() => {
              if (chapterPath) onGenerateIllustration(chapterPath)
            }}
          >
            <ImagePlus className="h-3.5 w-3.5" />
          </TooltipIconButton>
        )}
        <Button
          type="button"
          onClick={onSave}
          size="xs"
          variant="ghost"
          className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
        >
          <Save className="w-3.5 h-3.5" />
          {t('editor.save')}
        </Button>
        <Popover open={settingsOpen} onOpenChange={onSettingsOpenChange}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              size="xs"
              variant="ghost"
              className="flex items-center gap-1 text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              aria-label={t('editor.settings')}
            >
              <Settings className="h-3.5 w-3.5" />
              {t('editor.settingsShort')}
            </Button>
          </PopoverTrigger>
          <PopoverContent
            align="end"
            side="bottom"
            className="nova-editor-settings-panel w-[340px] overflow-hidden rounded-lg border border-[var(--nova-border)] p-0 text-[var(--nova-text)]"
          >
            <EditorSettingsPanel
              settings={settings}
              onChange={onSettingsChange}
              onClose={() => onSettingsOpenChange(false)}
            />
          </PopoverContent>
        </Popover>
      </div>
    </div>
  )
}
