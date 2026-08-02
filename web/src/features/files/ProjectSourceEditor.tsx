import { Editor, type OnMount } from '@monaco-editor/react'
import { FileWarning, Maximize2 } from 'lucide-react'
import { useTheme } from 'next-themes'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common/EmptyState'
import { ImagePreviewDialog } from '@/components/common/ImagePreviewDialog'
import { projectFileAssetURL, type ProjectFileDocument } from './api'
import { projectFileLanguage } from './file-language'

interface ProjectSourceEditorProps {
  projectId: string
  document: ProjectFileDocument
  value: string
  onChange: (value: string) => void
  onSave: () => void
}

/** Monaco-backed source surface shared by Book and General projects. */
export function ProjectSourceEditor({ projectId, document, value, onChange, onSave }: ProjectSourceEditorProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const onSaveRef = useRef(onSave)
  onSaveRef.current = onSave

  if (document.kind === 'image') {
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

  if (document.kind === 'binary') {
    return (
      <EmptyState
        variant="page"
        icon={FileWarning}
        title={t('files.editor.binaryTitle')}
        description={t('files.editor.binaryDescription')}
      />
    )
  }

  const handleMount: OnMount = (editor, monaco) => {
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => onSaveRef.current())
  }
  return (
    <Editor
      height="100%"
      path={projectFileModelPath(projectId, document.path)}
      language={projectFileLanguage(document.path)}
      theme={resolvedTheme === 'light' ? 'light' : 'vs-dark'}
      value={value}
      onChange={(next) => onChange(next ?? '')}
      onMount={handleMount}
      options={{
        ariaLabel: t('files.editor.source', { path: document.path }),
        automaticLayout: true,
        bracketPairColorization: { enabled: true },
        fixedOverflowWidgets: true,
        folding: true,
        formatOnPaste: true,
        glyphMargin: false,
        lineDecorationsWidth: 10,
        lineNumbers: 'on',
        lineNumbersMinChars: 3,
        minimap: { enabled: false },
        padding: { top: 12, bottom: 12 },
        renderLineHighlight: 'line',
        scrollBeyondLastLine: false,
        scrollbar: { horizontalScrollbarSize: 10, verticalScrollbarSize: 10 },
        tabSize: 2,
        wordWrap: 'off',
      }}
    />
  )
}

function projectFileModelPath(projectId: string, path: string) {
  const encodedPath = path.split('/').map(encodeURIComponent).join('/')
  return `file:///denova-project/${encodeURIComponent(projectId)}/${encodedPath}`
}
