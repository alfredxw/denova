import { type Monaco, type OnChange, type OnMount } from '@monaco-editor/react'
import { FileWarning, Maximize2 } from 'lucide-react'
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { IDisposable, editor } from 'monaco-editor'
import { EmptyState } from '@/components/common/EmptyState'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { DenovaMonacoEditor } from '@/components/monaco/DenovaMonaco'
import { projectFileAssetURL, type ProjectFileDocument } from '@/lib/api-client/project-files'
import { projectFileLanguage } from './file-language'

export interface ProjectSourceEditorProps {
  projectId: string
  document: ProjectFileDocument
  value: string
  wordWrap: boolean
  onWordWrapToggle: () => void
  onChange: (value: string) => void
  onSave: () => void
}

export interface ProjectTextEditorHandle {
  getValue: () => string
  replaceValue: (value: string) => void
  revealLine: (line: number) => void
  focus: () => void
}

export interface ProjectTextEditorProps extends ProjectSourceEditorProps {
  /** Files owns its value; Writing delegates external rebases to its shared persistence lane. */
  syncExternalValue?: boolean
  onQuoteSelection?: (selection: { content: string; startLine: number; endLine: number }) => void
}

/** Monaco-backed source surface shared by Writing and Game projects. */
export function ProjectSourceEditor(props: ProjectSourceEditorProps) {
  if (props.document.kind === 'image') return <ProjectImagePreview {...props} />
  if (props.document.kind === 'binary') return <ProjectBinaryPreview />
  return <ProjectTextEditor {...props} />
}

export const ProjectTextEditor = forwardRef<ProjectTextEditorHandle, ProjectTextEditorProps>(function ProjectTextEditor({
  projectId,
  document,
  value,
  wordWrap,
  onWordWrapToggle,
  onChange,
  onSave,
  syncExternalValue = true,
  onQuoteSelection,
}, forwardedRef) {
  const { t } = useTranslation()
  const editorRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const actionDisposablesRef = useRef<IDisposable[]>([])
  const initialValueRef = useRef(value)
  const valueRef = useRef(value)
  const onSaveRef = useRef(onSave)
  const onWordWrapToggleRef = useRef(onWordWrapToggle)
  const onQuoteSelectionRef = useRef(onQuoteSelection)
  onSaveRef.current = onSave
  onWordWrapToggleRef.current = onWordWrapToggle
  onQuoteSelectionRef.current = onQuoteSelection
  valueRef.current = value

  useImperativeHandle(forwardedRef, () => ({
    getValue: () => editorRef.current?.getValue() ?? valueRef.current,
    replaceValue: (nextValue) => {
      const model = editorRef.current?.getModel()
      if (model && model.getValue() !== nextValue) model.setValue(nextValue)
      valueRef.current = nextValue
    },
    revealLine: (line) => {
      const editorInstance = editorRef.current
      if (!editorInstance) return
      const safeLine = Math.max(1, Math.min(line, editorInstance.getModel()?.getLineCount() ?? 1))
      editorInstance.revealLineInCenter(safeLine)
      editorInstance.setPosition({ lineNumber: safeLine, column: 1 })
      editorInstance.focus()
    },
    focus: () => editorRef.current?.focus(),
  }), [])

  const options = useMemo<editor.IStandaloneEditorConstructionOptions>(() => ({
    ariaLabel: t('files.editor.source', { path: document.path }),
    automaticLayout: true,
    bracketPairColorization: { enabled: true },
    // vscode.dev uses Monaco's stable DOM text path by default. Keep the same
    // renderer until Monaco's experimental GPU path has equivalent lifecycle guarantees.
    experimentalGpuAcceleration: 'off',
    fixedOverflowWidgets: true,
    folding: true,
    formatOnPaste: true,
    glyphMargin: true,
    guides: {
      bracketPairs: 'active',
      indentation: true,
    },
    largeFileOptimizations: true,
    lineDecorationsWidth: 10,
    lineNumbers: 'on',
    lineNumbersMinChars: 3,
    minimap: {
      enabled: true,
      showSlider: 'mouseover',
      size: 'proportional',
    },
    renderLineHighlight: 'line',
    readOnly: !document.editable,
    smoothScrolling: false,
    stickyScroll: { enabled: true },
    tabSize: 2,
    wordWrap: wordWrap ? 'on' : 'off',
    wrappingStrategy: 'simple',
  }), [document.editable, document.path, t, wordWrap])

  const handleChange = useCallback<OnChange>((next, event) => {
    // Monaco emits a flush event when a model is created or replaced through
    // setValue. That is content synchronization, not a user edit, and must
    // never enter the dirty/autosave lifecycle.
    if (event?.isFlush) return
    onChange(next ?? '')
  }, [onChange])

  const handleMount = useCallback<OnMount>((editorInstance, monaco) => {
    editorRef.current = editorInstance
    const model = editorInstance.getModel()
    if (model && model.getValue() !== valueRef.current) model.setValue(valueRef.current)
    actionDisposablesRef.current.forEach((disposable) => disposable.dispose())
    actionDisposablesRef.current = installEditorActions(editorInstance, monaco, {
      saveLabel: t('files.editor.save'),
      wrapLabel: t('files.editor.toggleWordWrap'),
      quoteLabel: t('editor.quoteSelectionShortcut'),
      onSave: () => onSaveRef.current(),
      onWordWrapToggle: () => onWordWrapToggleRef.current(),
      onQuoteSelection: onQuoteSelectionRef.current ? () => {
        const selection = editorInstance.getSelection()
        const selectedText = selection ? editorInstance.getModel()?.getValueInRange(selection) ?? '' : ''
        if (!selection || !selectedText.trim()) return
        onQuoteSelectionRef.current?.({
          content: selectedText,
          startLine: selection.startLineNumber,
          endLine: selection.endLineNumber,
        })
      } : undefined,
    })
  }, [t])

  useEffect(() => () => {
    actionDisposablesRef.current.forEach((disposable) => disposable.dispose())
    actionDisposablesRef.current = []
    editorRef.current = null
  }, [])

  // The wrapper receives defaultValue rather than a controlled value. Normal
  // keystrokes therefore stay inside Monaco; only an external rebase touches
  // the model, eliminating a React round trip from the scrolling hot path.
  useEffect(() => {
    const model = editorRef.current?.getModel()
    if (!syncExternalValue) return
    const externalValue = valueRef.current
    if (model && model.getValue() !== externalValue) model.setValue(externalValue)
  }, [document.revision, syncExternalValue])

  return (
    <div
      className="h-full min-h-0 min-w-0 overflow-hidden"
      // Monaco mutates line DOM while scrolling. Size/layout/style containment
      // keeps those invalidations inside the editor without clipping widgets.
      style={{ contain: 'size layout style' }}
    >
      <DenovaMonacoEditor
        height="100%"
        path={projectFileModelPath(projectId, document.path)}
        language={projectFileLanguage(document.path)}
        defaultValue={initialValueRef.current}
        keepCurrentModel={false}
        saveViewState={false}
        onChange={handleChange}
        onMount={handleMount}
        options={options}
      />
    </div>
  )
})

