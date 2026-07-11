import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Editor, type OnMount } from '@monaco-editor/react'
import { Braces, ChevronDown, ChevronRight, Eye } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatPresetJSON, isPlainObject, loadPresetConfigViewMode, savePresetConfigViewMode, type PresetConfigViewMode } from './utils'

export const PRESET_CONFIG_HEADER_ACTIONS_TARGET_ID = 'preset-config-header-actions'

export function PresetConfigSectionEditor<T extends object>({
  sectionId,
  resetKey,
  title,
  description,
  summary,
  value,
  onChange,
  onSave,
  onValidityChange,
  layout = 'card',
  hideHeaderText = false,
  headerActionsTargetId,
  children,
}: {
  sectionId: string
  resetKey: string
  title: string
  description: string
  summary: string
  value: T
  onChange: (value: T) => void
  onSave: () => void
  onValidityChange?: (valid: boolean) => void
  layout?: 'card' | 'flush'
  hideHeaderText?: boolean
  headerActionsTargetId?: string
  children: (props: {
    value: T
    onChange: (value: T) => void
    onValidityChange: (valid: boolean) => void
    resetKey: string
  }) => ReactNode
}) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [viewMode, setViewMode] = useState<PresetConfigViewMode>(() => loadPresetConfigViewMode(sectionId))
  const [jsonDraft, setJsonDraft] = useState(() => formatPresetJSON(value))
  const [jsonError, setJsonError] = useState('')
  const [visualValid, setVisualValid] = useState(true)
  const [folded, setFolded] = useState(false)
  const [headerActionsTarget, setHeaderActionsTarget] = useState<HTMLElement | null>(null)
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null)
  const onSaveRef = useRef(onSave)
  const validRef = useRef(true)
  const monacoTheme = resolvedTheme === 'light' ? 'light' : 'vs-dark'
  const jsonValueIsArray = Array.isArray(value)
  const valid = !jsonError && visualValid
  const flush = layout === 'flush'

  useEffect(() => {
    onSaveRef.current = onSave
  }, [onSave])

  useEffect(() => {
    validRef.current = valid
    onValidityChange?.(valid)
  }, [onValidityChange, valid])

  useEffect(() => {
    setJsonDraft(formatPresetJSON(value))
    setJsonError('')
    setVisualValid(true)
    setFolded(false)
  }, [resetKey])

  useEffect(() => {
    if (viewMode === 'visual' || !jsonError) setJsonDraft(formatPresetJSON(value))
  }, [jsonError, value, viewMode])

  useEffect(() => {
    if (!headerActionsTargetId) {
      setHeaderActionsTarget(null)
      return
    }
    setHeaderActionsTarget(document.getElementById(headerActionsTargetId))
    return () => setHeaderActionsTarget(null)
  }, [headerActionsTargetId])

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = editor
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      if (validRef.current) onSaveRef.current()
    })
  }

  const setMode = (mode: PresetConfigViewMode) => {
    if (mode === 'visual' && jsonError) return
    setViewMode(mode)
    savePresetConfigViewMode(sectionId, mode)
  }

  const updateJSON = (nextValue: string) => {
    setJsonDraft(nextValue)
    try {
      const parsed = JSON.parse(nextValue)
      if (jsonValueIsArray && !Array.isArray(parsed)) {
        throw new Error(t('settingPanel.presetConfig.jsonArrayRequired'))
      }
      if (!jsonValueIsArray && !isPlainObject(parsed)) {
        throw new Error(t('settingPanel.storyDirector.jsonObjectRequired'))
      }
      setJsonError('')
      onChange(parsed as T)
    } catch (err) {
      setJsonError(err instanceof Error ? err.message : t('settingPanel.storyDirector.invalidJSON'))
    }
  }

  const runEditorAction = (actionId: string) => {
    const action = editorRef.current?.getAction(actionId)
    void action?.run()
    editorRef.current?.focus()
  }

  const toggleFolding = () => {
    const nextFolded = !folded
    runEditorAction(nextFolded ? 'editor.foldAll' : 'editor.unfoldAll')
    setFolded(nextFolded)
  }
  const modeButtonClassName = (active: boolean) => cn(
    'h-7 rounded-[9px] border-0 px-3 text-[11px] transition-colors',
    active
      ? 'bg-[var(--nova-active)] text-[var(--nova-text)]'
      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
  )
  const headerActions = (
    <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
      <div className="flex h-8 items-center gap-1 rounded-[11px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1">
        <Button
          type="button"
          className={modeButtonClassName(viewMode === 'visual')}
          variant="ghost"
          size="sm"
          onClick={() => setMode('visual')}
          aria-pressed={viewMode === 'visual'}
        >
          <Eye />
          {t('settingPanel.presetConfig.visualView')}
        </Button>
        <Button
          type="button"
          className={modeButtonClassName(viewMode === 'json')}
          variant="ghost"
          size="sm"
          onClick={() => setMode('json')}
          aria-pressed={viewMode === 'json'}
        >
          <Braces />
          {t('settingPanel.presetConfig.jsonView')}
        </Button>
      </div>
      {viewMode === 'json' ? (
        <Button
          type="button"
          className="nova-nav-item h-8 gap-1.5 rounded-[10px] border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[11px] text-[var(--nova-text-muted)] transition-colors hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
          variant="outline"
          size="sm"
          onClick={toggleFolding}
        >
          {folded ? <ChevronDown /> : <ChevronRight />}
          {folded ? t('settingPanel.json.expandAll') : t('settingPanel.json.collapseAll')}
        </Button>
      ) : null}
    </div>
  )
  const showInlineHeaderActions = !headerActionsTarget
  const showHeader = !hideHeaderText || showInlineHeaderActions

  return (
    <section className={cn(
      flush
        ? 'flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-bg)]'
        : 'w-full self-start overflow-hidden rounded-[14px] border border-[var(--nova-border)] bg-[var(--nova-surface)]',
    )}>
      <div className={cn(
        flush
          ? 'flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-[var(--nova-surface)]'
          : 'overflow-hidden bg-transparent',
      )}>
        {headerActionsTarget ? createPortal(headerActions, headerActionsTarget) : null}
        {showHeader ? (
          <div className={cn(
            'flex flex-wrap items-start justify-between gap-x-3 gap-y-2 border-b border-[var(--nova-border)]',
            flush ? 'shrink-0 px-3 py-2' : 'px-3 py-2.5',
          )}>
            {!hideHeaderText ? (
              <div className="min-w-0 flex-1 basis-48">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <div className="truncate text-sm font-semibold text-[var(--nova-text)]">{title}</div>
                  <Badge variant="outline" className="h-5 rounded-full border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[10px] font-normal text-[var(--nova-text-faint)]">
                    {summary}
                  </Badge>
                </div>
                <div className="mt-0.5 line-clamp-2 max-w-[78ch] text-[11px] leading-4 text-[var(--nova-text-faint)]">{description}</div>
              </div>
            ) : null}
            {showInlineHeaderActions ? headerActions : null}
          </div>
        ) : null}

        {viewMode === 'visual' ? (
          <div className={cn('preset-config-visual-container', flush ? 'min-h-0 flex-1 p-0' : 'p-3')} data-testid="preset-config-visual-editor">
            {children({ value, onChange, onValidityChange: setVisualValid, resetKey })}
          </div>
        ) : (
          <div
            className={cn(
              'nova-field overflow-hidden rounded-[12px] p-0',
              flush ? 'm-3 min-h-44 flex-1' : 'm-3 h-[320px] min-h-44 resize-y',
            )}
            data-testid="story-director-json-editor"
          >
            <Editor
              height="100%"
              language="json"
              theme={monacoTheme}
              value={jsonDraft}
              onChange={(nextValue) => updateJSON(nextValue ?? '')}
              onMount={handleMount}
              options={{
                ariaLabel: title,
                automaticLayout: true,
                fixedOverflowWidgets: true,
                folding: true,
                foldingStrategy: 'indentation',
                formatOnPaste: true,
                formatOnType: true,
                glyphMargin: false,
                lineDecorationsWidth: 10,
                lineNumbers: 'on',
                lineNumbersMinChars: 3,
                minimap: { enabled: false },
                padding: { top: 12, bottom: 12 },
                renderLineHighlight: 'line',
                roundedSelection: true,
                scrollBeyondLastLine: false,
                scrollbar: {
                  horizontalScrollbarSize: 10,
                  verticalScrollbarSize: 10,
                },
                tabSize: 2,
                wordWrap: 'on',
              }}
            />
          </div>
        )}
        {jsonError ? <div className="mx-3 mb-3 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{jsonError}</div> : null}
        {jsonError && viewMode === 'json' ? <div className="mx-3 mb-3 text-[11px] text-[var(--nova-danger)]">{t('settingPanel.presetConfig.fixJSONBeforeVisual')}</div> : null}
      </div>
    </section>
  )
}
