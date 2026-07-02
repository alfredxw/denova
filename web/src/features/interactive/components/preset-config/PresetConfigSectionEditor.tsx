import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Editor, type OnMount } from '@monaco-editor/react'
import { Braces, ChevronDown, ChevronRight, Eye } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
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

  return (
    <section className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] p-4">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-xs font-medium text-[var(--nova-text)]">{title}</div>
          <div className="mt-1 text-[11px] leading-5 text-[var(--nova-text-faint)]">{description}</div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          <span className="rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 py-1 text-[11px] text-[var(--nova-text-faint)]">{summary}</span>
          <div className="flex h-7 overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
            <Button
              type="button"
              className={`h-full rounded-none border-0 px-2 text-[11px] ${viewMode === 'visual' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
              variant="ghost"
              size="sm"
              onClick={() => setMode('visual')}
              aria-pressed={viewMode === 'visual'}
            >
              <Eye className="h-3.5 w-3.5" />
              {t('settingPanel.presetConfig.visualView')}
            </Button>
            <Button
              type="button"
              className={`h-full rounded-none border-0 px-2 text-[11px] ${viewMode === 'json' ? 'bg-[var(--nova-active)] text-[var(--nova-text)]' : 'text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]'}`}
              variant="ghost"
              size="sm"
              onClick={() => setMode('json')}
              aria-pressed={viewMode === 'json'}
            >
              <Braces className="h-3.5 w-3.5" />
              {t('settingPanel.presetConfig.jsonView')}
            </Button>
          </div>
          {viewMode === 'json' ? (
            <Button
              type="button"
              className="nova-nav-item h-7 gap-1.5 rounded-[var(--nova-radius)] border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)] hover:text-[var(--nova-text)]"
              variant="outline"
              size="sm"
              onClick={toggleFolding}
            >
              {folded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
              {folded ? t('settingPanel.json.expandAll') : t('settingPanel.json.collapseAll')}
            </Button>
          ) : null}
        </div>
      </div>

      {viewMode === 'visual' ? (
        <div data-testid="preset-config-visual-editor">
          {children({ value, onChange, onValidityChange: setVisualValid })}
        </div>
      ) : (
        <div className="nova-field h-[320px] min-h-44 max-h-[65vh] resize-y overflow-hidden rounded-[var(--nova-radius)] p-0" data-testid="story-director-json-editor">
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
      {jsonError ? <div className="mt-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger-border)] bg-[var(--nova-danger-bg)] px-2 py-1 text-[11px] text-[var(--nova-danger)]">{jsonError}</div> : null}
      {jsonError && viewMode === 'json' ? <div className="mt-1 text-[11px] text-[var(--nova-danger)]">{t('settingPanel.presetConfig.fixJSONBeforeVisual')}</div> : null}
    </section>
  )
}