function ProjectImagePreview({ projectId, document }: ProjectSourceEditorProps) {
  const { t } = useTranslation()
  const src = projectFileAssetURL(projectId, document.path, document.revision)
  const alt = t('files.editor.imageAlt', { path: document.path })
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto bg-[var(--nova-surface-2)] p-4">
      <ImagePreviewDialog src={src} title={alt} alt={alt} path={document.path}>
        <button
          type="button"
          className="group relative flex max-h-full max-w-full items-center justify-center rounded-lg border border-[var(--nova-border)] bg-black/80 p-2 shadow-[var(--nova-shadow)] transition hover:border-[var(--nova-accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--nova-accent)]"
          aria-label={alt}
        >
          <img src={src} alt={alt} className="max-h-[calc(100vh-9rem)] max-w-full rounded-md object-contain" />
          <span className="absolute right-3 top-3 rounded-md border border-white/20 bg-black/50 p-1.5 text-white opacity-0 backdrop-blur transition group-hover:opacity-100 group-focus-visible:opacity-100">
            <Maximize2 className="size-4" aria-hidden="true" />
          </span>
        </button>
      </ImagePreviewDialog>
    </div>
  )
}

export function ProjectBinaryPreview() {
  const { t } = useTranslation()
  return (
    <EmptyState
      variant="page"
      icon={FileWarning}
      title={t('files.editor.binaryTitle')}
      description={t('files.editor.binaryDescription')}
    />
  )
}

function installEditorActions(editorInstance: editor.IStandaloneCodeEditor, monaco: Monaco, actions: {
  saveLabel: string
  wrapLabel: string
  quoteLabel: string
  onSave: () => void
  onWordWrapToggle: () => void
  onQuoteSelection?: () => void
}): IDisposable[] {
  const disposables = [
    editorInstance.addAction({
      id: 'denova.project-file.save',
      label: actions.saveLabel,
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
      run: actions.onSave,
    }),
    editorInstance.addAction({
      id: 'denova.project-file.toggle-word-wrap',
      label: actions.wrapLabel,
      keybindings: [monaco.KeyMod.Alt | monaco.KeyCode.KeyZ],
      run: actions.onWordWrapToggle,
    }),
  ]
  if (actions.onQuoteSelection) {
    disposables.push(editorInstance.addAction({
      id: 'denova.project-file.quote-selection',
      label: actions.quoteLabel,
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyL],
      run: actions.onQuoteSelection,
    }))
  }
  return disposables
}

export function projectFileModelPath(projectId: string, path: string) {
  const encodedPath = path.split('/').map(encodeURIComponent).join('/')
  return `file:///denova-project/${encodeURIComponent(projectId)}/${encodedPath}`
}
