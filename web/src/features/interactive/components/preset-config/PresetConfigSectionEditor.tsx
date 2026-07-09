import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Editor, type OnMount } from '@monaco-editor/react'
import { Braces, ChevronDown, ChevronRight, Eye } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatPresetJSON, isPlainObject, loadPresetConfigViewMode, savePresetConfigViewMode, type PresetConfigViewMode } from './utils'

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
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null)
  const onSaveRef = useRef(onSave)
  const validRef = useRef(true)
  const monacoTheme = resolvedTheme === 'light' ? 'light' : 'vs-dark'
  const valid = !jsonError && visualValid

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
      if (!isPlainObject(parsed)) throw new Error(t('settingPanel.storyDirector.jsonObjectRequired'))
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
    'h-7 rounded-full border-0 px-3 text-[11px] transition-[background-color,color,box-shadow,transform] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] active:scale-[0.98]',
    active
      ? 'bg-[var(--nova-text)] text-[var(--nova-surface)] shadow-[0_8px_20px_rgba(0,0,0,0.12)]'
      : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]',
  )

  return (
    <section className="rounded-[26px] border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.06)]">
      <div className="overflow-hidden rounded-[21px] bg-[var(--nova-surface)] shadow-[inset_0_1px_1px_rgba(255,255,255,0.05)]">
        <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[var(--nova-border)] px-4 py-4">
          <div className="min-w-0">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <div className="truncate text-[15px] font-semibold text-[var(--nova-text)]">{title}</div>
              <Badge variant="outline" className="h-6 rounded-full border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2.5 text-[11px] font-normal text-[var(--nova-text-faint)]">
                {summary}
              </Badge>
            </div>
            <div className="mt-1 max-w-[78ch] text-xs leading-5 text-[var(--nova-text-faint)]">{description}</div>
          </div>
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
            <div className="flex h-9 items-center gap-1 rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] p-1 shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]">
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
                className="nova-nav-item h-8 gap-1.5 rounded-full border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 text-[11px] text-[var(--nova-text-muted)] transition-[background-color,color,transform] duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)] active:scale-[0.98]"
                variant="outline"
                size="sm"
                onClick={toggleFolding}
              >
                {folded ? <ChevronDown /> : <ChevronRight />}
                {folded ? t('settingPanel.json.expandAll') : t('settingPanel.json.collapseAll')}
              </Button>
            ) : null}
          </div>
        </div>

        {viewMode === 'visual' ? (
          <div className="p-2 md:p-3" data-testid="preset-config-visual-editor">
            {children({ value, onChange, onValidityChange: setVisualValid, resetKey })}
          </div>
        ) : (
          <div className="nova-field m-3 h-[320px] min-h-44 max-h-[65vh] resize-y overflow-hidden rounded-[18px] p-0" data-testid="story-director-json-editor">
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
