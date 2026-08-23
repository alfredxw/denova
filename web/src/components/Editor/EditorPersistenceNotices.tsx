import { TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { MISSING_WORKSPACE_REVISION } from '@/lib/api-client/workspace'
import type { ExternalContentConflict } from './useEditorDraftPersistence'

interface EditorPersistenceNoticesProps {
  workspace: string
  fileName: string
  revision: string
  externalConflict: ExternalContentConflict | null
  externalConflictSaving: boolean
  onKeepLocal: () => void | Promise<void>
  onLoadExternal: () => void
}

/** Shared data-safety notices for every Writing document surface. */
export function EditorPersistenceNotices({
  workspace,
  fileName,
  revision,
  externalConflict,
  externalConflictSaving,
  onKeepLocal,
  onLoadExternal,
}: EditorPersistenceNoticesProps) {
  const { t } = useTranslation()
  const visibleConflict = externalConflict?.workspace === workspace && externalConflict.fileName === fileName
    ? externalConflict
    : null

  return (
    <>
      {revision === MISSING_WORKSPACE_REVISION ? (
        <div role="alert" className="flex shrink-0 items-start gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-[var(--nova-warning)]" />
          <div className="min-w-0">
            <div className="font-medium text-[var(--nova-text)]">{t('editor.orphaned.title')}</div>
            <div className="mt-0.5 text-[var(--nova-text-faint)]">{t('editor.orphaned.description')}</div>
          </div>
        </div>
      ) : null}
      {visibleConflict ? (
        <div role="alert" className="flex shrink-0 flex-wrap items-center gap-2 border-b border-[var(--nova-warning)]/30 bg-[var(--nova-warning-bg)] px-3 py-2 text-[11px] text-[var(--nova-text-muted)]">
          <TriangleAlert className="h-4 w-4 shrink-0 text-[var(--nova-warning)]" />
          <div className="min-w-[180px] flex-1">
            <div className="font-medium text-[var(--nova-text)]">{t('editor.externalConflict.title')}</div>
            <div className="mt-0.5 text-[var(--nova-text-faint)]">{t('editor.externalConflict.description')}</div>
            {visibleConflict.recoveryID ? (
              <div className="mt-0.5 font-mono text-[10px] text-[var(--nova-text-faint)]">
                {t('editor.externalConflict.recovery', { id: visibleConflict.recoveryID })}
              </div>
            ) : null}
          </div>
          <Button type="button" size="xs" variant="outline" disabled={externalConflictSaving} onClick={() => void onKeepLocal()}>
            {t('editor.externalConflict.keepLocal')}
          </Button>
          <Button type="button" size="xs" disabled={externalConflictSaving} onClick={onLoadExternal}>
            {t('editor.externalConflict.loadExternal')}
          </Button>
        </div>
      ) : null}
    </>
  )
}
