import { Code2, Crosshair, Ellipsis, ImagePlus, PanelLeft, Save, Settings, WrapText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { formatLocaleNumber } from '@/i18n'
import { EditorSettingsPanel } from './EditorSettingsPanel'
import { EditorSaveStatus } from './EditorSaveStatus'
import type { EditorToolbarProps } from './EditorToolbar'

/** Document context belongs to the tab strip; this row owns editing status and commands. */
export function MobileEditorToolbar(props: EditorToolbarProps) {
  const { t } = useTranslation()
  const { chapterPath, chapterWords, saveStatus, editorMode, onOpenOutline, onGenerateIllustration, onRevealChapter } = props
  return (
    <div className="nova-mobile-editor-toolbar flex h-11 shrink-0 items-center justify-between gap-2 border-b px-3">
      <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
        {chapterWords !== undefined && <span className="truncate">{t('common.words', { count: formatLocaleNumber(chapterWords) })}{editorMode === 'source' && props.currentLine !== undefined && ` · ${t('editor.currentLine', { line: formatLocaleNumber(props.currentLine) })}`}</span>}
        <EditorSaveStatus status={saveStatus} />
      </div>
      <div className="flex shrink-0 items-center">
        {editorMode === 'document' && <Sheet open={props.settingsOpen} onOpenChange={props.onSettingsOpenChange}>
          <SheetTrigger asChild><Button variant="ghost" size="icon" aria-label={t('editor.settings')}><Settings /></Button></SheetTrigger>
          <SheetContent side="right" showCloseButton={false} className="nova-mobile-sheet gap-0 p-0">
            <SheetTitle className="sr-only">{t('editor.settings')}</SheetTitle>
            <div className="min-h-0 overflow-y-auto"><EditorSettingsPanel settings={props.settings} onChange={props.onSettingsChange} onClose={() => props.onSettingsOpenChange(false)} readingTypography={props.readingTypography} /></div>
          </SheetContent>
        </Sheet>}

        <DropdownMenu>
          <DropdownMenuTrigger asChild><Button variant="ghost" size="sm" aria-label={t('editor.tools')}><Ellipsis />{t('editor.tools')}</Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-64">
            <DropdownMenuGroup>
              <DropdownMenuCheckboxItem checked={editorMode === 'source'} onCheckedChange={(checked) => props.onEditorModeChange(checked ? 'source' : 'document')}><Code2 />{t('editor.mode.source')}</DropdownMenuCheckboxItem>
              {editorMode === 'source' && <DropdownMenuCheckboxItem checked={props.sourceWordWrap} onCheckedChange={props.onSourceWordWrapToggle}><WrapText />{t('files.editor.enableWordWrap')}</DropdownMenuCheckboxItem>}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              {onOpenOutline && <DropdownMenuItem onSelect={onOpenOutline}><PanelLeft />{t('planning.outlineNavigation')}</DropdownMenuItem>}
              {chapterPath && onRevealChapter && <DropdownMenuItem onSelect={() => onRevealChapter(chapterPath)}><Crosshair />{t('editor.revealChapterInOutlineTooltip')}</DropdownMenuItem>}
              {onGenerateIllustration && <DropdownMenuItem disabled={props.generateIllustrationDisabled || !chapterPath} onSelect={() => { if (chapterPath) onGenerateIllustration(chapterPath) }}><ImagePlus />{t('editor.generateIllustration')}</DropdownMenuItem>}
              <DropdownMenuItem onSelect={() => void props.onSave()}><Save />{t('editor.save')}</DropdownMenuItem>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}
