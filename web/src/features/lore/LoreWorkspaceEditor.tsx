import { useMemo, useState, type ReactNode } from 'react'
import {
  AtSign,
  BookMarked,
  ChevronDown,
  LibraryBig,
  SlidersHorizontal,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { LoreItem } from '@/lib/api'
import { SearchHighlightTextarea } from '@/components/common/SearchHighlightTextarea'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  AutosaveStatusIndicator,
  type AutosaveStatus,
} from '@/components/forms/autosave-status'
import { MobilePaneTrigger } from '@/components/layout/mobile-pane-trigger'
import type {
  DocumentReviewController,
  DocumentReviewNavigationIntent,
} from '@/features/document-review/controller'
import {
  IMPORTANCE_OPTIONS,
  LOAD_MODE_OPTIONS,
  loreImportanceLabel,
  loreLoadModeLabel,
  loreTypeLabel,
  TYPE_OPTIONS,
} from './options'
import { LoreContentEditor } from './LoreContentEditor'

interface LoreWorkspaceEditorProps {
  projectId: string
  draft: LoreItem
  tagDraft: string
  autosaveStatus: AutosaveStatus
  autosaveError: string | null
  documentReview: DocumentReviewController
  navigationIntent?: DocumentReviewNavigationIntent | null
  highlightQuery?: string
  onDraftChange: (draft: LoreItem) => void
  onTagDraftChange: (value: string) => void
  onPrepareSnapshot: () => Promise<{ content: string; revision: string }>
  onFlush: () => Promise<boolean>
  onDelete: (id: string) => Promise<boolean>
  onOpenDirectory?: () => void
  onOpenLibrary?: () => void
  onReferenceItem?: (id: string) => void
}

