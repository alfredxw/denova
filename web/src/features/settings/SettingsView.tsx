import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LoadingState } from '@/components/common/LoadingState'
import { AutosaveStatusIndicator } from '@/components/forms/autosave-status'
import { withErrorLogID } from '@/lib/api-client'
import { GLOBAL_SETTINGS_TARGET, settingsQueryOptions } from './query'
import { useLayeredSettingsDraft } from './use-layered-settings-draft'
import { SettingsPageFrame } from './SettingsPageFrame'
import { SettingsNavigation } from './SettingsNavigation'
import { SettingsSections } from './SettingsSections'
import { useSettingsNavigation } from './use-settings-navigation'

export function SettingsView({ visible = true }: { visible?: boolean }) {
  const { t } = useTranslation()
  // Display the shared snapshot immediately; revalidate on entry and browser focus.
  // The draft hook rebases query updates without discarding pending edits.
  const { error: refreshError } = useQuery({
    ...settingsQueryOptions(GLOBAL_SETTINGS_TARGET),
    enabled: visible,
    staleTime: 0,
    refetchOnWindowFocus: 'always',
  })
  const { layered, draft, setDraft, error, autosaveStatus, autosaveError, saveNow } = useLayeredSettingsDraft({
    target: GLOBAL_SETTINGS_TARGET,
    layer: 'user',
    sourcePrefix: 'settings-view',
  })
  const { contentRef, activeSection, fieldRequest, selectSection } = useSettingsNavigation({ visible, ready: Boolean(layered) })
  const [search, setSearch] = useState('')
  const showDebugSettings = layered?.runtime?.dev_mode === true
  useEffect(() => {
    if (refreshError) console.warn('[settings] failed to revalidate settings', refreshError)
  }, [refreshError])

  return (
    <SettingsPageFrame
      visible={visible}
      title={t('settings.title')}
      className="nova-settings-view"
      error={error || (refreshError ? withErrorLogID(t('settings.error.load'), refreshError) : null)}
      errorTitle={t(error ? 'settings.error.save' : 'settings.error.load')}
      navigation={layered ? (
        <SettingsNavigation activeId={activeSection} search={search} onSearchChange={setSearch} onSelect={(id, fieldKey) => { selectSection(id, fieldKey); setSearch('') }}
          showDebug={showDebugSettings} goos={layered.runtime?.goos} />
      ) : undefined}
      onSaveShortcut={() => saveNow().catch(() => undefined)}
      actions={(
        <AutosaveStatusIndicator
          status={autosaveStatus}
          error={autosaveError}
          onRetry={() => saveNow().catch(() => undefined)}
        />
      )}
    >
      {!layered ? (
        <LoadingState label={t('common.loading')} className="min-h-0 flex-1" />
      ) : (
        <div ref={contentRef} data-nova-settings-content="true" className="h-full min-h-0 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6 [container-type:size]">
          <SettingsSections layered={layered} draft={draft} setDraft={setDraft} fieldRequest={fieldRequest} onSelectSection={selectSection} onSave={saveNow} />
        </div>
      )}
    </SettingsPageFrame>
  )
}
