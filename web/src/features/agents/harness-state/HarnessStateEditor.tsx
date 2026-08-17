import { useEffect, useRef } from 'react'
import type { Monaco, OnMount } from '@monaco-editor/react'
import type { editor } from 'monaco-editor'
import { AlertCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DenovaMonacoEditor } from '@/components/monaco/DenovaMonaco'
import type { HarnessStateDiagnostic } from '@/lib/api'

interface HarnessStateEditorProps {
  path: string
  value: string
  diagnostics: HarnessStateDiagnostic[]
  selectedDiagnostic?: HarnessStateDiagnostic
  readOnly?: boolean
  onChange: (value: string) => void
  onSave: () => void
  onSelectDiagnostic: (diagnostic: HarnessStateDiagnostic) => void
}

const markerOwner = 'denova-harness-state'
const scriptDiagnosticTranslationKeys: Record<string, string> = {
  invalid_script_frontmatter: 'continualLearning.diagnostic.invalidScriptFrontmatter',
  script_tool_name_mismatch: 'continualLearning.diagnostic.scriptToolNameMismatch',
  script_description_missing: 'continualLearning.diagnostic.scriptDescriptionMissing',
  script_description_not_english: 'continualLearning.diagnostic.scriptDescriptionNotEnglish',
  script_agents_missing: 'continualLearning.diagnostic.scriptAgentsMissing',
  script_agent_invalid: 'continualLearning.diagnostic.scriptAgentInvalid',
  script_body_empty: 'continualLearning.diagnostic.scriptBodyEmpty',
  script_engine_unavailable: 'continualLearning.diagnostic.scriptEngineUnavailable',
  script_engine_invalid: 'continualLearning.diagnostic.scriptEngineInvalid',
  script_cancelled: 'continualLearning.diagnostic.scriptCancelled',
  script_source_invalid: 'continualLearning.diagnostic.scriptSourceInvalid',
  script_source_limit: 'continualLearning.diagnostic.scriptSourceLimit',
  script_schema_missing: 'continualLearning.diagnostic.scriptSchemaMissing',
  script_schema_invalid: 'continualLearning.diagnostic.scriptSchemaInvalid',
  script_schema_unknown_keyword: 'continualLearning.diagnostic.scriptSchemaUnknownKeyword',
  script_tool_name_conflict: 'continualLearning.diagnostic.scriptToolNameConflict',
  script_tool_exceeds_parent: 'continualLearning.diagnostic.scriptToolExceedsParent',
  script_compile_failed: 'continualLearning.diagnostic.scriptCompileFailed',
}

export function HarnessStateEditor({
  path,
  value,
  diagnostics,
  selectedDiagnostic,
  readOnly,
  onChange,
  onSave,
  onSelectDiagnostic,
}: HarnessStateEditorProps) {
  const { t } = useTranslation()
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const monacoRef = useRef<Monaco | null>(null)
  const saveRef = useRef(onSave)
  saveRef.current = onSave
  const pathDiagnostics = diagnostics.filter(diagnostic => diagnostic.path === path)

  const handleMount: OnMount = (instance, monaco) => {
    editorRef.current = instance
    monacoRef.current = monaco
    instance.addAction({
      id: 'denova.harness-state.save',
      label: 'Save Harness State',
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
      run: () => saveRef.current(),
    })
  }

  useEffect(() => {
    const model = editorRef.current?.getModel()
    const monaco = monacoRef.current
    if (!model || !monaco) return
    monaco.editor.setModelMarkers(model, markerOwner, pathDiagnostics.map(diagnostic => ({
      severity: monaco.MarkerSeverity.Error,
      message: localizedDiagnosticMessage(t, diagnostic),
      code: diagnostic.code,
      startLineNumber: Math.max(1, diagnostic.line || 1),
      startColumn: Math.max(1, diagnostic.column || 1),
      endLineNumber: Math.max(1, diagnostic.line || 1),
      endColumn: Math.max(2, (diagnostic.column || 1) + 1),
    })))
    return () => monaco.editor.setModelMarkers(model, markerOwner, [])
  }, [path, pathDiagnostics, t])

  useEffect(() => {
    if (!selectedDiagnostic || selectedDiagnostic.path !== path || !editorRef.current) return
    const position = {
      lineNumber: Math.max(1, selectedDiagnostic.line || 1),
      column: Math.max(1, selectedDiagnostic.column || 1),
    }
    editorRef.current.setPosition(position)
    editorRef.current.revealPositionInCenter(position)
    editorRef.current.focus()
  }, [path, selectedDiagnostic])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-hidden">
        <DenovaMonacoEditor
          height="100%"
          path={`harness-state://${path}`}
          language={harnessStateLanguage(path)}
          value={value}
          onChange={next => onChange(next || '')}
          onMount={handleMount}
          options={{
            automaticLayout: true,
            ariaLabel: t('continualLearning.editorLabel', { path }),
            readOnly,
            domReadOnly: readOnly,
            minimap: { enabled: false },
            fontSize: 12,
            lineHeight: 20,
            scrollBeyondLastLine: false,
            wordWrap: 'on',
            padding: { top: 12, bottom: 16 },
            stickyScroll: { enabled: false },
          }}
        />
      </div>
      {diagnostics.length > 0 ? (
        <section className="max-h-36 shrink-0 overflow-y-auto border-t border-[var(--nova-border)] bg-[var(--nova-surface-2)]" aria-label={t('continualLearning.problems')}>
          <div className="sticky top-0 flex items-center gap-1.5 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-3 py-1.5 text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--nova-danger)]">
            <AlertCircle className="size-3" />
            {t('continualLearning.problemCount', { count: diagnostics.length })}
          </div>
          {diagnostics.map((diagnostic, index) => (
            <button
              type="button"
              key={`${diagnostic.path || ''}:${diagnostic.line || 0}:${diagnostic.column || 0}:${diagnostic.code}:${index}`}
              className="flex w-full min-w-0 items-start gap-2 border-b border-[var(--nova-border)] px-3 py-1.5 text-left text-[11px] hover:bg-[var(--nova-hover)]"
              onClick={() => onSelectDiagnostic(diagnostic)}
            >
              <code className="shrink-0 text-[var(--nova-danger)]">{diagnostic.code}</code>
              <span className="min-w-0 flex-1 text-[var(--nova-text-muted)]">
                {localizedDiagnosticMessage(t, diagnostic)}
              </span>
              <span className="shrink-0 font-mono text-[var(--nova-text-faint)]">
                {diagnostic.path || path}{diagnostic.line ? `:${diagnostic.line}:${diagnostic.column || 1}` : ''}
              </span>
            </button>
          ))}
        </section>
      ) : null}
    </div>
  )
}

function harnessStateLanguage(path: string) {
  if (path.endsWith('.js')) return 'javascript'
  if (path.endsWith('.toml')) return 'toml'
  return 'markdown'
}

function localizedDiagnosticMessage(
  t: ReturnType<typeof useTranslation>['t'],
  diagnostic: HarnessStateDiagnostic,
) {
  const key = scriptDiagnosticTranslationKeys[diagnostic.code]
  return key ? t(key, { defaultValue: diagnostic.message }) : diagnostic.message
}