/** Focused lore editor for the writing workspace; bulk/library operations stay on the full page. */
export function LoreWorkspaceEditor({
  projectId,
  draft,
  tagDraft,
  autosaveStatus,
  autosaveError,
  documentReview,
  navigationIntent,
  highlightQuery,
  onDraftChange,
  onTagDraftChange,
  onPrepareSnapshot,
  onFlush,
  onDelete,
  onOpenDirectory,
  onOpenLibrary,
  onReferenceItem,
}: LoreWorkspaceEditorProps) {
  const { t } = useTranslation()
  const [metadataOpen, setMetadataOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string
    name: string
  } | null>(null)
  const target = useMemo(
    () => ({
      kind: 'lore_item' as const,
      id: draft.id,
      field: 'content' as const,
    }),
    [draft.id],
  )

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col bg-[var(--nova-bg)] text-[var(--nova-text)]">
      <div className="flex min-h-12 shrink-0 items-center gap-2 border-b border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-2">
        {onOpenDirectory ? (
          <MobilePaneTrigger
            side="left"
            label={t('loreWorkspace.openDirectory')}
            onClick={onOpenDirectory}
            appearance="compact"
          />
        ) : null}
        <BookMarked className="h-4 w-4 shrink-0 text-[var(--nova-success)]" />
        <div className="min-w-0 flex-1">
          <Input
            value={draft.name}
            onChange={(event) =>
              onDraftChange({ ...draft, name: event.target.value })
            }
            aria-label={t('settingPanel.field.name')}
            className="h-7 min-w-0 border-0 bg-transparent px-1 text-sm font-medium shadow-none focus-visible:ring-0"
          />
          <div className="flex min-w-0 items-center gap-1.5 px-1 text-[10px] text-[var(--nova-text-faint)]">
            <span>{loreTypeLabel(draft.type, t)}</span>
            <span aria-hidden>·</span>
            <span>{loreImportanceLabel(draft.importance, t)}</span>
            <span aria-hidden>·</span>
            <span>{loreLoadModeLabel(draft.load_mode, t)}</span>
          </div>
        </div>
        <AutosaveStatusIndicator
          status={autosaveStatus}
          error={autosaveError}
          onRetry={() => void onFlush()}
        />
        {onReferenceItem ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={() => onReferenceItem(draft.id)}
            aria-label={t('loreWorkspace.referenceAgent')}
          >
            <AtSign />
          </Button>
        ) : null}
        {onOpenLibrary ? (
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onOpenLibrary}
            aria-label={t('loreWorkspace.openLibrary')}
          >
            <LibraryBig />
          </Button>
        ) : null}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={autosaveStatus === 'saving'}
          onClick={() => setDeleteTarget({ id: draft.id, name: draft.name })}
          className="text-[var(--nova-text-faint)] hover:bg-[var(--nova-danger-bg)] hover:text-[var(--nova-danger)]"
          aria-label={t('settingPanel.deleteLore')}
        >
          <Trash2 />
        </Button>
      </div>

      <Collapsible
        open={metadataOpen}
        onOpenChange={setMetadataOpen}
        className="shrink-0 border-b border-[var(--nova-border)] bg-[var(--nova-surface-2)]"
      >
        <div className="flex min-w-0 items-center">
          <CollapsibleTrigger asChild>
            <button
              type="button"
              className="nova-nav-item group flex h-8 min-w-0 flex-1 items-center gap-2 px-3 text-left text-[11px] text-[var(--nova-text-muted)] hover:bg-[var(--nova-hover)]"
            >
              <SlidersHorizontal className="h-3.5 w-3.5 shrink-0" />
              <span className="shrink-0">{t('loreWorkspace.metadata')}</span>
              <span className="hidden min-w-0 flex-1 truncate text-right text-[var(--nova-text-faint)] sm:block">
                {draft.brief_description || t('loreWorkspace.noBrief')}
              </span>
              <ChevronDown className="h-3.5 w-3.5 shrink-0 transition-transform group-data-[state=open]:rotate-180" />
            </button>
          </CollapsibleTrigger>
        </div>
        <CollapsibleContent className="grid min-w-0 gap-2 border-t border-[var(--nova-border)] px-4 py-3 sm:grid-cols-2 xl:grid-cols-4">
          <MetadataField label={t('settingPanel.field.enabled')}>
            <div className="flex h-8 items-center justify-between rounded-md border border-[var(--nova-border)] bg-[var(--nova-surface)] px-2">
              <span className="text-[11px] text-[var(--nova-text-muted)]">
                {draft.enabled === false
                  ? t('settingPanel.disabled')
                  : t('settingPanel.enabled')}
              </span>
              <Switch
                aria-label={t('settingPanel.field.enabled')}
                checked={draft.enabled !== false}
                onCheckedChange={(enabled) =>
                  onDraftChange({ ...draft, enabled })
                }
              />
            </div>
          </MetadataField>
          <MetadataField label={t('settingPanel.field.type')}>
            <Select
              value={draft.type}
              onValueChange={(type) =>
                onDraftChange({ ...draft, type: type as LoreItem['type'] })
              }
            >
              <SelectTrigger
                aria-label={t('settingPanel.field.type')}
                size="sm"
                className="nova-field w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {TYPE_OPTIONS.map(({ value }) => (
                    <SelectItem key={value} value={value}>
                      {loreTypeLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </MetadataField>
          <MetadataField label={t('settingPanel.field.importance')}>
            <Select
              value={draft.importance}
              onValueChange={(importance) =>
                onDraftChange({
                  ...draft,
                  importance: importance as LoreItem['importance'],
                })
              }
            >
              <SelectTrigger
                aria-label={t('settingPanel.field.importance')}
                size="sm"
                className="nova-field w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {IMPORTANCE_OPTIONS.map(({ value }) => (
                    <SelectItem key={value} value={value}>
                      {loreImportanceLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </MetadataField>
          <MetadataField label={t('settingPanel.field.loadMode')}>
            <Select
              value={draft.load_mode || 'auto'}
              onValueChange={(load_mode) =>
                onDraftChange({
                  ...draft,
                  load_mode: load_mode as LoreItem['load_mode'],
                })
              }
            >
              <SelectTrigger
                aria-label={t('settingPanel.field.loadMode')}
                size="sm"
                className="nova-field w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {LOAD_MODE_OPTIONS.map(({ value }) => (
                    <SelectItem key={value} value={value}>
                      {loreLoadModeLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </MetadataField>
          <MetadataField
            label={t('settingPanel.field.tags')}
            className="sm:col-span-2"
          >
            <Input
              aria-label={t('settingPanel.field.tags')}
              className="nova-field h-8"
              value={tagDraft}
              onChange={(event) => onTagDraftChange(event.target.value)}
              placeholder={t('settingPanel.placeholder.tags')}
            />
          </MetadataField>
          <MetadataField
            label={t('settingPanel.field.brief')}
            className="sm:col-span-2"
          >
            <SearchHighlightTextarea
              aria-label={t('settingPanel.field.brief')}
              autoResize
              highlightQuery={highlightQuery}
              value={draft.brief_description || ''}
              onChange={(event) =>
                onDraftChange({
                  ...draft,
                  brief_description: event.target.value,
                })
              }
              className="nova-field min-h-8 text-xs"
            />
          </MetadataField>
        </CollapsibleContent>
      </Collapsible>

      <LoreContentEditor
          projectId={projectId}
          resourceKey={draft.id}
          value={draft.content || ''}
          onChange={(content) => onDraftChange({ ...draft, content })}
          onSaveShortcut={() => {
            void onFlush()
          }}
          highlightQuery={highlightQuery}
          review={{
            target,
            resourceLabel: draft.name,
            controller: documentReview,
            prepareSnapshot: onPrepareSnapshot,
            navigationIntent,
          }}
          richAriaLabel={t('loreWorkspace.contentLabel', { name: draft.name })}
          sourceAriaLabel={t('loreWorkspace.rawContentLabel', { name: draft.name })}
          editorClassName="bg-[var(--nova-bg)] text-sm leading-7 [&_.tiptap]:mx-auto [&_.tiptap]:min-h-full [&_.tiptap]:w-full [&_.tiptap]:max-w-[880px] [&_.tiptap]:px-6 [&_.tiptap]:py-8 md:[&_.tiptap]:px-10 md:[&_.tiptap]:py-10"
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('settingPanel.deleteLore')}
        description={t('settingPanel.confirmDeleteLore', {
          name: deleteTarget?.name || '',
        })}
        confirmLabel={t('common.delete')}
        tone="danger"
        onConfirm={() =>
          deleteTarget ? onDelete(deleteTarget.id) : false
        }
      />
    </div>
  )
}

function MetadataField({
  label,
  className = '',
  children,
}: {
  label: string
  className?: string
  children: ReactNode
}) {
  return (
    <div
      role="group"
      aria-label={label}
      className={`grid min-w-0 gap-1 text-[10px] text-[var(--nova-text-faint)] ${className}`}
    >
      <span>{label}</span>
      {children}
    </div>
  )
}
