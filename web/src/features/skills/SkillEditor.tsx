import { useMemo } from 'react'
import { Copy, FileCode2, FileText, ListTree, Loader2, Lock, RefreshCw, Settings2, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { MarkdownEditPreview, MarkdownViewToggle } from '@/components/common/MarkdownEditPreview'
import { FileTree } from '@/components/Sidebar/FileTree'
import type { SkillDocument, SkillFileDocument, SkillScopeInfo, SkillSummary } from '@/lib/api'
import type { SkillContentViewMode } from './skill-utils'
import {
  collectSkillFileTreeDirs,
  isMarkdownSkillFile,
  keyOf,
  scopeLabel,
  skillDisplayPath,
  skillEntryFile,
  skillFileTreeForDocument,
  stripSkillMarkdownFrontmatter,
} from './skill-utils'

interface SkillEditorProps {
  document: SkillDocument
  fileDocument: SkillFileDocument | null
  draft: string
  fileDraft: string
  dirty: boolean
  selectedFilePath: string
  viewMode: SkillContentViewMode
  fileTreeOpen: boolean
  fileLoading: boolean
  saving: boolean
  builtinOverride: SkillSummary | null
  builtinOverrideScope: SkillScopeInfo | null
  builtinPeer: SkillSummary | null
  onDraftChange: (value: string) => void
  onFileDraftChange: (value: string) => void
  onSelectFile: (path: string) => void
  onToggleFileTree: () => void
  onViewModeChange: (mode: SkillContentViewMode) => void
  onOpenConfig: () => void
  onDelete: () => void
  onRestoreBuiltin: () => void
  onCreateBuiltinOverride: () => void
}

/** Skill 编辑器：文件头栏 + 目录 FileTree + Markdown 预览/原文编辑区。 */
export function SkillEditor({
  document,
  fileDocument,
  draft,
  fileDraft,
  dirty,
  selectedFilePath,
  viewMode,
  fileTreeOpen,
  fileLoading,
  saving,
  builtinOverride,
  builtinOverrideScope,
  builtinPeer,
  onDraftChange,
  onFileDraftChange,
  onSelectFile,
  onToggleFileTree,
  onViewModeChange,
  onOpenConfig,
  onDelete,
  onRestoreBuiltin,
  onCreateBuiltinOverride,
}: SkillEditorProps) {
  const { t } = useTranslation()
  const editingEntryFile = selectedFilePath === skillEntryFile
  const activeContent = editingEntryFile ? draft : fileDraft
  const activePreviewContent = stripSkillMarkdownFrontmatter(activeContent)
  const activeEditable = editingEntryFile ? Boolean(document.editable) : Boolean(fileDocument?.file.editable)
  const activeDisplayPath = skillDisplayPath(document, selectedFilePath)
  const activeIsMarkdown = isMarkdownSkillFile(selectedFilePath)
  const activeViewMode: SkillContentViewMode = activeIsMarkdown ? viewMode : 'raw'
  const skillFileTree = useMemo(() => skillFileTreeForDocument(document), [document])
  const skillFileTreeExpandedPaths = useMemo(() => collectSkillFileTreeDirs(skillFileTree), [skillFileTree])

  return (
    <>
      <div className="flex min-h-12 shrink-0 items-center gap-3 border-b border-[var(--nova-border)] px-4">
        {editingEntryFile ? <FileCode2 className="h-4 w-4 text-[var(--nova-text-muted)]" /> : <FileText className="h-4 w-4 text-[var(--nova-text-muted)]" />}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm text-[var(--nova-text)]">{editingEntryFile ? `/${document.name}` : selectedFilePath}</span>
            <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{scopeLabel(document.scope, t)}</span>
            {!editingEntryFile && <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{t('skills.files.reference')}</span>}
            {!document.active && <span className="rounded bg-[var(--nova-warning-bg)] px-1.5 py-0.5 text-[10px] text-[var(--nova-warning)]">{t('skills.shadowed')}</span>}
            {document.agent && <span className="rounded bg-[var(--nova-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--nova-text-muted)]">{document.agent}</span>}
            {!activeEditable && <Lock className="h-3.5 w-3.5 text-[var(--nova-text-faint)]" />}
          </div>
          <div className="mt-0.5 truncate text-[11px] text-[var(--nova-text-faint)]" title={activeDisplayPath}>{activeDisplayPath}</div>
        </div>
        {dirty && <span className="text-[11px] text-[var(--nova-warning)]">{t('skills.unsaved')}</span>}
        {document.editable && (
          <>
            <button
              type="button"
              onClick={onOpenConfig}
              className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px]"
            >
              <Settings2 className="h-3.5 w-3.5" />
              {t('skills.config.action')}
            </button>
            {builtinPeer && (
              <button
                type="button"
                onClick={onRestoreBuiltin}
                disabled={saving}
                className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] disabled:cursor-not-allowed disabled:opacity-45"
              >
                <RefreshCw className="h-3.5 w-3.5" />
                {t('skills.restoreBuiltin.action')}
              </button>
            )}
            <button
              type="button"
              onClick={onDelete}
              disabled={saving}
              className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] text-[var(--nova-danger)] disabled:cursor-not-allowed disabled:opacity-45"
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t('skills.delete.action')}
            </button>
          </>
        )}
        <button
          type="button"
          onClick={onToggleFileTree}
          aria-pressed={fileTreeOpen}
          className={`nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] px-2 text-[11px] ${fileTreeOpen ? 'is-active' : 'bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]'}`}
        >
          <ListTree className="h-3.5 w-3.5" />
          {t('skills.files.title')}
        </button>
        <MarkdownViewToggle
          preview={activeViewMode === 'preview'}
          onPreviewChange={(preview) => onViewModeChange(preview ? 'preview' : 'raw')}
          previewDisabled={!activeIsMarkdown}
          previewDisabledReason={t('skills.editor.previewUnavailable')}
        />
        {document.scope === 'builtin' && (
          <button
            type="button"
            onClick={onCreateBuiltinOverride}
            disabled={saving || (!builtinOverrideScope && !builtinOverride)}
            title={!builtinOverrideScope && !builtinOverride ? t('skills.override.noWritable') : undefined}
            className="nova-nav-item inline-flex h-7 shrink-0 items-center gap-1 rounded border border-[var(--nova-border)] bg-[var(--nova-surface-2)] px-2 text-[11px] disabled:cursor-not-allowed disabled:opacity-45"
          >
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : builtinOverride ? <FileCode2 className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
            {builtinOverride
              ? t('skills.override.open', { scope: scopeLabel(builtinOverride.scope, t) })
              : t('skills.override.create', { scope: scopeLabel(builtinOverrideScope?.scope || 'user', t) })}
          </button>
        )}
      </div>
      <div className="flex min-h-0 flex-1">
        {fileTreeOpen && (
          <aside className="flex min-h-0 w-[min(42vw,15rem)] min-w-36 shrink-0 flex-col border-r border-[var(--nova-border)] bg-[var(--nova-surface)]">
            <div className="flex h-9 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] px-3 text-[10px] font-medium uppercase text-[var(--nova-text-faint)]">
              <FileText className="h-3.5 w-3.5" />
              <span className="truncate">{t('skills.files.title')}</span>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-2">
              <FileTree
                key={keyOf(document)}
                nodes={skillFileTree}
                selectedFile={selectedFilePath}
                onSelectFile={onSelectFile}
                defaultExpandedPaths={skillFileTreeExpandedPaths}
              />
            </div>
          </aside>
        )}
        <div className="min-h-0 min-w-0 flex flex-1 flex-col">
          {fileLoading ? (
            <div className="flex min-h-0 flex-1 items-center justify-center gap-2 text-xs text-[var(--nova-text-faint)]">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t('skills.files.loading')}
            </div>
          ) : (
            <MarkdownEditPreview
              value={activeContent}
              onChange={(value) => editingEntryFile ? onDraftChange(value) : onFileDraftChange(value)}
              preview={activeViewMode === 'preview'}
              readOnly={!activeEditable}
              previewContent={activePreviewContent}
            />
          )}
        </div>
      </div>
    </>
  )
}
