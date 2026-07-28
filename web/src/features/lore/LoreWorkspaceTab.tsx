import { BookMarked, Database, LibraryBig } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { AdaptiveSurface } from '@/components/layout/adaptive-surface'
import { ResourceDirectory } from '@/components/resource-directory/ResourceDirectory'
import type {
  ResourceDirectoryItem,
  ResourceDirectorySection,
} from '@/components/resource-directory/types'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/common/EmptyState'
import { InlineErrorNotice } from '@/components/common/inline-error-notice'
import type { EditorFlushHandler } from '@/components/Editor/useEditorDraftPersistence'
import type {
  DocumentReviewController,
  DocumentReviewNavigationIntent,
} from '@/features/document-review/controller'
import { workspaceAssetURL, type LoreItem } from '@/lib/api'
import { KNOWLEDGE_SECTIONS, sectionItems } from './knowledge-sections'
import { loreLoadModeLabel } from './options'
import { LoreWorkspaceEditor } from './LoreWorkspaceEditor'
import { useLoreWorkspace } from './use-lore-workspace'

interface LoreWorkspaceTabProps {
  workspace: string
  documentReview: DocumentReviewController
  navigationIntent?: DocumentReviewNavigationIntent | null
  onEditorFlushHandlerChange: (handler: EditorFlushHandler | null) => void
  onOpenLibrary?: () => void
  onReferenceItem?: (id: string) => void
}

/** Writing-side projection of the lore library: quick editing, review and Agent handoff. */
export function LoreWorkspaceTab({
  workspace,
  documentReview,
  navigationIntent,
  onEditorFlushHandlerChange,
  onOpenLibrary,
  onReferenceItem,
}: LoreWorkspaceTabProps) {
  const { t } = useTranslation()
  const lore = useLoreWorkspace({
    workspace,
    onFlushHandlerChange: onEditorFlushHandlerChange,
  })
  const navigationTargetID = navigationIntent
    ? documentReview.comments.find(
        (comment) => comment.id === navigationIntent.commentID,
      )?.target.id || ''
    : ''
  useEffect(() => {
    if (!navigationTargetID || navigationTargetID === lore.activeId) return
    void lore.selectItem(navigationTargetID)
  }, [lore.activeId, lore.selectItem, navigationTargetID])
  const sections = useMemo<ResourceDirectorySection[]>(
    () =>
      KNOWLEDGE_SECTIONS.map((section) => ({
        id: section.id,
        label: t(section.labelKey),
        icon: section.icon,
        items: sectionItems(lore.items, section).map((item) =>
          loreDirectoryItem(item, t),
        ),
        onCreate: () => {
          void lore.createItem({
            enabled: true,
            type: section.createType,
            name: t(section.createNameKey),
            importance: 'important',
            load_mode: 'auto',
            tags: section.tag ? [section.tag] : [],
            brief_description: '',
            keywords: [],
            content: '',
          })
        },
        createLabel: t('loreWorkspace.createInSection', {
          section: t(section.labelKey),
        }),
        toggleOnHeaderClick: true,
      })),
    [lore.createItem, lore.items, t],
  )

  const directory = (
    <div className="nova-sidebar flex h-full min-h-0 flex-col bg-[var(--nova-surface-2)]">
      {lore.loading && lore.items.length === 0 ? (
        <div className="grid gap-2 p-3" aria-label={t('common.loading')}>
          {Array.from({ length: 6 }).map((_, index) => (
            <div
              key={index}
              className="h-8 animate-pulse rounded bg-[var(--nova-surface)]"
            />
          ))}
        </div>
      ) : lore.error && lore.items.length === 0 ? (
        <div className="grid gap-2 p-3">
          <InlineErrorNotice message={lore.error} />
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void lore.reload(lore.activeId)
            }}
          >
            {t('common.retry')}
          </Button>
        </div>
      ) : (
        <ResourceDirectory
          sections={sections}
          activeId={lore.activeId || null}
          onSelect={(id) => {
            void lore.selectItem(id)
          }}
          saving={lore.autosaveStatus === 'saving'}
          searchPlaceholder={t('loreWorkspace.search')}
          emptySectionsLast
          headerContent={
            <div className="grid gap-2">
              <div className="flex items-start gap-2 px-1 pb-1">
                <BookMarked className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-success)]" />
                <div className="min-w-0 flex-1">
                  <div className="text-xs font-medium text-[var(--nova-text)]">
                    {t('loreWorkspace.directoryTitle')}
                  </div>
                  <div className="mt-0.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">
                    {t('loreWorkspace.directoryDescription')}
                  </div>
                </div>
                {onOpenLibrary ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={onOpenLibrary}
                    aria-label={t('loreWorkspace.openLibrary')}
                    title={t('loreWorkspace.openLibrary')}
                  >
                    <LibraryBig />
                  </Button>
                ) : null}
              </div>
              {lore.error ? <InlineErrorNotice message={lore.error} /> : null}
            </div>
          }
          emptyContent={
            <div className="px-2 py-8 text-center text-xs text-[var(--nova-text-faint)]">
              {t('loreWorkspace.emptyDirectory')}
            </div>
          }
        />
      )}
    </div>
  )

  return (
    <section
      className="h-full min-h-0 min-w-0 bg-[var(--nova-bg)]"
      aria-label={t('loreWorkspace.title')}
    >
      <AdaptiveSurface
        left={{
          id: 'writing-lore-directory',
          title: t('loreWorkspace.directoryTitle'),
          side: 'left',
          icon: <BookMarked className="h-4 w-4 text-[var(--nova-success)]" />,
          content: directory,
          desktopClassName: 'min-h-0 w-60 border-r border-[var(--nova-border)]',
          mobileClassName: 'w-[min(88vw,340px)]',
        }}
        desktopGridClassName="grid-cols-[15rem_minmax(0,1fr)]"
        collapseAt={720}
      >
        {({ isMobile, openLeft }) =>
          lore.loading && !lore.draft ? (
            <div role="status" className="flex h-full items-center justify-center text-xs text-[var(--nova-text-muted)]">
              {t('common.loading')}
            </div>
          ) : lore.error && lore.items.length === 0 ? (
            <div className="grid h-full place-content-center gap-3 px-6">
              <InlineErrorNotice message={lore.error} />
              <Button variant="outline" size="sm" onClick={() => void lore.reload()}>
                {t('common.retry')}
              </Button>
            </div>
          ) : lore.draft ? (
            <LoreWorkspaceEditor
              draft={lore.draft}
              tagDraft={lore.tagDraft}
              autosaveStatus={lore.autosaveStatus}
              autosaveError={lore.autosaveError}
              documentReview={documentReview}
              navigationIntent={
                navigationTargetID === lore.activeId ? navigationIntent : null
              }
              onDraftChange={lore.setDraft}
              onTagDraftChange={lore.setTagDraft}
              onPrepareSnapshot={lore.prepareSnapshot}
              onFlush={lore.flush}
              onOpenDirectory={isMobile ? openLeft : undefined}
              onOpenLibrary={onOpenLibrary}
              onReferenceItem={onReferenceItem}
            />
          ) : (
            <div className="relative flex h-full min-h-0 items-center justify-center">
              {isMobile ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={openLeft}
                  className="absolute left-3 top-3"
                >
                  <BookMarked />
                  {t('loreWorkspace.openDirectory')}
                </Button>
              ) : null}
              <EmptyState
                icon={Database}
                title={t('loreWorkspace.emptyTitle')}
                description={t('loreWorkspace.emptyDescription')}
                action={{
                  label: t('loreWorkspace.emptyAction'),
                  onClick: () => {
                    void lore.createItem({
                      enabled: true,
                      type: 'character',
                      name: t('settingPanel.lore.newCharacter'),
                      importance: 'important',
                      load_mode: 'auto',
                      content: '',
                    })
                  },
                }}
                variant="page"
              />
            </div>
          )
        }
      </AdaptiveSurface>
    </section>
  )
}

function loreDirectoryItem(
  item: LoreItem,
  t: (key: string) => string,
): ResourceDirectoryItem {
  const imagePath = item.image?.image_path || ''
  return {
    id: item.id,
    title: item.name,
    summary: item.brief_description || undefined,
    thumbnailUrl: imagePath ? workspaceAssetURL(imagePath) : null,
    disabled: item.enabled === false,
    searchText: `${(item.tags || []).join(' ')} ${(item.keywords || []).join(' ')} ${item.content || ''}`,
    badges: [
      {
        label:
          item.load_mode === 'resident'
            ? t('settingPanel.lore.loadModeBadge.resident')
            : t('settingPanel.lore.loadModeBadge.onDemand'),
        title: loreLoadModeLabel(item.load_mode, t),
        tone: item.load_mode === 'resident' ? 'default' : 'outline',
      },
    ],
  }
}
